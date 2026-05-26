# pvs-monitoring

An MCP server that exposes real-time solar power data from a SunPower PVS6 monitor via WebSocket.

## Tools

| Tool | Description |
|------|-------------|
| `get_current_power` | Instantaneous solar production, home load, and grid draw (kW) |
| `get_energy_summary` | Cumulative energy totals for solar, load, and grid (kWh) |
| `get_average_power` | Average power over a time window (e.g. `7d`, `24h`) — requires historical data |
| `get_device_list` | Per-device readings from all inverters, meters, and battery — requires device list config |

`get_current_power` and `get_energy_summary` return an error if no reading has arrived yet or if the most recent reading is stale (default: older than 5 seconds).

## Prerequisites

- Go 1.22+
- A SunPower PVS6 monitor accessible on the local network

## Build

```sh
make build
# produces bin/pvs-monitor
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

### Precedence

`--addr` flag > `PVS_ADDR` env var > config file > built-in default

### Flags

```
--config       path to config file (default: ~/.config/pvs-monitor/config.yaml)
--addr         PVS6 WebSocket address
--db           path to SQLite database (default: ~/.local/share/pvs-monitor/readings.db, empty to disable)
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
