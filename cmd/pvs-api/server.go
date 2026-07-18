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
	mux.HandleFunc("GET /api/maintenance-events", s.handleMaintenanceEvents)
	mux.HandleFunc("POST /api/maintenance-events", s.handleCreateMaintenanceEvent)
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
	Since      time.Time       `json:"since"`
	Until      time.Time       `json:"until"`
	EarliestAt *time.Time      `json:"earliest_at,omitempty"`
	Current    *currentReading `json:"current"`
	Summary    summaryData     `json:"summary"`
	Series     []seriesPoint   `json:"series"`
}

type summaryData struct {
	SolarKWh   float64 `json:"solar_kwh"`
	LoadKWh    float64 `json:"load_kwh"`
	NetKWh     float64 `json:"net_kwh"`
	AvgSolarKW float64 `json:"avg_solar_kw"`
	AvgLoadKW  float64 `json:"avg_load_kw"`
}

// seriesPoint uses compact keys to minimise JSON payload size.
// SolarKW and LoadKW are pointers so gap sentinel points serialize as null,
// which causes Highcharts to break the line instead of connecting across the gap.
type seriesPoint struct {
	TimeMS  int64    `json:"t"` // milliseconds — Highcharts datetime axis format
	SolarKW *float64 `json:"s"`
	LoadKW  *float64 `json:"l"`
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

	earliestAt, err := s.store.EarliestReadingAt(r.Context())
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
		Series: toSeriesPoints(pts, bucket),
	}
	if !earliestAt.IsZero() {
		resp.EarliestAt = &earliestAt
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

func toSeriesPoints(pts []pvs.SeriesPoint, bucketSeconds int64) []seriesPoint {
	if len(pts) == 0 {
		return nil
	}
	gap := time.Duration(bucketSeconds*2) * time.Second
	out := make([]seriesPoint, 0, len(pts))
	for i, p := range pts {
		if i > 0 && p.Time.Sub(pts[i-1].Time) > gap {
			// Insert a null sentinel so Highcharts breaks the line across the gap.
			mid := pts[i-1].Time.Add(p.Time.Sub(pts[i-1].Time) / 2)
			out = append(out, seriesPoint{TimeMS: mid.UnixMilli()})
		}
		s, l := p.SolarKW, p.LoadKW
		out = append(out, seriesPoint{TimeMS: p.Time.UnixMilli(), SolarKW: &s, LoadKW: &l})
	}
	return out
}

type maintenanceEventResponse struct {
	ID        int64      `json:"id"`
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	EventType string     `json:"event_type"`
	Notes     string     `json:"notes,omitempty"`
}

func (s *apiServer) handleMaintenanceEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListMaintenanceEvents(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := make([]maintenanceEventResponse, len(events))
	for i, e := range events {
		resp[i] = maintenanceEventResponse{
			ID:        e.ID,
			StartAt:   e.StartAt,
			EndAt:     optionalTime(e.EndAt),
			EventType: e.EventType,
			Notes:     e.Notes,
		}
	}
	writeJSON(w, resp)
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

type createMaintenanceEventRequest struct {
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	EventType string     `json:"event_type"`
	Notes     string     `json:"notes,omitempty"`
}

func (s *apiServer) handleCreateMaintenanceEvent(w http.ResponseWriter, r *http.Request) {
	var req createMaintenanceEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.StartAt.IsZero() {
		http.Error(w, "start_at is required", http.StatusBadRequest)
		return
	}
	if req.EventType == "" {
		http.Error(w, "event_type is required", http.StatusBadRequest)
		return
	}
	event := pvs.MaintenanceEvent{
		StartAt:   req.StartAt,
		EventType: req.EventType,
		Notes:     req.Notes,
	}
	if req.EndAt != nil {
		event.EndAt = *req.EndAt
	}
	id, err := s.store.SaveMaintenanceEvent(r.Context(), event)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, maintenanceEventResponse{
		ID:        id,
		StartAt:   event.StartAt,
		EndAt:     optionalTime(event.EndAt),
		EventType: event.EventType,
		Notes:     event.Notes,
	})
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
