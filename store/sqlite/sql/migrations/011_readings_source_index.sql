-- Partial index over backfilled rows only.
--
-- /api/data reports how much of a range was reconstructed rather than measured
-- live, which means asking "are there any non-NULL source rows in this window".
-- Without an index that scans every reading in the range — millions of rows for
-- a month-long request, on every call. A partial index covers only the handful
-- of backfilled rows that exist (3,449 today), so the query touches nothing else
-- and costs almost no space.
CREATE INDEX idx_readings_backfilled ON readings(received_at) WHERE source IS NOT NULL;
