package pvs

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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
	varsBase string // base URL for /vars calls
	interval time.Duration
	username string
	password string
	client   httpDoer
	logger   *slog.Logger
	store    Store

	mu                sync.RWMutex
	current           []Device
	lastInverterState map[string]string // serial → last saved state; guards outage open/close logic
}

// newPVSTLSConfig returns a TLS config for the PVS6 HTTPS connection.
// When fingerprint is non-empty the certificate is pinned to that SHA-256 digest
// (hex, colons optional); otherwise verification is skipped entirely.
// InsecureSkipVerify must remain true in both cases because the PVS6 uses a
// self-signed cert that Go would reject before calling VerifyPeerCertificate.
func newPVSTLSConfig(fingerprint string) *tls.Config {
	cfg := &tls.Config{ //nolint:gosec
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
	}
	if fingerprint == "" {
		return cfg
	}
	want := strings.ToLower(strings.ReplaceAll(fingerprint, ":", ""))
	cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("pvs6: no certificate presented")
		}
		got := fmt.Sprintf("%x", sha256.Sum256(rawCerts[0]))
		if got != want {
			return fmt.Errorf("pvs6: certificate fingerprint mismatch: got %s want %s", got, want)
		}
		return nil
	}
	return cfg
}

// NewDevicePoller creates a DevicePoller from config. store may be nil.
func NewDevicePoller(cfg config.DeviceListConfig, store Store, logger *slog.Logger) *DevicePoller {
	base := strings.TrimRight(cfg.URL, "/")
	authURL := cfg.AuthURL
	if authURL == "" {
		authURL = base + "/auth?login"
	}
	if cfg.TLSFingerprint == "" {
		logger.Warn("TLS certificate verification is disabled; set pvs_tls_fingerprint in config to pin the PVS6 certificate")
	}
	return &DevicePoller{
		url:      base + "/cgi-bin/dl_cgi/devices/list",
		authURL:  authURL,
		varsBase: base,
		interval: cfg.Interval.Duration(),
		username: cfg.Username,
		password: cfg.Password,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				// Force HTTP/1.1 in case TLS is in use; avoids Go's HTTP/2 client
				// hanging on ALPN negotiation with InsecureSkipVerify.
				TLSClientConfig: newPVSTLSConfig(cfg.TLSFingerprint),
			},
		},
		logger:            logger,
		store:             store,
		lastInverterState: make(map[string]string),
	}
}

// seedFromStore initialises lastInverterState from any open outages in the store.
// This ensures that inverters which recovered while the daemon was down have
// their outages closed correctly on the first poll after a restart.
func (p *DevicePoller) seedFromStore(ctx context.Context) {
	serials, err := p.store.ListOpenInverterOutages(ctx)
	if err != nil {
		p.logger.Warn("could not load open outages for state seeding", "err", err)
		return
	}
	for _, serial := range serials {
		p.lastInverterState[serial] = "error"
	}
}

// Run polls the device list on a ticker until ctx is cancelled.
// Returns immediately with an error on authentication failure.
func (p *DevicePoller) Run(ctx context.Context) error {
	if p.store != nil {
		p.seedFromStore(ctx)
	}
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

	stateCounts := make(map[string]int)
	for _, d := range devices {
		stateCounts[d.State]++
		p.logger.Debug("device", "type", d.DeviceType, "serial", d.Serial, "model", d.Model, "state", d.State, "descr", d.StateDescr)
	}
	args := []any{"count", len(devices)}
	for state, n := range stateCounts {
		args = append(args, state, n)
	}
	p.logger.Info("device list updated", args...)

	// Build the list of devices to persist. Inverters in a sustained error state are
	// tracked in inverter_outages instead of inverter_readings to avoid accumulating
	// thousands of identical rows overnight.
	toSave := make([]Device, 0, len(devices))
	for _, d := range devices {
		if d.DeviceType != "Inverter" {
			toSave = append(toSave, d)
			continue
		}
		prev := p.lastInverterState[d.Serial]
		switch {
		case d.State == "error" && prev == "error":
			// sustained error: skip
		case d.State == "error":
			// transition to error: open an outage record, don't write to inverter_readings
			p.lastInverterState[d.Serial] = "error"
			if p.store != nil {
				if err := p.store.OpenInverterOutage(ctx, d.Serial, now); err != nil {
					p.logger.Error("open outage failed", "serial", d.Serial, "err", err)
				}
			}
		case prev == "error":
			// recovery: close the outage and save the healthy reading
			p.lastInverterState[d.Serial] = d.State
			if p.store != nil {
				if err := p.store.CloseInverterOutage(ctx, d.Serial, now); err != nil {
					p.logger.Error("close outage failed", "serial", d.Serial, "err", err)
				}
			}
			toSave = append(toSave, d)
		default:
			// healthy (working → working, or first poll)
			p.lastInverterState[d.Serial] = d.State
			toSave = append(toSave, d)
		}
	}

	if p.store != nil && len(toSave) > 0 {
		if err := p.store.SaveDevices(ctx, toSave, now); err != nil {
			return fmt.Errorf("save devices: %w", err)
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
	telemetryURL := p.varsBase + "/vars?set=/sys/telemetryws/enable=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, telemetryURL, nil)
	if err != nil {
		return fmt.Errorf("build telemetry request: %w", err)
	}
	req.AddCookie(cookie)
	p.logger.Debug("telemetry request", "url", telemetryURL)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("enable telemetry: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	p.logger.Debug("telemetry response", "status", resp.StatusCode)
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
	p.logger.Debug("auth request", "url", p.authURL)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	p.logger.Debug("auth response", "status", resp.StatusCode)

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
	p.logger.Debug("device list request", "url", p.url)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch device list: %w", err)
	}
	defer resp.Body.Close()
	p.logger.Debug("device list response", "status", resp.StatusCode)

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
