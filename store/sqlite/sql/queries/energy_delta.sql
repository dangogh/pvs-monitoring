SELECT COALESCE(MAX(solar_kwh)-MIN(solar_kwh), 0),
       COALESCE(MAX(load_kwh)-MIN(load_kwh), 0),
       COALESCE(MAX(load_kwh)-MIN(load_kwh), 0) - COALESCE(MAX(solar_kwh)-MIN(solar_kwh), 0)
FROM readings WHERE received_at >= ? AND received_at <= ?
