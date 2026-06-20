package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dangogh/pvs-monitoring/pvs"
)

// Store persists readings in a SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applying any pending migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// migrations is an ordered list of SQL DDL strings, one per schema version.
// Index i brings the DB from version i → i+1.
var migrations = []string{
	// version 1: initial schema
	`CREATE TABLE IF NOT EXISTS readings (
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
	CREATE INDEX IF NOT EXISTS idx_device_received_at ON device_readings(received_at);`,

	// version 2: typed inverter table + pvs/meter tables; migrate device_readings
	`CREATE TABLE inverter_readings (
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
	CREATE INDEX IF NOT EXISTS idx_meter_received_at ON meter_readings(received_at);`,

	// version 3: consolidate pvs_readings + meter_readings into aux_device_readings
	`CREATE TABLE aux_device_readings (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		received_at INTEGER NOT NULL,
		device_type TEXT    NOT NULL,
		serial      TEXT    NOT NULL,
		payload     TEXT    NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_aux_received_at ON aux_device_readings(received_at);`,

	// version 4: inverter outage tracking (one row per error period)
	`CREATE TABLE inverter_outages (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		serial     TEXT    NOT NULL,
		error_at   INTEGER NOT NULL,
		healthy_at INTEGER
	);
	CREATE INDEX IF NOT EXISTS idx_outage_serial ON inverter_outages(serial);
	CREATE INDEX IF NOT EXISTS idx_outage_open ON inverter_outages(serial) WHERE healthy_at IS NULL;`,
}

// migrateV2 copies rows from device_readings into the typed tables, then drops device_readings.
func migrateV2(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT received_at, device_type, serial, payload FROM device_readings`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var receivedAt int64
		var deviceType, serial, payload string
		if err := rows.Scan(&receivedAt, &deviceType, &serial, &payload); err != nil {
			return err
		}
		t := time.Unix(receivedAt, 0)
		d := pvs.Device{DeviceType: deviceType, Serial: serial, Raw: []byte(payload)}

		switch deviceType {
		case "Inverter":
			inv, err := d.ToInverter(t)
			if err != nil {
				slog.Default().Warn("migrateV2: skipping unparseable inverter row", "serial", serial, "err", err)
				continue
			}
			if _, err := tx.Exec(
				`INSERT INTO inverter_readings
				 (received_at, serial, state, state_descr, power_kw, lifetime_kwh, voltage_v, current_a,
				  power_mppt1_kw, voltage_mppt1_v, current_mppt1_a, temp_c, freq_hz)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				receivedAt, inv.Serial, inv.State, inv.StateDescr,
				inv.PowerKW, inv.LifetimeKWh, inv.VoltageV, inv.CurrentA,
				inv.PowerMPPT1KW, inv.VoltageMPPT1V, inv.CurrentMPPT1A,
				inv.TempC, inv.FreqHz,
			); err != nil {
				return err
			}
		default:
			if _, err := tx.Exec(
				`INSERT INTO pvs_readings
				 (received_at, serial, state, state_descr, err_count, comm_err, uptime_sec, cpu_load, mem_used, flash_avail)
				 VALUES (?, ?, ?, '', 0, 0, 0, 0, 0, 0)`,
				receivedAt, serial, "",
			); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(`DROP TABLE device_readings`)
	return err
}

// migrateV3 moves pvs_readings and meter_readings into aux_device_readings, then drops them.
func migrateV3(tx *sql.Tx) error {
	// pvs_readings has no payload — reconstruct minimal JSON for the aux table.
	pvRows, err := tx.Query(`SELECT received_at, serial, state, state_descr, err_count, comm_err, uptime_sec, cpu_load, mem_used, flash_avail FROM pvs_readings`)
	if err != nil {
		return err
	}
	defer pvRows.Close()
	for pvRows.Next() {
		var ra int64
		var serial, state, stateDescr string
		var errCount, commErr, uptime, memUsed, flashAvail int64
		var cpuLoad float64
		if err := pvRows.Scan(&ra, &serial, &state, &stateDescr, &errCount, &commErr, &uptime, &cpuLoad, &memUsed, &flashAvail); err != nil {
			return err
		}
		payload := fmt.Sprintf(
			`{"SERIAL":%q,"STATE":%q,"STATEDESCR":%q,"dl_err_count":"%d","dl_comm_err":"%d","dl_uptime":"%d","dl_cpu_load":"%.2f","dl_mem_used":"%d","dl_flash_avail":"%d"}`,
			serial, state, stateDescr, errCount, commErr, uptime, cpuLoad, memUsed, flashAvail,
		)
		if _, err := tx.Exec(
			`INSERT INTO aux_device_readings (received_at, device_type, serial, payload) VALUES (?, 'PVS', ?, ?)`,
			ra, serial, payload,
		); err != nil {
			return err
		}
	}
	if err := pvRows.Err(); err != nil {
		return err
	}

	mRows, err := tx.Query(`SELECT received_at, serial, state, state_descr, subtype, lifetime_kwh, power_kw, reactive_power_kvar, apparent_power_kva, power_factor, freq_hz, current_a, voltage_v FROM meter_readings`)
	if err != nil {
		return err
	}
	defer mRows.Close()
	for mRows.Next() {
		var ra int64
		var serial, state, stateDescr, subtype string
		var lifetimeKWh, powerKW, reactivePower, apparentPower, powerFactor, freqHz, currentA, voltageV float64
		if err := mRows.Scan(&ra, &serial, &state, &stateDescr, &subtype,
			&lifetimeKWh, &powerKW, &reactivePower, &apparentPower, &powerFactor, &freqHz, &currentA, &voltageV,
		); err != nil {
			return err
		}
		payload := fmt.Sprintf(
			`{"SERIAL":%q,"STATE":%q,"STATEDESCR":%q,"subtype":%q,"net_ltea_3phsum_kwh":"%.4f","p_3phsum_kw":"%.4f","q_3phsum_kvar":"%.4f","s_3phsum_kva":"%.4f","tot_pf_rto":"%.4f","freq_hz":"%.2f","i_a":"%.4f","v12_v":"%.4f"}`,
			serial, state, stateDescr, subtype,
			lifetimeKWh, powerKW, reactivePower, apparentPower, powerFactor, freqHz, currentA, voltageV,
		)
		if _, err := tx.Exec(
			`INSERT INTO aux_device_readings (received_at, device_type, serial, payload) VALUES (?, 'Power Meter', ?, ?)`,
			ra, serial, payload,
		); err != nil {
			return err
		}
	}
	if err := mRows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE pvs_readings`); err != nil {
		return err
	}
	_, err = tx.Exec(`DROP TABLE meter_readings`)
	return err
}

// migrateV4 backfills inverter_outages from existing inverter_readings by detecting
// state transitions (→error opens an outage, →working closes it).
func migrateV4(tx *sql.Tx) error {
	rows, err := tx.Query(
		`SELECT serial, received_at, state FROM inverter_readings ORDER BY serial, received_at`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type outageRow struct {
		serial    string
		errorAt   int64
		healthyAt *int64
	}
	var outages []outageRow
	prevState := map[string]string{}
	openErrorAt := map[string]int64{}

	for rows.Next() {
		var serial, state string
		var receivedAt int64
		if err := rows.Scan(&serial, &receivedAt, &state); err != nil {
			return err
		}
		prev := prevState[serial]
		if state == "error" && prev != "error" {
			openErrorAt[serial] = receivedAt
		} else if state != "error" && prev == "error" {
			ea := openErrorAt[serial]
			ha := receivedAt
			outages = append(outages, outageRow{serial, ea, &ha})
			delete(openErrorAt, serial)
		}
		prevState[serial] = state
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for serial, errorAt := range openErrorAt {
		outages = append(outages, outageRow{serial, errorAt, nil})
	}

	for _, o := range outages {
		if o.healthyAt != nil {
			if _, err := tx.Exec(
				`INSERT INTO inverter_outages (serial, error_at, healthy_at) VALUES (?, ?, ?)`,
				o.serial, o.errorAt, *o.healthyAt,
			); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(
				`INSERT INTO inverter_outages (serial, error_at) VALUES (?, ?)`,
				o.serial, o.errorAt,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// dataMigrations holds optional data-migration functions keyed by migration index (0-based).
var dataMigrations = map[int]func(*sql.Tx) error{
	1: migrateV2,
	2: migrateV3,
	3: migrateV4,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}

	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback() //nolint:errcheck
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if fn, ok := dataMigrations[i]; ok {
			if err := fn(tx); err != nil {
				tx.Rollback() //nolint:errcheck
				return fmt.Errorf("migration %d data: %w", i+1, err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback() //nolint:errcheck
			return fmt.Errorf("set user_version %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}
	return nil
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

func (s *Store) LatestReading(ctx context.Context) (*pvs.Reading, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT received_at, reading_time, solar_kw, load_kw, net_kw, solar_kwh, load_kwh, net_kwh
		 FROM readings ORDER BY received_at DESC LIMIT 1`)
	var receivedAt, readingTime int64
	var r pvs.Reading
	err := row.Scan(&receivedAt, &readingTime, &r.SolarKW, &r.LoadKW, &r.NetKW, &r.SolarKWh, &r.LoadKWh, &r.NetKWh)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query latest reading: %w", err)
	}
	r.ReceivedAt = time.Unix(receivedAt, 0)
	r.Time = time.Unix(readingTime, 0)
	return &r, nil
}

func (s *Store) EarliestReadingAt(ctx context.Context) (time.Time, error) {
	row := s.db.QueryRowContext(ctx, `SELECT MIN(received_at) FROM readings`)
	var ts sql.NullInt64
	if err := row.Scan(&ts); err != nil {
		return time.Time{}, fmt.Errorf("query earliest reading: %w", err)
	}
	if !ts.Valid {
		return time.Time{}, nil
	}
	return time.Unix(ts.Int64, 0), nil
}

func (s *Store) AveragePower(ctx context.Context, since, until time.Time) (pvs.PowerAvg, error) {
	if !until.IsZero() && since.After(until) {
		return pvs.PowerAvg{}, fmt.Errorf("since (%s) is after until (%s)", since, until)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT AVG(solar_kw), AVG(load_kw), AVG(net_kw), COUNT(*)
		 FROM readings WHERE received_at >= ? AND received_at <= ?`,
		since.Unix(), until.Unix(),
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

func (s *Store) EnergyDelta(ctx context.Context, since, until time.Time) (pvs.EnergyDelta, error) {
	if !until.IsZero() && since.After(until) {
		return pvs.EnergyDelta{}, fmt.Errorf("since (%s) is after until (%s)", since, until)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(solar_kwh)-MIN(solar_kwh), 0),
		        COALESCE(MAX(load_kwh)-MIN(load_kwh), 0),
		        COALESCE(MAX(net_kwh)-MIN(net_kwh), 0)
		 FROM readings WHERE received_at >= ? AND received_at <= ?`,
		since.Unix(), until.Unix(),
	)
	var solar, load, net float64
	if err := row.Scan(&solar, &load, &net); err != nil {
		return pvs.EnergyDelta{}, fmt.Errorf("query energy delta: %w", err)
	}
	return pvs.EnergyDelta{
		SolarKWh: solar,
		LoadKWh:  load,
		NetKWh:   net,
	}, nil
}

func (s *Store) SaveDevices(ctx context.Context, devices []pvs.Device, receivedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, d := range devices {
		switch d.DeviceType {
		case "Inverter":
			inv, err := d.ToInverter(receivedAt)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO inverter_readings
				 (received_at, serial, state, state_descr, power_kw, lifetime_kwh, voltage_v, current_a,
				  power_mppt1_kw, voltage_mppt1_v, current_mppt1_a, temp_c, freq_hz)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				receivedAt.Unix(), inv.Serial, inv.State, inv.StateDescr,
				inv.PowerKW, inv.LifetimeKWh, inv.VoltageV, inv.CurrentA,
				inv.PowerMPPT1KW, inv.VoltageMPPT1V, inv.CurrentMPPT1A,
				inv.TempC, inv.FreqHz,
			); err != nil {
				return err
			}
		default:
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO aux_device_readings (received_at, device_type, serial, payload) VALUES (?, ?, ?, ?)`,
				receivedAt.Unix(), d.DeviceType, d.Serial, string(d.Raw),
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) LatestInverters(ctx context.Context) ([]pvs.InverterDevice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT serial, state, state_descr, received_at, power_kw, lifetime_kwh, voltage_v, current_a,
		        power_mppt1_kw, voltage_mppt1_v, current_mppt1_a, temp_c, freq_hz
		 FROM inverter_readings
		 WHERE received_at = (SELECT MAX(received_at) FROM inverter_readings)
		 ORDER BY serial`)
	if err != nil {
		return nil, fmt.Errorf("query latest inverters: %w", err)
	}
	defer rows.Close()
	var out []pvs.InverterDevice
	for rows.Next() {
		var d pvs.InverterDevice
		var receivedAt int64
		if err := rows.Scan(
			&d.Serial, &d.State, &d.StateDescr, &receivedAt,
			&d.PowerKW, &d.LifetimeKWh, &d.VoltageV, &d.CurrentA,
			&d.PowerMPPT1KW, &d.VoltageMPPT1V, &d.CurrentMPPT1A,
			&d.TempC, &d.FreqHz,
		); err != nil {
			return nil, fmt.Errorf("scan inverter: %w", err)
		}
		d.ReceivedAt = time.Unix(receivedAt, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) LatestAuxDevices(ctx context.Context) ([]pvs.AuxDevice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_type, serial, received_at, payload
		 FROM aux_device_readings
		 WHERE received_at = (SELECT MAX(received_at) FROM aux_device_readings)
		 ORDER BY device_type, serial`)
	if err != nil {
		return nil, fmt.Errorf("query latest aux devices: %w", err)
	}
	defer rows.Close()
	var out []pvs.AuxDevice
	for rows.Next() {
		var d pvs.AuxDevice
		var receivedAt int64
		var payload string
		if err := rows.Scan(&d.DeviceType, &d.Serial, &receivedAt, &payload); err != nil {
			return nil, fmt.Errorf("scan aux device: %w", err)
		}
		d.ReceivedAt = time.Unix(receivedAt, 0)
		d.Payload = []byte(payload)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ReadingsSeries(ctx context.Context, since, until time.Time, bucketSeconds int64) ([]pvs.SeriesPoint, error) {
	if !until.IsZero() && since.After(until) {
		return nil, fmt.Errorf("since (%s) is after until (%s)", since, until)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT CAST(received_at / ? AS INTEGER) * ? AS bucket, AVG(solar_kw), AVG(load_kw)
		 FROM readings WHERE received_at >= ? AND received_at <= ?
		 GROUP BY bucket ORDER BY bucket`,
		bucketSeconds, bucketSeconds, since.Unix(), until.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("query series: %w", err)
	}
	defer rows.Close()
	var pts []pvs.SeriesPoint
	for rows.Next() {
		var bucket int64
		var solar, load float64
		if err := rows.Scan(&bucket, &solar, &load); err != nil {
			return nil, fmt.Errorf("scan series: %w", err)
		}
		pts = append(pts, pvs.SeriesPoint{Time: time.Unix(bucket, 0), SolarKW: solar, LoadKW: load})
	}
	return pts, rows.Err()
}

func (s *Store) CountReadings(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM readings`).Scan(&count)
	return count, err
}

// OpenInverterOutage records that serial entered error state at at.
// If an open outage already exists for this serial (e.g. after a service restart),
// this is a no-op so we don't create duplicate records.
func (s *Store) OpenInverterOutage(ctx context.Context, serial string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO inverter_outages (serial, error_at)
		 SELECT ?, ? WHERE NOT EXISTS (
		     SELECT 1 FROM inverter_outages WHERE serial = ? AND healthy_at IS NULL
		 )`,
		serial, at.Unix(), serial,
	)
	return err
}

// CloseInverterOutage records that serial returned to a healthy state at at.
func (s *Store) CloseInverterOutage(ctx context.Context, serial string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE inverter_outages SET healthy_at = ?
		 WHERE id = (
		     SELECT id FROM inverter_outages
		     WHERE serial = ? AND healthy_at IS NULL
		     ORDER BY error_at DESC LIMIT 1
		 )`,
		at.Unix(), serial,
	)
	return err
}

func (s *Store) ListOpenInverterOutages(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT serial FROM inverter_outages WHERE healthy_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("query open outages: %w", err)
	}
	defer rows.Close()
	var serials []string
	for rows.Next() {
		var serial string
		if err := rows.Scan(&serial); err != nil {
			return nil, fmt.Errorf("scan serial: %w", err)
		}
		serials = append(serials, serial)
	}
	return serials, rows.Err()
}

func (s *Store) Close() error {
	return s.db.Close()
}
