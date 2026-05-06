package app

import (
	"bytes"
	"strings"
	"testing"
)

// TestFormatAutopilotEvent_BlockedFire covers the operator contract for
// the autopilot debug hook: when SessionEnd is blocked by the
// summary_required guard, a stable, single-line, key=value record is
// produced so `grep autopilot` finds every fire across logs.
func TestFormatAutopilotEvent_BlockedFire(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		status    string
		reason    string
		want      string
	}{
		{
			name:      "blocked end with summary_required",
			sessionID: "s-42",
			status:    "blocked",
			reason:    "summary_required",
			want:      "ntcli-autopilot event=session_end status=blocked session=s-42 reason=summary_required\n",
		},
		{
			name:      "ok end emits status=ok with no reason",
			sessionID: "s-99",
			status:    "ok",
			reason:    "",
			want:      "ntcli-autopilot event=session_end status=ok session=s-99\n",
		},
		{
			name:      "session id with whitespace is quoted",
			sessionID: "s 42",
			status:    "blocked",
			reason:    "summary_required",
			want:      `ntcli-autopilot event=session_end status=blocked session="s 42" reason=summary_required` + "\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatAutopilotEvent("session_end", tc.status, tc.sessionID, tc.reason)
			if got != tc.want {
				t.Fatalf("formatAutopilotEvent: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEmitAutopilotEvent_OffByDefault documents the default contract:
// when NTCLI_AUTOPILOT_DEBUG is unset/empty/"0", the helper writes
// nothing. The autopilot guard must remain silent on stderr unless
// the operator explicitly opts in — same pattern as NTCLI_MCP_DEBUG.
func TestEmitAutopilotEvent_OffByDefault(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{name: "unset → silent", env: ""},
		{name: "explicit 0 → silent", env: "0"},
		{name: "whitespace → silent", env: "  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NTCLI_AUTOPILOT_DEBUG", tc.env)
			var buf bytes.Buffer
			emitAutopilotEvent(&buf, "session_end", "blocked", "s-1", "summary_required")
			if buf.Len() != 0 {
				t.Fatalf("expected silence with env=%q, got %q", tc.env, buf.String())
			}
		})
	}
}

// TestEmitAutopilotEvent_OnWritesEvent triangulates the on-path: when
// NTCLI_AUTOPILOT_DEBUG=1 is set, the helper writes the formatted
// line to the provided writer. Pairs with the OffByDefault test so
// the FF semantics are pinned from both sides.
func TestEmitAutopilotEvent_OnWritesEvent(t *testing.T) {
	t.Setenv("NTCLI_AUTOPILOT_DEBUG", "1")
	var buf bytes.Buffer
	emitAutopilotEvent(&buf, "session_end", "blocked", "s-1", "summary_required")
	got := buf.String()
	if !strings.Contains(got, "event=session_end") ||
		!strings.Contains(got, "status=blocked") ||
		!strings.Contains(got, "session=s-1") ||
		!strings.Contains(got, "reason=summary_required") {
		t.Fatalf("emitAutopilotEvent on-path produced unexpected line: %q", got)
	}
}
