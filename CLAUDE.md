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
PVS6 WebSocket                 HTTP client of pvs-api
→ pvs.Monitor                  → pvs.Store (read methods)
→ pvs.DevicePoller             → MCP tools (stdio)
→ SQLite writes

pvs-api (HTTP server)          pvs-ui (web UI)
─────────────────────          ───────────────
SQLite reads only              embeds static/index.html
→ GET /api/current             reverse-proxies /api/ → pvs-api
→ GET /api/data
→ GET /api/devices
→ GET /api/panel-health
```

`pvs-monitor` runs as a long-lived daemon. `pvs-mcp` is spawned on demand by Claude Desktop and exits when the MCP stdio session ends. `pvs-api` is an HTTP REST server; `pvs-ui` serves the embedded SPA and proxies API requests to `pvs-api`. `pvs-monitor`, `pvs-api`, and `pvs-ui` share the same SQLite database file; WAL mode allows concurrent access. `pvs-mcp` holds no database handle — it reads over HTTP from `pvs-api`, because the MCP client that spawns it is not necessarily the machine holding the database.

### Packages

- **`pvs`** — core domain. `Monitor` maintains a persistent WebSocket connection to the PVS6, parses `power` notification frames, and persists each reading via `Store`. `DevicePoller` polls a separate HTTP endpoint for per-device data. `Store` is the persistence interface. `Client` (in `client.go`) is a read-only HTTP client for `pvs-api`, satisfying the `API` interface; MCP tool handlers in `tools.go` read exclusively through `API`. Wire types shared by server and client live in `api.go`.

- **`config`** — YAML config with XDG path defaulting. Supports custom `Duration` type for YAML unmarshaling. Precedence: `--addr` flag > `PVS_ADDR` env > config file > built-in default.

- **`store/sqlite`** — `Store` implementation. Two tables: `readings` (time-series power data) and `device_readings` (per-device snapshots as raw JSON payloads). Schema is applied inline at open time.

- **`cmd/pvs-monitor`** — daemon entrypoint. Wires config → store → monitor → optional poller. Blocks until SIGINT/SIGTERM.

- **`cmd/pvs-mcp`** — MCP server entrypoint. Builds a `pvs.Client` for `--api` (default `http://solar.local`), registers tools, runs stdio transport. The `StdioTransport` owns the process lifetime. The API is not probed at startup, so a client launching while the monitoring host is down still comes up.

- **`cmd/pvs-api`** — HTTP REST server. Reads from SQLite and exposes `/api/current`, `/api/data`, `/api/devices`, and `/api/panel-health` with CORS headers.

- **`cmd/pvs-ui`** — Serves an embedded `static/index.html` and reverse-proxies `/api/` to `pvs-api`.

### Key design points

- `Monitor` and `DevicePoller` are injectable via interfaces (`dialer`, `httpDoer`) for testing without real network connections.
- Three MCP tools, all reading through `API`: `get_status` (current power + staleness + panel health), `get_history` (energy/average power over a range), `get_panel_health`.
- MCP failures are split deliberately. A tool errors only when `pvs-api` could not be reached (`ErrUnreachable`); a reading that is merely old returns normally with `stale` and `age_seconds`. The distinction is diagnostic: an error means the host or network is down, a stale result means the host is fine and the PVS6 link is not. `get_status` also degrades rather than fails if panel health alone is unavailable.
- `get_history` flags results that should not be taken at face value: a range starting before the earliest recorded reading, and negative energy totals (cumulative counters are assumed monotonic, and firmware has broken that assumption before).
- `pvs.EvaluatePanelHealth` (behind `/api/panel-health`) detects inverters producing far less than their peers. Detection is deliberately generic — no serial is special-cased, because an alarm fitted to the panels that already failed cannot catch the next failure elsewhere. Panels are compared against the 90th percentile of the array (not the median, which goes blind once more than half the array is out), and no verdict is given at all unless the array is producing enough to tell a fault from nightfall. The result is stateless, so callers that raise alarms should require the same serials on consecutive polls.
- Reconnect uses exponential backoff between `ReconnectInitialInterval` and `ReconnectMaxInterval`.
- `DevicePoller` uses a two-step auth flow: GET `/auth?login` with Basic auth to get a session cookie, then use it for subsequent requests. Uses the same scheme as `cfg.URL` (plain HTTP on most PVS6 units). The HTTP client forces HTTP/1.1 via `TLSClientConfig.NextProtos` in case TLS is in use, to avoid a hang from Go's HTTP/2 + `InsecureSkipVerify`.
- On startup, `DevicePoller` enables WebSocket telemetry via `POST /vars?set=/sys/telemetryws/enable=1`. PVS6 firmware 2025.10+ disables this by default and resets it on reboot.

### Running as a service

```sh
cp launchd/com.dangogh.pvs-monitor.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.dangogh.pvs-monitor.plist
```

Logs: `~/.local/share/pvs-monitor/pvs-monitor.log`
