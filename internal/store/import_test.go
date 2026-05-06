package store

import (
	"testing"

	"nt-cli/internal/app"
)

// TestImportStore_DedupesByTopicAndHash covers spec scenario "Re-import
// produces no duplicates". The same logical row (same topic_key + content)
// MUST collapse to a single insert across calls.
func TestImportStore_DedupesByTopicAndHash(t *testing.T) {
	s := newTestStore(t)

	rows := []app.ImportRecord{
		{Content: "alpha body", Title: "A", Type: "decision", TopicKey: "arch/a", Scope: "project"},
		{Content: "beta body", Title: "B", Type: "decision", TopicKey: "arch/b", Scope: "project"},
	}

	r1, err := s.ImportRecords(rows)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if r1.Inserted != 2 || r1.Skipped != 0 {
		t.Fatalf("first import expected 2 inserts/0 skips, got %+v", r1)
	}

	// Second pass with the same payload: zero new rows.
	r2, err := s.ImportRecords(rows)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if r2.Inserted != 0 || r2.Skipped != 2 {
		t.Fatalf("re-import expected 0 inserts/2 skips, got %+v", r2)
	}

	// Confirm row count is still 2.
	all, err := s.List(100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 total rows, got %d", len(all))
	}
}

// TestImportStore_DifferentContentSameTopicInsertsNew proves the dedupe
// key is `(topic_key, content_hash)`: same topic with different content
// is a NEW row (not an upsert — that's a separate code path on Save).
func TestImportStore_DifferentContentSameTopicInsertsNew(t *testing.T) {
	s := newTestStore(t)

	first := []app.ImportRecord{{Content: "v1", TopicKey: "arch/a", Type: "decision"}}
	second := []app.ImportRecord{{Content: "v2", TopicKey: "arch/a", Type: "decision"}}

	if _, err := s.ImportRecords(first); err != nil {
		t.Fatalf("first: %v", err)
	}
	r, err := s.ImportRecords(second)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if r.Inserted != 1 {
		t.Fatalf("different content under same topic should insert, got %+v", r)
	}
}

// TestImportStore_EmptyTopicKeyStillDedupesByContent: when topic_key is
// empty (treated as "" literal), dedupe still works on content_hash so
// pure-content imports stay idempotent.
func TestImportStore_EmptyTopicKeyStillDedupesByContent(t *testing.T) {
	s := newTestStore(t)

	rows := []app.ImportRecord{{Content: "no topic"}}
	if _, err := s.ImportRecords(rows); err != nil {
		t.Fatalf("first: %v", err)
	}
	r, err := s.ImportRecords(rows)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if r.Skipped != 1 || r.Inserted != 0 {
		t.Fatalf("expected dedupe with empty topic, got %+v", r)
	}
}
