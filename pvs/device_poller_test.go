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

func newTestPoller(t *testing.T, srv *httptest.Server, store Store) *DevicePoller {
	t.Helper()
	cfg := config.DeviceListConfig{
		URL:      srv.URL,
		Interval: config.Duration(time.Hour), // long — tests drive polling manually
		Username: "user",
		Password: "pass",
	}
	return NewDevicePoller(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

var twoDevices = []map[string]any{
	{"SERIAL": "INV001", "DEVICE_TYPE": "Inverter", "TYPE": "MI", "MODEL": "SPR-X22", "STATE": "working", "STATEDESCR": "Working"},
	{"SERIAL": "MTR001", "DEVICE_TYPE": "Power Meter", "TYPE": "PVS5-METER-P", "MODEL": "PVS6M", "STATE": "working", "STATEDESCR": "Working"},
}

func TestDevicePollerFetch(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantDevices int
		wantErr     string
	}{
		{
			name: "returns devices on 200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write(deviceListBody(twoDevices))
			},
			wantDevices: 2,
		},
		{
			name: "401 returns authError",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: "authentication failed",
		},
		{
			name: "403 returns authError",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: "authentication failed",
		},
		{
			name: "500 returns generic error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.Write(deviceListBody(nil))
	}))
	defer srv.Close()

	p := newTestPoller(t, srv, nil)
	_, err := p.fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "user", gotUser)
	assert.Equal(t, "pass", gotPass)
}

func TestDevicePollerCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(deviceListBody(twoDevices))
	}))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(deviceListBody(twoDevices))
	}))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	p := newTestPoller(t, srv, nil)
	p.interval = 10 * time.Millisecond

	err := p.Run(context.Background())
	assert.ErrorAs(t, err, &authError{})
}

func TestDevicePollerRunContinuesOnTransientError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(deviceListBody(twoDevices))
	}))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(deviceListBody(twoDevices))
	}))
	defer srv.Close()
	p := newTestPoller(t, srv, nil)
	p.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
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
		Body:       io.NopCloser(bytes.NewReader(f.body)),
	}, nil
}

func newPollerWithDoer(t *testing.T, doer httpDoer) *DevicePoller {
	t.Helper()
	p := &DevicePoller{
		url:      "http://fake/cgi-bin/dl_cgi/devices/list",
		interval: time.Hour,
		username: "user",
		password: "pass",
		client:   doer,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return p
}

func TestDevicePollerFetchWithFakeDoer(t *testing.T) {
	tests := []struct {
		name        string
		doer        *fakeDoer
		wantDevices int
		wantErr     string
	}{
		{
			name:        "200 returns devices",
			doer:        &fakeDoer{statusCode: http.StatusOK, body: deviceListBody(twoDevices)},
			wantDevices: 2,
		},
		{
			name:    "401 returns authError",
			doer:    &fakeDoer{statusCode: http.StatusUnauthorized},
			wantErr: "authentication failed",
		},
		{
			name:    "403 returns authError",
			doer:    &fakeDoer{statusCode: http.StatusForbidden},
			wantErr: "authentication failed",
		},
		{
			name:    "500 returns generic error",
			doer:    &fakeDoer{statusCode: http.StatusInternalServerError},
			wantErr: "Internal Server Error",
		},
		{
			name:    "network error propagates",
			doer:    &fakeDoer{err: errors.New("connection refused")},
			wantErr: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPollerWithDoer(t, tt.doer)
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

// fakeDeviceStore records SaveDevices calls.
type fakeDeviceStore struct {
	mu          sync.Mutex
	saveCount   int
	lastDevices []Device
}

func (f *fakeDeviceStore) SaveReading(_ context.Context, _ *Reading) error { return nil }
func (f *fakeDeviceStore) AveragePower(_ context.Context, _ time.Time) (PowerAvg, error) {
	return PowerAvg{}, nil
}
func (f *fakeDeviceStore) SaveDevices(_ context.Context, devices []Device, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCount++
	f.lastDevices = devices
	return nil
}
func (f *fakeDeviceStore) LatestDevices(_ context.Context) ([]Device, error) { return nil, nil }
func (f *fakeDeviceStore) Close() error                                       { return nil }
