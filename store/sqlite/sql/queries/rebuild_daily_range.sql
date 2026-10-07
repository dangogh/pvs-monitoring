-- Rebuild readings_daily for one bucket range, weighted by time coverage.
-- See rebuild_hourly_range.sql for why sample_count is a weight and not a row
-- count, and why backfill_daily.sql stays unweighted.
INSERT INTO readings_daily
    (bucket, avg_solar_kw, avg_load_kw, avg_net_kw, sample_count,
     min_solar_kwh, max_solar_kwh, min_load_kwh, max_load_kwh)
SELECT CAST(received_at/86400 AS INTEGER)*86400,
       SUM(solar_kw * w)/SUM(w), SUM(load_kw * w)/SUM(w), SUM(net_kw * w)/SUM(w), SUM(w),
       MIN(solar_kwh), MAX(solar_kwh), MIN(load_kwh), MAX(load_kwh)
FROM (SELECT *, CASE WHEN source IS NULL THEN 1 ELSE 60 END AS w
      FROM readings WHERE received_at >= ? AND received_at < ?)
GROUP BY CAST(received_at/86400 AS INTEGER)*86400
