package pvs

import (
	"fmt"
	"sort"
	"time"
)

// Panel-health detection. The question answered here is deliberately generic:
// "is any inverter producing far less than its peers right now?" No serial is
// special-cased. Watching only the panels that have already failed cannot catch
// the next failure somewhere new, so detection is by peer comparison and any
// grouping of serials is left to the caller as presentation.
//
// Two rules keep this quiet without needing a hardcoded daylight window:
//
//   - Panels are compared against a high percentile of the whole array, so the
//     test scales with the sun and with cloud cover.
//   - No verdict is given at all unless the array is meaningfully producing.
//     At dusk every panel ramps to zero together and the reference collapses
//     with them; a real fault leaves the healthy panels untouched while a
//     subset drops. On one measured array the reference at nightfall was
//     ~0.004 kW against ~0.19 kW during an actual branch failure.
//
// The reference is a high percentile rather than the median so that a large
// outage stays detectable. With the median, an outage affecting more than half
// the array drags the reference itself to zero and the fault reads as
// nightfall; at the 90th percentile the healthy panels still set the reference
// until roughly 90% of the array is out. Median is normally preferred for
// robustness against outliers, but a panel cannot fail by reading too high, so
// there is nothing here for the median to protect against.
//
// The result is a point-in-time observation with no memory. A caller that
// raises alarms should require the same serials to appear on consecutive polls,
// which filters the brief zeros caused by a cloud edge crossing the array.
const (
	// PanelDeadFraction is the share of the array reference below which a panel
	// counts as not producing. Panels shaded at a low sun angle still manage
	// well above this; a failed inverter reports 0. Passing cloud never zeroes
	// a panel -- measured on one array, the weakest panel under heavy cloud
	// still made 0.023 kW while a tripped branch read exactly 0.000.
	PanelDeadFraction = 0.05

	// PanelReferencePercentile picks the healthy-panel reference. High enough
	// that a large outage cannot drag it down, below 1.0 so a single odd
	// reading cannot set it.
	PanelReferencePercentile = 0.90

	// PanelMinReferenceKW is the reference below which no verdict is given,
	// because there is not enough production to tell a fault from nightfall.
	PanelMinReferenceKW = 0.10

	// PanelMaxAge is how old the newest reading may be and still be judged.
	PanelMaxAge = 30 * time.Minute
)

// Panel-health states.
const (
	PanelHealthOK        = "ok"
	PanelHealthDegraded  = "degraded"
	PanelHealthNoVerdict = "no_verdict"
)

// PanelHealth is a point-in-time assessment of every inverter in the array.
type PanelHealth struct {
	// State is one of PanelHealthOK, PanelHealthDegraded, PanelHealthNoVerdict.
	State string `json:"state"`
	// Reason explains a no_verdict result; empty otherwise.
	Reason string `json:"reason,omitempty"`
	// NotProducing lists the serials below the cutoff, sorted. Never nil, so
	// JSON consumers always see an array.
	NotProducing []string `json:"not_producing"`
	// Total and Producing count inverters reporting, and those above cutoff.
	Total     int `json:"total"`
	Producing int `json:"producing"`
	// ReferenceKW is the healthy-panel output every panel was compared against,
	// and ArrayKW the array total, at the moment of assessment.
	ReferenceKW float64 `json:"reference_kw"`
	ArrayKW     float64 `json:"array_kw"`
	// ObservedAt is the newest reading time across the inverters.
	ObservedAt time.Time `json:"observed_at"`
}

// EvaluatePanelHealth assesses whether any inverter has stopped producing while
// its peers continue. now is the reference for the staleness check.
func EvaluatePanelHealth(devices []InverterDevice, now time.Time) PanelHealth {
	h := PanelHealth{NotProducing: []string{}, Total: len(devices)}

	if len(devices) == 0 {
		h.State = PanelHealthNoVerdict
		h.Reason = "no inverter data"
		return h
	}

	powers := make([]float64, len(devices))
	for i, d := range devices {
		powers[i] = d.PowerKW
		h.ArrayKW += d.PowerKW
		if d.ReceivedAt.After(h.ObservedAt) {
			h.ObservedAt = d.ReceivedAt
		}
	}

	if age := now.Sub(h.ObservedAt); age > PanelMaxAge {
		h.State = PanelHealthNoVerdict
		h.Reason = fmt.Sprintf("readings are %s old", age.Round(time.Minute))
		return h
	}

	h.ReferenceKW = percentile(powers, PanelReferencePercentile)

	// Not enough production to distinguish a fault from nightfall.
	if h.ReferenceKW < PanelMinReferenceKW {
		h.State = PanelHealthNoVerdict
		h.Reason = "array not producing enough to judge (night or heavy overcast)"
		h.Producing = len(devices)
		return h
	}

	cutoff := PanelDeadFraction * h.ReferenceKW
	for _, d := range devices {
		if d.PowerKW < cutoff {
			h.NotProducing = append(h.NotProducing, d.Serial)
		}
	}
	sort.Strings(h.NotProducing)

	h.Producing = len(devices) - len(h.NotProducing)
	h.State = PanelHealthOK
	if len(h.NotProducing) > 0 {
		h.State = PanelHealthDegraded
	}
	return h
}

// percentile returns the value at fraction p (0..1) through vals, using
// nearest-rank. vals is copied, so the caller's slice keeps its order.
func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)

	i := int(p * float64(len(s)))
	if i >= len(s) {
		i = len(s) - 1
	}
	if i < 0 {
		i = 0
	}
	return s[i]
}
