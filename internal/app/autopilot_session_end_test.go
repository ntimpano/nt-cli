package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// withFFAutopilot toggles NTCLI_FF_AUTOPILOT for one test and restores
// the previous value on cleanup. The autopilot summary guard is opt-in
// (per spec) so we exercise both states explicitly.
func withFFAutopilot(t *testing.T, value string) {
	t.Helper()
	prev, had := os.LookupEnv("NTCLI_FF_AUTOPILOT")
	if value == "" {
		_ = os.Unsetenv("NTCLI_FF_AUTOPILOT")
	} else {
		_ = os.Setenv("NTCLI_FF_AUTOPILOT", value)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("NTCLI_FF_AUTOPILOT", prev)
		} else {
			_ = os.Unsetenv("NTCLI_FF_AUTOPILOT")
		}
	})
}

// autopilotFakeStore extends fakeStore with SessionStore + an
// in-memory event log so we can pre-seed lifecycle history and assert
// what new rows the service writes (or doesn't).
type autopilotFakeStore struct {
	fakeStore
	events []SessionEvent
}

func (a *autopilotFakeStore) SessionStart(id string, at time.Time) error {
	a.events = append(a.events, SessionEvent{SessionID: id, Kind: "start", CreatedAt: at})
	return nil
}

func (a *autopilotFakeStore) SessionSummary(id, summary string, at time.Time) error {
	a.events = append(a.events, SessionEvent{SessionID: id, Kind: "summary", Summary: summary, CreatedAt: at})
	return nil
}

func (a *autopilotFakeStore) SessionEnd(id string, at time.Time) error {
	a.events = append(a.events, SessionEvent{SessionID: id, Kind: "end", CreatedAt: at})
	return nil
}

func (a *autopilotFakeStore) SessionEvents(id string) ([]SessionEvent, error) {
	out := []SessionEvent{}
	for _, e := range a.events {
		if e.SessionID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

var _ SessionStore = (*autopilotFakeStore)(nil)

// TestSessionEnd_RejectsMissingSummary covers the spec requirement:
// when `session end` runs and the session has no `summary` row, the
// service MUST refuse with a `summary_required` sentinel error so the
// CLI can map it to a non-zero exit and a clear human message.
func TestSessionEnd_RejectsMissingSummary(t *testing.T) {
	withFFAutopilot(t, "1")
	store := &autopilotFakeStore{}
	svc := NewService(store)

	if err := svc.SessionStart("sess-A"); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	// No summary recorded — end MUST be blocked.
	err := svc.SessionEnd("sess-A")
	if err == nil {
		t.Fatalf("SessionEnd MUST fail when no summary present")
	}
	if !errors.Is(err, ErrSummaryRequired) {
		t.Fatalf("expected ErrSummaryRequired, got %v", err)
	}
	if !strings.Contains(err.Error(), "summary_required") {
		t.Fatalf("error message MUST surface 'summary_required' sentinel, got %q", err.Error())
	}
	// And NO end-row was written — the lifecycle log MUST stay clean
	// when the autopilot block fires (otherwise --force becomes a lie).
	for _, e := range store.events {
		if e.Kind == "end" {
			t.Fatalf("end-row was written despite missing summary: %+v", store.events)
		}
	}
}

// TestSessionEnd_AllowsWhenSummaryPresent triangulates the happy path:
// once a summary is recorded the autopilot guard MUST step aside and
// the end-row MUST be written verbatim.
func TestSessionEnd_AllowsWhenSummaryPresent(t *testing.T) {
	withFFAutopilot(t, "1")
	store := &autopilotFakeStore{}
	svc := NewService(store)

	if err := svc.SessionStart("sess-B"); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := svc.SessionSummary("sess-B", "wrapped up auth refactor"); err != nil {
		t.Fatalf("SessionSummary: %v", err)
	}
	if err := svc.SessionEnd("sess-B"); err != nil {
		t.Fatalf("SessionEnd MUST succeed when summary present: %v", err)
	}
	var sawEnd bool
	for _, e := range store.events {
		if e.SessionID == "sess-B" && e.Kind == "end" {
			sawEnd = true
		}
	}
	if !sawEnd {
		t.Fatalf("end-row missing from log: %+v", store.events)
	}
}

// TestSessionEndForce_BypassesSummaryGuard covers the explicit-override
// path: SessionEndForce MUST write the end-row even with no summary, so
// operators can close abandoned sessions (test crashes, PoCs) without
// fabricating a fake summary.
func TestSessionEndForce_BypassesSummaryGuard(t *testing.T) {
	withFFAutopilot(t, "1")
	store := &autopilotFakeStore{}
	svc := NewService(store)

	if err := svc.SessionStart("sess-C"); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := svc.SessionEndForce("sess-C"); err != nil {
		t.Fatalf("SessionEndForce MUST succeed without summary: %v", err)
	}
	var sawEnd bool
	for _, e := range store.events {
		if e.SessionID == "sess-C" && e.Kind == "end" {
			sawEnd = true
		}
	}
	if !sawEnd {
		t.Fatalf("force-end did not write end-row: %+v", store.events)
	}
}

// TestSessionEnd_RejectsEmptyID guards: even with the new summary
// guard, the existing empty-id rejection MUST still fire FIRST. Order
// matters — if we accidentally check summary before id we'd query the
// store with an empty id and surface summary_required for the wrong
// reason.
// TestSessionEnd_FFOff_NoGuard pins backward compatibility: without
// NTCLI_FF_AUTOPILOT, SessionEnd MUST NOT block on missing summary —
// the guard is strictly opt-in per spec ("opt-in hooks"). This is the
// safety net that lets PR6 ship without breaking existing callers.
func TestSessionEnd_FFOff_NoGuard(t *testing.T) {
	withFFAutopilot(t, "")
	store := &autopilotFakeStore{}
	svc := NewService(store)

	if err := svc.SessionStart("sess-D"); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	// No summary, FF off — MUST succeed (legacy behaviour).
	if err := svc.SessionEnd("sess-D"); err != nil {
		t.Fatalf("FF-off SessionEnd MUST not block: %v", err)
	}
}

func TestSessionEnd_RejectsEmptyID(t *testing.T) {
	withFFAutopilot(t, "1")
	store := &autopilotFakeStore{}
	svc := NewService(store)
	for _, id := range []string{"", "   "} {
		if err := svc.SessionEnd(id); err == nil {
			t.Fatalf("expected error for id %q", id)
		} else if errors.Is(err, ErrSummaryRequired) {
			t.Fatalf("empty id MUST NOT surface as summary_required: got %v", err)
		}
	}
}
