package pvs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// noArgs is the input type for tools that take no arguments.
type noArgs struct{}

type avgArgs struct {
	Period string `json:"period"`
}

// RegisterTools adds the PVS6 MCP tools to the server.
func RegisterTools(s *mcp.Server, m *Monitor) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_current_power",
		Description: "Returns the latest instantaneous power readings from the PVS6 solar monitor (kW).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return currentPower(m)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_energy_summary",
		Description: "Returns cumulative energy totals from the PVS6 solar monitor (kWh).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return energySummary(m)
	})

	if m.store != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "get_average_power",
			Description: "Returns average power over a time window (e.g. '7d', '24h', '1h'). Requires historical data to have been collected.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args avgArgs) (*mcp.CallToolResult, any, error) {
			return averagePower(ctx, m, args.Period)
		})
	}
}

func currentReading(m *Monitor) (*Reading, error) {
	r := m.Current()
	if r == nil {
		return nil, fmt.Errorf("no reading available yet")
	}
	if r.IsStale(m.StaleThreshold()) {
		return nil, fmt.Errorf("reading is stale (last received %s ago)", time.Since(r.ReceivedAt).Round(time.Second))
	}
	return r, nil
}

func currentPower(m *Monitor) (*mcp.CallToolResult, any, error) {
	r, err := currentReading(m)
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

func averagePower(ctx context.Context, m *Monitor, period string) (*mcp.CallToolResult, any, error) {
	d, err := parsePeriod(period)
	if err != nil {
		return nil, nil, err
	}
	avg, err := m.store.AveragePower(ctx, time.Now().Add(-d))
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

func energySummary(m *Monitor) (*mcp.CallToolResult, any, error) {
	r, err := currentReading(m)
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
