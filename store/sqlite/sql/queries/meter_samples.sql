-- Reconstruct reading-shaped rows from the paired Power Meter payloads.
--
-- PVS5-METER-P is gross production, PVS5-METER-C is net consumption at the
-- service entrance (negative while exporting). The mapping below was validated
-- against a window where both the meters and the 1 Hz stream recorded: solar
-- and all three cumulative counters agree to within 0.04, while instantaneous
-- load can differ by up to ~2.5 kW because the two sources sample at different
-- instants and household load swings fast with AC compressor cycling.
SELECT c.received_at,
       p.kw                AS solar_kw,
       p.kw + c.kw         AS load_kw,
       c.kw                AS net_kw,
       p.kwh               AS solar_kwh,
       p.kwh + c.kwh       AS load_kwh,
       c.kwh               AS net_kwh
FROM (SELECT received_at,
             CAST(json_extract(payload, '$.p_3phsum_kw') AS REAL) AS kw,
             CAST(json_extract(payload, '$.net_ltea_3phsum_kwh') AS REAL) AS kwh
      FROM aux_device_readings
      WHERE device_type = 'Power Meter'
        AND json_extract(payload, '$.TYPE') = 'PVS5-METER-C'
        AND received_at >= ? AND received_at < ?) c
JOIN (SELECT received_at,
             CAST(json_extract(payload, '$.p_3phsum_kw') AS REAL) AS kw,
             CAST(json_extract(payload, '$.net_ltea_3phsum_kwh') AS REAL) AS kwh
      FROM aux_device_readings
      WHERE device_type = 'Power Meter'
        AND json_extract(payload, '$.TYPE') = 'PVS5-METER-P'
        AND received_at >= ? AND received_at < ?) p USING (received_at)
WHERE c.kw IS NOT NULL AND p.kw IS NOT NULL
  AND c.kwh IS NOT NULL AND p.kwh IS NOT NULL
ORDER BY c.received_at
