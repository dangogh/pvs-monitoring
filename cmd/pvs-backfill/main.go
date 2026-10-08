// Command pvs-backfill repairs gaps in the 1 Hz readings stream from the
// 1/min Power Meter payloads the device poller recorded alongside it.
//
// A gap is only recoverable when the PVS6 stayed reachable while the WebSocket
// power stream was dead — a telemetry stall, as on 2026-09-22, when 58 hours of
// readings went missing while both meters kept logging. When the PVS6 itself was
// off nothing was recorded anywhere and the gap is permanent; those are reported
// and skipped.
//
// Reconstructed rows are tagged source='meter-1min' so they are never mistaken
// for live 1 Hz samples. Energy totals from them are sound; instantaneous load
// detail is approximate, because the meters sample at different instants than
// the power stream did and household load moves fast.
//
// Nothing is written without -apply. The default run reports what it would do.
//
// With -apply, each gap is filled in a single transaction — rows and rollup
// rebuild together — so a failure leaves the database exactly as it was. A
// consistent VACUUM INTO backup is taken first unless -no-backup is given, and
// only when there is actually something to write.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dangogh/pvs-monitoring/internal/version"
	"github.com/dangogh/pvs-monitoring/pvs"
	"github.com/dangogh/pvs-monitoring/store/sqlite"
)

const backfillSource = "meter-1min"

func main() {
	dbPath := flag.String("db", "/var/lib/pvs-monitor/readings.db", "path to the readings database")
	minGap := flag.Duration("min-gap", 10*time.Minute, "ignore gaps shorter than this")
	from := flag.String("from", "", "only consider gaps starting at or after this time (RFC3339)")
	to := flag.String("to", "", "only consider gaps ending at or before this time (RFC3339)")
	apply := flag.Bool("apply", false, "actually write; without this the run only reports")
	backup := flag.String("backup", "", "write a consistent copy here before touching the database (default: /var/tmp/<db>-backup-<timestamp>.db)")
	noBackup := flag.Bool("no-backup", false, "skip the pre-write backup")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	if err := run(*dbPath, *minGap, *from, *to, *apply, *backup, *noBackup); err != nil {
		log.Fatalf("pvs-backfill: %v", err)
	}
}

func run(dbPath string, minGap time.Duration, from, to string, apply bool, backupPath string, noBackup bool) error {
	lo, err := parseBound(from)
	if err != nil {
		return fmt.Errorf("-from: %w", err)
	}
	hi, err := parseBound(to)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	if !lo.IsZero() && !hi.IsZero() && hi.Before(lo) {
		return fmt.Errorf("-to (%s) is before -from (%s)", hi, lo)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()
	gaps, err := store.ReadingGaps(ctx, minGap)
	if err != nil {
		return err
	}

	var selected []sqlite.ReadingGap
	for _, g := range gaps {
		if !lo.IsZero() && g.Start.Before(lo) {
			continue
		}
		if !hi.IsZero() && g.End.After(hi) {
			continue
		}
		selected = append(selected, g)
	}

	if len(selected) == 0 {
		fmt.Printf("no gaps longer than %s in range\n", minGap)
		return nil
	}

	if !apply {
		fmt.Println("DRY RUN — nothing will be written. Re-run with -apply to commit.")
	}

	// Back up before the first write, not at startup: a dry run and a run that
	// finds only unrecoverable gaps should not leave a 2 GB file behind.
	fillable := 0
	for _, g := range selected {
		if g.Recoverable() {
			fillable++
		}
	}
	if apply && fillable > 0 && !noBackup {
		if backupPath == "" {
			backupPath = defaultBackupPath(dbPath)
		}
		fmt.Printf("backing up to %s …\n", backupPath)
		if err := store.BackupTo(ctx, backupPath); err != nil {
			return fmt.Errorf("%w (use -no-backup to proceed without one)", err)
		}
		fi, err := os.Stat(backupPath)
		if err != nil {
			return fmt.Errorf("backup reported success but %s is missing: %w", backupPath, err)
		}
		fmt.Printf("      backup written, %.1f GB\n", float64(fi.Size())/(1<<30))
	}

	var totalWritten int
	for _, g := range selected {
		dur := g.End.Sub(g.Start)
		if !g.Recoverable() {
			fmt.Printf("SKIP  %s → %s  (%s)  no meter samples — PVS6 was off, unrecoverable\n",
				ts(g.Start), ts(g.End), dur.Round(time.Second))
			continue
		}
		samples, err := store.MeterSamples(ctx, g.Start, g.End)
		if err != nil {
			return err
		}
		expected := int(dur.Minutes())
		fmt.Printf("FILL  %s → %s  (%s)  %d meter samples for ~%d minutes (%.1f%% coverage)\n",
			ts(g.Start), ts(g.End), dur.Round(time.Second), len(samples), expected,
			100*float64(len(samples))/float64(max(expected, 1)))
		if !apply {
			continue
		}
		written, err := fill(ctx, store, g, samples)
		if err != nil {
			return fmt.Errorf("fill %s → %s: %w", ts(g.Start), ts(g.End), err)
		}
		totalWritten += written
		fmt.Printf("      wrote %d rows, rebuilt rollups\n", written)
	}
	if apply {
		fmt.Printf("done — %d rows inserted as source=%q\n", totalWritten, backfillSource)
	}
	return nil
}

// fill writes one gap atomically: the reconstructed rows and the rollup rebuild
// land in a single transaction, so a failure anywhere leaves the database
// exactly as it was rather than holding rows whose rollups were never redone.
func fill(ctx context.Context, store *sqlite.Store, g sqlite.ReadingGap, samples []*pvs.Reading) (int, error) {
	if err := store.BackfillRange(ctx, g.Start, g.End, samples, backfillSource); err != nil {
		return 0, err
	}
	return len(samples), nil
}

// defaultBackupPath puts the copy on real disk rather than beside the database.
// /var/tmp is deliberate: /tmp on the monitoring host is a tmpfs, so a 2 GB
// snapshot there is 2 GB of RAM.
func defaultBackupPath(dbPath string) string {
	base := strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
	return filepath.Join("/var/tmp", fmt.Sprintf("%s-backup-%s.db", base, time.Now().Format("20060102-150405")))
}

func parseBound(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func ts(t time.Time) string { return t.Local().Format("2006-01-02 15:04:05") }
