package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dangogh/pvs-monitoring/config"
	"github.com/dangogh/pvs-monitoring/internal/version"
	"github.com/dangogh/pvs-monitoring/pvs"
)

type apiServer struct {
	store  pvs.Store
	logger *slog.Logger
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /api/current", s.handleCurrent)
	mux.HandleFunc("GET /api/data", s.handleData)
	mux.HandleFunc("GET /api/devices", s.handleDevices)
	mux.HandleFunc("GET /api/panel-health", s.handlePanelHealth)
	mux.HandleFunc("GET /api/inverter-series", s.handleInverterSeries)
	mux.HandleFunc("GET /api/maintenance-events", s.handleMaintenanceEvents)
	mux.HandleFunc("POST /api/maintenance-events", s.handleCreateMaintenanceEvent)
	mux.HandleFunc("PATCH /api/maintenance-events/{id}", s.handleUpdateMaintenanceEvent)
	mux.HandleFunc("DELETE /api/maintenance-events/{id}", s.handleDeleteMaintenanceEvent)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PATCH /api/config", s.handleUpdateConfig)
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *apiServer) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"version": version.Version})
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
	writeJSON(w, pvs.CurrentReading{
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

	resp := pvs.DataResponse{
		Since: since,
		Until: until,
		Summary: pvs.SummaryData{
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
		cr := pvs.CurrentReading{
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

func toSeriesPoints(pts []pvs.SeriesPoint, bucketSeconds int64) []pvs.SeriesJSON {
	if len(pts) == 0 {
		return nil
	}
	gap := time.Duration(bucketSeconds*2) * time.Second
	out := make([]pvs.SeriesJSON, 0, len(pts))
	for i, p := range pts {
		if i > 0 && p.Time.Sub(pts[i-1].Time) > gap {
			// Insert a null sentinel so Highcharts breaks the line across the gap.
			mid := pts[i-1].Time.Add(p.Time.Sub(pts[i-1].Time) / 2)
			out = append(out, pvs.SeriesJSON{TimeMS: mid.UnixMilli()})
		}
		s, l := p.SolarKW, p.LoadKW
		out = append(out, pvs.SeriesJSON{TimeMS: p.Time.UnixMilli(), SolarKW: &s, LoadKW: &l})
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

func (s *apiServer) handleUpdateMaintenanceEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
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
		ID:        id,
		StartAt:   req.StartAt,
		EventType: req.EventType,
		Notes:     req.Notes,
	}
	if req.EndAt != nil {
		event.EndAt = *req.EndAt
	}
	switch err := s.store.UpdateMaintenanceEvent(r.Context(), event); {
	case errors.Is(err, pvs.ErrNotFound):
		http.Error(w, "maintenance event not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, maintenanceEventResponse{
		ID:        event.ID,
		StartAt:   event.StartAt,
		EndAt:     optionalTime(event.EndAt),
		EventType: event.EventType,
		Notes:     event.Notes,
	})
}

func (s *apiServer) handleDeleteMaintenanceEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	switch err := s.store.DeleteMaintenanceEvent(r.Context(), id); {
	case errors.Is(err, pvs.ErrNotFound):
		http.Error(w, "maintenance event not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// passwordMask is returned in place of the stored device-list password so the
// credential is never echoed to the client. A PATCH that sends this sentinel
// back leaves the stored value untouched.
const passwordMask = "********"

// maskConfig hides the device-list password value in a settings map.
func maskConfig(settings map[string]string) map[string]string {
	out := make(map[string]string, len(settings))
	for k, v := range settings {
		if k == config.KeyDeviceListPassword && v != "" {
			v = passwordMask
		}
		out[k] = v
	}
	return out
}

func (s *apiServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.Settings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, maskConfig(settings))
}

// configKeys is the set of settings a client may write via PATCH /api/config.
var configKeys = map[string]bool{
	config.KeyAddr:                     true,
	config.KeyReconnectInitialInterval: true,
	config.KeyReconnectMaxInterval:     true,
	config.KeyStaleThreshold:           true,
	config.KeyDeviceListURL:            true,
	config.KeyDeviceListAuthURL:        true,
	config.KeyDeviceListInterval:       true,
	config.KeyDeviceListUsername:       true,
	config.KeyDeviceListPassword:       true,
	config.KeyDeviceListTLSFingerprint: true,
}

func (s *apiServer) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Validate all keys before writing any, so a bad request is atomic.
	for key := range req {
		if !configKeys[key] {
			http.Error(w, "unknown config key: "+key, http.StatusBadRequest)
			return
		}
	}
	for key, val := range req {
		// The masked sentinel means "unchanged" — don't clobber the stored password.
		if key == config.KeyDeviceListPassword && val == passwordMask {
			continue
		}
		if err := s.store.SetSetting(r.Context(), key, val); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	settings, err := s.store.Settings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, maskConfig(settings))
}

func (s *apiServer) handleInverterSeries(w http.ResponseWriter, r *http.Request) {
	since, until, err := parseTimeRange(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pts, err := s.store.InverterSeries(r.Context(), since, until)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]pvs.InverterSeriesJSON, len(pts))
	for i, p := range pts {
		out[i] = pvs.InverterSeriesJSON{TimeMS: p.Time.UnixMilli(), Serial: p.Serial, PowerKW: p.PowerKW}
	}
	writeJSON(w, out)
}

func (s *apiServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	inverters, err := s.store.LatestInverters(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, inverters)
}

// handlePanelHealth reports whether any inverter has stopped producing while
// its peers continue. The verdict has no memory of previous calls, so a client
// that raises alarms should require the same serials on consecutive polls
// rather than acting on a single degraded response.
func (s *apiServer) handlePanelHealth(w http.ResponseWriter, r *http.Request) {
	inverters, err := s.store.LatestInverters(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, pvs.EvaluatePanelHealth(inverters, time.Now()))
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
