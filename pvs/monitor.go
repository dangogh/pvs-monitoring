package pvs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/dangogh/pvs-monitoring/config"
)

// Reading holds the most recent power snapshot from the PVS6.
type Reading struct {
	Time       time.Time
	ReceivedAt time.Time
	SolarKW    float64 // pv_p
	LoadKW     float64 // site_load_p
	NetKW      float64 // net_p (positive = drawing, negative = exporting)
	SolarKWh   float64 // pv_en
	NetKWh     float64 // net_en
	LoadKWh    float64 // site_load_en
}

// IsStale reports whether the reading is older than threshold.
func (r *Reading) IsStale(threshold time.Duration) bool {
	return time.Since(r.ReceivedAt) > threshold
}

// PowerJSON is the JSON representation of instantaneous power fields.
type PowerJSON struct {
	Time    string  `json:"time"`
	SolarKW float64 `json:"solar_kw"`
	LoadKW  float64 `json:"load_kw"`
	NetKW   float64 `json:"net_kw"`
}

// EnergyJSON is the JSON representation of cumulative energy fields.
type EnergyJSON struct {
	Time     string  `json:"time"`
	SolarKWh float64 `json:"solar_kwh"`
	LoadKWh  float64 `json:"load_kwh"`
	NetKWh   float64 `json:"net_kwh"`
}

// Power returns the instantaneous power fields as a PowerJSON.
func (r *Reading) Power() PowerJSON {
	return PowerJSON{
		Time:    r.Time.Format(time.RFC3339),
		SolarKW: r.SolarKW,
		LoadKW:  r.LoadKW,
		NetKW:   r.NetKW,
	}
}

// Energy returns the cumulative energy fields as an EnergyJSON.
func (r *Reading) Energy() EnergyJSON {
	return EnergyJSON{
		Time:     r.Time.Format(time.RFC3339),
		SolarKWh: r.SolarKWh,
		LoadKWh:  r.LoadKWh,
		NetKWh:   r.NetKWh,
	}
}

type notification struct {
	Notification string             `json:"notification"`
	Params       notificationParams `json:"params"`
}

type notificationParams struct {
	Time     int64   `json:"time"`
	SolarKW  float64 `json:"pv_p"`
	LoadKW   float64 `json:"site_load_p"`
	NetKW    float64 `json:"net_p"`
	SolarKWh float64 `json:"pv_en"`
	NetKWh   float64 `json:"net_en"`
	LoadKWh  float64 `json:"site_load_en"`
}

func (p notificationParams) toReading() *Reading {
	return &Reading{
		Time:     time.Unix(p.Time, 0),
		SolarKW:  p.SolarKW,
		LoadKW:   p.LoadKW,
		NetKW:    p.NetKW,
		SolarKWh: p.SolarKWh,
		NetKWh:   p.NetKWh,
		LoadKWh:  p.LoadKWh,
	}
}

// notificationReader is the interface for reading a stream of notifications.
type notificationReader interface {
	read(ctx context.Context, n *notification) error
}

// wsReader wraps a WebSocket connection as a notificationReader.
type wsReader struct {
	conn *websocket.Conn
}

func (w *wsReader) read(ctx context.Context, n *notification) error {
	return wsjson.Read(ctx, w.conn, n)
}

// Monitor connects to a PVS6 WebSocket and keeps the latest Reading.
type Monitor struct {
	addr             string
	reconnectInitial time.Duration
	reconnectMax     time.Duration
	staleThreshold   time.Duration
	logger           *slog.Logger
	store            Store

	mu      sync.RWMutex
	current *Reading
}

// NewMonitor creates a Monitor targeting the given WebSocket address.
// store may be nil to disable persistence.
func NewMonitor(addr string, cfg config.Config, store Store, logger *slog.Logger) *Monitor {
	return &Monitor{
		addr:             addr,
		reconnectInitial: cfg.ReconnectInitialInterval.Duration(),
		reconnectMax:     cfg.ReconnectMaxInterval.Duration(),
		staleThreshold:   cfg.StaleThreshold.Duration(),
		logger:           logger,
		store:            store,
	}
}

// Run connects and streams readings, reconnecting with exponential backoff
// until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	backoff := m.reconnectInitial
	for {
		m.logger.Debug("connecting to PVS6", "addr", m.addr)
		err := m.connect(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.logger.Error("connection lost, reconnecting", "err", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > m.reconnectMax {
			backoff = m.reconnectMax
		}
	}
}

func (m *Monitor) connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, m.addr, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", m.addr, err)
	}
	defer conn.CloseNow()
	return m.runLoop(ctx, &wsReader{conn: conn})
}

func (m *Monitor) runLoop(ctx context.Context, r notificationReader) error {
	for {
		var n notification
		if err := r.read(ctx, &n); err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if n.Notification != "power" {
			m.logger.Debug("ignoring notification", "type", n.Notification)
			continue
		}
		reading := n.Params.toReading()
		reading.ReceivedAt = time.Now()
		m.mu.Lock()
		m.current = reading
		m.mu.Unlock()
		m.logger.Debug("reading updated", "solar_kw", reading.SolarKW, "load_kw", reading.LoadKW, "net_kw", reading.NetKW)
		if m.store != nil {
			if err := m.store.SaveReading(ctx, reading); err != nil {
				m.logger.Error("store save failed", "err", err)
			}
		}
	}
}

// Current returns the most recent Reading, or nil if none has arrived yet.
func (m *Monitor) Current() *Reading {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// StaleThreshold returns the configured staleness threshold.
func (m *Monitor) StaleThreshold() time.Duration {
	return m.staleThreshold
}
