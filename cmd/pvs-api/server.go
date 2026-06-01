package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
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
	mux.HandleFunc("GET /api/devices", s.handleDevices)
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
	Since   time.Time       `json:"since"`
	Until   time.Time       `json:"until"`
	Current *currentReading `json:"current"`
	Summary summaryData     `json:"summary"`
	Series  []seriesPoint   `json:"series"`
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
	since, until, err := parseTimeRange(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
		Since: since,
		Until: until,
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

// parseTimeRange reads since and until as Unix timestamps (seconds) from the query string.
func parseTimeRange(r *http.Request) (since, until time.Time, err error) {
	sinceStr := r.URL.Query().Get("since")
	untilStr := r.URL.Query().Get("until")
	if sinceStr == "" || untilStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("since and until are required (Unix seconds)")
	}
	sinceUnix, err := strconv.ParseInt(sinceStr, 10, 64)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid since: %w", err)
	}
	untilUnix, err := strconv.ParseInt(untilStr, 10, 64)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid until: %w", err)
	}
	return time.Unix(sinceUnix, 0), time.Unix(untilUnix, 0), nil
}

func toSeriesPoints(pts []pvs.SeriesPoint) []seriesPoint {
	out := make([]seriesPoint, len(pts))
	for i, p := range pts {
		out[i] = seriesPoint{TimeMS: p.Time.UnixMilli(), SolarKW: p.SolarKW, LoadKW: p.LoadKW}
	}
	return out
}

func (s *apiServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	inverters, err := s.store.LatestInverters(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, inverters)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
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
