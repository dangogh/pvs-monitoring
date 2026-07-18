package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dangogh/pvs-monitoring/pvs"
)

//go:embed sql
var sqlFS embed.FS

func mustSQL(path string) string {
	b, err := sqlFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("missing SQL file %s: %v", path, err))
	}
	return string(b)
}

// Store persists readings in a SQLite database.
type Store struct {
	db *sql.DB
}

// migrations is an ordered list of SQL DDL strings, one per schema version.
// Index i brings the DB from version i → i+1.
var migrations = []string{
	mustSQL("sql/migrations/001_initial.sql"),
	mustSQL("sql/migrations/002_inverter_tables.sql"),
	mustSQL("sql/migrations/003_aux_devices.sql"),
	mustSQL("sql/migrations/004_inverter_outages.sql"),
	mustSQL("sql/migrations/005_rollup_tables.sql"),
	mustSQL("sql/migrations/006_maintenance_events.sql"),
	mustSQL("sql/migrations/007_maintenance_event_timestamps.sql"),
	mustSQL("sql/migrations/008_inverter_serial_received_index.sql"),
}

var (
	sqlInsertReading      = mustSQL("sql/queries/insert_reading.sql")
	sqlLatestReading      = mustSQL("sql/queries/latest_reading.sql")
	sqlEarliestReading    = mustSQL("sql/queries/earliest_reading_at.sql")
	sqlAveragePower       = mustSQL("sql/queries/average_power.sql")
	sqlAveragePowerRollup = mustSQL("sql/queries/average_power_rollup.sql")
	sqlEnergyDelta        = mustSQL("sql/queries/energy_delta.sql")
	sqlEnergyDeltaRollup  = mustSQL("sql/queries/energy_delta_rollup.sql")
	sqlSeriesRaw          = mustSQL("sql/queries/series_raw.sql")
	sqlSeriesHourly       = mustSQL("sql/queries/series_hourly.sql")
	sqlSeriesHourly6h     = mustSQL("sql/queries/series_hourly_6h.sql")
	sqlSeriesDaily        = mustSQL("sql/queries/series_daily.sql")
	sqlUpsertHourly       = mustSQL("sql/queries/upsert_hourly.sql")
	sqlUpsertDaily        = mustSQL("sql/queries/upsert_daily.sql")
	sqlBackfillHourly     = mustSQL("sql/queries/backfill_hourly.sql")
	sqlBackfillDaily      = mustSQL("sql/queries/backfill_daily.sql")
	sqlCountReadings      = mustSQL("sql/queries/count_readings.sql")
	sqlInsertInverter     = mustSQL("sql/queries/insert_inverter_reading.sql")
	sqlInsertAuxDevice    = mustSQL("sql/queries/insert_aux_device.sql")
	sqlLatestInverters    = mustSQL("sql/queries/latest_inverters.sql")
	sqlLatestAuxDevices   = mustSQL("sql/queries/latest_aux_devices.sql")
	sqlOpenOutage              = mustSQL("sql/queries/open_outage.sql")
	sqlCloseOutage             = mustSQL("sql/queries/close_outage.sql")
	sqlListOpenOutages         = mustSQL("sql/queries/list_open_outages.sql")
	sqlInsertMaintenanceEvent  = mustSQL("sql/queries/insert_maintenance_event.sql")
	sqlListMaintenanceEvents   = mustSQL("sql/queries/list_maintenance_events.sql")
)

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
			if _, err := tx.Exec(sqlInsertInverter,
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

// migrateV5 backfills readings_hourly and readings_daily from the raw readings table.
func migrateV5(tx *sql.Tx) error {
	if _, err := tx.Exec(sqlBackfillHourly); err != nil {
		return err
	}
	_, err := tx.Exec(sqlBackfillDaily)
	return err
}

// dataMigrations holds optional data-migration functions keyed by migration index (0-based).
var dataMigrations = map[int]func(*sql.Tx) error{
	1: migrateV2,
	2: migrateV3,
	3: migrateV4,
	4: migrateV5,
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
		slog.Default().Info("applied database migration", "version", i+1)
	}
	return nil
}

func (s *Store) SaveReading(ctx context.Context, r *pvs.Reading) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	ts := r.ReceivedAt.Unix()
	if _, err := tx.ExecContext(ctx, sqlInsertReading,
		ts, r.Time.Unix(),
		r.SolarKW, r.LoadKW, r.NetKW,
		r.SolarKWh, r.LoadKWh, r.NetKWh,
	); err != nil {
		return err
	}
	hourBucket := (ts / 3600) * 3600
	if _, err := tx.ExecContext(ctx, sqlUpsertHourly,
		hourBucket, r.SolarKW, r.LoadKW, r.NetKW, r.SolarKWh, r.SolarKWh, r.LoadKWh, r.LoadKWh,
	); err != nil {
		return err
	}
	dayBucket := (ts / 86400) * 86400
	if _, err := tx.ExecContext(ctx, sqlUpsertDaily,
		dayBucket, r.SolarKW, r.LoadKW, r.NetKW, r.SolarKWh, r.SolarKWh, r.LoadKWh, r.LoadKWh,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LatestReading(ctx context.Context) (*pvs.Reading, error) {
	row := s.db.QueryRowContext(ctx, sqlLatestReading)
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
	row := s.db.QueryRowContext(ctx, sqlEarliestReading)
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
	if until.Sub(since) > 48*time.Hour {
		sinceUnix := since.Unix()
		sinceHour := (sinceUnix / 3600) * 3600
		row := s.db.QueryRowContext(ctx, sqlAveragePowerRollup, sinceHour, until.Unix())
		var solarKW, loadKW, netKW sql.NullFloat64
		var samples sql.NullInt64
		if err := row.Scan(&solarKW, &loadKW, &netKW, &samples); err != nil {
			return pvs.PowerAvg{}, fmt.Errorf("query average rollup: %w", err)
		}
		return pvs.PowerAvg{
			SolarKW: solarKW.Float64,
			LoadKW:  loadKW.Float64,
			NetKW:   netKW.Float64,
			Samples: int(samples.Int64),
		}, nil
	}
	row := s.db.QueryRowContext(ctx, sqlAveragePower, since.Unix(), until.Unix())
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
	if until.Sub(since) > 48*time.Hour {
		sinceUnix := since.Unix()
		sinceHour := (sinceUnix / 3600) * 3600
		row := s.db.QueryRowContext(ctx, sqlEnergyDeltaRollup, sinceHour, until.Unix())
		var solar, load, net float64
		if err := row.Scan(&solar, &load, &net); err != nil {
			return pvs.EnergyDelta{}, fmt.Errorf("query energy delta rollup: %w", err)
		}
		return pvs.EnergyDelta{
			SolarKWh: solar,
			LoadKWh:  load,
			NetKWh:   net,
		}, nil
	}
	row := s.db.QueryRowContext(ctx, sqlEnergyDelta, since.Unix(), until.Unix())
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
			if _, err := tx.ExecContext(ctx, sqlInsertInverter,
				receivedAt.Unix(), inv.Serial, inv.State, inv.StateDescr,
				inv.PowerKW, inv.LifetimeKWh, inv.VoltageV, inv.CurrentA,
				inv.PowerMPPT1KW, inv.VoltageMPPT1V, inv.CurrentMPPT1A,
				inv.TempC, inv.FreqHz,
			); err != nil {
				return err
			}
		default:
			if _, err := tx.ExecContext(ctx, sqlInsertAuxDevice,
				receivedAt.Unix(), d.DeviceType, d.Serial, string(d.Raw),
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) LatestInverters(ctx context.Context) ([]pvs.InverterDevice, error) {
	midnight := time.Now().Truncate(24 * time.Hour).Unix()
	rows, err := s.db.QueryContext(ctx, sqlLatestInverters, midnight)
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
			&d.TempC, &d.FreqHz, &d.TodayKWh,
		); err != nil {
			return nil, fmt.Errorf("scan inverter: %w", err)
		}
		d.ReceivedAt = time.Unix(receivedAt, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) LatestAuxDevices(ctx context.Context) ([]pvs.AuxDevice, error) {
	rows, err := s.db.QueryContext(ctx, sqlLatestAuxDevices)
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
	sinceUnix := since.Unix()
	var rows *sql.Rows
	var err error
	switch bucketSeconds {
	case 3600:
		sinceHour := (sinceUnix / 3600) * 3600
		rows, err = s.db.QueryContext(ctx, sqlSeriesHourly, sinceHour, until.Unix())
	case 21600:
		sinceHour := (sinceUnix / 3600) * 3600
		rows, err = s.db.QueryContext(ctx, sqlSeriesHourly6h, sinceHour, until.Unix())
	case 86400:
		sinceDay := (sinceUnix / 86400) * 86400
		rows, err = s.db.QueryContext(ctx, sqlSeriesDaily, sinceDay, until.Unix())
	default:
		rows, err = s.db.QueryContext(ctx, sqlSeriesRaw,
			bucketSeconds, bucketSeconds, sinceUnix, until.Unix(),
		)
	}
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
	err := s.db.QueryRowContext(ctx, sqlCountReadings).Scan(&count)
	return count, err
}

// OpenInverterOutage records that serial entered error state at at.
// If an open outage already exists for this serial (e.g. after a service restart),
// this is a no-op so we don't create duplicate records.
func (s *Store) OpenInverterOutage(ctx context.Context, serial string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, sqlOpenOutage, serial, at.Unix(), serial)
	return err
}

// CloseInverterOutage records that serial returned to a healthy state at at.
func (s *Store) CloseInverterOutage(ctx context.Context, serial string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, sqlCloseOutage, at.Unix(), serial)
	return err
}

func (s *Store) ListOpenInverterOutages(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, sqlListOpenOutages)
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

func (s *Store) SaveMaintenanceEvent(ctx context.Context, e pvs.MaintenanceEvent) (int64, error) {
	var endAt any
	if !e.EndAt.IsZero() {
		endAt = e.EndAt.Unix()
	}
	res, err := s.db.ExecContext(ctx, sqlInsertMaintenanceEvent,
		e.StartAt.Unix(), endAt, e.EventType, e.Notes, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("insert maintenance event: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) ListMaintenanceEvents(ctx context.Context) ([]pvs.MaintenanceEvent, error) {
	rows, err := s.db.QueryContext(ctx, sqlListMaintenanceEvents)
	if err != nil {
		return nil, fmt.Errorf("list maintenance events: %w", err)
	}
	defer rows.Close()

	var events []pvs.MaintenanceEvent
	for rows.Next() {
		var e pvs.MaintenanceEvent
		var startAt int64
		var endAt sql.NullInt64
		var createdAt int64
		if err := rows.Scan(&e.ID, &startAt, &endAt, &e.EventType, &e.Notes, &createdAt); err != nil {
			return nil, fmt.Errorf("scan maintenance event: %w", err)
		}
		e.StartAt = time.Unix(startAt, 0)
		if endAt.Valid {
			e.EndAt = time.Unix(endAt.Int64, 0)
		}
		e.CreatedAt = time.Unix(createdAt, 0)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) Checkpoint(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
