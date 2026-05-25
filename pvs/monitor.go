package pvs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Reading holds the most recent power snapshot from the PVS6.
type Reading struct {
	Time     time.Time
	SolarKW  float64 // pv_p
	LoadKW   float64 // site_load_p
	NetKW    float64 // net_p (positive = drawing, negative = exporting)
	SolarKWh float64 // pv_en
	NetKWh   float64 // net_en
	LoadKWh  float64 // site_load_en
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

// Monitor connects to a PVS6 WebSocket and keeps the latest Reading.
type Monitor struct {
	addr   string
	logger *slog.Logger

	mu      sync.RWMutex
	current *Reading
}

// NewMonitor creates a Monitor targeting the given WebSocket address.
func NewMonitor(addr string, logger *slog.Logger) *Monitor {
	return &Monitor{addr: addr, logger: logger}
}

// Run connects and streams readings until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	m.logger.Debug("connecting to PVS6", "addr", m.addr)
	conn, _, err := websocket.Dial(ctx, m.addr, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", m.addr, err)
	}
	defer conn.CloseNow()

	for {
		var n notification
		if err := wsjson.Read(ctx, conn, &n); err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if n.Notification != "power" {
			m.logger.Debug("ignoring notification", "type", n.Notification)
			continue
		}
		r := n.Params.toReading()
		m.mu.Lock()
		m.current = r
		m.mu.Unlock()
		m.logger.Debug("reading updated", "solar_kw", r.SolarKW, "load_kw", r.LoadKW, "net_kw", r.NetKW)
	}
}

// Current returns the most recent Reading, or nil if none has arrived yet.
func (m *Monitor) Current() *Reading {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}
