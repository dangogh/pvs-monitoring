package pvs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAPI is a stand-in for a running pvs-api.
type fakeAPI struct {
	current    CurrentReading
	currentErr error
	data       DataResponse
	dataErr    error
	health     PanelHealth
	healthErr  error

	gotSince, gotUntil time.Time
}

func (f *fakeAPI) Current(context.Context) (CurrentReading, error) {
	return f.current, f.currentErr
}

func (f *fakeAPI) Data(_ context.Context, since, until time.Time) (DataResponse, error) {
	f.gotSince, f.gotUntil = since, until
	return f.data, f.dataErr
}

func (f *fakeAPI) PanelHealth(context.Context) (PanelHealth, error) {
	return f.health, f.healthErr
}

// decode unmarshals a tool result's text content into v.
func decode(t *testing.T, result *mcp.CallToolResult, v any) {
	t.Helper()
	require.NotNil(t, result)
	text := result.Content[0].(*mcp.TextContent).Text
	require.NoError(t, json.Unmarshal([]byte(text), v))
}

func TestGetStatusFresh(t *testing.T) {
	api := &fakeAPI{
		current: CurrentReading{SolarKW: 4.2, LoadKW: 1.8, NetKW: 2.4, UpdatedAt: time.Now().Add(-3 * time.Second)},
		health:  PanelHealth{State: PanelHealthOK, Total: 48, Producing: 48},
	}

	result, _, err := getStatus(context.Background(), api, time.Minute)
	require.NoError(t, err)

	var got statusResult
	decode(t, result, &got)
	assert.Equal(t, 4.2, got.SolarKW)
	assert.False(t, got.Stale)
	assert.LessOrEqual(t, got.AgeSeconds, int64(5))
	require.NotNil(t, got.PanelHealth)
	assert.Equal(t, PanelHealthOK, got.PanelHealth.State)
}

// A stale reading is data, not a failure: the caller needs the last known
// values and the age in order to tell "the monitor stopped" from "the host is
// down", which is what an error would mean.
func TestGetStatusStaleIsNotAnError(t *testing.T) {
	api := &fakeAPI{
		current: CurrentReading{SolarKW: 3.1, UpdatedAt: time.Now().Add(-2 * time.Hour)},
	}

	result, _, err := getStatus(context.Background(), api, time.Minute)
	require.NoError(t, err)

	var got statusResult
	decode(t, result, &got)
	assert.True(t, got.Stale)
	assert.Equal(t, 3.1, got.SolarKW)
	assert.InDelta(t, 7200, got.AgeSeconds, 5)
}

func TestGetStatusUnreachableIsAnError(t *testing.T) {
	api := &fakeAPI{currentErr: ErrUnreachable}
	_, _, err := getStatus(context.Background(), api, time.Minute)
	assert.ErrorIs(t, err, ErrUnreachable)
}

// Panel health is supplementary; losing it must not cost the caller the power
// reading it also asked for.
func TestGetStatusSurvivesPanelHealthFailure(t *testing.T) {
	api := &fakeAPI{
		current:   CurrentReading{SolarKW: 4.2, UpdatedAt: time.Now()},
		healthErr: ErrNoData,
	}

	result, _, err := getStatus(context.Background(), api, time.Minute)
	require.NoError(t, err)

	var got statusResult
	decode(t, result, &got)
	assert.Equal(t, 4.2, got.SolarKW)
	assert.Nil(t, got.PanelHealth)
}

func TestGetHistoryPeriod(t *testing.T) {
	api := &fakeAPI{data: DataResponse{
		Summary: SummaryData{SolarKWh: 31.5, LoadKWh: 20.0, NetKWh: -11.5, AvgSolarKW: 1.3},
		Series:  []SeriesJSON{{TimeMS: 1756500000000}},
	}}

	result, _, err := getHistory(context.Background(), api, historyArgs{Period: "7d"})
	require.NoError(t, err)

	var got historyResult
	decode(t, result, &got)
	assert.Equal(t, 31.5, got.SolarKWh)
	assert.Equal(t, 1.3, got.AvgSolarKW)
	assert.Empty(t, got.Warning)
	// The series is large, so it is omitted unless asked for.
	assert.Empty(t, got.Series)
	assert.InDelta(t, 7*24*time.Hour, api.gotUntil.Sub(api.gotSince), float64(time.Second))
}

func TestGetHistorySeriesOnRequest(t *testing.T) {
	api := &fakeAPI{data: DataResponse{Series: []SeriesJSON{{TimeMS: 1756500000000}}}}

	result, _, err := getHistory(context.Background(), api, historyArgs{Period: "1d", Series: true})
	require.NoError(t, err)

	var got historyResult
	decode(t, result, &got)
	assert.Len(t, got.Series, 1)
}

func TestGetHistoryEchoesResolvedRange(t *testing.T) {
	api := &fakeAPI{}
	result, _, err := getHistory(context.Background(), api, historyArgs{Start: "2026-08-23", End: "2026-08-23"})
	require.NoError(t, err)

	var got historyResult
	decode(t, result, &got)
	// A date-only end covers the whole day, and the absolute range comes back
	// so a timezone-shifted boundary is visible rather than silent.
	assert.Equal(t, api.gotSince.Format(time.RFC3339), got.Start)
	assert.Equal(t, api.gotUntil.Format(time.RFC3339), got.End)
	assert.InDelta(t, 24*time.Hour, api.gotUntil.Sub(api.gotSince), float64(time.Second))
}

// Cumulative counters are assumed to only climb; firmware has broken that
// before, and a negative total must not read as a real figure.
func TestGetHistoryWarnsOnNegativeEnergy(t *testing.T) {
	api := &fakeAPI{data: DataResponse{Summary: SummaryData{SolarKWh: -4.2}}}

	result, _, err := getHistory(context.Background(), api, historyArgs{Period: "1d"})
	require.NoError(t, err)

	var got historyResult
	decode(t, result, &got)
	assert.Contains(t, got.Warning, "backwards")
}

func TestGetHistoryWarnsWhenRangePredatesData(t *testing.T) {
	earliest := time.Now().Add(-48 * time.Hour)
	api := &fakeAPI{data: DataResponse{EarliestAt: &earliest}}

	result, _, err := getHistory(context.Background(), api, historyArgs{Period: "30d"})
	require.NoError(t, err)

	var got historyResult
	decode(t, result, &got)
	assert.Contains(t, got.Warning, "earliest recorded reading")
	assert.Equal(t, earliest.Format(time.RFC3339), got.EarliestAt)
}

func TestGetHistoryBadArgs(t *testing.T) {
	tests := []struct {
		name string
		args historyArgs
	}{
		{"no range at all", historyArgs{}},
		{"end without start", historyArgs{End: "2026-08-23"}},
		{"unparseable period", historyArgs{Period: "fortnight"}},
		{"unparseable start", historyArgs{Start: "last tuesday"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := getHistory(context.Background(), &fakeAPI{}, tt.args)
			assert.Error(t, err)
		})
	}
}

func TestPanelHealthTool(t *testing.T) {
	api := &fakeAPI{health: PanelHealth{
		State:        PanelHealthDegraded,
		NotProducing: []string{"E00121", "E00122"},
		Total:        48,
		Producing:    46,
	}}

	result, _, err := panelHealth(context.Background(), api)
	require.NoError(t, err)

	var got PanelHealth
	decode(t, result, &got)
	assert.Equal(t, PanelHealthDegraded, got.State)
	assert.Equal(t, []string{"E00121", "E00122"}, got.NotProducing)
}

func TestPanelHealthToolUnreachable(t *testing.T) {
	_, _, err := panelHealth(context.Background(), &fakeAPI{healthErr: ErrUnreachable})
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "7d", want: 7 * 24 * time.Hour},
		{in: "24h", want: 24 * time.Hour},
		{in: "1h30m", want: 90 * time.Minute},
		{in: "xd", wantErr: true},
		{in: "nonsense", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parsePeriod(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
