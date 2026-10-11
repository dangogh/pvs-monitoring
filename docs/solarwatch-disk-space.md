# solarwatch — disk exhaustion incident & prevention plan

## What happened

`solarwatch` is a friend's PVS6 monitoring host running the Debian package
(`pvs-monitor` + `pvs-api` + `pvs-ui` + `pvs-heartbeat`). Its root filesystem is
a **4 GB SD card**. The card ran low on free space; once SQLite could no longer
write, `pvs-monitor` **missed readings** — a data-loss window whose length is
bounded only by how long the disk stayed full.

This is a different failure from the 2026-09-22 telemetry stall. That one was a
dead WebSocket with a healthy disk; this one is a healthy link with no room to
store what it receives. Both end the same way — gaps in `readings` — so the
dead-man switch (`pvs-heartbeat`) would have *detected* the staleness, but
detection is not prevention, and nothing in the stack watches **free space**
before it becomes a gap.

> Unknowns to confirm on the host (see Phase 0). We do not yet know *which*
> files filled the card — the DB, the journal, `/var/log/pvs-update.log`, or the
> apt cache are all candidates. Do not act until measured.

## Why a 4 GB card fills

There is **no retention or pruning anywhere in this codebase today** (confirmed:
no `DELETE`-by-age path, no retention knob in `config`). Every stream grows
without bound.

### Growth is deterministic, not per-host

The stream rates are fixed by the data model and the PVS array, **not** by the
SD card — so bytes/day is the same across installs (up to panel count), and the
sizing math can be done **once**, here, rather than measured on each host:

- `readings` is **1 Hz on every install** — 86,400 rows/day, identical everywhere.
- `inverter_readings` / `aux_device_readings` scale with **device count**, a
  property of the *array* (knowable via `SELECT COUNT(DISTINCT serial) FROM
  inverter_readings`), not the card.
- Rollups are fixed (24 + 1 rows/day).

Measured from a real 72.8-day, **48-panel** production DB (table + its indexes):

| Stream | **MB/day** | Scales with | Notes |
|---|---|---|---|
| `readings` (1 Hz) | **~7.4** | nothing (fixed) | ~88 B/row incl. index |
| `inverter_readings` | **~7.5** | panel count | 3 indexes add ~2.9 MB/day alone |
| `aux_device_readings` | **~3.9** | device count | ~900 B JSON/row |
| `readings_hourly`/`_daily` | ~0.002 | fixed | negligible |
| **total** | **~18.9** (48-panel); ~13 (24-panel) | | |

The rollup tables already serve every query over a 48 h span, so the raw 1 Hz
rows are, past a short zoom window, the biggest cost for the least marginal
value — but see the correction below: they are **not** the only stream that must
be bounded.

### The hard conclusion depends on the card

At ~19 MB/day a 48-panel array is ~6.9 GB/yr. What that means splits by host:

- **4 GB card (solarwatch, today):** 90 days ≈ 1.7 GB, 180 days ≈ 3.4 GB —
  over budget once the OS (~1.5–2 GB) is counted. A small card *forces* a short
  window. This incident was inevitable, not unlucky. **The real fix is a bigger
  card** (planned — helios runs 256 GB); retention is only a stopgap until then.
- **256 GB card (helios):** ~6.9 GB/yr is nothing — it can keep *years* of
  full-resolution per-panel history. Pruning here would destroy data it has
  room for.

> **This is why retention must default OFF.** A fixed 90-day prune shipped to all
> installs would silently delete helios's history. Retention is **opt-in,
> per-host**, armed only on a card too small to keep everything. The universal,
> always-safe fixes are the guards in 2a/2b/2d — not pruning.

> **Correction to an earlier assumption.** `inverter_readings` is the
> *joint-largest* consumer (tied with `readings`), and `aux` is also large —
> together ~60% of growth. The idea of "keep aux/inverter longer because they're
> the backfill / panel-health source" does not survive the math: on a 4 GB card
> there is no room to keep them a year. Retention must cover **all three** raw
> streams with a single short window.
>
> **Free schema win (all installs).** `idx_inv_serial` (serial only) is redundant
> with `idx_inv_serial_received_at` (serial, received_at) — the composite serves
> serial-only lookups. Dropping it reclaims ~77 MB now and ~1 MB/day forever,
> with no data loss. Ship as a migration.

## Constraints

1. **No further data loss.** Every step must preserve the measured data and the
   rollups. Prefer back-up-then-act. The guiding rule from `pvs-backfill`:
   `VACUUM INTO` a backup before any destructive DB operation.
2. **Remote host, friend's hardware.** Changes must be scriptable, idempotent,
   and safe to run unattended via the package — not hand-surgery that rots.
3. **Keep what enables repair.** `aux_device_readings` is the *only* source
   `pvs-backfill` can rebuild a stalled 1 Hz stream from; `inverter_readings`
   backs panel-health history. On a card with room (helios), keep them
   indefinitely; prune them only where the card forces it.
4. **Headroom, not a cliff.** The target is to keep steady-state usage well below
   the card size so a transient (big upgrade, log burst) can't tip it over.

---

## Phase 0 — One-time sanity check (not a per-host prerequisite)

The retention window is sized from the deterministic model above **for the
array's panel count**, not tuned by trial on each host. This step only (a)
confirms *which* files filled `solarwatch`'s card this time, and (b) sanity-
checks that its growth matches the model (same 1 Hz `readings`; aux/inverter
scaled by its own panel count). It does **not** gate shipping the fix. Run on
`solarwatch`, capture output. Do **not** delete yet.

```sh
# Overall + what's eating the root fs
df -h /
sudo du -xhd1 /var /usr /home 2>/dev/null | sort -h | tail -20

# Journal footprint
journalctl --disk-usage

# DB file + WAL + per-table byte cost
ls -lh /var/lib/pvs-monitor/readings.db*
sudo -u pvs-monitoring sqlite3 /var/lib/pvs-monitor/readings.db \
  "SELECT name, SUM(pgsize) AS bytes FROM dbstat GROUP BY name ORDER BY bytes DESC;"
# (dbstat needs SQLITE_ENABLE_DBSTAT_VTAB; if unavailable, fall back to row counts)
sudo -u pvs-monitoring sqlite3 /var/lib/pvs-monitor/readings.db \
  "SELECT 'readings', COUNT(*), MIN(received_at), MAX(received_at) FROM readings
   UNION ALL SELECT 'aux', COUNT(*), MIN(received_at), MAX(received_at) FROM aux_device_readings
   UNION ALL SELECT 'inverter', COUNT(*), MIN(received_at), MAX(received_at) FROM inverter_readings;"

# Log + cache strays
ls -lh /var/log/pvs-update.log 2>/dev/null
du -sh /var/cache/apt 2>/dev/null
```

Decisions that depend on Phase 0:
- Is the **DB** or the **journal/logs** the primary offender? (Fixes differ.)
- Panel count → confirms which point on the growth curve this array sits at.
- Confirm rollups are fully populated before any raw prune:
  `SELECT MAX(bucket) FROM readings_daily;` vs `MAX(received_at) FROM readings`.

---

## Phase 1 — Immediate, reversible reclaim

Order matters: free the easy wins first so the card isn't full while we work.

1. **Cap and vacuum the journal** (reclaims instantly, loses only old logs):
   ```sh
   sudo journalctl --vacuum-size=200M
   ```
2. **Truncate the update log** if large (it's pure append, not data):
   ```sh
   sudo truncate -s 0 /var/log/pvs-update.log
   ```
3. **Clean the apt cache**:
   ```sh
   sudo apt-get clean
   ```
4. **Checkpoint the WAL** so a bloated `-wal` folds back into the DB. The store
   already exposes `PRAGMA wal_checkpoint(TRUNCATE)` (`sqlite.go:1022`); if no
   trigger is wired, do it manually while the service is stopped briefly, or
   rely on the DB-maintenance tool from Phase 2.

Only **after** there is breathing room, take a safe backup and reclaim DB space
via the new retention tool (Phase 2) — never `DELETE` on a near-full card, and
never while the card has no room for the `VACUUM INTO` copy.

---

## Phase 2 — Prevent recurrence (the actual fix)

Four independent guards, because the consumers fail independently.

### 2a. Cap the journal permanently
Ship a drop-in so the journal can never be the cause again:
`/etc/systemd/journald.conf.d/pvs.conf`
```ini
[Journal]
SystemMaxUse=200M
```
Add to the package (postinst drop-in + `systemctl restart systemd-journald`).

### 2b. Rotate `/var/log/pvs-update.log`
Ship a `logrotate` snippet (`/etc/logrotate.d/pvs-monitoring`): weekly, keep 4,
`compress`, `missingok`, `copytruncate`. Removes the slow creep.

### 2c. DB retention — opt-in stopgap, not a universal default
Add a **retention tool** modeled on `cmd/pvs-backfill` (not ad-hoc SQL):

- New binary `cmd/pvs-prune` (or a `pvs-monitor --prune` maintenance mode):
  - **Defaults OFF (unlimited retention).** Enabled per-host only on a card too
    small to keep everything. helios (256 GB) must never prune; solarwatch needs
    it only until its card is swapped.
  - When enabled, **one window for all three raw streams** — `readings`,
    `inverter_readings`, and `aux_device_readings` (inverter/aux grow with panel
    count, so a large array on a small card fills fastest). ~90 days ≈ 1.7 GB
    on a 48-panel array.
  - **Delete raw rows only where the covering rollup buckets exist**, so
    long-range history (which already reads from rollups past 48 h) is untouched.
  - **Delete in bounded batches by `received_at` range**, each in its own
    transaction, honouring `busy_timeout` — so it never holds a long write lock
    against the 1 Hz writer (that lock contention is a plausible cause of the
    "long stall" symptom).
  - **`VACUUM INTO` backup first** (reuse the `pvs-backfill` pattern), then a
    real `VACUUM` to return freed pages to the filesystem — a plain `DELETE`
    frees pages inside the file but not the card.
  - **Dry-run by default**, `-apply` to write; report rows and reclaimed bytes.
- Add a single `retention` window to `config`, **defaulting to unlimited/off**,
  synced into existing installs via `config.SeedMissingSettings` (as `Timezone`
  was) with the off value — so no upgrade ever starts pruning on its own. Set it
  per-host only where the card requires it. The `pvs-prune.timer` ships but is a
  **no-op while retention is off**, so it is safe on helios and every large-card
  host.
- Ship the redundant-index drop (`idx_inv_serial`, subsumed by
  `idx_inv_serial_received_at`) as a migration — ~1 MB/day saved on every
  install, independent of retention.
- Drive the prune from a **systemd timer** (`pvs-prune.timer`, daily, off-peak),
  shipped by the package, logging to the journal (which is now capped).

Target: on solarwatch (until the card swap) all raw streams capped at the
window; on helios and any large-card host retention stays off and full history
is kept. Rollups are kept forever (tiny) everywhere.

### 2d. Free-space dead-man switch
Extend the heartbeat (`pvs-heartbeat`) — or add a sibling check — to **fail the
healthcheck ping when free space on `/var/lib/pvs-monitor` drops below a
threshold** (e.g. < 500 MB). Rationale mirrors the existing design: the current
switch catches stale *streams* but is blind to a filling *disk* until the stream
stalls. Report the actual free-space number to the journal so a near-miss is
visible before it's a gap — the September post-mortem's lesson was that silent
output is how a detected problem goes unnoticed for 58 hours.

---

## Phase 3 — Verify

- `df -h /` shows sustained headroom across an upgrade cycle.
- `pvs-prune` timer runs and logs reclaimed bytes; `readings` oldest row tracks
  the retention window.
- Rollups still answer long-range `/api/data` queries identically (spot-check a
  > 48 h range before and after a prune — numbers must match).
- Heartbeat fails on an artificially low free-space threshold in a test, then
  recovers.
- Confirm **no gap** was introduced by the prune: `reading_gaps.sql` over the
  retained window is clean.

---

## Open decisions

1. **The headline fix for solarwatch is the card swap** (planned), not retention.
   Retention is the stopgap that keeps it alive until then — and the long-term
   safety net for any future small-card host.
2. Retention default: **off/unlimited**, opt-in per host. Confirm. (A fixed
   90-day default would delete helios's history.)
3. Prune as a **separate binary** vs a `pvs-monitor` maintenance subcommand.
4. Free-space threshold for the heartbeat, and whether it's a new check or folded
   into `pvs-heartbeat`. This is the one guard that matters on *every* host,
   large card or small.
5. After solarwatch's swap: drop or lengthen its retention window so it, too,
   keeps full history.
