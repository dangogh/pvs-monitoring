package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/dangogh/pvs-monitoring/pvs"
)

type apiServer struct {
	store  pvs.Store
	logger *slog.Logger
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/current", s.handleCurrent)
	mux.HandleFunc("GET /api/data", s.handleData)
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type currentReading struct {
	SolarKW   float64   `json:"solar_kw"`
	LoadKW    float64   `json:"load_kw"`
	NetKW     float64   `json:"net_kw"`
	UpdatedAt time.Time `json:"updated_at"`
}

type dataResponse struct {
	Range   rangeInfo       `json:"range"`
	Current *currentReading `json:"current"`
	Summary summaryData     `json:"summary"`
	Series  []seriesPoint   `json:"series"`
}

type rangeInfo struct {
	Name  string    `json:"name"`
	Label string    `json:"label"`
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

type summaryData struct {
	SolarKWh   float64 `json:"solar_kwh"`
	LoadKWh    float64 `json:"load_kwh"`
	NetKWh     float64 `json:"net_kwh"`
	AvgSolarKW float64 `json:"avg_solar_kw"`
	AvgLoadKW  float64 `json:"avg_load_kw"`
}

// seriesPoint uses compact keys to minimise JSON payload size.
type seriesPoint struct {
	TimeMS  int64   `json:"t"` // milliseconds — Highcharts datetime axis format
	SolarKW float64 `json:"s"`
	LoadKW  float64 `json:"l"`
}

func (s *apiServer) handleCurrent(w http.ResponseWriter, r *http.Request) {
	reading, err := s.store.LatestReading(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reading == nil {
		http.Error(w, "no data", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, currentReading{
		SolarKW:   reading.SolarKW,
		LoadKW:    reading.LoadKW,
		NetKW:     reading.NetKW,
		UpdatedAt: reading.ReceivedAt,
	})
}

func (s *apiServer) handleData(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "today"
	}

	since, until, label := parseRange(rangeName, time.Now())
	bucket := bucketSeconds(since, until)

	reading, err := s.store.LatestReading(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	energy, err := s.store.EnergyDelta(r.Context(), since, until)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	avg, err := s.store.AveragePower(r.Context(), since, until)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pts, err := s.store.ReadingsSeries(r.Context(), since, until, bucket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := dataResponse{
		Range: rangeInfo{
			Name:  rangeName,
			Label: label,
			Since: since,
			Until: until,
		},
		Summary: summaryData{
			SolarKWh:   energy.SolarKWh,
			LoadKWh:    energy.LoadKWh,
			NetKWh:     energy.NetKWh,
			AvgSolarKW: avg.SolarKW,
			AvgLoadKW:  avg.LoadKW,
		},
		Series: toSeriesPoints(pts),
	}
	if reading != nil {
		cr := currentReading{
			SolarKW:   reading.SolarKW,
			LoadKW:    reading.LoadKW,
			NetKW:     reading.NetKW,
			UpdatedAt: reading.ReceivedAt,
		}
		resp.Current = &cr
	}

	writeJSON(w, resp)
}

func toSeriesPoints(pts []pvs.SeriesPoint) []seriesPoint {
	out := make([]seriesPoint, len(pts))
	for i, p := range pts {
		out[i] = seriesPoint{TimeMS: p.Time.UnixMilli(), SolarKW: p.SolarKW, LoadKW: p.LoadKW}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parseRange(name string, now time.Time) (since, until time.Time, label string) {
	until = now
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)

	switch name {
	case "today":
		return today, until, "Today"
	case "this_week":
		return today.AddDate(0, 0, -int(now.Weekday())), until, "This Week"
	case "this_month":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc), until, "This Month"
	case "this_year":
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc), until, "This Year"
	case "past_24h":
		return now.Add(-24 * time.Hour), until, "Past 24 Hours"
	case "past_7d":
		return now.AddDate(0, 0, -7), until, "Past 7 Days"
	case "past_30d":
		return now.AddDate(0, 0, -30), until, "Past 30 Days"
	case "past_year":
		return now.AddDate(-1, 0, 0), until, "Past Year"
	case "lifetime":
		return time.Unix(0, 0), until, "Lifetime"
	default:
		return today, until, "Today"
	}
}

func bucketSeconds(since, until time.Time) int64 {
	span := until.Sub(since)
	switch {
	case span <= 48*time.Hour:
		return 300
	case span <= 14*24*time.Hour:
		return 3600
	case span <= 90*24*time.Hour:
		return 6 * 3600
	default:
		return 86400
	}
}
