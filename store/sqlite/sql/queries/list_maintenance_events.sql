SELECT id, start_at, end_at, event_type, notes, created_at
FROM maintenance_events
ORDER BY start_at DESC
