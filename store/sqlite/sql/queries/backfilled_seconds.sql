-- Seconds of a range covered by reconstructed rather than live rows.
-- Each 'meter-1min' row stands for 60 seconds; see rebuild_hourly_range.sql.
-- Served by the idx_readings_backfilled partial index, so only backfilled rows
-- are visited regardless of how long the range is.
SELECT COALESCE(COUNT(*) * 60, 0)
FROM readings
WHERE source IS NOT NULL AND received_at >= ? AND received_at < ?
