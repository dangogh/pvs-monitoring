CREATE INDEX IF NOT EXISTS idx_inv_serial_received_at ON inverter_readings(serial, received_at);
