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
	Period string `json:"period"`
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
		Description: "Returns cumulative energy totals from the PVS6 solar monitor (kWh).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return energySummary(ctx, store, stale)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_average_power",
		Description: "Returns average power over a time window (e.g. '7d', '24h', '1h'). Requires historical data to have been collected.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args avgArgs) (*mcp.CallToolResult, any, error) {
		return averagePower(ctx, store, args.Period)
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
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
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

func averagePower(ctx context.Context, store Store, period string) (*mcp.CallToolResult, any, error) {
	d, err := parsePeriod(period)
	if err != nil {
		return nil, nil, err
	}
	avg, err := store.AveragePower(ctx, time.Now().Add(-d))
	if err != nil {
		return nil, nil, err
	}
	if avg.Samples == 0 {
		return nil, nil, fmt.Errorf("no readings in the past %s", period)
	}
	data, err := json.Marshal(avgResult{
		Period:  period,
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

func deviceList(ctx context.Context, store Store) (*mcp.CallToolResult, any, error) {
	devices, err := store.LatestDevices(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(devices) == 0 {
		return nil, nil, fmt.Errorf("no device list available yet")
	}
	// Return the raw payloads as a JSON array so all device-specific fields are visible.
	raws := make([]json.RawMessage, len(devices))
	for i, d := range devices {
		raws[i] = d.Raw
	}
	data, err := json.Marshal(raws)
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
