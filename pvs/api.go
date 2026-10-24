package pvs

// Wire types shared by pvs-api (which serves them) and Client (which decodes
// them). They live here rather than in either command so the two cannot drift:
// a field renamed on the server is a compile error in the client.

import "time"

// CurrentReading is the latest instantaneous power reading. UpdatedAt is when
// the monitor received it, which is what staleness is measured against.
type CurrentReading struct {
	SolarKW   float64   `json:"solar_kw"`
	LoadKW    float64   `json:"load_kw"`
	NetKW     float64   `json:"net_kw"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DataResponse covers a time range: energy totals, average power, and an
// optional charting series. EarliestAt is the oldest reading in the database,
// which tells a caller whether a range that came back empty predates the data
// or merely found nothing.
type DataResponse struct {
	Since      time.Time       `json:"since"`
	Until      time.Time       `json:"until"`
	EarliestAt *time.Time      `json:"earliest_at,omitempty"`
	Current    *CurrentReading `json:"current"`
	Summary    SummaryData     `json:"summary"`
	Series     []SeriesJSON    `json:"series"`
}

// SummaryData holds the aggregates for a DataResponse range.
type SummaryData struct {
	SolarKWh   float64 `json:"solar_kwh"`
	LoadKWh    float64 `json:"load_kwh"`
	NetKWh     float64 `json:"net_kwh"`
	AvgSolarKW float64 `json:"avg_solar_kw"`
	AvgLoadKW  float64 `json:"avg_load_kw"`
	// BackfilledSeconds is how much of the range came from rows reconstructed
	// from the 1/min meter payloads instead of the live 1 Hz stream (see
	// cmd/pvs-backfill). Energy figures from such rows are sound; instantaneous
	// power detail is approximate. Omitted when the range is all live data.
	BackfilledSeconds int64 `json:"backfilled_seconds,omitempty"`
}

// SeriesJSON uses compact keys to minimise JSON payload size.
// SolarKW and LoadKW are pointers so gap sentinel points serialize as null,
// which causes Highcharts to break the line instead of connecting across the gap.
type SeriesJSON struct {
	TimeMS  int64    `json:"t"` // milliseconds — Highcharts datetime axis format
	SolarKW *float64 `json:"s"`
	LoadKW  *float64 `json:"l"`
}

// InverterSeriesJSON is one 5-minute bucket of output for a single inverter.
type InverterSeriesJSON struct {
	TimeMS  int64   `json:"t"`
	Serial  string  `json:"serial"`
	PowerKW float64 `json:"p"`
}

// CostLine is the net grid energy and its price within one tariff period,
// summed across every stretch of the range that falls in that period.
type CostLine struct {
	Season string  `json:"season"`
	Period string  `json:"period"`
	PerKWh float64 `json:"per_kwh"`
	// NetKWh is energy drawn from the grid less energy exported in the period.
	// Negative means the period was a net export.
	NetKWh float64 `json:"net_kwh"`
	Cost   float64 `json:"cost"`
}

// CostResponse prices a range under a time-of-use plan.
type CostResponse struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
	Plan  string    `json:"plan"`
	// Timezone is the zone the plan's hours were evaluated in.
	Timezone string     `json:"timezone"`
	Lines    []CostLine `json:"lines"`
	// EnergyCost is the sum of Lines. It nets import against export within each
	// period at that period's retail price, and applies no baseline credit.
	EnergyCost float64 `json:"energy_cost"`
	// BaseCharge is the plan's fixed daily charge prorated over the range.
	BaseCharge float64 `json:"base_charge"`
	TotalCost  float64 `json:"total_cost"`
}
