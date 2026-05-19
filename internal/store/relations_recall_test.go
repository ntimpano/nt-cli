package store

import (
	"strings"
	"testing"
	"time"

	"flint/internal/app"
)

// PR4a — Graph-Aware Recall (store layer)
//
// These tests pin the store-level behavior of RecallGraphAware:
//
//   - When the graph is empty, output MUST equal the plain FTS recall
//     order — i.e. graph awareness is non-destructive when there is
//     nothing to add.
//   - Neighbors of the top match connected by {related, refines,
//     depends_on} MUST receive a bounded boost so they outrank
//     unrelated rows of equal-or-lower base relevance, but they MUST
//     NOT leapfrog the top match itself (≤20% of base rank cap).
//   - `supersedes` predecessors (rows that were superseded BY a row
//     in the result set) MUST be hidden from the response unless
//     IncludeSuperseded is true.
//   - The final slice MUST honor opts.Limit even when over-fetching
//     internally to compute boosts.

// seedRecallCorpus seeds a deterministic 5-row corpus where the FTS
// query "alpha" produces a known base ranking, then returns the row
// ids by name so tests can wire relations between them.
func seedRecallCorpus(t *testing.T, s *SQLiteStore) (ids map[string]int64) {
	t.Helper()
	base := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	ids = map[string]int64{}
	rows := []struct {
		key, content string
	}{
		// Top hit: highest term density on "alpha".
		{"top", "alpha alpha alpha core decision"},
		// Mid hit: one occurrence of alpha, plus a unique word so it
		// can be addressed later by relation tests.
		{"mid", "alpha midline note"},
		// Tail hit: one occurrence of alpha buried in noise.
		{"tail", "alpha tail filler filler filler"},
		// Connected-but-low: only matches alpha weakly so the boost
		// has visible effect when applied.
		{"linked", "alpha linked candidate"},
		// Unrelated: matches alpha but is not in the graph.
		{"other", "alpha other unconnected"},
	}
	for i, r := range rows {
		id, err := s.Save(r.content, base.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("seed %s: %v", r.key, err)
		}
		ids[r.key] = id
	}
	return ids
}

func mustRecallGraphAware(t *testing.T, s *SQLiteStore, opts app.RecallOptions) []app.MemoryItem {
	t.Helper()
	items, err := s.RecallGraphAware(opts)
	if err != nil {
		t.Fatalf("RecallGraphAware(%+v): %v", opts, err)
	}
	return items
}

// TestRecallGraphAware_EmptyGraph_PreservesPlainOrder — task 4.1:
// With no relations stored, graph-aware recall MUST return the same
// rows in the same order as the plain FTS recall path. This is the
// "no regression vs prior baseline" guarantee from the spec.
func TestRecallGraphAware_EmptyGraph_PreservesPlainOrder(t *testing.T) {
	s := newTestStore(t)
	seedRecallCorpus(t, s)

	plain, err := s.Recall("alpha", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(plain) == 0 {
		t.Fatal("plain recall returned no rows; FTS not seeded")
	}

	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "alpha", Limit: 5})

	if len(got) != len(plain) {
		t.Fatalf("graph-aware len mismatch: got %d, want %d", len(got), len(plain))
	}
	for i := range plain {
		if got[i].ID != plain[i].ID {
			t.Fatalf("position %d differs: got id=%d, want id=%d (full plain=%v got=%v)",
				i, got[i].ID, plain[i].ID, idsOf(plain), idsOf(got))
		}
	}
}

// TestRecallGraphAware_BoostsConnectedNeighbor — task 4.1:
// When a low-ranked row is connected to the top match by an allowed
// relation type, it MUST be boosted above an unconnected row of
// equal-or-lower base rank. The top match MUST remain at position 0.
func TestRecallGraphAware_BoostsConnectedNeighbor(t *testing.T) {
	s := newTestStore(t)
	ids := seedRecallCorpus(t, s)

	// Link top → linked with `related`. The boost should push
	// `linked` ahead of `other` in the final order.
	if err := s.CreateRelation(ids["top"], ids["linked"], "related", time.Now().UTC()); err != nil {
		t.Fatalf("create relation: %v", err)
	}

	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "alpha", Limit: 5})

	pos := positionsByID(got)
	if pos[ids["top"]] != 0 {
		t.Fatalf("top match must stay at position 0; got pos=%d, full=%v", pos[ids["top"]], idsOf(got))
	}
	linkedPos, linkedOK := pos[ids["linked"]]
	otherPos, otherOK := pos[ids["other"]]
	if !linkedOK || !otherOK {
		t.Fatalf("expected both linked and other in result; linked_in=%v other_in=%v full=%v",
			linkedOK, otherOK, idsOf(got))
	}
	if !(linkedPos < otherPos) {
		t.Fatalf("linked (boosted) should outrank other (unboosted): linked@%d other@%d full=%v",
			linkedPos, otherPos, idsOf(got))
	}
}

// TestRecallGraphAware_BoostBoundedByCap — task 4.1:
// The boost MUST NOT let a connected neighbor leapfrog the top match.
// We connect `tail` (last in plain ranking) to `top` and assert it
// moves up but does NOT reach position 0.
func TestRecallGraphAware_BoostBoundedByCap(t *testing.T) {
	s := newTestStore(t)
	ids := seedRecallCorpus(t, s)

	if err := s.CreateRelation(ids["top"], ids["tail"], "refines", time.Now().UTC()); err != nil {
		t.Fatalf("create relation: %v", err)
	}

	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "alpha", Limit: 5})
	pos := positionsByID(got)

	if pos[ids["top"]] != 0 {
		t.Fatalf("top match leapfrogged by neighbor (boost cap violated); top@%d full=%v",
			pos[ids["top"]], idsOf(got))
	}
}

// TestRecallGraphAware_SuppressesSupersededPredecessor — task 4.1:
// If row A is `supersedes`-linked to row B (meaning B supersedes A,
// per outbound edge A→B with type=supersedes... NOTE: the spec says
// "supersedes predecessors MUST be suppressed" — we treat the
// PREDECESSOR as the row that has been superseded BY another. In
// edge-direction terms, if there's an edge X→Y typed `supersedes`,
// X is the new revision and Y is the older predecessor that should
// be hidden. We document this convention in the impl.
func TestRecallGraphAware_SuppressesSupersededPredecessor(t *testing.T) {
	s := newTestStore(t)
	ids := seedRecallCorpus(t, s)

	// `top` supersedes `mid` → mid is the predecessor and MUST be hidden.
	if err := s.CreateRelation(ids["top"], ids["mid"], "supersedes", time.Now().UTC()); err != nil {
		t.Fatalf("create supersedes relation: %v", err)
	}

	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "alpha", Limit: 5})
	pos := positionsByID(got)

	if _, present := pos[ids["mid"]]; present {
		t.Fatalf("predecessor 'mid' MUST be suppressed by supersedes edge; full=%v", idsOf(got))
	}
	if _, present := pos[ids["top"]]; !present {
		t.Fatalf("successor 'top' must remain present; full=%v", idsOf(got))
	}
}

// TestRecallGraphAware_IncludeSupersededOptIn — task 4.1:
// IncludeSuperseded MUST restore the predecessor in the response,
// preserving the prior recall order otherwise.
func TestRecallGraphAware_IncludeSupersededOptIn(t *testing.T) {
	s := newTestStore(t)
	ids := seedRecallCorpus(t, s)

	if err := s.CreateRelation(ids["top"], ids["mid"], "supersedes", time.Now().UTC()); err != nil {
		t.Fatalf("create supersedes relation: %v", err)
	}

	got := mustRecallGraphAware(t, s, app.RecallOptions{
		Query:             "alpha",
		Limit:             5,
		IncludeSuperseded: true,
	})
	pos := positionsByID(got)

	if _, present := pos[ids["mid"]]; !present {
		t.Fatalf("IncludeSuperseded=true MUST restore predecessor; full=%v", idsOf(got))
	}
}

// TestRecallGraphAware_ConflictsWithDoesNotBoost — task 4.1
// triangulation: only `related|refines|depends_on` qualify for the
// boost. A `conflicts_with` edge from the top match MUST NOT alter
// the candidate's position. Pins the relation-type whitelist so a
// future widening (or a typo in isBoostableRelation) breaks loudly.
func TestRecallGraphAware_ConflictsWithDoesNotBoost(t *testing.T) {
	s := newTestStore(t)
	ids := seedRecallCorpus(t, s)

	if err := s.CreateRelation(ids["top"], ids["tail"], "conflicts_with", time.Now().UTC()); err != nil {
		t.Fatalf("create relation: %v", err)
	}

	plain, err := s.Recall("alpha", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "alpha", Limit: 5})

	// Order MUST equal the plain path because conflicts_with is not
	// in the boost whitelist and there is no supersedes edge.
	if len(got) != len(plain) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(plain))
	}
	for i := range plain {
		if got[i].ID != plain[i].ID {
			t.Fatalf("position %d differs after non-boostable edge: got %d, want %d (full plain=%v got=%v)",
				i, got[i].ID, plain[i].ID, idsOf(plain), idsOf(got))
		}
	}
}

// TestRecallGraphAware_HonorsLimit — task 4.1:
// Even if we over-fetch internally to compute boosts, the public slice
// MUST be capped to opts.Limit.
func TestRecallGraphAware_HonorsLimit(t *testing.T) {
	s := newTestStore(t)
	ids := seedRecallCorpus(t, s)
	if err := s.CreateRelation(ids["top"], ids["linked"], "depends_on", time.Now().UTC()); err != nil {
		t.Fatalf("create relation: %v", err)
	}

	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "alpha", Limit: 2})
	if len(got) != 2 {
		t.Fatalf("limit not honored: got %d rows, want 2 (full=%v)", len(got), idsOf(got))
	}
}

// TestRecallGraphAware_EmptyQueryRejected — defensive: same contract
// as plain Recall path; an empty/whitespace query MUST be rejected by
// the caller chain. We check the store-level method tolerates it
// gracefully (returns empty, no panic) — service-layer validation is
// the real guard.
func TestRecallGraphAware_EmptyQueryRejected(t *testing.T) {
	s := newTestStore(t)
	got := mustRecallGraphAware(t, s, app.RecallOptions{Query: "   ", Limit: 5})
	if len(got) != 0 {
		t.Fatalf("expected empty result for whitespace query, got %d rows", len(got))
	}
}

// idsOf is a tiny helper for readable test failures.
func idsOf(items []app.MemoryItem) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

// positionsByID maps each item id to its 0-based position in the slice.
func positionsByID(items []app.MemoryItem) map[int64]int {
	m := make(map[int64]int, len(items))
	for i, it := range items {
		m[it.ID] = i
	}
	return m
}

// guard against unused-import lint when iterating tests on subsets.
var _ = strings.TrimSpace
