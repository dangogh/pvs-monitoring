package pvs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(devices) != tt.wantDevices {
				t.Errorf("got %d devices, want %d", len(devices), tt.wantDevices)
			}
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
	if _, err := p.fetch(context.Background()); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotUser != "user" || gotPass != "pass" {
		t.Errorf("basic auth: got %q/%q, want user/pass", gotUser, gotPass)
	}
}

func TestDevicePollerCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(deviceListBody(twoDevices))
	}))
	defer srv.Close()
	p := newTestPoller(t, srv, nil)

	if got := p.Current(); got != nil {
		t.Errorf("expected nil before first poll, got %v", got)
	}

	if err := p.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	got := p.Current()
	if len(got) != 2 {
		t.Fatalf("got %d devices, want 2", len(got))
	}
	if got[0].Serial != "INV001" {
		t.Errorf("device[0].Serial = %q, want INV001", got[0].Serial)
	}
	if got[1].DeviceType != "Power Meter" {
		t.Errorf("device[1].DeviceType = %q, want Power Meter", got[1].DeviceType)
	}
}

func TestDevicePollerPollCallsStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(deviceListBody(twoDevices))
	}))
	defer srv.Close()

	store := &fakeDeviceStore{}
	p := newTestPoller(t, srv, store)

	if err := p.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.saveCount != 1 {
		t.Errorf("SaveDevices called %d times, want 1", store.saveCount)
	}
	if len(store.lastDevices) != 2 {
		t.Errorf("saved %d devices, want 2", len(store.lastDevices))
	}
}

func TestDevicePollerRunStopsOnAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	p := newTestPoller(t, srv, nil)
	p.interval = 10 * time.Millisecond

	err := p.Run(context.Background())
	var ae authError
	if !errors.As(err, &ae) {
		t.Errorf("expected authError, got %v", err)
	}
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

	// Run until we get a successful poll (current becomes non-nil).
	for p.Current() == nil && ctx.Err() == nil {
		_ = p.poll(ctx)
	}

	if p.Current() == nil {
		t.Error("expected devices after recovery, got nil")
	}
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
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// fakeDeviceStore records SaveDevices calls.
type fakeDeviceStore struct {
	mu          sync.Mutex
	saveCount   int
	lastDevices []Device
}

func (f *fakeDeviceStore) SaveReading(_ context.Context, _ *Reading) error  { return nil }
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
func (f *fakeDeviceStore) Close() error { return nil }
