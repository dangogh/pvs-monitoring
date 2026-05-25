package pvs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools adds the PVS6 MCP tools to the server.
func RegisterTools(s *mcp.Server, m *Monitor) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_current_power",
		Description: "Returns the latest instantaneous power readings from the PVS6 solar monitor (kW).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		return currentPower(m)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_energy_summary",
		Description: "Returns cumulative energy totals from the PVS6 solar monitor (kWh).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		return energySummary(m)
	})
}

func currentPower(m *Monitor) (*mcp.CallToolResult, any, error) {
	r := m.Current()
	if r == nil {
		return nil, nil, fmt.Errorf("no reading available yet")
	}
	data, err := json.Marshal(r.Power())
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func energySummary(m *Monitor) (*mcp.CallToolResult, any, error) {
	r := m.Current()
	if r == nil {
		return nil, nil, fmt.Errorf("no reading available yet")
	}
	data, err := json.Marshal(r.Energy())
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}
