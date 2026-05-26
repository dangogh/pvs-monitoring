package pvs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			assert.True(t, got.Time.Equal(tt.want.Time), "Time")
			assert.Equal(t, tt.want.SolarKW, got.SolarKW, "SolarKW")
			assert.Equal(t, tt.want.LoadKW, got.LoadKW, "LoadKW")
			assert.Equal(t, tt.want.NetKW, got.NetKW, "NetKW")
			assert.Equal(t, tt.want.SolarKWh, got.SolarKWh, "SolarKWh")
			assert.Equal(t, tt.want.NetKWh, got.NetKWh, "NetKWh")
			assert.Equal(t, tt.want.LoadKWh, got.LoadKWh, "LoadKWh")
		})
	}
}

func TestMonitorCurrent(t *testing.T) {
	tests := []struct {
		name    string
		reading *Reading
	}{
		{name: "no reading yet", reading: nil},
		{name: "reading available", reading: &Reading{SolarKW: 5.0, LoadKW: 3.0, NetKW: -2.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Monitor{}
			if tt.reading != nil {
				m.current = tt.reading
			}
			assert.Equal(t, tt.reading, m.Current())
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
			assert.Equal(t, tt.want, tt.r.Power())
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
			m := NewMonitor("", config.Default(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := &fakeReader{notifications: tt.notifications, finalErr: tt.wantErr}
			err := m.runLoop(context.Background(), r)
			assert.ErrorIs(t, err, tt.wantErr)

			got := m.Current()
			if tt.wantReading == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, got.Time.Equal(tt.wantReading.Time), "Time")
			assert.Equal(t, tt.wantReading.SolarKW, got.SolarKW, "SolarKW")
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
			assert.Equal(t, tt.want, tt.r.Energy())
		})
	}
}
