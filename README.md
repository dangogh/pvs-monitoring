# pvs-monitoring

Monitors a SunPower PVS6 solar gateway and exposes the data via MCP tools for use with Claude Desktop.

Two binaries share a SQLite database:
- **`pvs-monitor`** — long-running daemon; maintains the WebSocket connection and writes readings to SQLite
- **`pvs-mcp`** — MCP server; spawned on demand by Claude Desktop; reads from SQLite only

## Tools

| Tool | Description |
|------|-------------|
| `get_current_power` | Instantaneous solar production, home load, and grid draw (kW) |
| `get_energy_summary` | Cumulative energy totals for solar, load, and grid (kWh) |
| `get_average_power` | Average power over a time window (e.g. `7d`, `24h`) |
| `get_device_list` | Per-device readings from all inverters, meters, and battery |

`get_current_power` and `get_energy_summary` return an error if no reading exists yet or if the most recent reading is stale (default: older than 5 seconds).

## Prerequisites

- Go 1.22+
- A SunPower PVS6 monitor accessible on the local network

## Build

```sh
make build
# produces bin/pvs-monitor and bin/pvs-mcp
```

## Configuration

Copy the example config and edit it for your system:

```sh
mkdir -p ~/.config/pvs-monitor
cp config.example.yaml ~/.config/pvs-monitor/config.yaml
$EDITOR ~/.config/pvs-monitor/config.yaml
```

The config file is read from `~/.config/pvs-monitor/config.yaml` (or `$XDG_CONFIG_HOME/pvs-monitor/config.yaml`). All fields are optional — see `config.example.yaml` for available settings and their defaults.

### Device list polling (per-inverter data)

To enable per-device readings, set your PVS serial password in the config:

```yaml
device_list:
  password: "XXXXX"   # last 5 characters of your PVS serial number
```

The serial number is printed on a sticker on the PVS6 unit. Leave `password` empty to disable this feature.

### pvs-monitor flags

```
--config       path to config file (default: ~/.config/pvs-monitor/config.yaml)
--addr         PVS6 WebSocket address (overrides config and PVS_ADDR env var)
--db           path to SQLite database (default: ~/.local/share/pvs-monitor/readings.db, empty to disable)
-v, --verbose  enable debug logging
```

Precedence: `--addr` flag > `PVS_ADDR` env var > config file > built-in default

### pvs-mcp flags

```
--config       path to config file (default: ~/.config/pvs-monitor/config.yaml)
--db           path to SQLite database (default: ~/.local/share/pvs-monitor/readings.db)
-v, --verbose  enable debug logging
```

## Running

Start the daemon (runs until SIGINT/SIGTERM):

```sh
bin/pvs-monitor
```

## Claude Desktop integration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "pvs-monitor": {
      "command": "/path/to/bin/pvs-mcp"
    }
  }
}
```

Replace `/path/to/bin/pvs-mcp` with the absolute path to the built binary.
