package app_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"nt-cli/internal/app"
)

// TestRunner_Doctor_JSON_EmitsAutopilotBlock covers the spec scenario
// "Doctor surfaces autopilot rate": running `nt-cli doctor --json`
// MUST emit a JSON document containing `autopilot.session_close_rate`
// (a number in [0,1]) and `autopilot.threshold` (the spec-mandated 0.9).
// We assert structurally on the parsed JSON rather than substring-
// matching so field ordering / pretty-printing changes don't break us.
func TestRunner_Doctor_JSON_EmitsAutopilotBlock(t *testing.T) {
	store := newDoctorRunnerStore()
	store.report = app.DoctorReport{
		SchemaVersion:    4,
		FTSHealthy:       true,
		IntegrityOK:      true,
		MemoryItemsCount: 0,
		SessionsCount:    0,
		Summary:          "schema_version=4  fts=healthy  integrity=ok  memory_items=0  sessions=0",
		Autopilot: app.AutopilotReport{
			SessionCloseRate: 0.75,
			Threshold:        0.9,
		},
	}
	svc := app.NewService(store)
	var stdout, stderr bytes.Buffer
	code := app.RunCLI(svc, []string{"doctor", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("doctor --json MUST emit valid JSON: %v\nstdout=%q", err, stdout.String())
	}
	autopilot, ok := got["autopilot"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected top-level `autopilot` object, got %T", got["autopilot"])
	}
	rate, ok := autopilot["session_close_rate"].(float64)
	if !ok {
		t.Fatalf("expected autopilot.session_close_rate as number, got %T", autopilot["session_close_rate"])
	}
	if rate < 0 || rate > 1 {
		t.Fatalf("session_close_rate MUST be in [0,1], got %v", rate)
	}
	if rate != 0.75 {
		t.Fatalf("expected rate=0.75 from fixture, got %v", rate)
	}
	threshold, ok := autopilot["threshold"].(float64)
	if !ok {
		t.Fatalf("expected autopilot.threshold as number, got %T", autopilot["threshold"])
	}
	if threshold != 0.9 {
		t.Fatalf("threshold MUST be 0.9 per spec, got %v", threshold)
	}
}

// TestRunner_Doctor_JSON_IncludesCoreFields proves the JSON output is
// not autopilot-only — the existing diagnostic axes (schema_version,
// fts, integrity, counts) MUST also be present so JSON consumers
// don't need to also call the human-text variant.
func TestRunner_Doctor_JSON_IncludesCoreFields(t *testing.T) {
	store := newDoctorRunnerStore()
	store.report = app.DoctorReport{
		SchemaVersion:    4,
		FTSHealthy:       true,
		IntegrityOK:      false,
		MemoryItemsCount: 12,
		SessionsCount:    3,
		Summary:          "summary line",
		Autopilot:        app.AutopilotReport{Threshold: 0.9},
	}
	svc := app.NewService(store)
	var stdout, stderr bytes.Buffer
	code := app.RunCLI(svc, []string{"doctor", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"schema_version", "fts_healthy", "integrity_ok", "memory_items_count", "sessions_count"} {
		if _, has := got[key]; !has {
			t.Fatalf("doctor --json missing required field %q; payload=%s", key, stdout.String())
		}
	}
}

// TestRunner_Doctor_HumanText_StillWorks pins backward compatibility:
// without `--json`, the legacy summary-line output MUST be unchanged
// — Phase 6 ADDS a flag, it doesn't replace the default surface.
func TestRunner_Doctor_HumanText_StillWorks(t *testing.T) {
	store := newDoctorRunnerStore()
	store.report = app.DoctorReport{
		Summary: "schema_version=4  fts=healthy  integrity=ok  memory_items=0  sessions=0",
	}
	svc := app.NewService(store)
	var stdout, stderr bytes.Buffer
	code := app.RunCLI(svc, []string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), store.report.Summary) {
		t.Fatalf("default surface MUST still emit summary line, got %q", stdout.String())
	}
	// And critically: human output MUST NOT accidentally emit JSON,
	// otherwise screen readers and grep pipelines break.
	if strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Fatalf("human output MUST NOT be JSON, got %q", stdout.String())
	}
}
