SELECT SUM(avg_solar_kw*sample_count)/SUM(sample_count),
       SUM(avg_load_kw*sample_count)/SUM(sample_count),
       SUM(avg_net_kw*sample_count)/SUM(sample_count),
       SUM(sample_count)
FROM readings_hourly WHERE bucket >= ? AND bucket <= ?
