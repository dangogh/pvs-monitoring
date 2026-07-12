CREATE TABLE readings_hourly (
	bucket        INTEGER PRIMARY KEY,
	avg_solar_kw  REAL    NOT NULL,
	avg_load_kw   REAL    NOT NULL,
	avg_net_kw    REAL    NOT NULL,
	sample_count  INTEGER NOT NULL,
	min_solar_kwh REAL    NOT NULL,
	max_solar_kwh REAL    NOT NULL,
	min_load_kwh  REAL    NOT NULL,
	max_load_kwh  REAL    NOT NULL
);
CREATE TABLE readings_daily (
	bucket        INTEGER PRIMARY KEY,
	avg_solar_kw  REAL    NOT NULL,
	avg_load_kw   REAL    NOT NULL,
	avg_net_kw    REAL    NOT NULL,
	sample_count  INTEGER NOT NULL,
	min_solar_kwh REAL    NOT NULL,
	max_solar_kwh REAL    NOT NULL,
	min_load_kwh  REAL    NOT NULL,
	max_load_kwh  REAL    NOT NULL
);
