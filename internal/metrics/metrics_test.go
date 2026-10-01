package metrics

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gremlin-in-a-box/internal/reports"
)

func scrape(t *testing.T, store *reports.Store) map[string]float64 {
	t.Helper()
	w := httptest.NewRecorder()
	NewCollector(store).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	values := map[string]float64{}
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		value, err := strconv.ParseFloat(line[i+1:], 64)
		if err != nil {
			t.Fatal(err)
		}
		values[line[:i]] = value
	}
	return values
}
func TestEvidenceAndLatestReport(t *testing.T) {
	store := reports.NewStore(filepath.Join(t.TempDir(), "reports.json"))
	rows := []reports.Report{
		{Attack: "cpu", StartedAt: time.Unix(5, 0), DurationMs: 300, RecoveryMs: 0, RecoveryStatus: "verified", Success: true},
		{Attack: "cpu", StartedAt: time.Unix(2, 0), DurationMs: 100, RecoveryMs: 0, RecoveryStatus: "skipped", Success: true},
		{Attack: "cpu", StartedAt: time.Unix(3, 0), DurationMs: 200, RecoveryMs: 0, Success: true},
		{Attack: "latency", StartedAt: time.Unix(4, 0), DurationMs: 400, RecoveryMs: -1, RecoveryStatus: "unverified", Success: false},
		{Attack: "cpu", StartedAt: time.Unix(1, 0), DurationMs: 201, RecoveryMs: 3, RecoveryStatus: "verified", Success: true},
	}
	for _, row := range rows {
		if err := store.Save(row); err != nil {
			t.Fatal(err)
		}
	}
	values := scrape(t, store)
	expected := map[string]float64{
		"gremlin_attacks_total": 5, "gremlin_attacks_failed_total": 1, "gremlin_success_rate_percent": 80,
		"gremlin_recovery_verified_total": 2, "gremlin_recovery_unverified_total": 3,
		"gremlin_recovery_skipped_total": 1, "gremlin_recovery_unknown_total": 1,
		"gremlin_recovery_coverage_percent": 40, "gremlin_last_recovery_ms": 0,
		"gremlin_last_report_timestamp_seconds": 5, "gremlin_last_duration_ms": 300,
		`gremlin_avg_recovery_ms_by_type{attack="cpu"}`: 1.5,
		`gremlin_avg_duration_ms_by_type{attack="cpu"}`: 200.25,
	}
	for key, want := range expected {
		if values[key] != want {
			t.Errorf("%s=%v want %v", key, values[key], want)
		}
	}
	if !math.IsNaN(values[`gremlin_avg_recovery_ms_by_type{attack="latency"}`]) {
		t.Fatal("missing recovery became a numeric measurement")
	}
}
func TestEmptyHistoryAndLegacyUnknown(t *testing.T) {
	store := reports.NewStore(filepath.Join(t.TempDir(), "reports.json"))
	values := scrape(t, store)
	if !math.IsNaN(values["gremlin_success_rate_percent"]) || !math.IsNaN(values["gremlin_last_recovery_ms"]) {
		t.Fatal("empty history has fabricated measurements")
	}
	if err := store.Save(reports.Report{Attack: "cpu", Success: true, RecoveryMs: -1}); err != nil {
		t.Fatal(err)
	}
	values = scrape(t, store)
	if values["gremlin_recovery_verified_total"] != 0 || values["gremlin_recovery_unknown_total"] != 1 || !math.IsNaN(values[`gremlin_avg_recovery_ms_by_type{attack="cpu"}`]) {
		t.Fatal("legacy failure promoted to verified recovery")
	}
}
func TestMalformedReportsFailScrape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	NewCollector(reports.NewStore(path)).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatal("corrupt report store returned success")
	}
}
