CREATE TABLE inverter_outages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	serial     TEXT    NOT NULL,
	error_at   INTEGER NOT NULL,
	healthy_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_outage_serial ON inverter_outages(serial);
CREATE INDEX IF NOT EXISTS idx_outage_open ON inverter_outages(serial) WHERE healthy_at IS NULL;
