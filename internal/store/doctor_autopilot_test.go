package store

import (
	"testing"
	"time"
)

// seedSession seeds a complete session lifecycle (start [+ optional
// summary] [+ optional end]) at the given UTC instant + N minutes.
// Pulled out so the table-driven tests below stay readable.
func seedSession(t *testing.T, s *SQLiteStore, id string, at time.Time, withSummary, withEnd bool) {
	t.Helper()
	if err := s.SessionStart(id, at); err != nil {
		t.Fatalf("SessionStart(%s): %v", id, err)
	}
	if withSummary {
		if err := s.SessionSummary(id, "summary for "+id, at.Add(time.Minute)); err != nil {
			t.Fatalf("SessionSummary(%s): %v", id, err)
		}
	}
	if withEnd {
		if err := s.SessionEnd(id, at.Add(2*time.Minute)); err != nil {
			t.Fatalf("SessionEnd(%s): %v", id, err)
		}
	}
}

// TestDoctor_AutopilotRate_NoSessions covers the empty-store case:
// when the rolling 14-day window contains zero session-start rows,
// `Autopilot.SessionCloseRate` MUST be 0.0 (NOT NaN, NOT -1) and the
// threshold MUST be the spec-mandated 0.9. Defining the empty case
// explicitly prevents division-by-zero from ever surfacing to the
// JSON output.
func TestDoctor_AutopilotRate_NoSessions(t *testing.T) {
	s := newTestStore(t)
	report, err := s.Doctor()
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if report.Autopilot.SessionCloseRate != 0.0 {
		t.Fatalf("empty store rate MUST be 0.0, got %v", report.Autopilot.SessionCloseRate)
	}
	if report.Autopilot.Threshold != 0.9 {
		t.Fatalf("threshold MUST be 0.9, got %v", report.Autopilot.Threshold)
	}
}

// TestDoctor_AutopilotRate_Computes covers the spec scenario "Doctor
// surfaces autopilot rate": a mix of sessions inside the rolling
// 14-day window — some with summary+end, some with start only —
// MUST produce SessionCloseRate = (sessions with summary) / (sessions
// started in window). We seed 4 sessions: 3 with summary, 1 without →
// expected rate 0.75. Bound checks (rate ∈ [0,1]) are also asserted.
func TestDoctor_AutopilotRate_Computes(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	// 3 closed sessions (summary present) within the window.
	seedSession(t, s, "sess-1", now.Add(-3*24*time.Hour), true, true)
	seedSession(t, s, "sess-2", now.Add(-7*24*time.Hour), true, true)
	seedSession(t, s, "sess-3", now.Add(-1*24*time.Hour), true, false)
	// 1 abandoned session (start only) within the window.
	seedSession(t, s, "sess-4", now.Add(-2*24*time.Hour), false, false)

	report, err := s.Doctor()
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	got := report.Autopilot.SessionCloseRate
	want := 0.75
	if got < 0 || got > 1 {
		t.Fatalf("rate MUST be in [0,1], got %v", got)
	}
	// Allow a tiny epsilon; the rate is computed from integer counts so
	// it should be exact, but float comparison hygiene matters.
	if abs(got-want) > 1e-9 {
		t.Fatalf("rate = %v, want %v", got, want)
	}
}

// TestDoctor_AutopilotRate_ExcludesOldSessions enforces the 14-day
// rolling window: sessions started more than 14 days ago MUST NOT
// affect the rate. Without this boundary, stale data from months ago
// would dominate the metric and the autopilot threshold would be
// meaningless.
func TestDoctor_AutopilotRate_ExcludesOldSessions(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	// 1 fresh session WITH summary (in window) → numerator 1, denom 1.
	seedSession(t, s, "fresh", now.Add(-2*24*time.Hour), true, true)
	// 5 ancient sessions WITHOUT summary (outside 14d) — these MUST
	// be excluded entirely; if they leaked in, rate would drop to 1/6.
	for i := 0; i < 5; i++ {
		old := now.Add(-30 * 24 * time.Hour).Add(-time.Duration(i) * time.Hour)
		seedSession(t, s, "ancient-"+string(rune('a'+i)), old, false, false)
	}
	report, err := s.Doctor()
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if report.Autopilot.SessionCloseRate != 1.0 {
		t.Fatalf("only fresh session counts; expected rate=1.0, got %v", report.Autopilot.SessionCloseRate)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
