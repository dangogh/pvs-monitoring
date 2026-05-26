package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dangogh/pvs-monitoring/pvs"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
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
				if err != nil {
					t.Fatalf("first Open: %v", err)
				}
				s.Close()
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(tt.path(t))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Open() err = %v, wantErr %v", err, tt.wantErr)
			}
			if s != nil {
				s.Close()
			}
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
				if err := s.SaveReading(ctx, r); err != nil {
					t.Fatalf("SaveReading: %v", err)
				}
			}

			got, err := s.AveragePower(ctx, tt.since)
			if err != nil {
				t.Fatalf("AveragePower: %v", err)
			}
			if got.Samples != tt.want.Samples {
				t.Errorf("Samples: got %d, want %d", got.Samples, tt.want.Samples)
			}
			if got.Samples == 0 {
				return
			}
			const epsilon = 1e-9
			if diff := got.SolarKW - tt.want.SolarKW; diff > epsilon || diff < -epsilon {
				t.Errorf("SolarKW: got %v, want %v", got.SolarKW, tt.want.SolarKW)
			}
			if diff := got.LoadKW - tt.want.LoadKW; diff > epsilon || diff < -epsilon {
				t.Errorf("LoadKW: got %v, want %v", got.LoadKW, tt.want.LoadKW)
			}
			if diff := got.NetKW - tt.want.NetKW; diff > epsilon || diff < -epsilon {
				t.Errorf("NetKW: got %v, want %v", got.NetKW, tt.want.NetKW)
			}
		})
	}
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
	if err := s.SaveReading(ctx, r); err != nil {
		t.Fatalf("SaveReading: %v", err)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT received_at, reading_time, solar_kw, load_kw, net_kw, solar_kwh, load_kwh, net_kwh FROM readings`)
	var recvAt, readingTime int64
	var solarKW, loadKW, netKW, solarKWh, loadKWh, netKWh float64
	if err := row.Scan(&recvAt, &readingTime, &solarKW, &loadKW, &netKW, &solarKWh, &loadKWh, &netKWh); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if recvAt != r.ReceivedAt.Unix() {
		t.Errorf("received_at: got %d, want %d", recvAt, r.ReceivedAt.Unix())
	}
	if readingTime != r.Time.Unix() {
		t.Errorf("reading_time: got %d, want %d", readingTime, r.Time.Unix())
	}
	if solarKW != r.SolarKW {
		t.Errorf("solar_kw: got %v, want %v", solarKW, r.SolarKW)
	}
	if loadKW != r.LoadKW {
		t.Errorf("load_kw: got %v, want %v", loadKW, r.LoadKW)
	}
	if netKW != r.NetKW {
		t.Errorf("net_kw: got %v, want %v", netKW, r.NetKW)
	}
	if solarKWh != r.SolarKWh {
		t.Errorf("solar_kwh: got %v, want %v", solarKWh, r.SolarKWh)
	}
	if loadKWh != r.LoadKWh {
		t.Errorf("load_kwh: got %v, want %v", loadKWh, r.LoadKWh)
	}
	if netKWh != r.NetKWh {
		t.Errorf("net_kwh: got %v, want %v", netKWh, r.NetKWh)
	}
}
