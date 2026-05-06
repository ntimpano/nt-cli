package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nt-cli/internal/app"
)

// newMetaStore returns a fresh SQLiteStore with a t.TempDir backup directory
// so migration snapshot tests do not pollute the user's HOME.
func newMetaStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	backupDir := filepath.Join(dir, "backups")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s.SetBackupDir(backupDir)
	if err := s.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, backupDir
}

// TestInit_AddsSchemaVersionTable proves spec scenario "Schema migration MUST
// be additive and safe": after Init the schema_version table exists and
// reports the current version.
func TestInit_AddsSchemaVersionTable(t *testing.T) {
	s, _ := newMetaStore(t)

	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion(): %v", err)
	}
	if v < 1 {
		t.Fatalf("expected schema_version >= 1, got %d", v)
	}
}

// TestInit_AddsMetadataColumns proves spec scenario "Save with full metadata
// persists fields": after Init the memory_items table carries the four new
// metadata columns.
func TestInit_AddsMetadataColumns(t *testing.T) {
	s, _ := newMetaStore(t)

	want := map[string]bool{"title": false, "type": false, "topic_key": false, "scope": false}
	rows, err := s.db.Query(`PRAGMA table_info(memory_items)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for col, ok := range want {
		if !ok {
			t.Fatalf("expected memory_items column %q after Init, missing", col)
		}
	}
}

// TestSaveWithMeta_PersistsAllFields proves spec scenario "Save with full
// metadata persists fields": every metadata field is round-tripped via Get.
func TestSaveWithMeta_PersistsAllFields(t *testing.T) {
	s, _ := newMetaStore(t)

	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	id, err := s.SaveWithMeta(app.SaveRequest{
		Content:   "decision body",
		Title:     "X",
		Type:      "decision",
		TopicKey:  "arch/auth",
		Scope:     "project",
		CreatedAt: created,
	})
	if err != nil {
		t.Fatalf("SaveWithMeta: %v", err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "X" {
		t.Fatalf("title: want %q got %q", "X", got.Title)
	}
	if got.Type != "decision" {
		t.Fatalf("type: want %q got %q", "decision", got.Type)
	}
	if got.TopicKey != "arch/auth" {
		t.Fatalf("topic_key: want %q got %q", "arch/auth", got.TopicKey)
	}
	if got.Scope != "project" {
		t.Fatalf("scope: want %q got %q", "project", got.Scope)
	}
	if got.Content != "decision body" {
		t.Fatalf("content: want %q got %q", "decision body", got.Content)
	}
}

// TestSaveWithMeta_BackwardCompatibleWithLegacySave proves spec scenario
// "Save without metadata stays backward-compatible": the legacy Save() path
// continues to insert rows with default/empty metadata.
func TestSaveWithMeta_BackwardCompatibleWithLegacySave(t *testing.T) {
	s, _ := newMetaStore(t)

	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	id, err := s.Save("legacy content", created)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "legacy content" {
		t.Fatalf("content drift: %q", got.Content)
	}
	if got.Title != "" || got.Type != "" || got.TopicKey != "" || got.Scope != "" {
		t.Fatalf("legacy Save must leave metadata empty, got %+v", got)
	}
}

// TestSaveWithMeta_TopicKeyUpsertKeepsSingleRow proves spec scenario
// "Same topic_key updates in place": saving twice with the same topic_key
// MUST keep row count = 1 and reflect the latest content.
func TestSaveWithMeta_TopicKeyUpsertKeepsSingleRow(t *testing.T) {
	s, _ := newMetaStore(t)

	t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	id1, err := s.SaveWithMeta(app.SaveRequest{
		Content: "first body", Type: "decision",
		TopicKey: "arch/auth", Scope: "project", CreatedAt: t1,
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	id2, err := s.SaveWithMeta(app.SaveRequest{
		Content: "second body", Type: "decision",
		TopicKey: "arch/auth", Scope: "project", CreatedAt: t2,
	})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("upsert should reuse same id: id1=%d id2=%d", id1, id2)
	}

	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_items WHERE topic_key = ? AND scope = ?`,
		"arch/auth", "project",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row for (topic_key,scope), got %d", count)
	}

	got, err := s.Get(id2)
	if err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got.Content != "second body" {
		t.Fatalf("expected latest content, got %q", got.Content)
	}
	if !got.UpdatedAt.Equal(t2) {
		t.Fatalf("expected updated_at=%s, got %s", t2, got.UpdatedAt)
	}
}

// TestSaveWithMeta_DifferentTopicKeyInsertsNewRow proves spec scenario
// "Different topic_key inserts new row".
func TestSaveWithMeta_DifferentTopicKeyInsertsNewRow(t *testing.T) {
	s, _ := newMetaStore(t)

	t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	id1, err := s.SaveWithMeta(app.SaveRequest{
		Content: "auth body", TopicKey: "arch/auth", Scope: "project", CreatedAt: t1,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	id2, err := s.SaveWithMeta(app.SaveRequest{
		Content: "db body", TopicKey: "arch/db", Scope: "project", CreatedAt: t1.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("different topic_key MUST produce different ids, got %d == %d", id1, id2)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memory_items`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

// TestSaveWithMeta_NoTopicKeyInsertsEachTime proves the upsert ONLY triggers
// when topic_key is non-empty — without it, every save inserts a new row.
func TestSaveWithMeta_NoTopicKeyInsertsEachTime(t *testing.T) {
	s, _ := newMetaStore(t)

	t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	id1, err := s.SaveWithMeta(app.SaveRequest{
		Content: "a", Type: "manual", Scope: "project", CreatedAt: t1,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	id2, err := s.SaveWithMeta(app.SaveRequest{
		Content: "b", Type: "manual", Scope: "project", CreatedAt: t1.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("no topic_key must insert distinct rows, got %d == %d", id1, id2)
	}
}

// TestInit_LegacyDBSnapshotsBackupBeforeMigration proves spec scenario
// "Migration on existing DB preserves rows": when migrating a legacy DB,
// nt-cli MUST write a pre-migration snapshot to backupDir before applying
// schema changes, and existing rows MUST remain readable.
func TestInit_LegacyDBSnapshotsBackupBeforeMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	backupDir := filepath.Join(dir, "backups")

	// Seed a legacy DB without metadata columns and without schema_version.
	old, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open old: %v", err)
	}
	if _, err := old.db.Exec(`
		CREATE TABLE memory_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME
		);
	`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	created := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	if _, err := old.db.Exec(
		`INSERT INTO memory_items(content, created_at, updated_at) VALUES(?, ?, ?)`,
		"legacy note", created.Format(time.RFC3339), created.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old: %v", err)
	}

	// Reopen with new code and run migration — backup MUST be written first.
	upgraded, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	upgraded.SetBackupDir(backupDir)
	defer upgraded.Close()
	if err := upgraded.Init(); err != nil {
		t.Fatalf("Init on legacy db: %v", err)
	}

	// Backup directory MUST contain at least one pre-migration-*.db file.
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backups: %v", err)
	}
	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "pre-migration-") && strings.HasSuffix(e.Name(), ".db") {
			found = true
			info, err := e.Info()
			if err == nil && info.Size() == 0 {
				t.Fatalf("snapshot %s exists but is empty", e.Name())
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected a pre-migration-*.db snapshot in %s, got %v", backupDir, entries)
	}

	// Existing row MUST remain readable.
	got, err := upgraded.Get(1)
	if err != nil {
		t.Fatalf("Get legacy row: %v", err)
	}
	if got.Content != "legacy note" {
		t.Fatalf("legacy content lost: %q", got.Content)
	}

	// Re-running Init MUST be idempotent and MUST NOT create another snapshot.
	beforeCount := len(entries)
	if err := upgraded.Init(); err != nil {
		t.Fatalf("Init idempotent: %v", err)
	}
	entries2, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backups 2: %v", err)
	}
	if len(entries2) != beforeCount {
		t.Fatalf("re-running Init must NOT create another snapshot: before=%d after=%d", beforeCount, len(entries2))
	}
}
