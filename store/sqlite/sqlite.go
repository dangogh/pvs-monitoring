package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dangogh/pvs-monitoring/pvs"
)

const schema = `
CREATE TABLE IF NOT EXISTS readings (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at  INTEGER NOT NULL,
	reading_time INTEGER NOT NULL,
	solar_kw     REAL NOT NULL,
	load_kw      REAL NOT NULL,
	net_kw       REAL NOT NULL,
	solar_kwh    REAL NOT NULL,
	load_kwh     REAL NOT NULL,
	net_kwh      REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_received_at ON readings(received_at);

CREATE TABLE IF NOT EXISTS device_readings (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at INTEGER NOT NULL,
	device_type TEXT NOT NULL,
	serial      TEXT NOT NULL,
	payload     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_device_received_at ON device_readings(received_at);
`

// Store persists readings in a SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, creating parent directories as needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
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
	var solarKW, loadKW, netKW sql.NullFloat64
	var samples int
	if err := row.Scan(&solarKW, &loadKW, &netKW, &samples); err != nil {
		return pvs.PowerAvg{}, fmt.Errorf("query average: %w", err)
	}
	return pvs.PowerAvg{
		SolarKW: solarKW.Float64,
		LoadKW:  loadKW.Float64,
		NetKW:   netKW.Float64,
		Samples: samples,
	}, nil
}

func (s *Store) SaveDevices(ctx context.Context, devices []pvs.Device, receivedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, d := range devices {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO device_readings (received_at, device_type, serial, payload) VALUES (?, ?, ?, ?)`,
			receivedAt.Unix(), d.DeviceType, d.Serial, string(d.Raw),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LatestDevices(ctx context.Context) ([]pvs.Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_type, serial, payload FROM device_readings
		 WHERE received_at = (SELECT MAX(received_at) FROM device_readings)`)
	if err != nil {
		return nil, fmt.Errorf("query latest devices: %w", err)
	}
	defer rows.Close()
	var devices []pvs.Device
	for rows.Next() {
		var d pvs.Device
		var payload string
		if err := rows.Scan(&d.DeviceType, &d.Serial, &payload); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		d.Raw = []byte(payload)
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Store) CountReadings(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings`).Scan(&count)
	return count, err
}

func (s *Store) Close() error {
	return s.db.Close()
}
