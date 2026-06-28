# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make fmt      # goimports -local github.com/dangogh -w .
make build    # produces bin/pvs-monitor, bin/pvs-mcp, bin/pvs-api, bin/pvs-ui
make test     # go test -race -coverprofile=coverage.out ./... + total coverage
make lint     # golangci-lint run
make cover    # open coverage HTML in browser (runs test first)
```

Run a single test:
```sh
go test -race -run TestName ./pvs/
```

## Architecture

Four binaries sharing a SQLite database:

```
pvs-monitor (daemon)           pvs-mcp (MCP server)
─────────────────────          ────────────────────
PVS6 WebSocket                 SQLite reads only
→ pvs.Monitor                  → pvs.Store (read methods)
→ pvs.DevicePoller             → MCP tools (stdio)
→ SQLite writes

pvs-api (HTTP server)          pvs-ui (web UI)
─────────────────────          ───────────────
SQLite reads only              embeds static/index.html
→ GET /api/current             reverse-proxies /api/ → pvs-api
→ GET /api/data
→ GET /api/devices
```

`pvs-monitor` runs as a long-lived daemon. `pvs-mcp` is spawned on demand by Claude Desktop and exits when the MCP stdio session ends. `pvs-api` is an HTTP REST server; `pvs-ui` serves the embedded SPA and proxies API requests to `pvs-api`. All four share the same SQLite database file; WAL mode allows concurrent access.

### Packages

- **`pvs`** — core domain. `Monitor` maintains a persistent WebSocket connection to the PVS6, parses `power` notification frames, and persists each reading via `Store`. `DevicePoller` polls a separate HTTP endpoint for per-device data. `Store` is the persistence interface. MCP tool handlers live in `tools.go` and read exclusively from `Store`.

- **`config`** — YAML config with XDG path defaulting. Supports custom `Duration` type for YAML unmarshaling. Precedence: `--addr` flag > `PVS_ADDR` env > config file > built-in default.

- **`store/sqlite`** — `Store` implementation. Two tables: `readings` (time-series power data) and `device_readings` (per-device snapshots as raw JSON payloads). Schema is applied inline at open time.

- **`cmd/pvs-monitor`** — daemon entrypoint. Wires config → store → monitor → optional poller. Blocks until SIGINT/SIGTERM.

- **`cmd/pvs-mcp`** — MCP server entrypoint. Opens SQLite read-only, registers tools, runs stdio transport. The `StdioTransport` owns the process lifetime.

- **`cmd/pvs-api`** — HTTP REST server. Reads from SQLite and exposes `/api/current`, `/api/data`, and `/api/devices` with CORS headers.

- **`cmd/pvs-ui`** — Serves an embedded `static/index.html` and reverse-proxies `/api/` to `pvs-api`.

### Key design points

- `Monitor` and `DevicePoller` are injectable via interfaces (`dialer`, `httpDoer`) for testing without real network connections.
- All MCP tools read from `Store`. `get_current_power` and `get_energy_summary` call `store.LatestReading()` and check staleness against `cfg.StaleThreshold`.
- `get_device_list` reads from `store.LatestDevices()` — all four tools are always registered; they return an error if no data exists yet.
- Reconnect uses exponential backoff between `ReconnectInitialInterval` and `ReconnectMaxInterval`.
- `DevicePoller` uses a two-step auth flow: GET `/auth?login` with Basic auth to get a session cookie, then use it for subsequent requests. Uses the same scheme as `cfg.URL` (plain HTTP on most PVS6 units). The HTTP client forces HTTP/1.1 via `TLSClientConfig.NextProtos` in case TLS is in use, to avoid a hang from Go's HTTP/2 + `InsecureSkipVerify`.
- On startup, `DevicePoller` enables WebSocket telemetry via `POST /vars?set=/sys/telemetryws/enable=1`. PVS6 firmware 2025.10+ disables this by default and resets it on reboot.

### Running as a service

```sh
cp launchd/com.dangogh.pvs-monitor.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.dangogh.pvs-monitor.plist
```

Logs: `~/.local/share/pvs-monitor/pvs-monitor.log`
