package app_test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"nt-cli/internal/app"
)

// autopilotRunnerStore is a session-aware fake that lets each test
// pre-seed events so we can prove the CLI's behaviour against a real
// summary row vs no summary row. Mirrors sessionMemStore but exposes
// an event log so SessionEvents can answer faithfully.
type autopilotRunnerStore struct {
	*memStore
	events    []app.SessionEvent
	endCalls  int
	lastEndID string
}

func newAutopilotRunnerStore() *autopilotRunnerStore {
	return &autopilotRunnerStore{memStore: newMemStore()}
}

func (s *autopilotRunnerStore) SessionStart(id string, at time.Time) error {
	s.events = append(s.events, app.SessionEvent{SessionID: id, Kind: "start", CreatedAt: at})
	return nil
}

func (s *autopilotRunnerStore) SessionSummary(id, summary string, at time.Time) error {
	s.events = append(s.events, app.SessionEvent{SessionID: id, Kind: "summary", Summary: summary, CreatedAt: at})
	return nil
}

func (s *autopilotRunnerStore) SessionEnd(id string, at time.Time) error {
	s.endCalls++
	s.lastEndID = id
	s.events = append(s.events, app.SessionEvent{SessionID: id, Kind: "end", CreatedAt: at})
	return nil
}

func (s *autopilotRunnerStore) SessionEvents(id string) ([]app.SessionEvent, error) {
	out := []app.SessionEvent{}
	for _, e := range s.events {
		if e.SessionID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

// withFFAutopilotRunner toggles NTCLI_FF_AUTOPILOT for one test and
// restores the previous value. Mirrors withFFAutopilot in the in-pkg
// suite so behaviour matches symmetrically.
func withFFAutopilotRunner(t *testing.T, value string) {
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

func runCLIAutopilot(t *testing.T, store app.Store, args ...string) (int, string, string) {
	t.Helper()
	svc := app.NewService(store)
	var stdout, stderr bytes.Buffer
	code := app.RunCLI(svc, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestRunCLI_SessionEnd_FFOn_BlocksWithoutSummary covers the spec
// scenario "Missing summary blocks clean end": with the autopilot FF
// on and no summary recorded, `nt-cli session end <id>` MUST exit
// non-zero, print `summary_required`, and NOT call the store's
// SessionEnd. Exit code MUST be 2 (distinct from generic 1) so scripts
// can branch on the autopilot block specifically.
func TestRunCLI_SessionEnd_FFOn_BlocksWithoutSummary(t *testing.T) {
	withFFAutopilotRunner(t, "1")
	store := newAutopilotRunnerStore()
	// Pre-seed a start so the session exists but has no summary row.
	if err := store.SessionStart("sess-A", time.Now()); err != nil {
		t.Fatalf("seed start: %v", err)
	}
	code, _, stderr := runCLIAutopilot(t, store, "session", "end", "sess-A")
	if code != 2 {
		t.Fatalf("expected exit 2, got %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stderr, "summary_required") {
		t.Fatalf("stderr MUST contain summary_required, got %q", stderr)
	}
	if store.endCalls != 0 {
		t.Fatalf("end-row written despite block: calls=%d", store.endCalls)
	}
}

// TestRunCLI_SessionEnd_FFOn_AllowsWithForce proves the explicit
// override: `--force` MUST bypass the guard and write the end-row
// even when no summary exists. Exit MUST be 0 and stdout MUST confirm.
func TestRunCLI_SessionEnd_FFOn_AllowsWithForce(t *testing.T) {
	withFFAutopilotRunner(t, "1")
	store := newAutopilotRunnerStore()
	if err := store.SessionStart("sess-B", time.Now()); err != nil {
		t.Fatalf("seed start: %v", err)
	}
	code, stdout, stderr := runCLIAutopilot(t, store, "session", "end", "sess-B", "--force")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if store.endCalls != 1 || store.lastEndID != "sess-B" {
		t.Fatalf("expected 1 force-end for sess-B, got calls=%d id=%q", store.endCalls, store.lastEndID)
	}
	if !strings.Contains(stdout, "sess-B") {
		t.Fatalf("stdout missing id: %q", stdout)
	}
}

// TestRunCLI_SessionEnd_FFOn_AllowsWithSummaryRecorded triangulates the
// happy path: a real summary row in the log MUST satisfy the guard
// without --force.
func TestRunCLI_SessionEnd_FFOn_AllowsWithSummaryRecorded(t *testing.T) {
	withFFAutopilotRunner(t, "1")
	store := newAutopilotRunnerStore()
	now := time.Now()
	if err := store.SessionStart("sess-C", now); err != nil {
		t.Fatalf("seed start: %v", err)
	}
	if err := store.SessionSummary("sess-C", "wrapped feature", now.Add(time.Minute)); err != nil {
		t.Fatalf("seed summary: %v", err)
	}
	code, _, stderr := runCLIAutopilot(t, store, "session", "end", "sess-C")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if store.endCalls != 1 {
		t.Fatalf("expected 1 end call, got %d", store.endCalls)
	}
}

// TestRunCLI_SessionEnd_FFOff_LegacyBehaviour pins backward compat:
// with FF off, `session end` works exactly as before — no summary
// required, exit 0, end-row written. Critical for safe rollout.
func TestRunCLI_SessionEnd_FFOff_LegacyBehaviour(t *testing.T) {
	withFFAutopilotRunner(t, "")
	store := newAutopilotRunnerStore()
	if err := store.SessionStart("sess-D", time.Now()); err != nil {
		t.Fatalf("seed start: %v", err)
	}
	code, _, stderr := runCLIAutopilot(t, store, "session", "end", "sess-D")
	if code != 0 {
		t.Fatalf("FF-off MUST not block, got exit=%d stderr=%q", code, stderr)
	}
	if store.endCalls != 1 {
		t.Fatalf("FF-off MUST still write end-row, got calls=%d", store.endCalls)
	}
}
