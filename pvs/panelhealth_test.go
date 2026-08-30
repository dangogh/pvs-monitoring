package pvs

import (
	"fmt"
	"testing"
	"time"
)

// inv builds a slice of inverters, all reporting at now, with the given powers.
func inv(now time.Time, powers ...float64) []InverterDevice {
	out := make([]InverterDevice, len(powers))
	for i, p := range powers {
		out[i] = InverterDevice{
			Serial:     string(rune('A' + i)),
			PowerKW:    p,
			ReceivedAt: now,
		}
	}
	return out
}

// rep repeats v n times, for building arrays of uniform panels.
func rep(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestEvaluatePanelHealthAllProducing(t *testing.T) {
	now := time.Now()
	got := EvaluatePanelHealth(inv(now, rep(0.25, 48)...), now)

	if got.State != PanelHealthOK {
		t.Errorf("State = %q, want %q (reason %q)", got.State, PanelHealthOK, got.Reason)
	}
	if len(got.NotProducing) != 0 {
		t.Errorf("NotProducing = %v, want empty", got.NotProducing)
	}
	if got.Producing != 48 || got.Total != 48 {
		t.Errorf("Producing/Total = %d/%d, want 48/48", got.Producing, got.Total)
	}
	if got.ReferenceKW != 0.25 {
		t.Errorf("ReferenceKW = %v, want 0.25", got.ReferenceKW)
	}
}

// A branch dropping out is the case this exists for: a minority at zero while
// the rest carry on. Modelled on the 2026-08-23 failure (12 of 48 at 0.0 with
// the array still near 0.19 kW median).
func TestEvaluatePanelHealthBranchDown(t *testing.T) {
	now := time.Now()
	powers := append(rep(0.0, 12), rep(0.19, 36)...)
	got := EvaluatePanelHealth(inv(now, powers...), now)

	if got.State != PanelHealthDegraded {
		t.Fatalf("State = %q, want %q (reason %q)", got.State, PanelHealthDegraded, got.Reason)
	}
	if len(got.NotProducing) != 12 {
		t.Errorf("NotProducing = %d serials, want 12", len(got.NotProducing))
	}
	if got.Producing != 36 {
		t.Errorf("Producing = %d, want 36", got.Producing)
	}
}

// A single dead inverter must be caught too -- the detector is not tuned to
// branch-sized groups.
func TestEvaluatePanelHealthSinglePanelDown(t *testing.T) {
	now := time.Now()
	powers := append([]float64{0.0}, rep(0.25, 47)...)
	got := EvaluatePanelHealth(inv(now, powers...), now)

	if got.State != PanelHealthDegraded {
		t.Fatalf("State = %q, want %q", got.State, PanelHealthDegraded)
	}
	if len(got.NotProducing) != 1 || got.NotProducing[0] != "A" {
		t.Errorf("NotProducing = %v, want [A]", got.NotProducing)
	}
}

// Nightfall: everything ramps to zero together, so there is nothing to judge.
// Reporting 48 dead panels here would be a nightly false alarm.
func TestEvaluatePanelHealthNightGivesNoVerdict(t *testing.T) {
	now := time.Now()
	got := EvaluatePanelHealth(inv(now, rep(0.004, 48)...), now)

	if got.State != PanelHealthNoVerdict {
		t.Fatalf("State = %q, want %q", got.State, PanelHealthNoVerdict)
	}
	if len(got.NotProducing) != 0 {
		t.Errorf("NotProducing = %v, want empty at night", got.NotProducing)
	}
}

// Panels shaded at a low sun angle still produce a useful fraction of the
// median. They must not be reported as dead.
func TestEvaluatePanelHealthShadedPanelsNotReported(t *testing.T) {
	now := time.Now()
	// Six panels at ~20% of the median: heavy shade, but clearly alive.
	powers := append(rep(0.03, 6), rep(0.15, 42)...)
	got := EvaluatePanelHealth(inv(now, powers...), now)

	if got.State != PanelHealthOK {
		t.Errorf("State = %q, want %q -- shaded panels must not read as dead (%v)",
			got.State, PanelHealthOK, got.NotProducing)
	}
}

func TestEvaluatePanelHealthStaleReadings(t *testing.T) {
	now := time.Now()
	devices := inv(now.Add(-2*time.Hour), rep(0.25, 48)...)
	got := EvaluatePanelHealth(devices, now)

	if got.State != PanelHealthNoVerdict {
		t.Fatalf("State = %q, want %q", got.State, PanelHealthNoVerdict)
	}
	if got.Reason == "" {
		t.Error("Reason should explain the stale readings")
	}
}

func TestEvaluatePanelHealthNoDevices(t *testing.T) {
	got := EvaluatePanelHealth(nil, time.Now())

	if got.State != PanelHealthNoVerdict {
		t.Fatalf("State = %q, want %q", got.State, PanelHealthNoVerdict)
	}
	if got.NotProducing == nil {
		t.Error("NotProducing must be non-nil so it marshals as [] not null")
	}
}

// An outage larger than half the array is exactly the case a median-based
// reference cannot see: the median itself goes to zero and the fault reads as
// nightfall. The high-percentile reference keeps the healthy panels in view.
func TestEvaluatePanelHealthMajorityDownIsDetected(t *testing.T) {
	for _, dead := range []int{25, 30, 40} {
		t.Run(fmt.Sprintf("%d_of_48_dead", dead), func(t *testing.T) {
			now := time.Now()
			powers := append(rep(0.0, dead), rep(0.25, 48-dead)...)
			got := EvaluatePanelHealth(inv(now, powers...), now)

			if got.State != PanelHealthDegraded {
				t.Fatalf("State = %q, want %q (reason %q, reference %v)",
					got.State, PanelHealthDegraded, got.Reason, got.ReferenceKW)
			}
			if len(got.NotProducing) != dead {
				t.Errorf("NotProducing = %d, want %d", len(got.NotProducing), dead)
			}
		})
	}
}

// Past roughly 90% dead there are too few healthy panels left to set the
// reference, and the array is indistinguishable from nightfall. This is the
// remaining blind spot; a total outage is caught by a separate liveness check.
func TestEvaluatePanelHealthNearTotalOutageIsBlind(t *testing.T) {
	now := time.Now()
	powers := append(rep(0.0, 46), rep(0.25, 2)...)
	got := EvaluatePanelHealth(inv(now, powers...), now)

	if got.State != PanelHealthNoVerdict {
		t.Errorf("State = %q, want %q at 46/48 dead", got.State, PanelHealthNoVerdict)
	}
}

func TestPercentile(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		p    float64
		want float64
	}{
		{"empty", nil, 0.9, 0},
		{"single", []float64{3}, 0.9, 3},
		{"max clamps to last", []float64{1, 2, 3, 4}, 1.0, 4},
		{"zero takes lowest", []float64{4, 1, 3, 2}, 0, 1},
		{"unsorted input", []float64{4, 1, 3, 2}, 0.5, 3},
		{"ignores low outliers", append(rep(0, 9), 5), 0.9, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentile(tt.in, tt.p); got != tt.want {
				t.Errorf("percentile(%v, %v) = %v, want %v", tt.in, tt.p, got, tt.want)
			}
		})
	}
}

func TestPercentileDoesNotReorderCaller(t *testing.T) {
	in := []float64{3, 1, 2}
	percentile(in, 0.9)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Errorf("percentile reordered the caller's slice: %v", in)
	}
}
