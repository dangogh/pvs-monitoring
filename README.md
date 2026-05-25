# pvs-monitoring

An MCP server that exposes real-time solar power data from a SunPower PVS6 monitor via WebSocket.

## Tools

| Tool | Description |
|------|-------------|
| `get_current_power` | Instantaneous solar production, home load, and grid draw (kW) |
| `get_energy_summary` | Cumulative energy totals for solar, load, and grid (kWh) |

Both tools return an error if no reading has arrived yet or if the most recent reading is stale (default: older than 5 seconds).

## Prerequisites

- Go 1.22+
- A SunPower PVS6 monitor accessible on the local network

## Build

```sh
make build
# produces bin/pvs-monitor
```

## Configuration

Configuration is read from `~/.config/pvs-monitor/config.yaml` (or `$XDG_CONFIG_HOME/pvs-monitor/config.yaml`).

```yaml
addr: ws://192.168.191.155:9002        # PVS6 WebSocket address
reconnect_initial_interval: 1s         # initial backoff on disconnect
reconnect_max_interval: 30s            # maximum backoff
stale_threshold: 5s                    # error if reading is older than this
```

All fields are optional; the values above are the defaults.

### Precedence

`--addr` flag > `PVS_ADDR` env var > config file > built-in default

### Flags

```
--config   path to config file (default: ~/.config/pvs-monitor/config.yaml)
--addr     PVS6 WebSocket address
-v, --verbose  enable debug logging
```

## Claude Desktop integration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "pvs-monitor": {
      "command": "/path/to/bin/pvs-monitor"
    }
  }
}
```

Replace `/path/to/bin/pvs-monitor` with the absolute path to the built binary.
