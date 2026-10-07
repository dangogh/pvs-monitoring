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
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	if err := run(*dbPath, *minGap, *from, *to, *apply); err != nil {
		log.Fatalf("pvs-backfill: %v", err)
	}
}

func run(dbPath string, minGap time.Duration, from, to string, apply bool) error {
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

// fill inserts the reconstructed rows then rebuilds the rollup buckets the gap
// touches. The rows go in first and the rollups are recomputed from scratch
// afterwards, because a bucket mixing 1 Hz and 1/min rows cannot be corrected
// by the incremental upserts SaveReading uses.
func fill(ctx context.Context, store *sqlite.Store, g sqlite.ReadingGap, samples []*pvs.Reading) (int, error) {
	for _, r := range samples {
		if err := store.SaveBackfilledReading(ctx, r, backfillSource); err != nil {
			return 0, err
		}
	}
	if err := store.RebuildRollups(ctx, g.Start, g.End); err != nil {
		return len(samples), fmt.Errorf("rebuild rollups: %w", err)
	}
	return len(samples), nil
}

func parseBound(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func ts(t time.Time) string { return t.Local().Format("2006-01-02 15:04:05") }
