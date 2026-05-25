package pvs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
		current        *Reading
		staleThreshold time.Duration
		wantErr        bool
		want           PowerJSON
	}{
		{
			name:    "no reading",
			current: nil,
			wantErr: true,
		},
		{
			name:           "stale reading",
			current:        staleReading(&Reading{Time: ts}),
			staleThreshold: 5 * time.Second,
			wantErr:        true,
		},
		{
			name:           "fresh reading",
			current:        freshReading(&Reading{Time: ts, SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92}),
			staleThreshold: 5 * time.Second,
			want:           PowerJSON{Time: ts.Format(time.RFC3339), SolarKW: 0.02, LoadKW: 3.94, NetKW: 3.92},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Monitor{current: tt.current, staleThreshold: tt.staleThreshold}
			result, _, err := currentPower(m)
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
		current        *Reading
		staleThreshold time.Duration
		wantErr        bool
		want           EnergyJSON
	}{
		{
			name:    "no reading",
			current: nil,
			wantErr: true,
		},
		{
			name:           "stale reading",
			current:        staleReading(&Reading{Time: ts}),
			staleThreshold: 5 * time.Second,
			wantErr:        true,
		},
		{
			name:           "fresh reading",
			current:        freshReading(&Reading{Time: ts, SolarKWh: 94400.05, LoadKWh: 65023.6, NetKWh: -29376.45}),
			staleThreshold: 5 * time.Second,
			want:           EnergyJSON{Time: ts.Format(time.RFC3339), SolarKWh: 94400.05, LoadKWh: 65023.6, NetKWh: -29376.45},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Monitor{current: tt.current, staleThreshold: tt.staleThreshold}
			result, _, err := energySummary(m)
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
