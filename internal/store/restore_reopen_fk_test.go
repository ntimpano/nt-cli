package store

import (
	"path/filepath"
	"testing"
	"time"

	"flint/internal/app"
)

// TestRestore_ReopenStore_ReappliesForeignKeys verifies BUG-07: after a
// restore/reopen cycle, PRAGMA foreign_keys must still be ON and FK cascade
// behavior must remain active.
func TestRestore_ReopenStore_ReappliesForeignKeys(t *testing.T) {
	s := newTestStore(t)
	a, err := s.Save("alpha", time.Now().UTC())
	if err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	b, err := s.Save("bravo", time.Now().UTC())
	if err != nil {
		t.Fatalf("seed bravo: %v", err)
	}
	if err := s.CreateRelation(a, b, "related", time.Now().UTC()); err != nil {
		t.Fatalf("seed relation: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := s.Backup(backupPath); err != nil {
		t.Fatalf("backup: %v", err)
	}

	if _, err := s.Delete(a); err != nil {
		t.Fatalf("pre-restore delete: %v", err)
	}

	if err := s.Restore(backupPath); err != nil {
		t.Fatalf("restore: %v", err)
	}

	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("read pragma foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys pragma mismatch after restore: got %d want 1", fk)
	}

	before, err := s.Neighbors(b, app.RelationDirectionInbound)
	if err != nil {
		t.Fatalf("neighbors before cascade check: %v", err)
	}
	if len(before) != 1 || before[0].SourceID != a || before[0].TargetID != b {
		t.Fatalf("expected restored relation a->b, got %+v", before)
	}

	if _, err := s.Delete(a); err != nil {
		t.Fatalf("delete after restore: %v", err)
	}
	after, err := s.Neighbors(b, app.RelationDirectionInbound)
	if err != nil {
		t.Fatalf("neighbors after cascade check: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected relation cascade delete after restore, got %d rows", len(after))
	}
}
