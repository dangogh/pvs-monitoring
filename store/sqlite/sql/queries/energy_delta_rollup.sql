-- Energy over a window from the hourly rollup, for spans too long to scan raw.
--
-- Mirrors energy_delta.sql: last-minus-first across the range, not MAX-MINUS-MIN.
-- The PVS6 lifetime counters can run backward (2026-08-20/21), and under
-- MAX-MINUS-MIN a declining counter reports the decline as if it were energy.
--
-- Takes the first value of the earliest bucket and the last value of the latest
-- bucket in range. COALESCE falls back to min/max for buckets predating migration
-- 010 whose raw readings were pruned and so could not be backfilled.
--
-- A negative result means the counter went backward across the window, which is
-- physically impossible; clamp to 0 rather than reporting negative energy.
WITH b AS (
    SELECT bucket,
           COALESCE(first_solar_kwh, min_solar_kwh) AS first_solar,
           COALESCE(last_solar_kwh,  max_solar_kwh) AS last_solar,
           COALESCE(first_load_kwh,  min_load_kwh)  AS first_load,
           COALESCE(last_load_kwh,   max_load_kwh)  AS last_load
    FROM readings_hourly
    WHERE bucket >= ? AND bucket <= ?
),
edge AS (
    SELECT (SELECT MIN(bucket) FROM b) AS lo,
           (SELECT MAX(bucket) FROM b) AS hi
),
delta AS (
    SELECT COALESCE(MAX(CASE WHEN b.bucket = edge.hi THEN b.last_solar END)
                  - MAX(CASE WHEN b.bucket = edge.lo THEN b.first_solar END), 0) AS solar,
           COALESCE(MAX(CASE WHEN b.bucket = edge.hi THEN b.last_load END)
                  - MAX(CASE WHEN b.bucket = edge.lo THEN b.first_load END), 0) AS load
    FROM b, edge
)
SELECT CASE WHEN solar > 0 THEN solar ELSE 0 END,
       CASE WHEN load  > 0 THEN load  ELSE 0 END,
       (CASE WHEN load  > 0 THEN load  ELSE 0 END)
     - (CASE WHEN solar > 0 THEN solar ELSE 0 END)
FROM delta
