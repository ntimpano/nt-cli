package store

import (
	"testing"
	"time"

	"nt-cli/internal/app"
)

// TestSessions_FullLifecycle covers spec scenario:
// "Full lifecycle writes three linked rows" — start, summary, end with the
// same session id MUST yield three queryable rows tagged to that session.
func TestSessions_FullLifecycle(t *testing.T) {
	s := newTestStore(t)
	id := "sess-1"
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	if err := s.SessionStart(id, now); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := s.SessionSummary(id, "halfway summary", now.Add(time.Minute)); err != nil {
		t.Fatalf("SessionSummary: %v", err)
	}
	if err := s.SessionEnd(id, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("SessionEnd: %v", err)
	}

	rows, err := s.SessionEvents(id)
	if err != nil {
		t.Fatalf("SessionEvents: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 lifecycle rows, got %d (%v)", len(rows), rows)
	}
	wantKinds := []string{"start", "summary", "end"}
	for i, want := range wantKinds {
		if rows[i].Kind != want {
			t.Fatalf("row[%d].Kind = %q, want %q", i, rows[i].Kind, want)
		}
		if rows[i].SessionID != id {
			t.Fatalf("row[%d].SessionID = %q, want %q", i, rows[i].SessionID, id)
		}
	}
	if rows[1].Summary != "halfway summary" {
		t.Fatalf("summary row content = %q, want %q", rows[1].Summary, "halfway summary")
	}
}

// TestSessions_Isolation: events for one session id must not leak into
// queries for a different session id.
func TestSessions_Isolation(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	if err := s.SessionStart("a", now); err != nil {
		t.Fatalf("start a: %v", err)
	}
	if err := s.SessionStart("b", now.Add(time.Second)); err != nil {
		t.Fatalf("start b: %v", err)
	}
	if err := s.SessionEnd("a", now.Add(time.Minute)); err != nil {
		t.Fatalf("end a: %v", err)
	}

	a, err := s.SessionEvents("a")
	if err != nil {
		t.Fatalf("events a: %v", err)
	}
	if len(a) != 2 {
		t.Fatalf("expected 2 events for a, got %d", len(a))
	}
	b, err := s.SessionEvents("b")
	if err != nil {
		t.Fatalf("events b: %v", err)
	}
	if len(b) != 1 {
		t.Fatalf("expected 1 event for b, got %d", len(b))
	}
}

// TestSessions_RejectsEmptyID guards the API against empty session ids
// which would silently merge unrelated sessions.
func TestSessions_RejectsEmptyID(t *testing.T) {
	s := newTestStore(t)
	cases := []struct {
		name string
		fn   func() error
	}{
		{"start empty", func() error { return s.SessionStart("", time.Now()) }},
		{"start spaces", func() error { return s.SessionStart("   ", time.Now()) }},
		{"end empty", func() error { return s.SessionEnd("", time.Now()) }},
		{"summary empty", func() error { return s.SessionSummary("", "x", time.Now()) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatalf("expected error on empty/whitespace session id")
			}
		})
	}
}

// ensureSessionEventType silences the "unused import" compile when the
// test file is compiled before the production type exists.
var _ = app.MemoryItem{}
