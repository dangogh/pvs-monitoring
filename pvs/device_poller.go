package pvs

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dangogh/pvs-monitoring/config"
)

// authError signals that the device rejected our credentials.
type authError struct{ status string }

func (e authError) Error() string {
	return fmt.Sprintf("device list authentication failed (%s): check pvs_password in config", e.status)
}

// httpDoer executes an HTTP request.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// DevicePoller periodically fetches the PVS6 device list via HTTP.
type DevicePoller struct {
	url      string
	authURL  string
	varsBase string // HTTPS base URL for /vars calls
	interval time.Duration
	username string
	password string
	client   httpDoer
	logger   *slog.Logger
	store    Store

	mu      sync.RWMutex
	current []Device
}

// NewDevicePoller creates a DevicePoller from config. store may be nil.
func NewDevicePoller(cfg config.DeviceListConfig, store Store, logger *slog.Logger) *DevicePoller {
	base := strings.TrimRight(cfg.URL, "/")
	httpsBase := strings.Replace(base, "http://", "https://", 1)
	authURL := cfg.AuthURL
	if authURL == "" {
		// Auth requires HTTPS; derive from the base URL.
		authURL = httpsBase + "/auth?login"
	}
	return &DevicePoller{
		url:      base + "/cgi-bin/dl_cgi/devices/list",
		authURL:  authURL,
		varsBase: httpsBase,
		interval: cfg.Interval.Duration(),
		username: cfg.Username,
		password: cfg.Password,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				// PVS6 uses a self-signed cert; force HTTP/1.1 to avoid Go's HTTP/2
				// client hanging on ALPN negotiation with InsecureSkipVerify.
				TLSClientConfig: &tls.Config{ //nolint:gosec
					InsecureSkipVerify: true,
					NextProtos:         []string{"http/1.1"},
				},
			},
		},
		logger: logger,
		store:  store,
	}
}

// Run polls the device list on a ticker until ctx is cancelled.
// Returns immediately with an error on authentication failure.
func (p *DevicePoller) Run(ctx context.Context) error {
	// Firmware 2025.10+ disables WebSocket telemetry by default; re-enable it on startup.
	if err := p.enableTelemetryWS(ctx); err != nil {
		if _, ok := err.(authError); ok {
			return err
		}
		p.logger.Warn("could not enable WebSocket telemetry", "err", err)
	}
	if err := p.poll(ctx); err != nil {
		if _, ok := err.(authError); ok {
			return err
		}
		p.logger.Error("device list poll failed", "err", err)
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				if _, ok := err.(authError); ok {
					return err
				}
				p.logger.Error("device list poll failed", "err", err)
			}
		}
	}
}

func (p *DevicePoller) poll(ctx context.Context) error {
	devices, err := p.fetch(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	p.mu.Lock()
	p.current = devices
	p.mu.Unlock()
	p.logger.Debug("device list updated", "count", len(devices))
	for _, d := range devices {
		p.logger.Debug("device", "type", d.DeviceType, "serial", d.Serial, "model", d.Model, "state", d.State, "descr", d.StateDescr)
	}
	if p.store != nil {
		if err := p.store.SaveDevices(ctx, devices, now); err != nil {
			p.logger.Error("store save devices failed", "err", err)
		}
	}
	return nil
}

// enableTelemetryWS sets /sys/telemetryws/enable=1 via the vars API.
// Newer PVS6 firmware disables WebSocket telemetry by default and the
// setting is reset on reboot.
func (p *DevicePoller) enableTelemetryWS(ctx context.Context) error {
	cookie, err := p.login(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.varsBase+"/vars?set=/sys/telemetryws/enable=1", nil)
	if err != nil {
		return fmt.Errorf("build telemetry request: %w", err)
	}
	req.AddCookie(cookie)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("enable telemetry: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enable telemetry returned %s", resp.Status)
	}
	p.logger.Debug("WebSocket telemetry enabled")
	return nil
}

// login authenticates with the PVS6 and returns the session cookie.
func (p *DevicePoller) login(ctx context.Context) (*http.Cookie, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build auth request: %w", err)
	}
	req.SetBasicAuth(p.username, p.password)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, authError{status: resp.Status}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth returned %s", resp.Status)
	}

	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			return c, nil
		}
	}
	return nil, fmt.Errorf("auth response missing session cookie")
}

func (p *DevicePoller) fetch(ctx context.Context) ([]Device, error) {
	cookie, err := p.login(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.AddCookie(cookie)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch device list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, authError{status: resp.Status}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device list returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return parseDeviceList(body)
}

// Current returns the most recently fetched device list.
func (p *DevicePoller) Current() []Device {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.current
}
