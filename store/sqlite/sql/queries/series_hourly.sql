SELECT bucket, avg_solar_kw, avg_load_kw
FROM readings_hourly
WHERE bucket >= ? AND bucket <= ?
ORDER BY bucket
