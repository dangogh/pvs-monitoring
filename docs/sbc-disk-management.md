# SBC Disk Management & Exhaustion Prevention

## The Problem

When deployed on a single-board computer with a small root filesystem (e.g., 4 GB), the SQLite database grows deterministically at ~13–19 MB/day (depending on panel count). Once the disk fills, SQLite cannot write new readings — resulting in data loss. The measuring link stays healthy; the monitoring link fails.

The dead-man switch detects stale streams after the fact. This guide prevents the condition entirely via deterministic sizing and optional retention.

## Growth is Predictable

Data streams are **fixed by the hardware and software**, not per-host variables. Bytes/day can be calculated once, then applied to any deployment:

- `readings` (1 Hz): **86,400 rows/day** (fixed across all installs)
- `inverter_readings`: scales with **panel count** (typically 4–48 panels)
- `aux_device_readings`: scales with **device count**
- Rollups: **24 + 1 rows/day** (negligible)

### Measured Growth (48-panel array, real production data)

| Stream | MB/day | Notes |
|---|---|---|
| `readings` (1 Hz) | ~7.4 | ~88 B/row incl. index |
| `inverter_readings` | ~7.5 | 3 indexes, scales with panels |
| `aux_device_readings` | ~3.9 | ~900 B JSON/row, scales with devices |
| Rollups | ~0.002 | Negligible |
| **Total** | ~19 (48-panel); ~13 (24-panel) | Scales linearly with panel count |

**Implications:**
- 4 GB device: ~210 days (minus OS footprint) at 19 MB/day — **fills in ~6 months**
- 256 GB device: ~37 years of full-res data — retention is unnecessary

## Prevention Strategy

### Tier 1: Sizing (Always)

Choose hardware with enough disk for your deployment window:

| Use Case | Panel Count | Min Storage | Years Retained |
|---|---|---|---|
| Development / testing | Any | 32 GB | 5+ |
| Production SBC | 12–24 | **64 GB** | 5+ |
| Production server | 24–48 | **256 GB** | 10+ |

**Do not deploy on 4 GB SD cards.** The device will fill within 6 months on a typical residential array.

### Tier 2: Disk Hygiene (Immediate)

Reclaim wasted space before implementing retention:

1. **Cap journald** — drop-in at `/etc/systemd/journald.conf.d/pvs.conf`:
   ```ini
   [Journal]
   SystemMaxUse=200M
   ```

2. **Rotate application logs** — logrotate snippet at `/etc/logrotate.d/pvs-monitoring`:
   ```
   /var/log/pvs-update.log {
     weekly
     rotate 4
     compress
     copytruncate
     missingok
   }
   ```

3. **Clean apt cache:**
   ```sh
   sudo apt-get clean
   ```

4. **Checkpoint WAL:**
   ```sh
   sudo sqlite3 /var/lib/pvs-monitor/readings.db "PRAGMA wal_checkpoint(TRUNCATE);"
   ```

### Tier 3: Retention (If needed on constrained hardware)

**Only deploy this on hardware too small to keep full history.** On larger devices, retention is disabled by default.

- **Single retention window** for all three raw streams (`readings`, `inverter_readings`, `aux_device_readings`), sized to device capacity. Recommended: **90 days** on 4 GB, **unlimited** on 64+ GB.
- **Retention is opt-in**, per-host. It defaults to unlimited (off). It must never be enabled globally; on large-capacity hosts it would delete data the device has room for.
- **Always backup before pruning.** Use `VACUUM INTO` to a safe location before any delete operation.
- **Delete in batches** to avoid blocking the 1 Hz writer during checkpoint.
- **Implement as a maintenance tool** (`pvs-prune`), not ad-hoc SQL, following the `pvs-backfill` pattern.

### Tier 4: Free-Space Monitoring (Always)

Extend the heartbeat to fail the healthcheck when `/var/lib/pvs-monitor` free space drops below 500 MB. This catches filling conditions *before* a gap appears.

```sh
if available_gb=$(df /var/lib/pvs-monitor | awk 'NR==2 {print $4/1024/1024}'); then
  if (( available_gb < 0.5 )); then
    # Report unhealthy; healthcheck.io will alert
    exit 1
  fi
fi
```

## Diagnostic Checklist

When approaching capacity:

```sh
# Overall disk usage
df -h /

# What's eating the root filesystem
sudo du -xhd1 /var /usr /home 2>/dev/null | sort -h | tail -20

# Journal footprint
journalctl --disk-usage

# Database size and WAL status
ls -lh /var/lib/pvs-monitor/readings.db*

# Per-table row counts and date range
sudo sqlite3 /var/lib/pvs-monitor/readings.db <<EOF
SELECT 'readings', COUNT(*), datetime(MIN(received_at), 'unixepoch'), datetime(MAX(received_at), 'unixepoch') FROM readings
UNION ALL SELECT 'inverter', COUNT(*), datetime(MIN(received_at), 'unixepoch'), datetime(MAX(received_at), 'unixepoch') FROM inverter_readings
UNION ALL SELECT 'aux', COUNT(*), datetime(MIN(received_at), 'unixepoch'), datetime(MAX(received_at), 'unixepoch') FROM aux_device_readings;
EOF
```

## Key Design Principles

1. **Prevent, don't detect.** Sizing for your deployment window beats reactive retention.
2. **Retention is optional.** Large-capacity hosts must never prune by default.
3. **Back up before acting.** Every destructive operation must have a safe copy first.
4. **Monitor free space constantly.** A healthcheck that watches disk before it becomes a gap catches near-misses early.
5. **Keep repair data.** `aux_device_readings` is the only source for backfilling a stalled 1 Hz stream. Never prune it below your desired retention window.
