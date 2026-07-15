package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/dangogh/pvs-monitoring/pvs"
)

// fakeStore is a configurable pvs.Store for handler tests.
type fakeStore struct {
	reading      *pvs.Reading
	readingErr   error
	energy       pvs.EnergyDelta
	energyErr    error
	avg          pvs.PowerAvg
	avgErr       error
	series       []pvs.SeriesPoint
	seriesErr    error
	inverters    []pvs.InverterDevice
	invertersErr error
}

func (f *fakeStore) SaveReading(_ context.Context, _ *pvs.Reading) error { return nil }
func (f *fakeStore) LatestReading(_ context.Context) (*pvs.Reading, error) {
	return f.reading, f.readingErr
}
func (f *fakeStore) AveragePower(_ context.Context, _, _ time.Time) (pvs.PowerAvg, error) {
	return f.avg, f.avgErr
}
func (f *fakeStore) EnergyDelta(_ context.Context, _, _ time.Time) (pvs.EnergyDelta, error) {
	return f.energy, f.energyErr
}
func (f *fakeStore) ReadingsSeries(_ context.Context, _, _ time.Time, _ int64) ([]pvs.SeriesPoint, error) {
	return f.series, f.seriesErr
}
func (f *fakeStore) CountReadings(_ context.Context) (int64, error)                   { return 0, nil }
func (f *fakeStore) EarliestReadingAt(_ context.Context) (time.Time, error)           { return time.Time{}, nil }
func (f *fakeStore) SaveDevices(_ context.Context, _ []pvs.Device, _ time.Time) error { return nil }
func (f *fakeStore) LatestInverters(_ context.Context) ([]pvs.InverterDevice, error) {
	return f.inverters, f.invertersErr
}
func (f *fakeStore) LatestAuxDevices(_ context.Context) ([]pvs.AuxDevice, error)        { return nil, nil }
func (f *fakeStore) OpenInverterOutage(_ context.Context, _ string, _ time.Time) error  { return nil }
func (f *fakeStore) CloseInverterOutage(_ context.Context, _ string, _ time.Time) error { return nil }
func (f *fakeStore) ListOpenInverterOutages(_ context.Context) ([]string, error)                      { return nil, nil }
func (f *fakeStore) SaveMaintenanceEvent(_ context.Context, _ pvs.MaintenanceEvent) (int64, error)   { return 0, nil }
func (f *fakeStore) ListMaintenanceEvents(_ context.Context) ([]pvs.MaintenanceEvent, error)         { return nil, nil }
func (f *fakeStore) Close() error                                                                    { return nil }

func newServer(store pvs.Store) *apiServer {
	return &apiServer{store: store}
}

// --- handleCurrent ---

func TestHandleCurrent_OK(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &fakeStore{reading: &pvs.Reading{SolarKW: 3.5, LoadKW: 1.2, NetKW: -2.3, ReceivedAt: now}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/current", nil)
	newServer(store).handleCurrent(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var got currentReading
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SolarKW != 3.5 || got.LoadKW != 1.2 || got.NetKW != -2.3 {
		t.Errorf("unexpected values: %+v", got)
	}
}

func TestHandleCurrent_NoData(t *testing.T) {
	w := httptest.NewRecorder()
	newServer(&fakeStore{}).handleCurrent(w, httptest.NewRequest(http.MethodGet, "/api/current", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

func TestHandleCurrent_StoreError(t *testing.T) {
	store := &fakeStore{readingErr: errors.New("db down")}
	w := httptest.NewRecorder()
	newServer(store).handleCurrent(w, httptest.NewRequest(http.MethodGet, "/api/current", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// --- handleData ---

func TestHandleData_OK(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	store := &fakeStore{
		reading: &pvs.Reading{SolarKW: 1, LoadKW: 2, NetKW: -1, ReceivedAt: now},
		energy:  pvs.EnergyDelta{SolarKWh: 10, LoadKWh: 5, NetKWh: -5},
		avg:     pvs.PowerAvg{SolarKW: 2, LoadKW: 1},
		series: []pvs.SeriesPoint{
			{Time: since, SolarKW: 1.5, LoadKW: 0.8},
		},
	}
	url := "/api/data?since=" + itoa(since.Unix()) + "&until=" + itoa(now.Unix())
	w := httptest.NewRecorder()
	newServer(store).handleData(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var got dataResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Current == nil {
		t.Fatal("expected current to be set")
	}
	if len(got.Series) != 1 {
		t.Errorf("want 1 series point, got %d", len(got.Series))
	}
}

func TestHandleData_NoCurrentReading(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	store := &fakeStore{}
	url := "/api/data?since=" + itoa(since.Unix()) + "&until=" + itoa(now.Unix())
	w := httptest.NewRecorder()
	newServer(store).handleData(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var got dataResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Current != nil {
		t.Error("expected current to be nil when no reading")
	}
}

func TestHandleData_MissingParams(t *testing.T) {
	w := httptest.NewRecorder()
	newServer(&fakeStore{}).handleData(w, httptest.NewRequest(http.MethodGet, "/api/data", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleData_InvalidSince(t *testing.T) {
	w := httptest.NewRecorder()
	newServer(&fakeStore{}).handleData(w, httptest.NewRequest(http.MethodGet, "/api/data?since=bad&until=1000", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleData_InvalidUntil(t *testing.T) {
	w := httptest.NewRecorder()
	newServer(&fakeStore{}).handleData(w, httptest.NewRequest(http.MethodGet, "/api/data?since=1000&until=bad", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleData_LatestReadingError(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	store := &fakeStore{readingErr: errors.New("fail")}
	url := "/api/data?since=" + itoa(since.Unix()) + "&until=" + itoa(now.Unix())
	w := httptest.NewRecorder()
	newServer(store).handleData(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestHandleData_EnergyError(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	store := &fakeStore{energyErr: errors.New("fail")}
	url := "/api/data?since=" + itoa(since.Unix()) + "&until=" + itoa(now.Unix())
	w := httptest.NewRecorder()
	newServer(store).handleData(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestHandleData_AvgError(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	store := &fakeStore{avgErr: errors.New("fail")}
	url := "/api/data?since=" + itoa(since.Unix()) + "&until=" + itoa(now.Unix())
	w := httptest.NewRecorder()
	newServer(store).handleData(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestHandleData_SeriesError(t *testing.T) {
	now := time.Now()
	since := now.Add(-time.Hour)
	store := &fakeStore{seriesErr: errors.New("fail")}
	url := "/api/data?since=" + itoa(since.Unix()) + "&until=" + itoa(now.Unix())
	w := httptest.NewRecorder()
	newServer(store).handleData(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// --- handleDevices ---

func TestHandleDevices_OK(t *testing.T) {
	store := &fakeStore{inverters: []pvs.InverterDevice{{Serial: "INV001", PowerKW: 1.2}}}
	w := httptest.NewRecorder()
	newServer(store).handleDevices(w, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var got []pvs.InverterDevice
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Serial != "INV001" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestHandleDevices_StoreError(t *testing.T) {
	store := &fakeStore{invertersErr: errors.New("fail")}
	w := httptest.NewRecorder()
	newServer(store).handleDevices(w, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// --- corsMiddleware ---

func TestCORSMiddleware_Options(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for OPTIONS")
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
}

func TestCORSMiddleware_PassThrough(t *testing.T) {
	called := false
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("inner handler not called")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
}

// --- bucketSeconds ---

func TestBucketSeconds(t *testing.T) {
	now := time.Now()
	tests := []struct {
		span time.Duration
		want int64
	}{
		{1 * time.Hour, 300},
		{48 * time.Hour, 300},
		{72 * time.Hour, 3600},
		{14 * 24 * time.Hour, 3600},
		{30 * 24 * time.Hour, 6 * 3600},
		{90 * 24 * time.Hour, 6 * 3600},
		{91 * 24 * time.Hour, 86400},
	}
	for _, tt := range tests {
		got := bucketSeconds(now, now.Add(tt.span))
		if got != tt.want {
			t.Errorf("span %v: want %d, got %d", tt.span, tt.want, got)
		}
	}
}

// --- toSeriesPoints ---

func TestToSeriesPoints(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	pts := []pvs.SeriesPoint{
		{Time: ts, SolarKW: 3.0, LoadKW: 1.5},
	}
	got := toSeriesPoints(pts, 300)
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if got[0].TimeMS != ts.UnixMilli() {
		t.Errorf("TimeMS: want %d, got %d", ts.UnixMilli(), got[0].TimeMS)
	}
	if got[0].SolarKW == nil || *got[0].SolarKW != 3.0 || got[0].LoadKW == nil || *got[0].LoadKW != 1.5 {
		t.Errorf("unexpected values: %+v", got[0])
	}
}

func TestToSeriesPoints_Empty(t *testing.T) {
	got := toSeriesPoints(nil, 300)
	if len(got) != 0 {
		t.Errorf("want empty, got %d", len(got))
	}
}

func TestToSeriesPoints_Gap(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	pts := []pvs.SeriesPoint{
		{Time: ts, SolarKW: 1.0, LoadKW: 0.5},
		{Time: ts.Add(300 * time.Second), SolarKW: 2.0, LoadKW: 1.0}, // exactly 1 bucket gap → no sentinel
		{Time: ts.Add(600 * time.Second), SolarKW: 3.0, LoadKW: 1.5}, // exactly 2 bucket gap → no sentinel (not strictly greater)
	}
	got := toSeriesPoints(pts, 300)
	if len(got) != 3 {
		t.Fatalf("want 3 points (no gap sentinel), got %d", len(got))
	}

	// Now a gap that exceeds 2× bucket (2×300s = 600s): use 700s gap.
	pts2 := []pvs.SeriesPoint{
		{Time: ts, SolarKW: 1.0, LoadKW: 0.5},
		{Time: ts.Add(700 * time.Second), SolarKW: 2.0, LoadKW: 1.0},
	}
	got2 := toSeriesPoints(pts2, 300)
	if len(got2) != 3 {
		t.Fatalf("want 3 points (2 data + 1 gap sentinel), got %d", len(got2))
	}
	if got2[1].SolarKW != nil || got2[1].LoadKW != nil {
		t.Errorf("middle point should be null sentinel, got %+v", got2[1])
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
