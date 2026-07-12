SELECT CAST(bucket/21600 AS INTEGER)*21600 AS b,
       SUM(avg_solar_kw*sample_count)/SUM(sample_count),
       SUM(avg_load_kw*sample_count)/SUM(sample_count)
FROM readings_hourly WHERE bucket >= ? AND bucket <= ?
GROUP BY b ORDER BY b
