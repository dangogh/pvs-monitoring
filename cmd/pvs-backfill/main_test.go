package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/dangogh/pvs-monitoring/pvs"
	"github.com/dangogh/pvs-monitoring/store/sqlite"
)

// openRaw opens the database file directly. Tests need to write aux payloads and
// count rows, neither of which the Store exposes — and adding an accessor to
// production code purely for tests would be the wrong trade.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seed builds a database with one recoverable gap: readings either side, meter
// payloads throughout the hole.
func seed(t *testing.T) (string, time.Time, time.Time) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := sqlite.Open(path)
	require.NoError(t, err)
	defer s.Close() //nolint:errcheck

	ctx := t.Context()
	base := time.Unix(1790000000, 0).Truncate(time.Hour)
	for _, at := range []time.Time{base, base.Add(time.Hour)} {
		require.NoError(t, s.SaveReading(ctx, &pvs.Reading{
			ReceivedAt: at, Time: at, SolarKW: 1, LoadKW: 1, SolarKWh: 1000, LoadKWh: 500,
		}))
	}
	db := openRaw(t, path)
	for i := 1; i < 60; i++ {
		at := base.Add(time.Duration(i) * time.Minute).Unix()
		for _, m := range []struct {
			typ     string
			kw, kwh float64
		}{{"PVS5-METER-P", 8, 1000}, {"PVS5-METER-C", -3, -400}} {
			_, err := db.ExecContext(ctx,
				`INSERT INTO aux_device_readings (received_at, device_type, serial, payload) VALUES (?,?,?,?)`,
				at, "Power Meter", "M"+m.typ,
				fmt.Sprintf(`{"TYPE":%q,"p_3phsum_kw":"%f","net_ltea_3phsum_kwh":"%f"}`, m.typ, m.kw, m.kwh))
			require.NoError(t, err)
		}
	}
	return path, base, base.Add(time.Hour)
}

func countRows(t *testing.T, path, where string) int {
	t.Helper()
	var n int
	require.NoError(t, openRaw(t, path).QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM readings WHERE "+where).Scan(&n))
	return n
}

func TestRunDryRunWritesNothing(t *testing.T) {
	path, _, _ := seed(t)
	before := countRows(t, path, "1=1")

	require.NoError(t, run(path, 10*time.Minute, "", "", false))

	assert.Equal(t, before, countRows(t, path, "1=1"), "dry run must not write")
	assert.Zero(t, countRows(t, path, "source IS NOT NULL"))
}

func TestRunApplyFillsGapAndTagsSource(t *testing.T) {
	path, _, _ := seed(t)

	require.NoError(t, run(path, 10*time.Minute, "", "", true))

	assert.Equal(t, 59, countRows(t, path, "source = 'meter-1min'"),
		"every meter sample in the gap should be reconstructed and tagged")
	assert.Equal(t, 2, countRows(t, path, "source IS NULL"),
		"live rows must be left alone")
}

func TestRunSkipsGapWithoutMeterCoverage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := sqlite.Open(path)
	require.NoError(t, err)
	ctx := t.Context()
	base := time.Unix(1790000000, 0).Truncate(time.Hour)
	for _, at := range []time.Time{base, base.Add(time.Hour)} {
		require.NoError(t, s.SaveReading(ctx, &pvs.Reading{
			ReceivedAt: at, Time: at, SolarKW: 1, LoadKW: 1, SolarKWh: 1, LoadKWh: 1,
		}))
	}
	require.NoError(t, s.Close())

	require.NoError(t, run(path, 10*time.Minute, "", "", true))
	assert.Zero(t, countRows(t, path, "source IS NOT NULL"),
		"a gap with no meter samples is unrecoverable and must be skipped, not invented")
}

func TestRunRangeNarrowingExcludesGap(t *testing.T) {
	path, _, end := seed(t)
	// Ask only for gaps starting after the one we have.
	require.NoError(t, run(path, 10*time.Minute, end.Add(time.Hour).Format(time.RFC3339), "", true))
	assert.Zero(t, countRows(t, path, "source IS NOT NULL"))
}

func TestRunRejectsInvertedRange(t *testing.T) {
	path, start, end := seed(t)
	err := run(path, 10*time.Minute, end.Format(time.RFC3339), start.Format(time.RFC3339), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is before")
}
