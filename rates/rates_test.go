package rates

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func la(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	return loc
}

func TestTOUD49PMIsValid(t *testing.T) {
	require.NoError(t, TOUD49PM().Validate())
}

func TestValidateRejectsGaps(t *testing.T) {
	p := TOUD49PM()
	p.Seasons = p.Seasons[:1] // drops October-May
	assert.Error(t, p.Validate())
}

func TestSegmentsSummerWeekday(t *testing.T) {
	loc := la(t)
	// Wednesday 2026-07-01, the whole local day.
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	segs := TOUD49PM().Segments(day, day.AddDate(0, 0, 1), loc)
	require.Len(t, segs, 3)
	assert.Equal(t, []string{"off_peak", "on_peak", "off_peak"}, []string{segs[0].Period, segs[1].Period, segs[2].Period})
	assert.Equal(t, 0.59, segs[1].PerKWh)
	assert.Equal(t, time.Date(2026, 7, 1, 16, 0, 0, 0, loc), segs[1].Start)
	assert.Equal(t, time.Date(2026, 7, 1, 21, 0, 0, 0, loc), segs[1].End)
}

func TestSegmentsSummerWeekendIsMidPeak(t *testing.T) {
	loc := la(t)
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, loc) // Saturday
	segs := TOUD49PM().Segments(day, day.AddDate(0, 0, 1), loc)
	require.Len(t, segs, 3)
	assert.Equal(t, "mid_peak", segs[1].Period)
	assert.Equal(t, 0.44, segs[1].PerKWh)
}

func TestSegmentsWinterHasSuperOffPeak(t *testing.T) {
	loc := la(t)
	day := time.Date(2026, 12, 2, 0, 0, 0, 0, loc)
	segs := TOUD49PM().Segments(day, day.AddDate(0, 0, 1), loc)
	periods := make([]string, len(segs))
	for i, s := range segs {
		periods[i] = s.Period
	}
	assert.Equal(t, []string{"off_peak", "super_off_peak", "mid_peak", "off_peak"}, periods)
	assert.Equal(t, 0.32, segs[1].PerKWh)
}

func TestSegmentsSeasonBoundary(t *testing.T) {
	loc := la(t)
	// May 31 (winter) into June 1 (summer): the season must switch at midnight.
	start := time.Date(2026, 5, 31, 22, 0, 0, 0, loc)
	segs := TOUD49PM().Segments(start, start.Add(4*time.Hour), loc)
	require.Len(t, segs, 2)
	assert.Equal(t, "winter", segs[0].Season)
	assert.Equal(t, 0.36, segs[0].PerKWh)
	assert.Equal(t, "summer", segs[1].Season)
	assert.Equal(t, 0.33, segs[1].PerKWh)
}

func TestSegmentsAreContiguousAndClipped(t *testing.T) {
	loc := la(t)
	start := time.Date(2026, 7, 1, 17, 30, 0, 0, loc)
	end := time.Date(2026, 7, 3, 9, 15, 0, 0, loc)
	segs := TOUD49PM().Segments(start, end, loc)
	require.NotEmpty(t, segs)
	assert.Equal(t, start, segs[0].Start)
	assert.Equal(t, end, segs[len(segs)-1].End)
	for i := 1; i < len(segs); i++ {
		assert.Equal(t, segs[i-1].End, segs[i].Start)
	}
}

func TestSegmentsAcrossDST(t *testing.T) {
	loc := la(t)
	// 2026-11-01 is the fall-back day (25 hours); on-peak hours must still be 4-9pm local.
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	segs := TOUD49PM().Segments(day, day.AddDate(0, 0, 1), loc)
	var covered time.Duration
	for _, s := range segs {
		covered += s.End.Sub(s.Start)
	}
	assert.Equal(t, 25*time.Hour, covered)
	assert.Equal(t, time.Date(2026, 11, 1, 16, 0, 0, 0, loc), segs[2].Start)
}
