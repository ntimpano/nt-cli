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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

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
