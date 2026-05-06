// Package app — autopilot wiring for Phase 6 (workflow-autopilot).
//
// This file adds the opt-in summary guard around SessionEnd. The guard
// is gated by NTCLI_FF_AUTOPILOT=1 so existing callers (and the older
// session_runner tests) keep working while the new behaviour rolls
// out. When the FF is on, ending a session without a prior summary
// row surfaces ErrSummaryRequired and writes nothing — the lifecycle
// log stays clean so SessionEndForce remains the only way to record an
// "end" with no summary.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// autopilotDebugEnabled reports whether the operator opted into the
// PR7 autopilot debug stream via NTCLI_AUTOPILOT_DEBUG=1. Same opt-in
// shape as NTCLI_MCP_DEBUG so operators reach for one habit.
func autopilotDebugEnabled() bool {
	v := strings.TrimSpace(os.Getenv("NTCLI_AUTOPILOT_DEBUG"))
	return v == "1"
}

// formatAutopilotEvent renders a single key=value line documenting an
// autopilot lifecycle decision (e.g. session_end allowed vs blocked).
// The line is grep-friendly:
//
//	ntcli-autopilot event=<name> status=<ok|blocked> session=<id> [reason=<r>]
//
// session ids containing whitespace are quoted so the line stays
// parseable. reason is omitted entirely when empty so the default
// success case stays compact. Pure function — easy to unit-test
// without touching env or stderr.
func formatAutopilotEvent(event, status, sessionID, reason string) string {
	sess := sessionID
	if strings.ContainsAny(sess, " \t") {
		sess = `"` + sess + `"`
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Sprintf("ntcli-autopilot event=%s status=%s session=%s\n",
			event, status, sess)
	}
	return fmt.Sprintf("ntcli-autopilot event=%s status=%s session=%s reason=%s\n",
		event, status, sess, reason)
}

// emitAutopilotEvent writes a formatted autopilot event to w when
// NTCLI_AUTOPILOT_DEBUG=1; otherwise it is a silent no-op. Production
// callers pass os.Stderr so the line lands next to other MCP debug
// output; tests pass a bytes.Buffer for inspection.
func emitAutopilotEvent(w io.Writer, event, status, sessionID, reason string) {
	if !autopilotDebugEnabled() {
		return
	}
	_, _ = io.WriteString(w, formatAutopilotEvent(event, status, sessionID, reason))
}

// autopilotDebugW is the sink the Service uses for autopilot debug
// events. Defaults to os.Stderr so production deployments emit next
// to other MCP debug output. Tests swap in a bytes.Buffer to assert
// on the emitted lines without touching the real stderr.
var autopilotDebugW io.Writer = os.Stderr

// ErrSummaryRequired is the sentinel returned by SessionEnd when the
// autopilot guard fires. The CLI maps it to exit code 2 and a human
// message containing the literal token `summary_required` so scripts
// can grep for it. errors.Is(err, ErrSummaryRequired) is the canonical
// check.
var ErrSummaryRequired = errors.New("summary_required: session has no summary row; record one or pass --force")

// autopilotEnabled reports whether the Phase 6 workflow-autopilot guard
// is opted in via env. Implemented as a function (not a constant) so
// tests can flip the env mid-run, mirroring graphRecallEnabled.
func autopilotEnabled() bool {
	return strings.TrimSpace(os.Getenv("NTCLI_FF_AUTOPILOT")) == "1"
}

// SessionEndForce writes the "end" lifecycle row unconditionally —
// bypassing the autopilot summary guard. This is the documented
// override for closing abandoned sessions (test crashes, PoCs) where
// no summary will ever exist. It still validates the id and still
// requires the store to implement SessionStore.
func (s *Service) SessionEndForce(id string) error {
	clean, err := s.validateSessionID(id)
	if err != nil {
		return err
	}
	sess, ok := s.repo.(SessionStore)
	if !ok {
		return errors.New("store does not support session operations")
	}
	return sess.SessionEnd(clean, time.Now().UTC())
}

// sessionHasSummary reports whether the session log contains at least
// one "summary" row for clean. Returns (false, nil) when the store
// answers cleanly with no summary events, (false, err) on store
// failure. Pulled out of SessionEnd so the autopilot guard reads
// linearly.
func sessionHasSummary(sess SessionStore, clean string) (bool, error) {
	events, err := sess.SessionEvents(clean)
	if err != nil {
		return false, fmt.Errorf("read session events: %w", err)
	}
	for _, e := range events {
		if e.Kind == "summary" && strings.TrimSpace(e.Summary) != "" {
			return true, nil
		}
	}
	return false, nil
}

// writeDoctorJSON encodes a DoctorReport as the wire-shape consumed by
// `nt-cli doctor --json`. We render explicitly (rather than tagging the
// struct) so the JSON keys stay snake_case while the Go fields keep
// idiomatic CamelCase — and so future struct additions don't leak into
// the public JSON surface unintentionally.
//
// The autopilot block is grouped under `autopilot.*` so JSON consumers
// can reach for `.autopilot.session_close_rate` directly per the spec
// scenario "Doctor surfaces autopilot rate".
func writeDoctorJSON(w io.Writer, r DoctorReport) error {
	payload := map[string]interface{}{
		"schema_version":      r.SchemaVersion,
		"fts_healthy":         r.FTSHealthy,
		"integrity_ok":        r.IntegrityOK,
		"integrity_messages":  ensureStringSlice(r.IntegrityMessages),
		"memory_items_count":  r.MemoryItemsCount,
		"sessions_count":      r.SessionsCount,
		"summary":             r.Summary,
		"autopilot": map[string]interface{}{
			"session_close_rate": r.Autopilot.SessionCloseRate,
			"threshold":          r.Autopilot.Threshold,
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode doctor json: %w", err)
	}
	return nil
}

// ensureStringSlice turns nil into [] so JSON consumers see an array,
// not null — same nil-coercion pattern as actionableRecallPayload.
func ensureStringSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
