CREATE TABLE IF NOT EXISTS readings (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at  INTEGER NOT NULL,
	reading_time INTEGER NOT NULL,
	solar_kw     REAL NOT NULL,
	load_kw      REAL NOT NULL,
	net_kw       REAL NOT NULL,
	solar_kwh    REAL NOT NULL,
	load_kwh     REAL NOT NULL,
	net_kwh      REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_received_at ON readings(received_at);

CREATE TABLE IF NOT EXISTS device_readings (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at INTEGER NOT NULL,
	device_type TEXT NOT NULL,
	serial      TEXT NOT NULL,
	payload     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_device_received_at ON device_readings(received_at);
