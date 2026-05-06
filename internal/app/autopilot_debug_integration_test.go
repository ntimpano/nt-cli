package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSessionEnd_AutopilotDebugEmitsBlockedEvent covers the PR7
// observability hook: when NTCLI_FF_AUTOPILOT=1 AND
// NTCLI_AUTOPILOT_DEBUG=1, a blocked SessionEnd MUST emit a
// `status=blocked reason=summary_required` line so operators can
// grep autopilot fires across logs.
func TestSessionEnd_AutopilotDebugEmitsBlockedEvent(t *testing.T) {
	withFFAutopilot(t, "1")
	t.Setenv("NTCLI_AUTOPILOT_DEBUG", "1")

	var buf bytes.Buffer
	prev := autopilotDebugW
	autopilotDebugW = &buf
	t.Cleanup(func() { autopilotDebugW = prev })

	store := &autopilotFakeStore{}
	svc := NewService(store)

	err := svc.SessionEnd("s-blocked")
	if !errors.Is(err, ErrSummaryRequired) {
		t.Fatalf("SessionEnd: got %v, want ErrSummaryRequired", err)
	}
	got := buf.String()
	if !strings.Contains(got, "event=session_end") ||
		!strings.Contains(got, "status=blocked") ||
		!strings.Contains(got, "session=s-blocked") ||
		!strings.Contains(got, "reason=summary_required") {
		t.Fatalf("expected blocked autopilot debug line, got %q", got)
	}
}

// TestSessionEnd_AutopilotDebugEmitsOkEvent triangulates the
// allowed-end branch: when a summary already exists, SessionEnd
// succeeds AND the debug stream records `status=ok`. Without this
// triangulation the guard could silently emit only blocks (looking
// like a one-sided log).
func TestSessionEnd_AutopilotDebugEmitsOkEvent(t *testing.T) {
	withFFAutopilot(t, "1")
	t.Setenv("NTCLI_AUTOPILOT_DEBUG", "1")

	var buf bytes.Buffer
	prev := autopilotDebugW
	autopilotDebugW = &buf
	t.Cleanup(func() { autopilotDebugW = prev })

	store := &autopilotFakeStore{}
	// Pre-seed a summary so the guard releases.
	store.events = append(store.events, SessionEvent{
		SessionID: "s-ok",
		Kind:      "summary",
		Summary:   "did the thing",
		CreatedAt: time.Now().UTC(),
	})
	svc := NewService(store)

	if err := svc.SessionEnd("s-ok"); err != nil {
		t.Fatalf("SessionEnd: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "status=ok") || !strings.Contains(got, "session=s-ok") {
		t.Fatalf("expected ok autopilot debug line, got %q", got)
	}
	if strings.Contains(got, "reason=") {
		t.Fatalf("ok event should not carry a reason, got %q", got)
	}
}

// TestSessionEnd_AutopilotDebugSilentWhenOff documents that when the
// debug flag is OFF the writer receives nothing — even if the FF is
// ON and the guard fires. This protects the default operator
// experience from log noise.
func TestSessionEnd_AutopilotDebugSilentWhenOff(t *testing.T) {
	withFFAutopilot(t, "1")
	t.Setenv("NTCLI_AUTOPILOT_DEBUG", "")

	var buf bytes.Buffer
	prev := autopilotDebugW
	autopilotDebugW = &buf
	t.Cleanup(func() { autopilotDebugW = prev })

	store := &autopilotFakeStore{}
	svc := NewService(store)

	_ = svc.SessionEnd("s-quiet") // expected ErrSummaryRequired, irrelevant here
	if buf.Len() != 0 {
		t.Fatalf("expected silence with debug OFF, got %q", buf.String())
	}
}
