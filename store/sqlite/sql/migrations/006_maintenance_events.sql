CREATE TABLE maintenance_events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	start_date TEXT    NOT NULL,
	end_date   TEXT,
	event_type TEXT    NOT NULL,
	notes      TEXT,
	created_at INTEGER NOT NULL
);
