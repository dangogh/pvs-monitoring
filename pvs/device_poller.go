package pvs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/dangogh/pvs-monitoring/config"
)

// authError signals that the device rejected our credentials.
type authError struct{ status string }

func (e authError) Error() string {
	return fmt.Sprintf("device list authentication failed (%s): check pvs_password in config", e.status)
}

// DevicePoller periodically fetches the PVS6 device list via HTTP.
type DevicePoller struct {
	url      string
	interval time.Duration
	username string
	password string
	client   *http.Client
	logger   *slog.Logger
	store    Store

	mu      sync.RWMutex
	current []Device
}

// NewDevicePoller creates a DevicePoller from config. store may be nil.
func NewDevicePoller(cfg config.DeviceListConfig, store Store, logger *slog.Logger) *DevicePoller {
	return &DevicePoller{
		url:      cfg.URL + "/cgi-bin/dl_cgi/devices/list",
		interval: cfg.Interval.Duration(),
		username: cfg.Username,
		password: cfg.Password,
		client:   &http.Client{Timeout: 15 * time.Second},
		logger:   logger,
		store:    store,
	}
}

// Run polls the device list on a ticker until ctx is cancelled.
// Returns immediately with an error on authentication failure.
func (p *DevicePoller) Run(ctx context.Context) error {
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
	if p.store != nil {
		if err := p.store.SaveDevices(ctx, devices, now); err != nil {
			p.logger.Error("store save devices failed", "err", err)
		}
	}
	return nil
}

func (p *DevicePoller) fetch(ctx context.Context) ([]Device, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(p.username, p.password)

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
