ALTER TABLE maintenance_events ADD COLUMN start_at INTEGER;
ALTER TABLE maintenance_events ADD COLUMN end_at INTEGER;

UPDATE maintenance_events SET start_at = CAST(strftime('%s', start_date) AS INTEGER);
UPDATE maintenance_events SET end_at = CAST(strftime('%s', end_date) AS INTEGER) WHERE end_date IS NOT NULL;

ALTER TABLE maintenance_events DROP COLUMN start_date;
ALTER TABLE maintenance_events DROP COLUMN end_date;
