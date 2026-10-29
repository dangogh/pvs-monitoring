# Database Sizing & Provisioning

## Current Growth Rate

As of 2026-10-14 (after 117 days of operation):

| Metric | Value |
|--------|-------|
| Database file size | 2.1 GB |
| Daily growth | 17.9 MB/day |
| Monthly growth | ~538 MB |
| Yearly growth | ~6.5 GB |

### Table Breakdown

| Table | Row Count | Rows/day | Purpose |
|-------|-----------|----------|---------|
| `readings` | 9,826,484 | 83,710 | 1 Hz power stream from PVS6 |
| `inverter_readings` | 4,421,932 | 37,859 | Per-panel power data (48 panels × ~1 Hz) |
| `aux_device_readings` | 501,315 | 4,273 | 1/minute device meter snapshots |
| `readings_daily` | 118 | 1.0 | Daily rollup (YoY comparison anchor) |
| `readings_hourly` | 2,795 | 23.8 | Hourly rollup |
| `inverter_outages` | 9,458 | — | Sparse event table |
| `maintenance_events` | 27 | — | Manual annotations |

## Provisioning Guidelines

### Server-Class Host

| Duration | Disk Required |
|----------|---------------|
| 1 year | ~10 GB (DB + WAL + headroom) |
| 5 years | ~50 GB |
| 10 years | ~100 GB |

**Recommended:** 256 GB minimum for long-term retention + comfortable growth margin.

### Single-Board Computer / Embedded

**Do not provision less than 32 GB.** Failures observed with 4 GB after ~4 months; even 16 GB is cutting it close.

| Duration | Disk Required |
|----------|---------------|
| 2 years | ~20 GB |
| 5 years | ~35 GB |

**Recommended:** 64 GB or larger for a Pi-class remote monitor.

### Multi-Host Deployments

Each monitoring instance accumulates data independently at ~17.9 MB/day. Plan 20 GB per instance per 2–3 years.

## Overhead & Headroom

- **WAL files:** ~10% of DB size (200–300 MB during normal ops)
- **VACUUM operations:** Require temporary 1.5× the DB size; ensure +20% free space
- **Snapshots:** Monthly compressed snapshot ≈ 400–500 MB

Reserve **at least 20% free disk** at all times to avoid exhaustion during maintenance.

## Archival Strategy (Optional)

If long-term cold storage is needed:

1. **Keep live:** Last 3 months of full data
2. **Archive:** Monthly compressed snapshots (zstd, ~400–500 MB each)
3. **Storage:** Local NAS, external drive, or S3 for 5+ years of history

5 years of monthly snapshots ≈ 30 GB (offline).

---

**Last updated:** 2026-10-14 based on 117-day baseline.
