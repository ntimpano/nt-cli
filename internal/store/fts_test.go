package store

import (
	"path/filepath"
	"testing"
	"time"
)

// TestFTS_RankedRecall_TopHitWins covers spec scenario "FTS top-3 meets baseline":
// a query MUST return rows ordered by FTS5 bm25 relevance, not by created_at.
// This test fails before FTS is wired because Recall() currently uses LIKE +
// ORDER BY created_at, which returns the most-recent insert first regardless
// of textual relevance.
func TestFTS_RankedRecall_TopHitWins(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	// Seed three rows. The OLDEST has the highest term density for "fts5";
	// LIKE+created_at would put it last. FTS+bm25 must put it first.
	if _, err := s.Save("fts5 fts5 fts5 ranking primer", base); err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if _, err := s.Save("unrelated note about coffee", base.Add(1*time.Minute)); err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	if _, err := s.Save("a note that mentions fts5 once", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("seed 3: %v", err)
	}

	got, err := s.Recall("fts5", 10)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matching rows for 'fts5', got %d", len(got))
	}
	// bm25 ranks denser term occurrence higher → first row wins.
	if want := "fts5 fts5 fts5 ranking primer"; got[0].Content != want {
		t.Fatalf("expected ranked top hit %q, got %q", want, got[0].Content)
	}
}

// TestFTS_TriggersSyncOnUpdate covers the requirement that UPDATEs to
// memory_items propagate into the FTS index, otherwise recall would return
// stale rows.
func TestFTS_TriggersSyncOnUpdate(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	id, err := s.Save("alpha original content", base)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := s.Update(id, "beta replacement payload", base.Add(1*time.Minute)); err != nil {
		t.Fatalf("update: %v", err)
	}

	// The new term must be findable.
	hits, err := s.Recall("beta", 10)
	if err != nil {
		t.Fatalf("recall beta: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != id {
		t.Fatalf("expected updated row to be findable by new term, got %+v", hits)
	}

	// The old term must no longer match.
	stale, err := s.Recall("alpha", 10)
	if err != nil {
		t.Fatalf("recall alpha: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected old term to be gone from FTS, got %d hits", len(stale))
	}
}

// TestFTS_TriggersSyncOnDelete makes sure deleted rows leave no shadow in
// the FTS index.
func TestFTS_TriggersSyncOnDelete(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	id, err := s.Save("gamma keyword body", base)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := s.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	hits, err := s.Recall("gamma", 10)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits after delete, got %d", len(hits))
	}
}

// TestFTS_FallbackToLIKE_WhenFTSCorrupt covers spec scenario
// "FTS unavailable falls back to LIKE": if memory_fts is missing or
// returning errors, Recall MUST still return results via LIKE without
// surfacing the FTS failure to the caller.
func TestFTS_FallbackToLIKE_WhenFTSCorrupt(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	if _, err := s.Save("delta needle in haystack", base); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Simulate FTS corruption: drop the virtual table out from under the
	// store. The store cached `useFTS=true` at Init; Recall must catch the
	// failure and fall back to LIKE transparently.
	if _, err := s.db.Exec(`DROP TABLE memory_fts`); err != nil {
		t.Fatalf("drop fts: %v", err)
	}

	hits, err := s.Recall("needle", 10)
	if err != nil {
		t.Fatalf("recall after fts drop must not error, got: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected LIKE fallback to find row, got %d hits", len(hits))
	}
}

// TestFTS_DetectedDisabled_AtStartup covers the case where FTS is missing
// in the binary at all. The store builds with FTS in this codebase, but the
// detection path itself (UseFTS()) must report a boolean reflecting reality.
func TestFTS_DetectedEnabled_AtStartup(t *testing.T) {
	s := newTestStore(t)
	if !s.UseFTS() {
		t.Fatalf("expected UseFTS()=true on a fresh init with FTS5 available")
	}
}

// TestFTS_BackfillsExistingRowsOnMigration ensures a legacy DB that already
// has memory_items rows gets those rows mirrored into memory_fts when the
// migration creates the FTS table for the first time. Without backfill,
// pre-M2 rows would be invisible to ranked recall.
func TestFTS_BackfillsExistingRowsOnMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	// Seed via M1-only schema (no memory_fts yet). Use Init() once at v1
	// behaviour by opening the store, then DROP memory_fts to simulate a
	// pre-M2 install.
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	if _, err := s.Save("epsilon legacy row", base); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Simulate "pre-M2 install": no FTS table, schema_version rolled back.
	if _, err := s.db.Exec(`DROP TABLE memory_fts`); err != nil {
		t.Fatalf("drop fts: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_version WHERE version >= 2`); err != nil {
		t.Fatalf("rollback version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Re-open and migrate — the M2 step must rebuild memory_fts AND
	// backfill the legacy row.
	upgraded, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer upgraded.Close()
	if err := upgraded.Init(); err != nil {
		t.Fatalf("init upgraded: %v", err)
	}
	if !upgraded.UseFTS() {
		t.Fatalf("expected FTS active after migration")
	}

	hits, err := upgraded.Recall("epsilon", 10)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected legacy row to be FTS-searchable post-migration, got %d hits", len(hits))
	}
}
