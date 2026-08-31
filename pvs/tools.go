package pvs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dangogh/pvs-monitoring/config"
)

// noArgs is the input type for tools that take no arguments.
type noArgs struct{}

type historyArgs struct {
	Period string `json:"period,omitempty"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
	Series bool   `json:"series,omitempty"`
}

// RegisterTools adds the PVS6 MCP tools to the server. All tools read through
// api, which is normally a Client pointing at a running pvs-api.
//
// Failures are reported in two distinct ways on purpose. A tool errors only
// when pvs-api could not be reached at all; data that is merely old comes back
// as a normal result carrying stale and age_seconds. The difference is itself
// diagnostic: an error means the host or network is down, while a stale result
// means the host is fine and the PVS6 link is not.
func RegisterTools(s *mcp.Server, api API, cfg config.Config) {
	stale := cfg.StaleThreshold.Duration()

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_status",
		Description: "Returns the current state of the solar array: instantaneous power (kW), " +
			"how old that reading is, and a panel-health verdict covering every inverter. " +
			"Start here for any question about how the system is doing right now. " +
			"A result with stale=true means the monitor has stopped reporting; the values are " +
			"the last ones seen, not current.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return getStatus(ctx, api, stale)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_history",
		Description: "Returns energy produced and consumed (kWh) plus average power over a time range. " +
			"Use period (e.g. '7d', '24h') for a trailing window, or start/end (YYYY-MM-DD or RFC3339). " +
			"Set series=true to also get the time-bucketed curve, which is large — omit it unless " +
			"the shape of the day matters. The response echoes the resolved absolute range, and " +
			"earliest_at reports where the recorded data begins.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args historyArgs) (*mcp.CallToolResult, any, error) {
		return getHistory(ctx, api, args)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_panel_health",
		Description: "Checks whether any inverter has stopped producing while its peers continue, " +
			"by comparing every panel against a high percentile of the array. " +
			"The verdict is a single point in time with no memory: a passing cloud can zero a few " +
			"panels for one sample. Before reporting a fault, call again and require the same " +
			"serials to appear on consecutive checks. A state of no_verdict means there is too " +
			"little production to distinguish a fault from nightfall.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return panelHealth(ctx, api)
	})
}

// jsonResult marshals v as the tool's text content.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

type statusResult struct {
	SolarKW     float64      `json:"solar_kw"`
	LoadKW      float64      `json:"load_kw"`
	NetKW       float64      `json:"net_kw"`
	UpdatedAt   string       `json:"updated_at"`
	AgeSeconds  int64        `json:"age_seconds"`
	Stale       bool         `json:"stale"`
	PanelHealth *PanelHealth `json:"panel_health,omitempty"`
}

func getStatus(ctx context.Context, api API, staleThreshold time.Duration) (*mcp.CallToolResult, any, error) {
	cur, err := api.Current(ctx)
	if err != nil {
		return nil, nil, err
	}
	age := time.Since(cur.UpdatedAt)
	out := statusResult{
		SolarKW:    cur.SolarKW,
		LoadKW:     cur.LoadKW,
		NetKW:      cur.NetKW,
		UpdatedAt:  cur.UpdatedAt.Format(time.RFC3339),
		AgeSeconds: int64(age.Seconds()),
		Stale:      age > staleThreshold,
	}
	// Panel health is supplementary: a missing verdict should not cost the
	// caller the power reading it also asked for.
	if h, err := api.PanelHealth(ctx); err == nil {
		out.PanelHealth = &h
	}
	return jsonResult(out)
}

func panelHealth(ctx context.Context, api API) (*mcp.CallToolResult, any, error) {
	h, err := api.PanelHealth(ctx)
	if errors.Is(err, ErrUnsupported) {
		return nil, nil, fmt.Errorf("this pvs-api is too old to report panel health (added in v1.14.0); upgrade the monitoring host")
	}
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(h)
}

type historyResult struct {
	Start      string  `json:"start"`
	End        string  `json:"end"`
	SolarKWh   float64 `json:"solar_kwh"`
	LoadKWh    float64 `json:"load_kwh"`
	NetKWh     float64 `json:"net_kwh"`
	AvgSolarKW float64 `json:"avg_solar_kw"`
	AvgLoadKW  float64 `json:"avg_load_kw"`
	EarliestAt string  `json:"earliest_at,omitempty"`
	// Warnings flag a result that should not be read at face value. There can
	// be more than one — a range predating the data and a counter regression
	// are independent, and a reader who is told only about the second would
	// draw a wrong conclusion from the first.
	Warnings []string     `json:"warnings,omitempty"`
	Series   []SeriesJSON `json:"series,omitempty"`
}

func getHistory(ctx context.Context, api API, args historyArgs) (*mcp.CallToolResult, any, error) {
	since, until, err := resolveRange(args)
	if err != nil {
		return nil, nil, err
	}

	data, err := api.Data(ctx, since, until)
	if err != nil {
		return nil, nil, err
	}

	out := historyResult{
		Start:      since.Format(time.RFC3339),
		End:        until.Format(time.RFC3339),
		SolarKWh:   data.Summary.SolarKWh,
		LoadKWh:    data.Summary.LoadKWh,
		NetKWh:     data.Summary.NetKWh,
		AvgSolarKW: data.Summary.AvgSolarKW,
		AvgLoadKW:  data.Summary.AvgLoadKW,
	}
	if data.EarliestAt != nil {
		out.EarliestAt = data.EarliestAt.Format(time.RFC3339)
		if since.Before(*data.EarliestAt) {
			out.Warnings = append(out.Warnings,
				"range starts before the earliest recorded reading; totals cover only the recorded part")
		}
	}
	// Energy is derived from cumulative counters, which are assumed to only
	// ever climb. A firmware fault has made them run backwards before, which
	// yields a plausible-looking negative total; say so rather than letting it
	// be read as a real number.
	if data.Summary.SolarKWh < 0 || data.Summary.LoadKWh < 0 {
		out.Warnings = append(out.Warnings,
			"negative energy total: the cumulative counters ran backwards over this range, so these figures are not trustworthy")
	}
	if args.Series {
		out.Series = data.Series
	}
	return jsonResult(out)
}

// resolveRange turns the tool's human-friendly arguments into an absolute
// range. Date-only arguments are interpreted in the local timezone and the
// resolved range is echoed back in the result, so a day boundary that lands
// somewhere unexpected is visible rather than silent.
func resolveRange(args historyArgs) (since, until time.Time, err error) {
	if args.Start != "" || args.End != "" {
		if args.Start == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("start is required when end is specified")
		}
		return parseTimeRange(args.Start, args.End)
	}
	if args.Period == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("provide period (e.g. '24h') or start/end dates")
	}
	d, err := parsePeriod(args.Period)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	until = time.Now()
	return until.Add(-d), until, nil
}

func parsePeriod(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("invalid period %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid period %q: use e.g. 7d, 24h, 1h30m", s)
	}
	return d, nil
}

func parseTimeArg(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q: use YYYY-MM-DD or RFC3339", s)
	}
	return t, nil
}

// parseTimeRange parses start/end strings. A date-only end (YYYY-MM-DD) is extended to end of day.
func parseTimeRange(startStr, endStr string) (since, until time.Time, err error) {
	since, err = parseTimeArg(startStr)
	if err != nil {
		return
	}
	if endStr != "" {
		until, err = parseTimeArg(endStr)
		if err != nil {
			return
		}
		if _, err2 := time.ParseInLocation("2006-01-02", strings.TrimSpace(endStr), time.Local); err2 == nil {
			until = until.Add(24*time.Hour - time.Second)
		}
	} else {
		until = time.Now()
	}
	return
}
