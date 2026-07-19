package pvs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolsStore is a fake Store for tools tests.
type toolsStore struct {
	reading    *Reading
	readingErr error
	avg        PowerAvg
	avgErr     error
}

func (f *toolsStore) SaveReading(_ context.Context, _ *Reading) error { return nil }
func (f *toolsStore) LatestReading(_ context.Context) (*Reading, error) {
	return f.reading, f.readingErr
}
func (f *toolsStore) AveragePower(_ context.Context, _, _ time.Time) (PowerAvg, error) {
	return f.avg, f.avgErr
}
func (f *toolsStore) EnergyDelta(_ context.Context, _, _ time.Time) (EnergyDelta, error) {
	return EnergyDelta{}, nil
}
func (f *toolsStore) ReadingsSeries(_ context.Context, _, _ time.Time, _ int64) ([]SeriesPoint, error) {
	return nil, nil
}
func (f *toolsStore) CountReadings(_ context.Context) (int64, error)                     { return 0, nil }
func (f *toolsStore) EarliestReadingAt(_ context.Context) (time.Time, error)             { return time.Time{}, nil }
func (f *toolsStore) SaveDevices(_ context.Context, _ []Device, _ time.Time) error       { return nil }
func (f *toolsStore) LatestInverters(_ context.Context) ([]InverterDevice, error)        { return nil, nil }
func (f *toolsStore) InverterSeries(_ context.Context, _, _ time.Time) ([]InverterSeriesPoint, error) { return nil, nil }
func (f *toolsStore) LatestAuxDevices(_ context.Context) ([]AuxDevice, error)            { return nil, nil }
func (f *toolsStore) OpenInverterOutage(_ context.Context, _ string, _ time.Time) error  { return nil }
func (f *toolsStore) CloseInverterOutage(_ context.Context, _ string, _ time.Time) error { return nil }
func (f *toolsStore) ListOpenInverterOutages(_ context.Context) ([]string, error)                    { return nil, nil }
func (f *toolsStore) SaveMaintenanceEvent(_ context.Context, _ MaintenanceEvent) (int64, error)     { return 0, nil }
func (f *toolsStore) ListMaintenanceEvents(_ context.Context) ([]MaintenanceEvent, error)           { return nil, nil }
func (f *toolsStore) Checkpoint(_ context.Context) error                                            { return nil }
func (f *toolsStore) Close() error                                                                  { return nil }

func freshReading(r *Reading) *Reading {
	r.ReceivedAt = time.Now()
	return r
}

func staleReading(r *Reading) *Reading {
	r.ReceivedAt = time.Now().Add(-time.Minute)
	return r
}

func TestCurrentPower(t *testing.T) {
	ts := time.Unix(1779680954, 0)
	tests := []struct {
		name           string
		reading        *Reading
		staleThreshold time.Duration
		wantErr        bool
		want           PowerJSON
	}{
		{
			name:    "no reading",
			reading: nil,
			wantErr: true,
		},
		{
			name:           "stale reading",
			reading:        staleReading(&Reading{Time: ts}),
			staleThreshold: 5 * time.Second,
			wantErr:        true,
		},
		{
			name:           "fresh reading",
			reading:        freshReading(&Reading{Time: ts, SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92}),
			staleThreshold: 5 * time.Second,
			want:           PowerJSON{Time: ts.Format(time.RFC3339), SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &toolsStore{reading: tt.reading}
			result, _, err := currentPower(context.Background(), store, tt.staleThreshold)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got PowerJSON
			text := result.Content[0].(*mcp.TextContent).Text
			if err := json.Unmarshal([]byte(text), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnergySummary(t *testing.T) {
	ts := time.Unix(1779680954, 0)
	tests := []struct {
		name           string
		reading        *Reading
		staleThreshold time.Duration
		wantErr        bool
		want           EnergyJSON
	}{
		{
			name:    "no reading",
			reading: nil,
			wantErr: true,
		},
		{
			name:           "stale reading",
			reading:        staleReading(&Reading{Time: ts}),
			staleThreshold: 5 * time.Second,
			wantErr:        true,
		},
		{
			name:           "fresh reading",
			reading:        freshReading(&Reading{Time: ts, SolarKWh: 94400.05, LoadKWh: 65023.6, NetKWh: -29376.45}),
			staleThreshold: 5 * time.Second,
			want:           EnergyJSON{Time: ts.Format(time.RFC3339), SolarKWh: 94400.05, LoadKWh: 65023.6, NetKWh: -29376.45},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &toolsStore{reading: tt.reading}
			result, _, err := energySummary(context.Background(), store, tt.staleThreshold)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got EnergyJSON
			text := result.Content[0].(*mcp.TextContent).Text
			if err := json.Unmarshal([]byte(text), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
