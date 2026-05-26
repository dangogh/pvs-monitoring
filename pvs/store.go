package pvs

import (
	"context"
	"time"
)

// PowerAvg holds average power values over a time window.
type PowerAvg struct {
	SolarKW float64
	LoadKW  float64
	NetKW   float64
	Samples int
}

// Store persists and queries readings.
type Store interface {
	SaveReading(ctx context.Context, r *Reading) error
	AveragePower(ctx context.Context, since time.Time) (PowerAvg, error)
	SaveDevices(ctx context.Context, devices []Device, receivedAt time.Time) error
	LatestDevices(ctx context.Context) ([]Device, error)
	Close() error
}
