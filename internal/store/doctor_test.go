package store_test

import (
	"strings"
	"testing"

	"nt-cli/internal/app"
	"nt-cli/internal/store"
)

// TestDoctor_HealthyStore proves the M3 doctor scenario "diagnostic
// surface": after Init, doctor MUST report schema_version=3, FTS healthy,
// integrity ok, and accurate row counts.
func TestDoctor_HealthyStore(t *testing.T) {
	src := openTempStoreT(t)
	svc := app.NewService(src)
	if err := svc.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.Save("note 1"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := svc.Save("note 2"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := svc.SessionStart("sess-1"); err != nil {
		t.Fatalf("session start: %v", err)
	}

	report, err := src.Doctor()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report.SchemaVersion != store.CurrentSchemaVersion {
		t.Fatalf("schema_version: want %d got %d",
			store.CurrentSchemaVersion, report.SchemaVersion)
	}
	if !report.FTSHealthy {
		t.Fatalf("expected FTS healthy after Init")
	}
	if !report.IntegrityOK {
		t.Fatalf("expected integrity_check=ok, got messages=%v", report.IntegrityMessages)
	}
	if report.MemoryItemsCount != 2 {
		t.Fatalf("memory_items count: want 2 got %d", report.MemoryItemsCount)
	}
	if report.SessionsCount != 1 {
		t.Fatalf("sessions count: want 1 got %d", report.SessionsCount)
	}
}

// TestDoctor_FreshStoreCounts: an empty store after Init reports zero
// counts and stays healthy.
func TestDoctor_FreshStoreCounts(t *testing.T) {
	src := openTempStoreT(t)
	if err := app.NewService(src).Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	report, err := src.Doctor()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report.MemoryItemsCount != 0 || report.SessionsCount != 0 {
		t.Fatalf("expected zero counts on fresh store, got %+v", report)
	}
	if !report.FTSHealthy || !report.IntegrityOK {
		t.Fatalf("fresh store must be healthy: %+v", report)
	}
}

// TestDoctor_ReportsContainSummaryLine sanity-checks the human-readable
// Summary field (used by CLI/MCP presentation) — must mention each
// reported axis so users see all signals at a glance.
func TestDoctor_ReportsContainSummaryLine(t *testing.T) {
	src := openTempStoreT(t)
	if err := app.NewService(src).Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	report, err := src.Doctor()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	summary := strings.ToLower(report.Summary)
	for _, axis := range []string{"schema", "fts", "integrity", "memory_items", "sessions"} {
		if !strings.Contains(summary, axis) {
			t.Fatalf("summary missing %q axis: %q", axis, report.Summary)
		}
	}
}
