package store

import (
	"testing"
	"time"

	"flint/internal/app"
)

// TestRecallFiltered_TypeNarrowsResults covers spec scenario
// "Type filter narrows results" from the ranked-recall capability:
// rows of mixed type, recall with type="decision" + query → only decision
// rows return.
func TestRecallFiltered_TypeNarrowsResults(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Three rows mentioning "alpha", different types.
	if _, err := s.SaveWithMeta(app.SaveRequest{
		Content: "alpha is a key concept", Type: "decision", Scope: "project", CreatedAt: base,
	}); err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if _, err := s.SaveWithMeta(app.SaveRequest{
		Content: "alpha note from log", Type: "manual", Scope: "project", CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	if _, err := s.SaveWithMeta(app.SaveRequest{
		Content: "alpha bugfix patched", Type: "bugfix", Scope: "project", CreatedAt: base.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed 3: %v", err)
	}

	got, err := s.RecallFiltered(app.RecallOptions{Query: "alpha", Type: "decision", Limit: 10})
	if err != nil {
		t.Fatalf("RecallFiltered: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 decision row, got %d (%v)", len(got), got)
	}
	if got[0].Type != "decision" {
		t.Fatalf("expected type=decision, got %q", got[0].Type)
	}
	if got[0].Content != "alpha is a key concept" {
		t.Fatalf("unexpected content: %q", got[0].Content)
	}
}

// TestRecallFiltered_DateRangeNarrowsResults covers the "date range" filter
// requirement on Recall: only rows in [since, until] must come back.
func TestRecallFiltered_DateRangeNarrowsResults(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Three rows spaced one day apart, all matching the same query token.
	if _, err := s.SaveWithMeta(app.SaveRequest{
		Content: "ranked entry one", Type: "manual", CreatedAt: base,
	}); err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if _, err := s.SaveWithMeta(app.SaveRequest{
		Content: "ranked entry two", Type: "manual", CreatedAt: base.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	if _, err := s.SaveWithMeta(app.SaveRequest{
		Content: "ranked entry three", Type: "manual", CreatedAt: base.Add(48 * time.Hour),
	}); err != nil {
		t.Fatalf("seed 3: %v", err)
	}

	since := base.Add(12 * time.Hour)
	until := base.Add(36 * time.Hour) // captures only the "two" row
	got, err := s.RecallFiltered(app.RecallOptions{
		Query: "ranked", Since: since, Until: until, Limit: 10,
	})
	if err != nil {
		t.Fatalf("RecallFiltered: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 row inside [%s,%s], got %d (%v)", since, until, len(got), got)
	}
	if got[0].Content != "ranked entry two" {
		t.Fatalf("unexpected content: %q", got[0].Content)
	}
}

// TestRecallFiltered_LimitCaps proves the limit filter still applies on top
// of metadata filters (no regression to the legacy Recall behavior).
func TestRecallFiltered_LimitCaps(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		if _, err := s.SaveWithMeta(app.SaveRequest{
			Content:   "delta entry",
			Type:      "manual",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	got, err := s.RecallFiltered(app.RecallOptions{Query: "delta", Limit: 2})
	if err != nil {
		t.Fatalf("RecallFiltered: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected limit=2 to cap results, got %d", len(got))
	}
}

// TestContext_ReturnsRecentN covers spec scenario "Context returns recent
// items": ≥20 rows seeded, Context(5) returns exactly 5 rows newest-first.
func TestContext_ReturnsRecentN(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		if _, err := s.SaveWithMeta(app.SaveRequest{
			Content:   contextSeedContent(i),
			Scope:     "project",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	got, err := s.Context(5, "")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected exactly 5 rows from Context(5), got %d", len(got))
	}
	// Newest-first ordering: the 5 most recent inserts (indices 19..15) win.
	for i, want := range []string{
		contextSeedContent(19),
		contextSeedContent(18),
		contextSeedContent(17),
		contextSeedContent(16),
		contextSeedContent(15),
	} {
		if got[i].Content != want {
			t.Fatalf("Context[%d] = %q, want %q", i, got[i].Content, want)
		}
	}
}

// TestContext_ScopeFilterNarrows covers the "optional scope filter" of the
// context view: when scope is supplied, only matching rows return.
func TestContext_ScopeFilterNarrows(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// 3 project + 2 personal rows, interleaved so newest-first ordering
	// would mix scopes if no filter were applied.
	mix := []struct {
		scope   string
		content string
	}{
		{"project", "p1"},
		{"personal", "x1"},
		{"project", "p2"},
		{"personal", "x2"},
		{"project", "p3"},
	}
	for i, m := range mix {
		if _, err := s.SaveWithMeta(app.SaveRequest{
			Content:   m.content,
			Scope:     m.scope,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	got, err := s.Context(10, "personal")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 personal rows, got %d (%v)", len(got), got)
	}
	if got[0].Content != "x2" || got[1].Content != "x1" {
		t.Fatalf("expected newest-first personal rows [x2, x1], got [%q, %q]", got[0].Content, got[1].Content)
	}
}

// contextSeedContent generates deterministic seed content distinct enough
// to assert ordering by insertion index.
func contextSeedContent(i int) string {
	return "ctx-row-" + itoa3(i)
}

// itoa3 zero-pads small ints so lexical and numeric ordering coincide in
// failure messages.
func itoa3(i int) string {
	digits := []byte{'0' + byte(i/100%10), '0' + byte(i/10%10), '0' + byte(i%10)}
	return string(digits)
}
