package pvs

import (
	"context"
	"errors"
	"fmt"
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

// fakeDialer returns a sequence of readers, one per dial call.
type fakeDialer struct {
	readers []notificationReader
	idx     int
	calls   int
	dialErr error
}

func (f *fakeDialer) dial(_ context.Context, _ string) (notificationReader, func(), error) {
	f.calls++
	if f.dialErr != nil {
		return nil, nil, f.dialErr
	}
	if f.idx >= len(f.readers) {
		return nil, nil, fmt.Errorf("no more readers")
	}
	r := f.readers[f.idx]
	f.idx++
	return r, func() {}, nil
}

func newTestMonitor(d dialer) *Monitor {
	cfg := config.Default()
	cfg.ReconnectInitialInterval = config.Duration(time.Millisecond)
	cfg.ReconnectMaxInterval = config.Duration(4 * time.Millisecond)
	m := NewMonitor("ws://test", cfg, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.dialer = d
	return m
}

func TestMonitorRunReconnectsAfterError(t *testing.T) {
	ts := int64(1779680954)

	// First connection errors immediately; second delivers a reading then stops.
	d := &fakeDialer{readers: []notificationReader{
		&fakeReader{finalErr: errors.New("connection reset")},
		&fakeReader{
			notifications: []notification{{
				Notification: "power",
				Params:       notificationParams{Time: ts, SolarKW: 5.0},
			}},
			finalErr: errors.New("done"),
		},
	}}
	m := newTestMonitor(d)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go m.Run(ctx) //nolint:errcheck

	require.Eventually(t, func() bool { return m.Current() != nil }, time.Second, time.Millisecond)
	assert.Equal(t, 5.0, m.Current().SolarKW)
}

func TestMonitorRunExitsOnContextCancel(t *testing.T) {
	d := &fakeDialer{dialErr: errors.New("refused")}
	m := newTestMonitor(d)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMonitorRunBackoffCapsAtMax(t *testing.T) {
	d := &fakeDialer{dialErr: errors.New("refused")}
	m := newTestMonitor(d)
	m.reconnectInitial = time.Millisecond
	m.reconnectMax = 2 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := m.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
	// With a 2ms max backoff and 20ms budget we should reconnect many times.
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.Greater(t, d.calls, 3, "expected multiple reconnect attempts")
}

// fakeStore is a minimal Store for stats tests.
type fakeStore struct {
	count int64
}

func (f *fakeStore) SaveReading(_ context.Context, _ *Reading) error   { return nil }
func (f *fakeStore) LatestReading(_ context.Context) (*Reading, error) { return nil, nil }
func (f *fakeStore) AveragePower(_ context.Context, _, _ time.Time) (PowerAvg, error) {
	return PowerAvg{}, nil
}
func (f *fakeStore) EnergyDelta(_ context.Context, _, _ time.Time) (EnergyDelta, error) {
	return EnergyDelta{}, nil
}
func (f *fakeStore) ReadingsSeries(_ context.Context, _, _ time.Time, _ int64) ([]SeriesPoint, error) {
	return nil, nil
}
func (f *fakeStore) CountReadings(_ context.Context) (int64, error)               { return f.count, nil }
func (f *fakeStore) SaveDevices(_ context.Context, _ []Device, _ time.Time) error  { return nil }
func (f *fakeStore) LatestInverters(_ context.Context) ([]InverterDevice, error)   { return nil, nil }
func (f *fakeStore) LatestAuxDevices(_ context.Context) ([]AuxDevice, error)              { return nil, nil }
func (f *fakeStore) OpenInverterOutage(_ context.Context, _ string, _ time.Time) error        { return nil }
func (f *fakeStore) CloseInverterOutage(_ context.Context, _ string, _ time.Time) error       { return nil }
func (f *fakeStore) ListOpenInverterOutages(_ context.Context) ([]string, error)              { return nil, nil }
func (f *fakeStore) Close() error                                                             { return nil }

func TestRunLoopCountsReadings(t *testing.T) {
	ts := int64(1779680954)
	notifications := []notification{
		{Notification: "power", Params: notificationParams{Time: ts, SolarKW: 1.0}},
		{Notification: "power", Params: notificationParams{Time: ts, SolarKW: 2.0}},
		{Notification: "status"},
		{Notification: "power", Params: notificationParams{Time: ts, SolarKW: 3.0}},
	}
	m := NewMonitor("", config.Default(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := &fakeReader{notifications: notifications, finalErr: errors.New("done")}
	_ = m.runLoop(context.Background(), r)

	assert.Equal(t, int64(3), m.totalAdded.Load())
	assert.Equal(t, int64(3), m.intervalAdded.Load())
}

func TestRunStatsLogsAndResetsInterval(t *testing.T) {
	store := &fakeStore{count: 42}
	m := NewMonitor("", config.Default(), store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.statsInterval = 10 * time.Millisecond
	m.totalAdded.Store(7)
	m.intervalAdded.Store(3)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go m.runStats(ctx)
	<-ctx.Done()

	// intervalAdded should have been reset to 0 (then incremented by zero new readings)
	assert.Equal(t, int64(0), m.intervalAdded.Load())
	// totalAdded is not reset
	assert.Equal(t, int64(7), m.totalAdded.Load())
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
