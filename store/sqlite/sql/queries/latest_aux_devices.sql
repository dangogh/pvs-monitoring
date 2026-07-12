SELECT device_type, serial, received_at, payload
FROM aux_device_readings
WHERE received_at = (SELECT MAX(received_at) FROM aux_device_readings)
ORDER BY device_type, serial
