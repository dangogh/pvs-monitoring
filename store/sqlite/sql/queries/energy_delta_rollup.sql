SELECT COALESCE(MAX(max_solar_kwh)-MIN(min_solar_kwh), 0),
       COALESCE(MAX(max_load_kwh)-MIN(min_load_kwh), 0),
       COALESCE(MAX(max_load_kwh)-MIN(min_load_kwh), 0) - COALESCE(MAX(max_solar_kwh)-MIN(min_solar_kwh), 0)
FROM readings_hourly WHERE bucket >= ? AND bucket <= ?
