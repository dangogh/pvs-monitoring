SELECT bucket, avg_solar_kw, avg_load_kw
FROM readings_daily
WHERE bucket >= ? AND bucket <= ?
ORDER BY bucket
