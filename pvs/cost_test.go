package pvs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangogh/pvs-monitoring/rates"
)

// perHourSource reports 1 kWh of net import for every hour of the segment.
type perHourSource struct{}

func (perHourSource) EnergyDelta(_ context.Context, since, until time.Time) (EnergyDelta, error) {
	return EnergyDelta{NetKWh: until.Sub(since).Hours()}, nil
}

func TestComputeCostSummerWeekday(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)

	got, err := ComputeCost(context.Background(), perHourSource{}, rates.TOUD49PM(), loc, day, day.AddDate(0, 0, 1))
	require.NoError(t, err)

	require.Len(t, got.Lines, 2)
	// off_peak: 19 h at 33c; on_peak: 5 h at 59c.
	assert.Equal(t, "off_peak", got.Lines[0].Period)
	assert.InDelta(t, 19, got.Lines[0].NetKWh, 1e-9)
	assert.Equal(t, "on_peak", got.Lines[1].Period)
	assert.InDelta(t, 5, got.Lines[1].NetKWh, 1e-9)
	assert.InDelta(t, 19*0.33+5*0.59, got.EnergyCost, 1e-9)
	assert.InDelta(t, 0.79, got.BaseCharge, 1e-9)
	assert.InDelta(t, got.EnergyCost+0.79, got.TotalCost, 1e-9)
	assert.Equal(t, "America/Los_Angeles", got.Timezone)
}

func TestComputeCostRejectsEmptyRange(t *testing.T) {
	now := time.Now()
	_, err := ComputeCost(context.Background(), perHourSource{}, rates.TOUD49PM(), time.UTC, now, now)
	assert.Error(t, err)
}
