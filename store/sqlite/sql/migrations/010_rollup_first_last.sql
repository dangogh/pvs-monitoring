-- Record the first and last kWh counter value in each rollup bucket.
--
-- The rollup tables only stored min/max, which forces energy_delta_rollup.sql to
-- compute MAX-MINUS-MIN. That is wrong whenever the PVS6 lifetime counters run
-- backward (see 2026-08-20/21, when both counters inverted for ~29h): a declining
-- counter reports the size of the decline as if it were energy. The raw-table
-- query was fixed to use last-minus-first; these columns let the rollup path do
-- the same.
--
-- Columns are nullable on purpose. Buckets whose raw readings have since been
-- pruned cannot be backfilled, and the query falls back to min/max for those
-- rather than losing the row entirely.
ALTER TABLE readings_hourly ADD COLUMN first_solar_kwh REAL;
ALTER TABLE readings_hourly ADD COLUMN last_solar_kwh  REAL;
ALTER TABLE readings_hourly ADD COLUMN first_load_kwh  REAL;
ALTER TABLE readings_hourly ADD COLUMN last_load_kwh   REAL;

ALTER TABLE readings_daily ADD COLUMN first_solar_kwh REAL;
ALTER TABLE readings_daily ADD COLUMN last_solar_kwh  REAL;
ALTER TABLE readings_daily ADD COLUMN first_load_kwh  REAL;
ALTER TABLE readings_daily ADD COLUMN last_load_kwh   REAL;

-- Backfill from the raw table. The half-open range predicate on received_at is
-- index-friendly (idx_received_at); matching on a computed bucket expression
-- would not be.
UPDATE readings_hourly SET
    first_solar_kwh = (SELECT r.solar_kwh FROM readings r
                       WHERE r.received_at >= readings_hourly.bucket
                         AND r.received_at <  readings_hourly.bucket + 3600
                       ORDER BY r.received_at ASC LIMIT 1),
    last_solar_kwh  = (SELECT r.solar_kwh FROM readings r
                       WHERE r.received_at >= readings_hourly.bucket
                         AND r.received_at <  readings_hourly.bucket + 3600
                       ORDER BY r.received_at DESC LIMIT 1),
    first_load_kwh  = (SELECT r.load_kwh FROM readings r
                       WHERE r.received_at >= readings_hourly.bucket
                         AND r.received_at <  readings_hourly.bucket + 3600
                       ORDER BY r.received_at ASC LIMIT 1),
    last_load_kwh   = (SELECT r.load_kwh FROM readings r
                       WHERE r.received_at >= readings_hourly.bucket
                         AND r.received_at <  readings_hourly.bucket + 3600
                       ORDER BY r.received_at DESC LIMIT 1);

UPDATE readings_daily SET
    first_solar_kwh = (SELECT r.solar_kwh FROM readings r
                       WHERE r.received_at >= readings_daily.bucket
                         AND r.received_at <  readings_daily.bucket + 86400
                       ORDER BY r.received_at ASC LIMIT 1),
    last_solar_kwh  = (SELECT r.solar_kwh FROM readings r
                       WHERE r.received_at >= readings_daily.bucket
                         AND r.received_at <  readings_daily.bucket + 86400
                       ORDER BY r.received_at DESC LIMIT 1),
    first_load_kwh  = (SELECT r.load_kwh FROM readings r
                       WHERE r.received_at >= readings_daily.bucket
                         AND r.received_at <  readings_daily.bucket + 86400
                       ORDER BY r.received_at ASC LIMIT 1),
    last_load_kwh   = (SELECT r.load_kwh FROM readings r
                       WHERE r.received_at >= readings_daily.bucket
                         AND r.received_at <  readings_daily.bucket + 86400
                       ORDER BY r.received_at DESC LIMIT 1);
