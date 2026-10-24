package pvs

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/dangogh/pvs-monitoring/rates"
)

// EnergySource is the part of Store that cost calculation needs.
type EnergySource interface {
	EnergyDelta(ctx context.Context, since, until time.Time) (EnergyDelta, error)
}

// ComputeCost prices the net grid energy in [since, until) under plan, with the
// plan's hours read in loc. Energy is measured per constant-price segment from
// the cumulative counters, so the result is only as sound as those counters.
func ComputeCost(ctx context.Context, src EnergySource, plan rates.Plan, loc *time.Location, since, until time.Time) (CostResponse, error) {
	if !until.After(since) {
		return CostResponse{}, fmt.Errorf("until (%s) must be after since (%s)", until, since)
	}
	type key struct{ season, period string }
	lines := map[key]*CostLine{}
	for _, seg := range plan.Segments(since, until, loc) {
		e, err := src.EnergyDelta(ctx, seg.Start, seg.End)
		if err != nil {
			return CostResponse{}, err
		}
		k := key{seg.Season, seg.Period}
		l, ok := lines[k]
		if !ok {
			l = &CostLine{Season: seg.Season, Period: seg.Period, PerKWh: seg.PerKWh}
			lines[k] = l
		}
		l.NetKWh += e.NetKWh
		l.Cost += e.NetKWh * seg.PerKWh
	}

	out := CostResponse{Since: since, Until: until, Plan: plan.Name, Timezone: loc.String()}
	for _, l := range lines {
		out.Lines = append(out.Lines, *l)
		out.EnergyCost += l.Cost
	}
	sort.Slice(out.Lines, func(i, j int) bool {
		a, b := out.Lines[i], out.Lines[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		return a.Period < b.Period
	})
	out.BaseCharge = plan.BaseChargePerDay * until.Sub(since).Hours() / 24
	out.TotalCost = out.EnergyCost + out.BaseCharge
	return out, nil
}
