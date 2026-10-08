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

- **`cmd/pvs-backfill`** — one-shot repair tool. Rebuilds gaps in the 1 Hz `readings` stream from the 1/min Power Meter payloads in `aux_device_readings`, for the case where the PVS6 stayed reachable while the WebSocket power stream was dead (see the 2026-09-22 stall). Auto-detects which gaps are recoverable: meter samples inside the gap mean a telemetry stall and a rebuild is possible, none means the PVS6 was off and the data is gone. Dry run by default; `-apply` writes. Reconstructed rows are tagged `source='meter-1min'`, which both keeps them distinguishable from measured samples and makes the repair reversible. Each gap is filled in one transaction, after a `VACUUM INTO` backup (`-no-backup` to skip).

- **`cmd/pvs-api`** — HTTP REST server. Reads from SQLite and exposes `/api/current`, `/api/data`, `/api/devices`, and `/api/panel-health` with CORS headers.

- **`cmd/pvs-ui`** — Serves an embedded `static/index.html` and reverse-proxies `/api/` to `pvs-api`.

### Key design points

- `Monitor` and `DevicePoller` are injectable via interfaces (`dialer`, `httpDoer`) for testing without real network connections.
- `pvs-mcp` takes `-api` (base URL), `-name` (server name announced to the MCP client, so one instance per array is distinguishable), and `-insecure`/`-ca` for TLS. Both monitoring hosts now serve SAN certificates, so `-ca` works with verification fully on; `-insecure` is only needed for a host whose cert predates the SAN fix (CN-only certs are rejected by Go regardless of trust store).
- Three MCP tools, all reading through `API`: `get_status` (current power + staleness + panel health), `get_history` (energy/average power over a range), `get_panel_health`.
- MCP failures are split deliberately. A tool errors only when `pvs-api` could not be reached (`ErrUnreachable`); a reading that is merely old returns normally with `stale` and `age_seconds`. The distinction is diagnostic: an error means the host or network is down, a stale result means the host is fine and the PVS6 link is not. `get_status` also degrades rather than fails if panel health alone is unavailable.
- `get_history` returns a `warnings` list for results that should not be taken at face value: a range starting before the earliest recorded reading, and negative energy totals (cumulative counters are assumed monotonic, and firmware has broken that assumption before). It is a list because the conditions are independent and can both hold at once.
- `pvs-ui` compares each panel against the median of its **peer group**, not the fleet (`static/js/peers.js`). A peer group is the set of panels that should produce alike at the same instant — decided by shading, not by breaker, since panels on one circuit can be shaded at opposite ends of the day. Groups come from a `map.csv` column headed `group` (resolved by header name: deployed files already carry other trailing columns). No group column means every panel pools into `ungrouped`, which still catches hard failures. Ratios are withheld, rather than guessed, below the low-light floor and for groups smaller than `MIN_PEERS`.
- `pvs.EvaluatePanelHealth` (behind `/api/panel-health`) detects inverters producing far less than their peers. Detection is deliberately generic — no serial is special-cased, because an alarm fitted to the panels that already failed cannot catch the next failure elsewhere. Panels are compared against the 90th percentile of the array (not the median, which goes blind once more than half the array is out), and no verdict is given at all unless the array is producing enough to tell a fault from nightfall. The result is stateless, so callers that raise alarms should require the same serials on consecutive polls.
- Reconnect uses exponential backoff between `ReconnectInitialInterval` and `ReconnectMaxInterval`.
- `DevicePoller` uses a two-step auth flow: GET `/auth?login` with Basic auth to get a session cookie, then use it for subsequent requests. Uses the same scheme as `cfg.URL` (plain HTTP on most PVS6 units). The HTTP client forces HTTP/1.1 via `TLSClientConfig.NextProtos` in case TLS is in use, to avoid a hang from Go's HTTP/2 + `InsecureSkipVerify`.
- `DevicePoller` enables WebSocket telemetry via `/vars?set=/sys/telemetryws/enable=1` on **every poll tick**, not just at startup. PVS6 firmware 2025.10+ disables this by default and resets it on reboot, and a one-shot enable left the `readings` stream dead for 58 hours on 2026-09-22 (the PVS6 briefly left the network and came back with telemetry off). The call is an idempotent GET; re-sending it cannot miss the event the way a reconnect-triggered enable would, since the PVS6 can drop the stream without our WebSocket noticing. Credentials are fatal only on the first attempt — bad config should fail loudly at startup, but a transient 401 hours later must not stop the daemon.
- `Monitor.runLoop` bounds every WebSocket read with `config.ReadTimeout` (default 60s). Power frames arrive about once a second, so silence that long is a fault; without a deadline the read blocks until TCP gives up, which is what made the 2026-09-22 stall last 58 hours instead of a minute. `ReadTimeout` is deliberately separate from `StaleThreshold` despite sharing the default — one asks whether the socket is dead, the other whether a reading is too old to trust.
- `Config.Timezone` (IANA zone name, auto-detected from the host at `Default()` time via `TZ` or `/etc/localtime`) travels through the DB-backed settings table to `/api/config`, where `pvs-ui` reads it to bucket and label charts (`resolveRange`/`computeShift`/the Highcharts axis) by the *site's* calendar day rather than the viewing browser's (#96) — a Highcharts `time.timezone`/`Intl.DateTimeFormat`-based fix (`static/js/tz.js`), not a storage change: `readings.received_at`/`reading_time` were already plain Unix epoch seconds. `config.SeedMissingSettings` backfills the key into existing installs' settings tables on upgrade, since `SeedSettingsIfEmpty` only seeds a completely empty table.

### Running as a service

```sh
cp launchd/com.dangogh.pvs-monitor.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.dangogh.pvs-monitor.plist
```

Logs: `~/.local/share/pvs-monitor/pvs-monitor.log`

### Dead-man switch

`pvs-heartbeat` (shipped to `/usr/bin`, driven by `pvs-heartbeat.timer` every 5 min as `pvs-monitoring`) pings healthchecks.io only while **both** data streams are fresh — `readings` within `READINGS_MAX_AGE` (900s) and `aux_device_readings` within `AUX_MAX_AGE` (1800s). Two streams because they fail independently: on 2026-09-22 the WebSocket power stream died for 58 hours while the device poller never missed a beat, and the reverse would blind panel health.

Per-host config is `/etc/pvs-monitor/heartbeat.conf`, seeded by `postinst` from `/usr/share/pvs-monitoring/heartbeat.conf.default` (**never shipped under `/etc`** — that registers a conffile, and postinst touching one makes dpkg prompt on the next upgrade, which wedges the unattended updater). With no `HEARTBEAT_UUID` the script exits 0 silently, so the package is safe to install on a host that doesn't want monitoring.

All output goes to the journal (`journalctl -u pvs-heartbeat`) and curl failures are reported rather than swallowed. The September outage was *detected* correctly within 15 minutes and still went unnoticed for 58 hours, because the old hand-installed cron entry discarded stdout and stderr — so there was no record of whether the alert had been sent.
