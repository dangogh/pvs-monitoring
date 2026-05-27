package sqlite

import (
	"context"
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
		Type:       "MI",
		Model:      "SPR-X22",
		State:      "working",
		StateDescr: "Working",
		Raw:        []byte(`{"SERIAL":"INV001","DEVICE_TYPE":"Inverter","p_3phsum_kw":8.5}`),
	}
	mtr := pvs.Device{
		Serial:     "MTR001",
		DeviceType: "Power Meter",
		Raw:        []byte(`{"SERIAL":"MTR001","DEVICE_TYPE":"Power Meter"}`),
	}

	t.Run("returns empty slice when no devices saved", func(t *testing.T) {
		s := openTestStore(t)
		devices, err := s.LatestDevices(ctx)
		require.NoError(t, err)
		assert.Empty(t, devices)
	})

	t.Run("returns devices from most recent poll", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv, mtr}, now))

		got, err := s.LatestDevices(ctx)
		require.NoError(t, err)
		require.Len(t, got, 2)

		serials := []string{got[0].Serial, got[1].Serial}
		assert.ElementsMatch(t, []string{"INV001", "MTR001"}, serials)
	})

	t.Run("latest poll supersedes earlier poll", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv}, now))
		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{mtr}, now.Add(time.Minute)))

		got, err := s.LatestDevices(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "MTR001", got[0].Serial)
	})

	t.Run("raw payload round-trips correctly", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv}, now))

		got, err := s.LatestDevices(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.JSONEq(t, string(inv.Raw), string(got[0].Raw))
	})

	t.Run("saves all devices in a transaction", func(t *testing.T) {
		s := openTestStore(t)

		require.NoError(t, s.SaveDevices(ctx, []pvs.Device{inv, mtr}, now))

		var count int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM device_readings`).Scan(&count))
		assert.Equal(t, 2, count)
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
