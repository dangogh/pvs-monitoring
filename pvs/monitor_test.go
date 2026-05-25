package pvs

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/dangogh/pvs-monitoring/config"
)

func TestNotificationParamsToReading(t *testing.T) {
	tests := []struct {
		name   string
		params notificationParams
		want   Reading
	}{
		{
			name: "typical reading",
			params: notificationParams{
				Time:     1779680954,
				SolarKW:  0.02,
				LoadKW:   3.94,
				NetKW:    3.92,
				SolarKWh: 94400.05,
				NetKWh:   -29376.45,
				LoadKWh:  65023.6,
			},
			want: Reading{
				Time:     time.Unix(1779680954, 0),
				SolarKW:  0.02,
				LoadKW:   3.94,
				NetKW:    3.92,
				SolarKWh: 94400.05,
				NetKWh:   -29376.45,
				LoadKWh:  65023.6,
			},
		},
		{
			name:   "zero values",
			params: notificationParams{},
			want:   Reading{Time: time.Unix(0, 0)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.params.toReading()
			if !got.Time.Equal(tt.want.Time) {
				t.Errorf("Time: got %v, want %v", got.Time, tt.want.Time)
			}
			if got.SolarKW != tt.want.SolarKW {
				t.Errorf("SolarKW: got %v, want %v", got.SolarKW, tt.want.SolarKW)
			}
			if got.LoadKW != tt.want.LoadKW {
				t.Errorf("LoadKW: got %v, want %v", got.LoadKW, tt.want.LoadKW)
			}
			if got.NetKW != tt.want.NetKW {
				t.Errorf("NetKW: got %v, want %v", got.NetKW, tt.want.NetKW)
			}
			if got.SolarKWh != tt.want.SolarKWh {
				t.Errorf("SolarKWh: got %v, want %v", got.SolarKWh, tt.want.SolarKWh)
			}
			if got.NetKWh != tt.want.NetKWh {
				t.Errorf("NetKWh: got %v, want %v", got.NetKWh, tt.want.NetKWh)
			}
			if got.LoadKWh != tt.want.LoadKWh {
				t.Errorf("LoadKWh: got %v, want %v", got.LoadKWh, tt.want.LoadKWh)
			}
		})
	}
}

func TestMonitorCurrent(t *testing.T) {
	tests := []struct {
		name    string
		reading *Reading
		wantNil bool
	}{
		{
			name:    "no reading yet",
			reading: nil,
			wantNil: true,
		},
		{
			name:    "reading available",
			reading: &Reading{SolarKW: 5.0, LoadKW: 3.0, NetKW: -2.0},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Monitor{}
			if tt.reading != nil {
				m.current = tt.reading
			}
			got := m.Current()
			if tt.wantNil && got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
			if !tt.wantNil && got != tt.reading {
				t.Errorf("expected %+v, got %+v", tt.reading, got)
			}
		})
	}
}

func TestReadingPower(t *testing.T) {
	ts := time.Unix(1779680954, 0)
	tests := []struct {
		name string
		r    Reading
		want PowerJSON
	}{
		{
			name: "typical",
			r:    Reading{Time: ts, SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92},
			want: PowerJSON{Time: ts.Format(time.RFC3339), SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92},
		},
		{
			name: "exporting",
			r:    Reading{Time: ts, SolarKW: 8.5, LoadKW: 2.1, NetKW: -6.4},
			want: PowerJSON{Time: ts.Format(time.RFC3339), SolarKW: 8.5, LoadKW: 2.1, NetKW: -6.4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Power()
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// fakeReader returns a fixed sequence of notifications, then an error.
type fakeReader struct {
	notifications []notification
	idx           int
	finalErr      error
}

func (f *fakeReader) read(_ context.Context, n *notification) error {
	if f.idx >= len(f.notifications) {
		return f.finalErr
	}
	*n = f.notifications[f.idx]
	f.idx++
	return nil
}

func TestRunLoop(t *testing.T) {
	ts := int64(1779680954)
	sentinel := errors.New("done")

	tests := []struct {
		name          string
		notifications []notification
		wantReading   *Reading
		wantErr       error
	}{
		{
			name: "power notification updates current",
			notifications: []notification{
				{
					Notification: "power",
					Params: notificationParams{
						Time: ts, SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92,
						SolarKWh: 94400.05, NetKWh: -29376.45, LoadKWh: 65023.6,
					},
				},
			},
			wantReading: &Reading{
				Time: time.Unix(ts, 0), SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92,
				SolarKWh: 94400.05, NetKWh: -29376.45, LoadKWh: 65023.6,
			},
			wantErr: sentinel,
		},
		{
			name: "non-power notification is ignored",
			notifications: []notification{
				{Notification: "status"},
				{
					Notification: "power",
					Params:       notificationParams{Time: ts, SolarKW: 1.0},
				},
			},
			wantReading: &Reading{Time: time.Unix(ts, 0), SolarKW: 1.0},
			wantErr:     sentinel,
		},
		{
			name:        "reader error propagates",
			wantReading: nil,
			wantErr:     sentinel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor("", config.Default(), nil, slog.New(slog.NewTextHandler(nil, nil)))
			r := &fakeReader{notifications: tt.notifications, finalErr: tt.wantErr}
			err := m.runLoop(context.Background(), r)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err: got %v, want %v", err, tt.wantErr)
			}
			got := m.Current()
			if tt.wantReading == nil {
				if got != nil {
					t.Errorf("expected nil current, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil current, got nil")
			}
			if !got.Time.Equal(tt.wantReading.Time) {
				t.Errorf("Time: got %v, want %v", got.Time, tt.wantReading.Time)
			}
			if got.SolarKW != tt.wantReading.SolarKW {
				t.Errorf("SolarKW: got %v, want %v", got.SolarKW, tt.wantReading.SolarKW)
			}
		})
	}
}

func TestReadingEnergy(t *testing.T) {
	ts := time.Unix(1779680954, 0)
	tests := []struct {
		name string
		r    Reading
		want EnergyJSON
	}{
		{
			name: "typical",
			r:    Reading{Time: ts, SolarKWh: 94400.05, LoadKWh: 65023.6, NetKWh: -29376.45},
			want: EnergyJSON{Time: ts.Format(time.RFC3339), SolarKWh: 94400.05, LoadKWh: 65023.6, NetKWh: -29376.45},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Energy()
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
