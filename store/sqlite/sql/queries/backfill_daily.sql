INSERT OR IGNORE INTO readings_daily
    (bucket, avg_solar_kw, avg_load_kw, avg_net_kw, sample_count,
     min_solar_kwh, max_solar_kwh, min_load_kwh, max_load_kwh)
SELECT CAST(received_at/86400 AS INTEGER)*86400,
       AVG(solar_kw), AVG(load_kw), AVG(net_kw), COUNT(*),
       MIN(solar_kwh), MAX(solar_kwh), MIN(load_kwh), MAX(load_kwh)
FROM readings GROUP BY CAST(received_at/86400 AS INTEGER)*86400
