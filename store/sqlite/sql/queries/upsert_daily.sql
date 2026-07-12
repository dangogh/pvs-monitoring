INSERT INTO readings_daily (bucket, avg_solar_kw, avg_load_kw, avg_net_kw, sample_count,
                            min_solar_kwh, max_solar_kwh, min_load_kwh, max_load_kwh)
VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(bucket) DO UPDATE SET
    avg_solar_kw  = (avg_solar_kw  * sample_count + excluded.avg_solar_kw)  / (sample_count + 1),
    avg_load_kw   = (avg_load_kw   * sample_count + excluded.avg_load_kw)   / (sample_count + 1),
    avg_net_kw    = (avg_net_kw    * sample_count + excluded.avg_net_kw)    / (sample_count + 1),
    sample_count  = sample_count + 1,
    min_solar_kwh = MIN(min_solar_kwh, excluded.min_solar_kwh),
    max_solar_kwh = MAX(max_solar_kwh, excluded.max_solar_kwh),
    min_load_kwh  = MIN(min_load_kwh,  excluded.min_load_kwh),
    max_load_kwh  = MAX(max_load_kwh,  excluded.max_load_kwh)
