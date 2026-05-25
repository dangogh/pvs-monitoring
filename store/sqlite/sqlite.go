package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dangogh/pvs-monitoring/pvs"
)

const schema = `
CREATE TABLE IF NOT EXISTS readings (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at INTEGER NOT NULL,
	reading_time INTEGER NOT NULL,
	solar_kw    REAL NOT NULL,
	load_kw     REAL NOT NULL,
	net_kw      REAL NOT NULL,
	solar_kwh   REAL NOT NULL,
	load_kwh    REAL NOT NULL,
	net_kwh     REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_received_at ON readings(received_at);
`

// Store persists readings in a SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) SaveReading(ctx context.Context, r *pvs.Reading) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO readings (received_at, reading_time, solar_kw, load_kw, net_kw, solar_kwh, load_kwh, net_kwh)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ReceivedAt.Unix(), r.Time.Unix(),
		r.SolarKW, r.LoadKW, r.NetKW,
		r.SolarKWh, r.LoadKWh, r.NetKWh,
	)
	return err
}

func (s *Store) AveragePower(ctx context.Context, since time.Time) (pvs.PowerAvg, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT AVG(solar_kw), AVG(load_kw), AVG(net_kw), COUNT(*)
		 FROM readings WHERE received_at >= ?`,
		since.Unix(),
	)
	var avg pvs.PowerAvg
	if err := row.Scan(&avg.SolarKW, &avg.LoadKW, &avg.NetKW, &avg.Samples); err != nil {
		return pvs.PowerAvg{}, fmt.Errorf("query average: %w", err)
	}
	return avg, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
