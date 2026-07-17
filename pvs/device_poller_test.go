package pvs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangogh/pvs-monitoring/config"
)

// deviceListBody builds a minimal DeviceList JSON response.
func deviceListBody(devices []map[string]any) []byte {
	data, _ := json.Marshal(map[string]any{"devices": devices})
	return data
}

// newDevServer creates a test server with separate handlers for auth and device list.
// authHandler may be nil to use a default that always returns a valid session cookie.
func newDevServer(t *testing.T, authHandler, devHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	if authHandler == nil {
		authHandler = func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "testsession"})
			_, _ = fmt.Fprint(w, `{"session":"testsession"}`)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/vars", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"values":[{"name":"/sys/telemetryws/enable","value":"1"}],"count":1}`)
	})
	mux.HandleFunc("/cgi-bin/dl_cgi/devices/list", devHandler)
	return httptest.NewServer(mux)
}

func newTestPoller(t *testing.T, srv *httptest.Server, store Store) *DevicePoller {
	t.Helper()
	cfg := config.DeviceListConfig{
		URL:      srv.URL,
		AuthURL:  srv.URL + "/auth",
		Interval: config.Duration(time.Hour), // long — tests drive polling manually
		Username: "user",
		Password: "pass",
	}
	p := NewDevicePoller(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.varsBase = srv.URL // override to HTTP for test server
	return p
}

var twoDevices = []map[string]any{
	{"SERIAL": "INV001", "DEVICE_TYPE": "Inverter", "TYPE": "MI", "MODEL": "SPR-X22", "STATE": "working", "STATEDESCR": "Working"},
	{"SERIAL": "MTR001", "DEVICE_TYPE": "Power Meter", "TYPE": "PVS5-METER-P", "MODEL": "PVS6M", "STATE": "working", "STATEDESCR": "Working"},
}

func TestDevicePollerFetch(t *testing.T) {
	tests := []struct {
		name        string
		devHandler  http.HandlerFunc
		wantDevices int
		wantErr     string
	}{
		{
			name: "returns devices on 200",
			devHandler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(deviceListBody(twoDevices))
			},
			wantDevices: 2,
		},
		{
			name: "401 returns authError",
			devHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: "authentication failed",
		},
		{
			name: "403 returns authError",
			devHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: "authentication failed",
		},
		{
			name: "500 returns generic error",
			devHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newDevServer(t, nil, tt.devHandler)
			defer srv.Close()
			p := newTestPoller(t, srv, nil)

			devices, err := p.fetch(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, devices, tt.wantDevices)
		})
	}
}

func TestDevicePollerFetchSetsBasicAuth(t *testing.T) {
	var gotUser, gotPass string
	authHandler := func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "testsession"})
		_, _ = fmt.Fprint(w, `{"session":"testsession"}`)
	}
	devHandler := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(deviceListBody(nil))
	}
	srv := newDevServer(t, authHandler, devHandler)
	defer srv.Close()

	p := newTestPoller(t, srv, nil)
	_, err := p.fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "user", gotUser)
	assert.Equal(t, "pass", gotPass)
}

func TestDevicePollerFetchSendsCookieToDeviceList(t *testing.T) {
	var gotCookie string
	devHandler := func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err == nil {
			gotCookie = c.Value
		}
		_, _ = w.Write(deviceListBody(nil))
	}
	srv := newDevServer(t, nil, devHandler)
	defer srv.Close()

	p := newTestPoller(t, srv, nil)
	_, err := p.fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "testsession", gotCookie)
}

func TestDevicePollerAuthFailure(t *testing.T) {
	tests := []struct {
		name       string
		authStatus int
	}{
		{"401 from auth", http.StatusUnauthorized},
		{"403 from auth", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authHandler := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.authStatus)
			}
			srv := newDevServer(t, authHandler, func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			p := newTestPoller(t, srv, nil)
			_, err := p.fetch(context.Background())
			require.Error(t, err)
			assert.ErrorAs(t, err, &authError{})
		})
	}
}

func TestDevicePollerCurrent(t *testing.T) {
	srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(deviceListBody(twoDevices))
	})
	defer srv.Close()
	p := newTestPoller(t, srv, nil)

	assert.Nil(t, p.Current())

	require.NoError(t, p.poll(context.Background()))

	got := p.Current()
	require.Len(t, got, 2)
	assert.Equal(t, "INV001", got[0].Serial)
	assert.Equal(t, "Power Meter", got[1].DeviceType)
}

func TestDevicePollerPollCallsStore(t *testing.T) {
	srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(deviceListBody(twoDevices))
	})
	defer srv.Close()

	store := &fakeDeviceStore{}
	p := newTestPoller(t, srv, store)

	require.NoError(t, p.poll(context.Background()))

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, 1, store.saveCount)
	assert.Len(t, store.lastDevices, 2)
}

func TestDevicePollerRunStopsOnAuthError(t *testing.T) {
	srv := newDevServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	p := newTestPoller(t, srv, nil)
	p.interval = 10 * time.Millisecond

	err := p.Run(context.Background())
	assert.ErrorAs(t, err, &authError{})
}

func TestDevicePollerRunContinuesOnTransientError(t *testing.T) {
	calls := 0
	srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(deviceListBody(twoDevices))
	})
	defer srv.Close()

	p := newTestPoller(t, srv, nil)
	p.interval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for p.Current() == nil && ctx.Err() == nil {
		_ = p.poll(ctx)
	}

	assert.NotNil(t, p.Current())
}

func TestDevicePollerRunCancelledByContext(t *testing.T) {
	srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(deviceListBody(twoDevices))
	})
	defer srv.Close()
	p := newTestPoller(t, srv, nil)
	p.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

// twoStepDoer handles auth (returns a session cookie) and delegates device list to devDoer.
type twoStepDoer struct {
	devDoer *fakeDoer
}

func (d *twoStepDoer) Do(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "auth") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Set-Cookie": []string{"session=testsession"}},
			Body:       io.NopCloser(strings.NewReader(`{"session":"testsession"}`)),
		}, nil
	}
	if strings.Contains(req.URL.Path, "vars") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"values":[{"name":"/sys/telemetryws/enable","value":"1"}],"count":1}`)),
		}, nil
	}
	return d.devDoer.Do(req)
}

// fakeDoer implements httpDoer with a configurable response.
type fakeDoer struct {
	statusCode int
	body       []byte
	err        error
}

func (f *fakeDoer) Do(_ *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: f.statusCode,
		Status:     fmt.Sprintf("%d %s", f.statusCode, http.StatusText(f.statusCode)),
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(f.body)),
	}, nil
}

func newPollerWithDoer(t *testing.T, doer httpDoer) *DevicePoller {
	t.Helper()
	p := &DevicePoller{
		url:               "http://fake/cgi-bin/dl_cgi/devices/list",
		authURL:           "http://fake/auth",
		varsBase:          "http://fake",
		interval:          time.Hour,
		username:          "user",
		password:          "pass",
		client:            doer,
		logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		lastInverterState: make(map[string]string),
	}
	return p
}

func TestDevicePollerFetchWithFakeDoer(t *testing.T) {
	tests := []struct {
		name        string
		devDoer     *fakeDoer
		wantDevices int
		wantErr     string
	}{
		{
			name:        "200 returns devices",
			devDoer:     &fakeDoer{statusCode: http.StatusOK, body: deviceListBody(twoDevices)},
			wantDevices: 2,
		},
		{
			name:    "401 returns authError",
			devDoer: &fakeDoer{statusCode: http.StatusUnauthorized},
			wantErr: "authentication failed",
		},
		{
			name:    "403 returns authError",
			devDoer: &fakeDoer{statusCode: http.StatusForbidden},
			wantErr: "authentication failed",
		},
		{
			name:    "500 returns generic error",
			devDoer: &fakeDoer{statusCode: http.StatusInternalServerError},
			wantErr: "Internal Server Error",
		},
		{
			name:    "network error propagates",
			devDoer: &fakeDoer{err: errors.New("connection refused")},
			wantErr: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPollerWithDoer(t, &twoStepDoer{devDoer: tt.devDoer})
			devices, err := p.fetch(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, devices, tt.wantDevices)
		})
	}
}

// fakeOutage records a single open/close outage event pair.
type fakeOutage struct {
	serial    string
	errorAt   time.Time
	healthyAt time.Time // zero if still open
}

// fakeDeviceStore records SaveDevices and outage calls.
type fakeDeviceStore struct {
	mu          sync.Mutex
	saveCount   int
	lastDevices []Device
	outages     []fakeOutage
	inverters   []InverterDevice // pre-populated for LatestInverters
}

func (f *fakeDeviceStore) SaveReading(_ context.Context, _ *Reading) error   { return nil }
func (f *fakeDeviceStore) LatestReading(_ context.Context) (*Reading, error) { return nil, nil }
func (f *fakeDeviceStore) AveragePower(_ context.Context, _, _ time.Time) (PowerAvg, error) {
	return PowerAvg{}, nil
}
func (f *fakeDeviceStore) EnergyDelta(_ context.Context, _, _ time.Time) (EnergyDelta, error) {
	return EnergyDelta{}, nil
}
func (f *fakeDeviceStore) SaveDevices(_ context.Context, devices []Device, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCount++
	f.lastDevices = devices
	return nil
}
func (f *fakeDeviceStore) ReadingsSeries(_ context.Context, _, _ time.Time, _ int64) ([]SeriesPoint, error) {
	return nil, nil
}
func (f *fakeDeviceStore) CountReadings(_ context.Context) (int64, error) { return 0, nil }
func (f *fakeDeviceStore) EarliestReadingAt(_ context.Context) (time.Time, error) {
	return time.Time{}, nil
}
func (f *fakeDeviceStore) LatestInverters(_ context.Context) ([]InverterDevice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inverters, nil
}
func (f *fakeDeviceStore) LatestAuxDevices(_ context.Context) ([]AuxDevice, error) { return nil, nil }
func (f *fakeDeviceStore) OpenInverterOutage(_ context.Context, serial string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outages = append(f.outages, fakeOutage{serial: serial, errorAt: at})
	return nil
}
func (f *fakeDeviceStore) CloseInverterOutage(_ context.Context, serial string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.outages) - 1; i >= 0; i-- {
		if f.outages[i].serial == serial && f.outages[i].healthyAt.IsZero() {
			f.outages[i].healthyAt = at
			return nil
		}
	}
	return nil
}
func (f *fakeDeviceStore) ListOpenInverterOutages(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, o := range f.outages {
		if o.healthyAt.IsZero() {
			out = append(out, o.serial)
		}
	}
	return out, nil
}
func (f *fakeDeviceStore) SaveMaintenanceEvent(_ context.Context, _ MaintenanceEvent) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceStore) ListMaintenanceEvents(_ context.Context) ([]MaintenanceEvent, error) {
	return nil, nil
}
func (f *fakeDeviceStore) Checkpoint(_ context.Context) error { return nil }
func (f *fakeDeviceStore) Close() error                       { return nil }

func TestDevicePollerOutageTracking(t *testing.T) {
	ctx := context.Background()

	inv := func(state string) map[string]any {
		return map[string]any{
			"SERIAL": "INV001", "DEVICE_TYPE": "Inverter",
			"TYPE": "MI", "MODEL": "SPR-X22", "STATE": state, "STATEDESCR": state,
		}
	}

	t.Run("sustained error only opens one outage", func(t *testing.T) {
		responses := [][]map[string]any{
			{inv("error")},
			{inv("error")},
			{inv("error")},
		}
		i := 0
		srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(deviceListBody(responses[i]))
			i++
		})
		defer srv.Close()
		store := &fakeDeviceStore{}
		p := newTestPoller(t, srv, store)

		for range responses {
			require.NoError(t, p.poll(ctx))
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		assert.Equal(t, 1, store.saveCount, "only one inverter_readings row written on first error transition")
		require.Len(t, store.outages, 1)
		assert.Equal(t, "INV001", store.outages[0].serial)
		assert.True(t, store.outages[0].healthyAt.IsZero(), "outage should still be open")
	})

	t.Run("working to error opens outage", func(t *testing.T) {
		responses := [][]map[string]any{
			{inv("working")},
			{inv("error")},
		}
		i := 0
		srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(deviceListBody(responses[i]))
			i++
		})
		defer srv.Close()
		store := &fakeDeviceStore{}
		p := newTestPoller(t, srv, store)

		require.NoError(t, p.poll(ctx)) // working → saved
		require.NoError(t, p.poll(ctx)) // error → outage opened, reading saved once

		store.mu.Lock()
		defer store.mu.Unlock()
		assert.Equal(t, 2, store.saveCount, "working poll and first error transition both saved")
		require.Len(t, store.outages, 1)
		assert.True(t, store.outages[0].healthyAt.IsZero())
	})

	t.Run("error to working closes outage and saves reading", func(t *testing.T) {
		responses := [][]map[string]any{
			{inv("error")},
			{inv("working")},
		}
		i := 0
		srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(deviceListBody(responses[i]))
			i++
		})
		defer srv.Close()
		store := &fakeDeviceStore{}
		p := newTestPoller(t, srv, store)

		require.NoError(t, p.poll(ctx)) // error → outage opened, reading saved once
		require.NoError(t, p.poll(ctx)) // working → outage closed, reading saved

		store.mu.Lock()
		defer store.mu.Unlock()
		assert.Equal(t, 2, store.saveCount)
		require.Len(t, store.outages, 1)
		assert.False(t, store.outages[0].healthyAt.IsZero(), "outage should be closed")
	})

	t.Run("restart: open outage closed when inverter already recovered", func(t *testing.T) {
		// Simulate daemon restart: store has an open outage from before the restart,
		// but the inverter is now healthy. Without seeding, lastInverterState is empty
		// so poll() would see prev="" and not close the outage.
		store := &fakeDeviceStore{
			outages:   []fakeOutage{{serial: "INV001", errorAt: time.Now().Add(-8 * time.Hour)}},
			inverters: []InverterDevice{{Serial: "INV001"}},
		}
		srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(deviceListBody([]map[string]any{inv("working")}))
		})
		defer srv.Close()
		p := newTestPoller(t, srv, store)

		p.seedFromStore(ctx)
		require.NoError(t, p.poll(ctx))

		store.mu.Lock()
		defer store.mu.Unlock()
		require.Len(t, store.outages, 1)
		assert.False(t, store.outages[0].healthyAt.IsZero(), "outage should be closed after recovery")
		assert.Equal(t, 1, store.saveCount, "healthy reading saved after recovery")
	})

	t.Run("non-inverter devices always saved", func(t *testing.T) {
		mtr := map[string]any{
			"SERIAL": "MTR001", "DEVICE_TYPE": "Power Meter",
			"TYPE": "PVS5-METER-P", "MODEL": "PVS6M", "STATE": "working", "STATEDESCR": "Working",
		}
		responses := [][]map[string]any{
			{inv("error"), mtr},
			{inv("error"), mtr},
		}
		i := 0
		srv := newDevServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(deviceListBody(responses[i]))
			i++
		})
		defer srv.Close()
		store := &fakeDeviceStore{}
		p := newTestPoller(t, srv, store)

		require.NoError(t, p.poll(ctx))
		require.NoError(t, p.poll(ctx))

		store.mu.Lock()
		defer store.mu.Unlock()
		assert.Equal(t, 2, store.saveCount, "meter saved on every poll")
		assert.Len(t, store.outages, 1, "only one outage opened for inverter")
	})
}
