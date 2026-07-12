CREATE TABLE inverter_readings (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at     INTEGER NOT NULL,
	serial          TEXT    NOT NULL,
	state           TEXT    NOT NULL,
	state_descr     TEXT    NOT NULL,
	power_kw        REAL    NOT NULL,
	lifetime_kwh    REAL    NOT NULL,
	voltage_v       REAL    NOT NULL,
	current_a       REAL    NOT NULL,
	power_mppt1_kw  REAL    NOT NULL,
	voltage_mppt1_v REAL    NOT NULL,
	current_mppt1_a REAL    NOT NULL,
	temp_c          REAL    NOT NULL,
	freq_hz         REAL    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_inv_received_at ON inverter_readings(received_at);
CREATE INDEX IF NOT EXISTS idx_inv_serial      ON inverter_readings(serial);

CREATE TABLE pvs_readings (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at INTEGER NOT NULL,
	serial      TEXT    NOT NULL,
	state       TEXT    NOT NULL,
	state_descr TEXT    NOT NULL,
	err_count   INTEGER NOT NULL,
	comm_err    INTEGER NOT NULL,
	uptime_sec  INTEGER NOT NULL,
	cpu_load    REAL    NOT NULL,
	mem_used    INTEGER NOT NULL,
	flash_avail INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pvs_received_at ON pvs_readings(received_at);

CREATE TABLE meter_readings (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at          INTEGER NOT NULL,
	serial               TEXT    NOT NULL,
	state                TEXT    NOT NULL,
	state_descr          TEXT    NOT NULL,
	subtype              TEXT    NOT NULL,
	lifetime_kwh         REAL    NOT NULL,
	power_kw             REAL    NOT NULL,
	reactive_power_kvar  REAL    NOT NULL,
	apparent_power_kva   REAL    NOT NULL,
	power_factor         REAL    NOT NULL,
	freq_hz              REAL    NOT NULL,
	current_a            REAL    NOT NULL,
	voltage_v            REAL    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_meter_received_at ON meter_readings(received_at);
