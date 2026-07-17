package pvs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dangogh/pvs-monitoring/config"
)

// noArgs is the input type for tools that take no arguments.
type noArgs struct{}

type avgArgs struct {
	Period string `json:"period,omitempty"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

type energyArgs struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// RegisterTools adds the PVS6 MCP tools to the server.
// All tools read from store; get_current_power and get_energy_summary return an error
// if no reading exists or if the latest reading is stale.
func RegisterTools(s *mcp.Server, store Store, cfg config.Config) {
	stale := cfg.StaleThreshold.Duration()

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_current_power",
		Description: "Returns the latest instantaneous power readings from the PVS6 solar monitor (kW).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return currentPower(ctx, store, stale)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_energy_summary",
		Description: "Returns cumulative energy totals (kWh) from the PVS6. Without arguments returns the latest live totals. With start/end (YYYY-MM-DD or RFC3339) returns energy generated in that period.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args energyArgs) (*mcp.CallToolResult, any, error) {
		if args.Start != "" || args.End != "" {
			return energyDelta(ctx, store, args.Start, args.End)
		}
		return energySummary(ctx, store, stale)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_average_power",
		Description: "Returns average power. Use period (e.g. '7d', '24h', '1h') for a trailing window, or start/end (YYYY-MM-DD or RFC3339) for a specific range. end defaults to now.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args avgArgs) (*mcp.CallToolResult, any, error) {
		return averagePower(ctx, store, args)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_device_list",
		Description: "Returns the latest per-device readings from the PVS6, including individual inverters, power meters, and battery (if present).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return deviceList(ctx, store)
	})

}

func latestReading(ctx context.Context, store Store, staleThreshold time.Duration) (*Reading, error) {
	r, err := store.LatestReading(ctx)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("no reading available yet")
	}
	if r.IsStale(staleThreshold) {
		return nil, fmt.Errorf("reading is stale (last received %s ago)", time.Since(r.ReceivedAt).Round(time.Second))
	}
	return r, nil
}

func currentPower(ctx context.Context, store Store, staleThreshold time.Duration) (*mcp.CallToolResult, any, error) {
	r, err := latestReading(ctx, store, staleThreshold)
	if err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(r.Power())
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
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

type avgResult struct {
	Period  string  `json:"period"`
	SolarKW float64 `json:"solar_kw"`
	LoadKW  float64 `json:"load_kw"`
	NetKW   float64 `json:"net_kw"`
	Samples int     `json:"samples"`
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

func averagePower(ctx context.Context, store Store, args avgArgs) (*mcp.CallToolResult, any, error) {
	var since, until time.Time
	var label string

	if args.Start != "" || args.End != "" {
		if args.Start == "" {
			return nil, nil, fmt.Errorf("start is required when end is specified")
		}
		var err error
		since, until, err = parseTimeRange(args.Start, args.End)
		if err != nil {
			return nil, nil, err
		}
		if args.End != "" {
			label = args.Start + "/" + args.End
		} else {
			label = args.Start + "/now"
		}
	} else {
		if args.Period == "" {
			return nil, nil, fmt.Errorf("provide period (e.g. '24h') or start/end dates")
		}
		d, err := parsePeriod(args.Period)
		if err != nil {
			return nil, nil, err
		}
		until = time.Now()
		since = until.Add(-d)
		label = args.Period
	}

	avg, err := store.AveragePower(ctx, since, until)
	if err != nil {
		return nil, nil, err
	}
	if avg.Samples == 0 {
		return nil, nil, fmt.Errorf("no readings in range %s", label)
	}
	data, err := json.Marshal(avgResult{
		Period:  label,
		SolarKW: avg.SolarKW,
		LoadKW:  avg.LoadKW,
		NetKW:   avg.NetKW,
		Samples: avg.Samples,
	})
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

type energyDeltaResult struct {
	Start    string  `json:"start"`
	End      string  `json:"end"`
	SolarKWh float64 `json:"solar_kwh"`
	LoadKWh  float64 `json:"load_kwh"`
	NetKWh   float64 `json:"net_kwh"`
}

func energyDelta(ctx context.Context, store Store, startStr, endStr string) (*mcp.CallToolResult, any, error) {
	if startStr == "" {
		return nil, nil, fmt.Errorf("start is required")
	}
	since, until, err := parseTimeRange(startStr, endStr)
	if err != nil {
		return nil, nil, err
	}
	delta, err := store.EnergyDelta(ctx, since, until)
	if err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(energyDeltaResult{
		Start:    since.Format(time.RFC3339),
		End:      until.Format(time.RFC3339),
		SolarKWh: delta.SolarKWh,
		LoadKWh:  delta.LoadKWh,
		NetKWh:   delta.NetKWh,
	})
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

type deviceListResult struct {
	Inverters []InverterDevice `json:"inverters"`
	Aux       []AuxDevice      `json:"aux"`
}

func deviceList(ctx context.Context, store Store) (*mcp.CallToolResult, any, error) {
	inverters, err := store.LatestInverters(ctx)
	if err != nil {
		return nil, nil, err
	}
	aux, err := store.LatestAuxDevices(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(inverters)+len(aux) == 0 {
		return nil, nil, fmt.Errorf("no device list available yet")
	}
	data, err := json.Marshal(deviceListResult{
		Inverters: inverters,
		Aux:       aux,
	})
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func energySummary(ctx context.Context, store Store, staleThreshold time.Duration) (*mcp.CallToolResult, any, error) {
	r, err := latestReading(ctx, store, staleThreshold)
	if err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(r.Energy())
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}
