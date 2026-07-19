SELECT CAST(received_at / 300 AS INTEGER) * 300 AS bucket, serial, AVG(power_kw) AS avg_power_kw
FROM inverter_readings
WHERE received_at >= ? AND received_at <= ?
GROUP BY bucket, serial
ORDER BY bucket, serial
