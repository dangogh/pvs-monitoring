SELECT received_at, reading_time, solar_kw, load_kw, net_kw, solar_kwh, load_kwh, net_kwh
FROM readings ORDER BY received_at DESC LIMIT 1
