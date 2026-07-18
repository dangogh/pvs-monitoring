package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangogh/pvs-monitoring/pvs"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen(t *testing.T) {
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "creates db and parent dirs",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "sub", "dir", "readings.db")
			},
		},
		{
			name: "opens existing db",
			path: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "readings.db")
				s, err := Open(p)
				require.NoError(t, err)
				s.Close()
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(tt.path(t))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			s.Close()
		})
	}
}

func TestSaveAndAveragePower(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name     string
		readings []*pvs.Reading
		since    time.Time
		want     pvs.PowerAvg
	}{
		{
			name: "single reading",
			readings: []*pvs.Reading{
				{ReceivedAt: now, SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0},
			},
			since: now.Add(-time.Minute),
			want:  pvs.PowerAvg{SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0, Samples: 1},
		},
		{
			name: "average of multiple readings",
			readings: []*pvs.Reading{
				{ReceivedAt: now, SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0},
				{ReceivedAt: now.Add(time.Second), SolarKW: 12.0, LoadKW: 6.0, NetKW: -4.0},
			},
			since: now.Add(-time.Minute),
			want:  pvs.PowerAvg{SolarKW: 11.0, LoadKW: 5.0, NetKW: -5.0, Samples: 2},
		},
		{
			name: "since filters out older readings",
			readings: []*pvs.Reading{
				{ReceivedAt: now.Add(-2 * time.Hour), SolarKW: 99.0, LoadKW: 99.0, NetKW: 99.0},
				{ReceivedAt: now, SolarKW: 8.0, LoadKW: 3.0, NetKW: -5.0},
			},
			since: now.Add(-time.Hour),
			want:  pvs.PowerAvg{SolarKW: 8.0, LoadKW: 3.0, NetKW: -5.0, Samples: 1},
		},
		{
			name:     "no readings in window",
			readings: []*pvs.Reading{},
			since:    now.Add(-time.Minute),
			want:     pvs.PowerAvg{Samples: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)

			for _, r := range tt.readings {
				require.NoError(t, s.SaveReading(ctx, r))
			}

			got, err := s.AveragePower(ctx, tt.since, time.Now().Add(time.Hour))
			require.NoError(t, err)
			assert.Equal(t, tt.want.Samples, got.Samples)
			if got.Samples == 0 {
				return
			}
			assert.InDelta(t, tt.want.SolarKW, got.SolarKW, 1e-9)
			assert.InDelta(t, tt.want.LoadKW, got.LoadKW, 1e-9)
			assert.InDelta(t, tt.want.NetKW, got.NetKW, 1e-9)
		})
	}
}

func TestSaveAndLatestDevices(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	inv := pvs.Device{
		Serial:     "INV001",
		DeviceType: "Inverter",
		Raw: []byte(`{"SERIAL":"INV001","DEVICE_TYPE":"Inverter","STATE":"working","STATEDESCR":"Working",` +
			`"p_3phsum_kw":"8.5","ltea_3phsum_kwh":"1000.0","vln_3phavg_v":"240.0","i_3phsum_a":"1.0",` +
			`"p_mppt1_kw":"8.5","v_mppt1_v":"48.0","i_mppt1_a":"1.0","t_htsnk_degc":"45","freq_hz":"60.0"}`),
	}
	mtr := pvs.Device{
		Serial:     "MTR001",
		DeviceType: "Power Meter",
		Raw: []byte(`{"SERIAL":"MTR001","DEVICE_TYPE":"Power Meter","STATE":"working","STATEDESCR":"Working",` +
			`"subtype":"GROSS_PRODUCTION_SITE","net_ltea_3phsum_kwh":"5000.0","p_3phsum_kw":"2.0",` +
			`"q_3phsum_kvar":"0.1","s_3phsum_kva":"2.1","tot_pf_rto":"0.95","freq_hz":"60.0","i_a":"1.0","v12_v":"240.0"}`),
	}

	t.Run("returns empty slice when no devices saved", func(t *testing.T) {
		s := openTestStore(t)
		inverters, err := s.LatestInverters(ctx)
		require.NoError(t, err)
		assert.Empty(t, inverters)
	})

	t.Run("saves and retrieves inverter", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv}, now))

		got, err := s.LatestInverters(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "INV001", got[0].Serial)
		assert.InDelta(t, 8.5, got[0].PowerKW, 1e-9)
		assert.InDelta(t, 1000.0, got[0].LifetimeKWh, 1e-9)
	})

	t.Run("saves and retrieves meter as aux device", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{mtr}, now))

		got, err := s.LatestAuxDevices(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "MTR001", got[0].Serial)
		assert.Equal(t, "Power Meter", got[0].DeviceType)
		assert.NotEmpty(t, got[0].Payload)
	})

	t.Run("latest poll supersedes earlier poll", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv}, now))
		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv}, now.Add(time.Minute)))

		got, err := s.LatestInverters(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, now.Add(time.Minute).Unix(), got[0].ReceivedAt.Unix())
	})

	t.Run("saves multiple device types in one transaction", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv, mtr}, now))

		inverters, err := s.LatestInverters(ctx)
		require.NoError(t, err)
		assert.Len(t, inverters, 1)

		aux, err := s.LatestAuxDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, aux, 1)
		assert.Equal(t, "Power Meter", aux[0].DeviceType)
	})
}

func TestInverterOutages(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("open creates a record with no healthy_at", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now))

		var serial string
		var errorAt int64
		var healthyAt *int64
		row := s.db.QueryRowContext(ctx, `SELECT serial, error_at, healthy_at FROM inverter_outages`)
		require.NoError(t, row.Scan(&serial, &errorAt, &healthyAt))
		assert.Equal(t, "INV001", serial)
		assert.Equal(t, now.Unix(), errorAt)
		assert.Nil(t, healthyAt)
	})

	t.Run("open is a no-op when outage already open", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now))
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now.Add(time.Minute)))

		var count int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inverter_outages`).Scan(&count))
		assert.Equal(t, 1, count, "duplicate open should be ignored")
	})

	t.Run("close sets healthy_at on most recent open outage", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now))
		require.NoError(t, s.CloseInverterOutage(ctx, "INV001", now.Add(time.Hour)))

		var healthyAt *int64
		row := s.db.QueryRowContext(ctx, `SELECT healthy_at FROM inverter_outages WHERE serial = 'INV001'`)
		require.NoError(t, row.Scan(&healthyAt))
		require.NotNil(t, healthyAt)
		assert.Equal(t, now.Add(time.Hour).Unix(), *healthyAt)
	})

	t.Run("open after close creates a new outage", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now))
		require.NoError(t, s.CloseInverterOutage(ctx, "INV001", now.Add(time.Hour)))
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now.Add(2*time.Hour)))

		var count int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inverter_outages`).Scan(&count))
		assert.Equal(t, 2, count)
	})

	t.Run("close is independent per serial", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now))
		require.NoError(t, s.OpenInverterOutage(ctx, "INV002", now))
		require.NoError(t, s.CloseInverterOutage(ctx, "INV001", now.Add(time.Hour)))

		var h1, h2 *int64
		require.NoError(t, s.db.QueryRowContext(ctx,
			`SELECT healthy_at FROM inverter_outages WHERE serial = 'INV001'`).Scan(&h1))
		require.NoError(t, s.db.QueryRowContext(ctx,
			`SELECT healthy_at FROM inverter_outages WHERE serial = 'INV002'`).Scan(&h2))
		assert.NotNil(t, h1, "INV001 should be closed")
		assert.Nil(t, h2, "INV002 should still be open")
	})

	t.Run("ListOpenInverterOutages returns only open serials", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.OpenInverterOutage(ctx, "INV001", now))
		require.NoError(t, s.OpenInverterOutage(ctx, "INV002", now))
		require.NoError(t, s.CloseInverterOutage(ctx, "INV001", now.Add(time.Hour)))

		serials, err := s.ListOpenInverterOutages(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"INV002"}, serials)
	})
}

func TestLatestReading(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("returns nil when no readings", func(t *testing.T) {
		s := openTestStore(t)
		r, err := s.LatestReading(ctx)
		require.NoError(t, err)
		assert.Nil(t, r)
	})

	t.Run("returns most recent reading", func(t *testing.T) {
		s := openTestStore(t)
		older := &pvs.Reading{ReceivedAt: now.Add(-time.Minute), Time: now.Add(-time.Minute), SolarKW: 1.0}
		newer := &pvs.Reading{ReceivedAt: now, Time: now, SolarKW: 2.0}
		require.NoError(t, s.SaveReading(ctx, older))
		require.NoError(t, s.SaveReading(ctx, newer))

		got, err := s.LatestReading(ctx)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, now.Unix(), got.ReceivedAt.Unix())
		assert.InDelta(t, 2.0, got.SolarKW, 1e-9)
	})
}

func TestRollupUpsert(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.UTC)

	t.Run("single reading creates hourly and daily rows", func(t *testing.T) {
		s := openTestStore(t)
		r := &pvs.Reading{ReceivedAt: base, SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0, SolarKWh: 100.0, LoadKWh: 50.0}
		require.NoError(t, s.SaveReading(ctx, r))

		var count int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings_hourly`).Scan(&count))
		assert.Equal(t, 1, count)

		var bucket int64
		var avgSolar, avgLoad float64
		var sampleCount int
		var minSolarKWh, maxSolarKWh float64
		require.NoError(t, s.db.QueryRowContext(ctx,
			`SELECT bucket, avg_solar_kw, avg_load_kw, sample_count, min_solar_kwh, max_solar_kwh FROM readings_hourly`).
			Scan(&bucket, &avgSolar, &avgLoad, &sampleCount, &minSolarKWh, &maxSolarKWh))
		assert.Equal(t, (base.Unix()/3600)*3600, bucket)
		assert.InDelta(t, 10.0, avgSolar, 1e-9)
		assert.InDelta(t, 4.0, avgLoad, 1e-9)
		assert.Equal(t, 1, sampleCount)
		assert.InDelta(t, 100.0, minSolarKWh, 1e-9)
		assert.InDelta(t, 100.0, maxSolarKWh, 1e-9)

		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings_daily`).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("two readings in same hour merge with weighted avg", func(t *testing.T) {
		s := openTestStore(t)
		r1 := &pvs.Reading{ReceivedAt: base, SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0, SolarKWh: 100.0, LoadKWh: 50.0}
		r2 := &pvs.Reading{ReceivedAt: base.Add(30 * time.Minute), SolarKW: 12.0, LoadKW: 6.0, NetKW: -4.0, SolarKWh: 100.5, LoadKWh: 50.5}
		require.NoError(t, s.SaveReading(ctx, r1))
		require.NoError(t, s.SaveReading(ctx, r2))

		var sampleCount int
		var avgSolar, avgLoad float64
		var minSolarKWh, maxSolarKWh float64
		require.NoError(t, s.db.QueryRowContext(ctx,
			`SELECT avg_solar_kw, avg_load_kw, sample_count, min_solar_kwh, max_solar_kwh FROM readings_hourly`).
			Scan(&avgSolar, &avgLoad, &sampleCount, &minSolarKWh, &maxSolarKWh))
		assert.Equal(t, 2, sampleCount)
		assert.InDelta(t, 11.0, avgSolar, 1e-9)
		assert.InDelta(t, 5.0, avgLoad, 1e-9)
		assert.InDelta(t, 100.0, minSolarKWh, 1e-9)
		assert.InDelta(t, 100.5, maxSolarKWh, 1e-9)
	})

	t.Run("readings in different hours create separate buckets", func(t *testing.T) {
		s := openTestStore(t)
		r1 := &pvs.Reading{ReceivedAt: base, SolarKW: 10.0}
		r2 := &pvs.Reading{ReceivedAt: base.Add(2 * time.Hour), SolarKW: 8.0}
		require.NoError(t, s.SaveReading(ctx, r1))
		require.NoError(t, s.SaveReading(ctx, r2))

		var count int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings_hourly`).Scan(&count))
		assert.Equal(t, 2, count)
	})

	t.Run("readings in same day merge into one daily bucket", func(t *testing.T) {
		s := openTestStore(t)
		r1 := &pvs.Reading{ReceivedAt: base, SolarKW: 10.0, SolarKWh: 100.0}
		r2 := &pvs.Reading{ReceivedAt: base.Add(3 * time.Hour), SolarKW: 8.0, SolarKWh: 102.0}
		require.NoError(t, s.SaveReading(ctx, r1))
		require.NoError(t, s.SaveReading(ctx, r2))

		var count, sampleCount int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings_daily`).Scan(&count))
		assert.Equal(t, 1, count)
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT sample_count FROM readings_daily`).Scan(&sampleCount))
		assert.Equal(t, 2, sampleCount)
	})
}

func TestAveragePowerRouting(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.UTC)

	t.Run("span over 48h uses rollup table", func(t *testing.T) {
		s := openTestStore(t)
		readings := []*pvs.Reading{
			{ReceivedAt: base, SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0, SolarKWh: 100.0, LoadKWh: 50.0},
			{ReceivedAt: base.Add(24 * time.Hour), SolarKW: 12.0, LoadKW: 6.0, NetKW: -4.0, SolarKWh: 110.0, LoadKWh: 60.0},
			{ReceivedAt: base.Add(50 * time.Hour), SolarKW: 8.0, LoadKW: 3.0, NetKW: -5.0, SolarKWh: 120.0, LoadKWh: 70.0},
		}
		for _, r := range readings {
			require.NoError(t, s.SaveReading(ctx, r))
		}
		since := base.Add(-time.Hour)
		until := base.Add(51 * time.Hour) // span > 48h

		got, err := s.AveragePower(ctx, since, until)
		require.NoError(t, err)
		assert.Equal(t, 3, got.Samples)
		assert.InDelta(t, 10.0, got.SolarKW, 1e-6)
		assert.InDelta(t, 13.0/3.0, got.LoadKW, 1e-6)
		assert.InDelta(t, -5.0, got.NetKW, 1e-6)
	})

	t.Run("span under 48h uses raw table", func(t *testing.T) {
		s := openTestStore(t)
		r := &pvs.Reading{ReceivedAt: base, SolarKW: 10.0, LoadKW: 4.0, NetKW: -6.0}
		require.NoError(t, s.SaveReading(ctx, r))
		since := base.Add(-time.Hour)
		until := base.Add(47 * time.Hour) // span < 48h

		got, err := s.AveragePower(ctx, since, until)
		require.NoError(t, err)
		assert.Equal(t, 1, got.Samples)
		assert.InDelta(t, 10.0, got.SolarKW, 1e-9)
	})
}

func TestEnergyDeltaRouting(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.UTC)

	t.Run("span over 48h uses rollup table", func(t *testing.T) {
		s := openTestStore(t)
		readings := []*pvs.Reading{
			{ReceivedAt: base, SolarKWh: 100.0, LoadKWh: 50.0},
			{ReceivedAt: base.Add(24 * time.Hour), SolarKWh: 110.0, LoadKWh: 60.0},
			{ReceivedAt: base.Add(50 * time.Hour), SolarKWh: 125.0, LoadKWh: 75.0},
		}
		for _, r := range readings {
			require.NoError(t, s.SaveReading(ctx, r))
		}
		since := base.Add(-time.Hour)
		until := base.Add(51 * time.Hour)

		got, err := s.EnergyDelta(ctx, since, until)
		require.NoError(t, err)
		assert.InDelta(t, 25.0, got.SolarKWh, 1e-6) // MAX(125) - MIN(100)
		assert.InDelta(t, 25.0, got.LoadKWh, 1e-6)  // MAX(75) - MIN(50)
		assert.InDelta(t, 0.0, got.NetKWh, 1e-6)    // 25 - 25
	})

	t.Run("span under 48h uses raw table", func(t *testing.T) {
		s := openTestStore(t)
		readings := []*pvs.Reading{
			{ReceivedAt: base, SolarKWh: 100.0, LoadKWh: 50.0},
			{ReceivedAt: base.Add(time.Hour), SolarKWh: 110.0, LoadKWh: 60.0},
		}
		for _, r := range readings {
			require.NoError(t, s.SaveReading(ctx, r))
		}
		since := base.Add(-time.Hour)
		until := base.Add(2 * time.Hour) // span < 48h

		got, err := s.EnergyDelta(ctx, since, until)
		require.NoError(t, err)
		assert.InDelta(t, 10.0, got.SolarKWh, 1e-9)
		assert.InDelta(t, 10.0, got.LoadKWh, 1e-9)
	})
}

func TestReadingsSeriesRouting(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.UTC)

	s := openTestStore(t)
	readings := []*pvs.Reading{
		{ReceivedAt: base, SolarKW: 10.0, LoadKW: 4.0},
		{ReceivedAt: base.Add(24 * time.Hour), SolarKW: 12.0, LoadKW: 6.0},
		{ReceivedAt: base.Add(48 * time.Hour), SolarKW: 8.0, LoadKW: 3.0},
	}
	for _, r := range readings {
		require.NoError(t, s.SaveReading(ctx, r))
	}
	since := base.Add(-time.Hour)
	until := base.Add(49 * time.Hour)

	t.Run("bucketSeconds=3600 reads from readings_hourly", func(t *testing.T) {
		pts, err := s.ReadingsSeries(ctx, since, until, 3600)
		require.NoError(t, err)
		require.Len(t, pts, 3)
		assert.InDelta(t, 10.0, pts[0].SolarKW, 1e-9)
		assert.InDelta(t, 12.0, pts[1].SolarKW, 1e-9)
		assert.InDelta(t, 8.0, pts[2].SolarKW, 1e-9)
	})

	t.Run("bucketSeconds=86400 reads from readings_daily", func(t *testing.T) {
		pts, err := s.ReadingsSeries(ctx, since, until, 86400)
		require.NoError(t, err)
		require.Len(t, pts, 3)
		assert.InDelta(t, 10.0, pts[0].SolarKW, 1e-9)
	})

	t.Run("bucketSeconds=21600 groups readings_hourly into 6h buckets", func(t *testing.T) {
		pts, err := s.ReadingsSeries(ctx, since, until, 21600)
		require.NoError(t, err)
		require.Len(t, pts, 3) // each reading is 24h apart, so each falls in a distinct 6h bucket
		assert.InDelta(t, 10.0, pts[0].SolarKW, 1e-9)
	})
}

func TestMigrateV5Backfill(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.UTC) // noon UTC — two readings fit in one hour and one day

	dbPath := filepath.Join(t.TempDir(), "test.db")
	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	defer db.Close()

	// Apply only schema migrations 0–3 (v4 schema, no rollup tables).
	for i := range 4 {
		_, err := db.Exec(migrations[i])
		require.NoError(t, err)
	}
	_, err = db.Exec(`PRAGMA user_version = 4`)
	require.NoError(t, err)

	// Insert raw readings directly, bypassing SaveReading (which would also populate rollup tables).
	_, err = db.ExecContext(ctx, `
		INSERT INTO readings (received_at, reading_time, solar_kw, load_kw, net_kw, solar_kwh, load_kwh, net_kwh)
		VALUES (?, ?, 10.0, 4.0, -6.0, 100.0, 50.0, -50.0),
		       (?, ?, 12.0, 6.0, -4.0, 110.0, 60.0, -50.0)`,
		base.Unix(), base.Unix(),
		base.Add(30*time.Minute).Unix(), base.Add(30*time.Minute).Unix())
	require.NoError(t, err)

	// Running migrate applies schema 005 (creates tables) then calls migrateV5 (backfills).
	require.NoError(t, migrate(db))

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings_hourly`).Scan(&count))
	assert.Equal(t, 1, count, "both readings in same hour → 1 hourly bucket")

	var sampleCount int
	var avgSolar float64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT sample_count, avg_solar_kw FROM readings_hourly`).Scan(&sampleCount, &avgSolar))
	assert.Equal(t, 2, sampleCount)
	assert.InDelta(t, 11.0, avgSolar, 1e-9) // AVG(10, 12) = 11

	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings_daily`).Scan(&count))
	assert.Equal(t, 1, count, "both readings in same day → 1 daily bucket")
}

func TestSaveReadingPersistsAllFields(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	now := time.Now().Truncate(time.Second)

	r := &pvs.Reading{
		ReceivedAt: now,
		Time:       now.Add(-time.Second),
		SolarKW:    12.16,
		LoadKW:     4.03,
		NetKW:      -8.13,
		SolarKWh:   94476.77,
		LoadKWh:    65063.92,
		NetKWh:     -29412.85,
	}
	require.NoError(t, s.SaveReading(ctx, r))

	row := s.db.QueryRowContext(ctx,
		`SELECT received_at, reading_time, solar_kw, load_kw, net_kw, solar_kwh, load_kwh, net_kwh FROM readings`)
	var recvAt, readingTime int64
	var solarKW, loadKW, netKW, solarKWh, loadKWh, netKWh float64
	require.NoError(t, row.Scan(&recvAt, &readingTime, &solarKW, &loadKW, &netKW, &solarKWh, &loadKWh, &netKWh))

	assert.Equal(t, r.ReceivedAt.Unix(), recvAt)
	assert.Equal(t, r.Time.Unix(), readingTime)
	assert.Equal(t, r.SolarKW, solarKW)
	assert.Equal(t, r.LoadKW, loadKW)
	assert.Equal(t, r.NetKW, netKW)
	assert.Equal(t, r.SolarKWh, solarKWh)
	assert.Equal(t, r.LoadKWh, loadKWh)
	assert.Equal(t, r.NetKWh, netKWh)
}

func TestSaveAndListMaintenanceEvents(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	start := time.Date(2026, 7, 17, 14, 30, 0, 0, time.UTC)
	end := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)

	id, err := s.SaveMaintenanceEvent(ctx, pvs.MaintenanceEvent{
		StartAt:   start,
		EndAt:     end,
		EventType: "hvac_outage",
		Notes:     "heat pump failed",
	})
	require.NoError(t, err)
	assert.NotZero(t, id)

	_, err = s.SaveMaintenanceEvent(ctx, pvs.MaintenanceEvent{
		StartAt:   start.Add(24 * time.Hour),
		EventType: "panel_cleaning",
	})
	require.NoError(t, err)

	events, err := s.ListMaintenanceEvents(ctx)
	require.NoError(t, err)
	require.Len(t, events, 2)

	// ORDER BY start_at DESC: the later-starting panel_cleaning event comes first.
	assert.Equal(t, "panel_cleaning", events[0].EventType)
	assert.True(t, events[0].EndAt.IsZero())

	assert.Equal(t, start.Unix(), events[1].StartAt.Unix())
	assert.Equal(t, end.Unix(), events[1].EndAt.Unix())
	assert.Equal(t, "hvac_outage", events[1].EventType)
	assert.Equal(t, "heat pump failed", events[1].Notes)
}
