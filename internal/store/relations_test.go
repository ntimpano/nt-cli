package store

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"flint/internal/app"
)

// seedTwo writes two memory rows and returns their ids in insertion order.
// Used by relation tests so we have known FK targets without coupling to
// specific id values (AUTOINCREMENT may not start at 1 if other tests
// in the same file seeded earlier).
func seedTwo(t *testing.T, s *SQLiteStore) (int64, int64) {
	t.Helper()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	a, err := s.Save("alpha note", now)
	if err != nil {
		t.Fatalf("seed a: %v", err)
	}
	b, err := s.Save("bravo note", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("seed b: %v", err)
	}
	return a, b
}

// TestSchemaVersionAdvancedToV4 — task 3.1 RED:
// memory_relations migration MUST bump CurrentSchemaVersion to 4 so doctor
// + downstream consumers see a stamped advancement.
func TestSchemaVersionAdvancedToV4(t *testing.T) {
	if CurrentSchemaVersion < 4 {
		t.Fatalf("CurrentSchemaVersion must be ≥4 once memory_relations migration lands, got %d", CurrentSchemaVersion)
	}
	s := newTestStore(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Fatalf("stamped schema_version mismatch: got %d, want %d", v, CurrentSchemaVersion)
	}
}

// TestRelations_RoundTrip — tasks 3.1 + 3.3:
// CreateRelation persists a typed edge; Neighbors returns it on outbound
// traversal and the inverse query returns the source on inbound.
func TestRelations_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	a, b := seedTwo(t, s)

	if err := s.CreateRelation(a, b, "supersedes", time.Now().UTC()); err != nil {
		t.Fatalf("create relation: %v", err)
	}

	out, err := s.Neighbors(a, app.RelationDirectionOutbound)
	if err != nil {
		t.Fatalf("neighbors outbound: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("outbound neighbors: got %d, want 1", len(out))
	}
	if out[0].SourceID != a || out[0].TargetID != b || out[0].RelationType != "supersedes" {
		t.Fatalf("outbound row mismatch: %+v", out[0])
	}

	in, err := s.Neighbors(b, app.RelationDirectionInbound)
	if err != nil {
		t.Fatalf("neighbors inbound: %v", err)
	}
	if len(in) != 1 {
		t.Fatalf("inbound neighbors: got %d, want 1", len(in))
	}
	if in[0].SourceID != a || in[0].TargetID != b {
		t.Fatalf("inbound row mismatch: %+v", in[0])
	}
}

// TestRelations_SelfLoopRejected — task 3.1:
// CHECK constraint MUST reject (source == target) edges. The store-level
// API is the contract surface; we assert the error reaches the caller.
func TestRelations_SelfLoopRejected(t *testing.T) {
	s := newTestStore(t)
	a, _ := seedTwo(t, s)
	err := s.CreateRelation(a, a, "related", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error on self-loop, got nil")
	}
}

// TestRelations_ForeignKeyCascade — task 3.1:
// Deleting a memory MUST cascade-delete relations referencing it (both
// directions) so neighbor traversal never returns dangling edges.
func TestRelations_ForeignKeyCascade(t *testing.T) {
	s := newTestStore(t)
	a, b := seedTwo(t, s)
	if err := s.CreateRelation(a, b, "refines", time.Now().UTC()); err != nil {
		t.Fatalf("create relation: %v", err)
	}
	if _, err := s.Delete(a); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	in, err := s.Neighbors(b, app.RelationDirectionInbound)
	if err != nil {
		t.Fatalf("neighbors after cascade: %v", err)
	}
	if len(in) != 0 {
		t.Fatalf("expected cascade-delete of relations, got %d remaining", len(in))
	}
}

// TestRelations_MigrationDownReversible — task 3.2:
// DropRelationsSchema MUST remove memory_relations cleanly; subsequent
// Init() restores it (forward migration is idempotent over a missing
// table). This validates the documented down-migration path.
func TestRelations_MigrationDownReversible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "down.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	a, b := seedTwo(t, s)
	if err := s.CreateRelation(a, b, "depends_on", time.Now().UTC()); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Down: drop memory_relations entirely.
	if err := s.DropRelationsSchema(); err != nil {
		t.Fatalf("drop relations: %v", err)
	}
	// memory_relations must no longer exist.
	if exists, _ := tableExists(s, "memory_relations"); exists {
		t.Fatal("memory_relations still exists after DropRelationsSchema")
	}
	// memory_items must remain intact.
	got, err := s.Get(a)
	if err != nil {
		t.Fatalf("get after drop: %v", err)
	}
	if got.ID != a {
		t.Fatalf("memory_items lost after drop: %+v", got)
	}

	// Re-Init must restore the schema additively (idempotent forward path).
	if err := s.Init(); err != nil {
		t.Fatalf("re-init after drop: %v", err)
	}
	if exists, _ := tableExists(s, "memory_relations"); !exists {
		t.Fatal("memory_relations missing after re-init")
	}
}

// tableExists is a test helper — kept in this _test.go file so it does
// not bloat the production binary's symbol table.
func tableExists(s *SQLiteStore, name string) (bool, error) {
	var got string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&got)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return false, nil
		}
		return false, err
	}
	return got == name, nil
}

// TestRelations_FilterByDirection — task 3.3:
// Outbound MUST list only edges where source=id; Inbound only edges
// where target=id. A node with one of each must show one row per
// direction (no double-count).
func TestRelations_FilterByDirection(t *testing.T) {
	s := newTestStore(t)
	a, b := seedTwo(t, s)
	now := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	c, err := s.Save("charlie note", now)
	if err != nil {
		t.Fatalf("seed c: %v", err)
	}
	if err := s.CreateRelation(a, b, "related", now); err != nil {
		t.Fatalf("rel a->b: %v", err)
	}
	if err := s.CreateRelation(c, a, "refines", now.Add(time.Second)); err != nil {
		t.Fatalf("rel c->a: %v", err)
	}

	out, err := s.Neighbors(a, app.RelationDirectionOutbound)
	if err != nil {
		t.Fatalf("out: %v", err)
	}
	if len(out) != 1 || out[0].TargetID != b {
		t.Fatalf("outbound from a: got %+v", out)
	}
	in, err := s.Neighbors(a, app.RelationDirectionInbound)
	if err != nil {
		t.Fatalf("in: %v", err)
	}
	if len(in) != 1 || in[0].SourceID != c {
		t.Fatalf("inbound to a: got %+v", in)
	}
}

// TestRelations_OrderingDeterministic — task 3.3:
// Neighbors MUST return edges in (created_at ASC, id ASC) so callers
// can render a stable history without sorting on their side.
func TestRelations_OrderingDeterministic(t *testing.T) {
	s := newTestStore(t)
	a, b := seedTwo(t, s)
	c, err := s.Save("charlie note", time.Now().UTC())
	if err != nil {
		t.Fatalf("seed c: %v", err)
	}
	t0 := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	if err := s.CreateRelation(a, c, "related", t0.Add(2*time.Second)); err != nil {
		t.Fatalf("rel ac: %v", err)
	}
	if err := s.CreateRelation(a, b, "refines", t0.Add(time.Second)); err != nil {
		t.Fatalf("rel ab: %v", err)
	}
	out, err := s.Neighbors(a, app.RelationDirectionOutbound)
	if err != nil {
		t.Fatalf("out: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 outbound, got %d", len(out))
	}
	// Earliest created_at first.
	if !sort.SliceIsSorted(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	}) {
		t.Fatalf("expected created_at ASC ordering, got: %+v", out)
	}
}
