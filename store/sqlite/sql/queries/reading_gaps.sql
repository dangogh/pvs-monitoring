-- Find gaps in the 1 Hz readings stream, with the number of Power Meter samples
-- that exist inside each one.
--
-- aux_samples > 0 means the PVS6 was still reachable and its meters were still
-- being polled while the WebSocket power stream was dead: a telemetry stall,
-- recoverable from the meter payloads. aux_samples = 0 means the PVS6 itself was
-- off and nothing was recorded anywhere, so the gap is permanent.
SELECT lo, hi, hi - lo AS secs,
       (SELECT COUNT(*) FROM aux_device_readings a
         WHERE a.received_at > g.lo AND a.received_at < g.hi
           AND a.device_type = 'Power Meter'
           AND json_extract(a.payload, '$.TYPE') = 'PVS5-METER-C') AS aux_samples
FROM (SELECT LAG(received_at) OVER (ORDER BY received_at) AS lo, received_at AS hi
      FROM readings) g
WHERE lo IS NOT NULL AND hi - lo > ?
ORDER BY lo
