CREATE TABLE aux_device_readings (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at INTEGER NOT NULL,
	device_type TEXT    NOT NULL,
	serial      TEXT    NOT NULL,
	payload     TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aux_received_at ON aux_device_readings(received_at);
