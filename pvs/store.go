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

// EnergyDelta holds the change in cumulative energy between two points in time.
type EnergyDelta struct {
	SolarKWh float64
	LoadKWh  float64
	NetKWh   float64
}

// SeriesPoint holds a time-bucketed average power reading for charting.
type SeriesPoint struct {
	Time    time.Time
	SolarKW float64
	LoadKW  float64
}

// Store persists and queries readings.
type Store interface {
	SaveReading(ctx context.Context, r *Reading) error
	LatestReading(ctx context.Context) (*Reading, error)
	AveragePower(ctx context.Context, since, until time.Time) (PowerAvg, error)
	EnergyDelta(ctx context.Context, since, until time.Time) (EnergyDelta, error)
	ReadingsSeries(ctx context.Context, since, until time.Time, bucketSeconds int64) ([]SeriesPoint, error)
	CountReadings(ctx context.Context) (int64, error)
	SaveDevices(ctx context.Context, devices []Device, receivedAt time.Time) error
	LatestInverters(ctx context.Context) ([]InverterDevice, error)
	LatestAuxDevices(ctx context.Context) ([]AuxDevice, error)
	OpenInverterOutage(ctx context.Context, serial string, at time.Time) error
	CloseInverterOutage(ctx context.Context, serial string, at time.Time) error
	Close() error
}
