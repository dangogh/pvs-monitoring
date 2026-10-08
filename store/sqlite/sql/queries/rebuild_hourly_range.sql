-- Rebuild readings_hourly for one bucket range, weighted by time coverage.
-- Paired with a DELETE over the same range: rollups must be rebuilt, never
-- upserted into, once a bucket holds a mix of resolutions.
--
-- sample_count is a WEIGHT, not a row count. Downstream queries combine buckets
-- as SUM(avg*sample_count)/SUM(sample_count) (see average_power_rollup.sql), so
-- it has to express how much TIME a bucket's average represents. A live 1 Hz row
-- covers one second; a 'meter-1min' row backfilled from the meter payloads
-- covers sixty. Counting rows would weight a backfilled hour 60x too little,
-- and in a partially-populated bucket would let the real 1 Hz samples dominate
-- a span they do not cover (2026-09-24 holds 49,455 real samples beside a
-- third of a day that has none).
--
-- This is the single weighted definition of a bucket. backfill_hourly.sql stays
-- unweighted on purpose: it runs inside migration 5, where the `source` column
-- does not exist yet and every row is necessarily live, so weight is 1 anyway.
INSERT INTO readings_hourly
    (bucket, avg_solar_kw, avg_load_kw, avg_net_kw, sample_count,
     min_solar_kwh, max_solar_kwh, min_load_kwh, max_load_kwh)
SELECT CAST(received_at/3600 AS INTEGER)*3600,
       SUM(solar_kw * w)/SUM(w), SUM(load_kw * w)/SUM(w), SUM(net_kw * w)/SUM(w), SUM(w),
       MIN(solar_kwh), MAX(solar_kwh), MIN(load_kwh), MAX(load_kwh)
FROM (SELECT *, CASE WHEN source IS NULL THEN 1 ELSE 60 END AS w
      FROM readings WHERE received_at >= ? AND received_at < ?)
GROUP BY CAST(received_at/3600 AS INTEGER)*3600
