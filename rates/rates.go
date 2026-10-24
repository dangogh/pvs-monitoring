// Package rates models time-of-use electricity pricing. It is pure: it knows
// nothing about the database or the network, only how to say which price
// applies at an instant and how to cut a time range into constant-price pieces.
package rates

import (
	"fmt"
	"time"
)

// Band is a run of constant price that starts at StartHour (local, 0-23) and
// lasts until the next band's StartHour, or midnight for the last band.
type Band struct {
	StartHour int
	Period    string
	// PerKWh is the price in dollars, before any baseline credit.
	PerKWh float64
}

// Season applies from FirstMonth through LastMonth inclusive. A season may wrap
// the new year (FirstMonth > LastMonth), as winter does.
type Season struct {
	Name       string
	FirstMonth time.Month
	LastMonth  time.Month
	// Weekday and Weekend must each start at hour 0 with ascending StartHours.
	Weekday []Band
	Weekend []Band
}

// Plan is a complete rate schedule.
type Plan struct {
	Name string
	// BaseChargePerDay is the fixed daily services charge, in dollars.
	BaseChargePerDay float64
	// BaselineCreditPerKWh is subtracted from the price of energy used up to the
	// monthly baseline allocation. It is informational: Cost does not apply it,
	// because the allocation depends on the customer's territory and season.
	BaselineCreditPerKWh float64
	Seasons              []Season
}

// Segment is a stretch of time during which one price applies.
type Segment struct {
	Start, End time.Time
	Season     string
	Period     string
	PerKWh     float64
}

// Validate checks the structure Segments relies on.
func (p Plan) Validate() error {
	if len(p.Seasons) == 0 {
		return fmt.Errorf("plan %q has no seasons", p.Name)
	}
	var covered [13]int
	for _, s := range p.Seasons {
		for _, bands := range [][]Band{s.Weekday, s.Weekend} {
			if len(bands) == 0 || bands[0].StartHour != 0 {
				return fmt.Errorf("season %q: bands must start at hour 0", s.Name)
			}
			for i := 1; i < len(bands); i++ {
				if bands[i].StartHour <= bands[i-1].StartHour || bands[i].StartHour > 23 {
					return fmt.Errorf("season %q: band start hours must ascend within 0-23", s.Name)
				}
			}
		}
		for m := time.January; m <= time.December; m++ {
			if s.contains(m) {
				covered[m]++
			}
		}
	}
	for m := time.January; m <= time.December; m++ {
		if covered[m] != 1 {
			return fmt.Errorf("plan %q: %s is covered by %d seasons, want 1", p.Name, m, covered[m])
		}
	}
	return nil
}

func (s Season) contains(m time.Month) bool {
	if s.FirstMonth <= s.LastMonth {
		return m >= s.FirstMonth && m <= s.LastMonth
	}
	return m >= s.FirstMonth || m <= s.LastMonth
}

// bandsFor returns the season and band list in force on the local calendar day.
func (p Plan) bandsFor(day time.Time) (Season, []Band) {
	for _, s := range p.Seasons {
		if s.contains(day.Month()) {
			if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
				return s, s.Weekend
			}
			return s, s.Weekday
		}
	}
	return Season{}, nil
}

// Segments splits [since, until) into pieces of constant price, evaluated in
// loc. Band boundaries are built with time.Date rather than by adding hours, so
// a daylight-saving change shifts the wall clock the way the tariff means it.
func (p Plan) Segments(since, until time.Time, loc *time.Location) []Segment {
	var out []Segment
	for cur := since; cur.Before(until); {
		local := cur.In(loc)
		season, bands := p.bandsFor(local)
		if len(bands) == 0 {
			return out
		}
		i := 0
		for i+1 < len(bands) && bands[i+1].StartHour <= local.Hour() {
			i++
		}
		var next time.Time
		if i+1 < len(bands) {
			next = time.Date(local.Year(), local.Month(), local.Day(), bands[i+1].StartHour, 0, 0, 0, loc)
		} else {
			next = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
		}
		if next.After(until) {
			next = until
		}
		out = append(out, Segment{
			Start: cur, End: next,
			Season: season.Name, Period: bands[i].Period, PerKWh: bands[i].PerKWh,
		})
		cur = next
	}
	return out
}

// TOUD49PM is SCE's residential TOU-D-4-9PM plan, bundled (delivery plus SCE
// generation) rates as published on sce.com. Customers on a community choice
// provider pay different generation rates. Holidays are not modelled; they are
// treated as the weekday or weekend they fall on.
func TOUD49PM() Plan {
	return Plan{
		Name:                 "SCE TOU-D-4-9PM",
		BaseChargePerDay:     0.79,
		BaselineCreditPerKWh: 0.09,
		Seasons: []Season{
			{
				Name: "summer", FirstMonth: time.June, LastMonth: time.September,
				Weekday: []Band{
					{0, "off_peak", 0.33},
					{16, "on_peak", 0.59},
					{21, "off_peak", 0.33},
				},
				Weekend: []Band{
					{0, "off_peak", 0.33},
					{16, "mid_peak", 0.44},
					{21, "off_peak", 0.33},
				},
			},
			{
				Name: "winter", FirstMonth: time.October, LastMonth: time.May,
				Weekday: winterBands(),
				Weekend: winterBands(),
			},
		},
	}
}

func winterBands() []Band {
	return []Band{
		{0, "off_peak", 0.36},
		{8, "super_off_peak", 0.32},
		{16, "mid_peak", 0.50},
		{21, "off_peak", 0.36},
	}
}
