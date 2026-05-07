package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInit_V5_FreshDB_CreatesDefaultProjectAndBackfills covers spec scenario
// "Fresh DB migration": after migrating to v5 from a pre-v5 DB, a `default`
// project MUST exist, every legacy memory row MUST have project_id pointing
// to it, and `active_project` MUST point at `default`.
func TestInit_V5_FreshDB_CreatesDefaultProjectAndBackfills(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	backupDir := filepath.Join(dir, "backups")

	// Seed a legacy DB at the v4 surface (memory_items present, no projects).
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
		t.Fatalf("seed legacy schema: %v", err)
	}
	created := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	for _, content := range []string{"row a", "row b", "row c"} {
		if _, err := old.db.Exec(
			`INSERT INTO memory_items(content, created_at, updated_at) VALUES(?, ?, ?)`,
			content, created, created,
		); err != nil {
			t.Fatalf("seed row: %v", err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old: %v", err)
	}

	// Migrate to v5.
	upgraded, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	upgraded.SetBackupDir(backupDir)
	defer upgraded.Close()
	if err := upgraded.Init(); err != nil {
		t.Fatalf("Init to v5: %v", err)
	}

	v, err := upgraded.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Fatalf("expected schema_version %d, got %d", CurrentSchemaVersion, v)
	}

	// projects table must contain exactly one row named "default".
	var count int
	if err := upgraded.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 project after backfill, got %d", count)
	}
	var projID int64
	var projName string
	if err := upgraded.db.QueryRow(
		`SELECT id, name FROM projects WHERE name = 'default'`,
	).Scan(&projID, &projName); err != nil {
		t.Fatalf("read default project: %v", err)
	}
	if projID <= 0 || projName != "default" {
		t.Fatalf("default project malformed: id=%d name=%q", projID, projName)
	}

	// active_project singleton must point at the default project.
	var activeID int64
	if err := upgraded.db.QueryRow(
		`SELECT project_id FROM active_project WHERE id = 1`,
	).Scan(&activeID); err != nil {
		t.Fatalf("read active_project: %v", err)
	}
	if activeID != projID {
		t.Fatalf("expected active_project.project_id=%d, got %d", projID, activeID)
	}

	// Every memory_items row must be backfilled to project_id = default.id.
	var nullCount int
	if err := upgraded.db.QueryRow(
		`SELECT COUNT(*) FROM memory_items WHERE project_id IS NULL`,
	).Scan(&nullCount); err != nil {
		t.Fatalf("null project_id count: %v", err)
	}
	if nullCount != 0 {
		t.Fatalf("expected 0 rows with NULL project_id after backfill, got %d", nullCount)
	}
	var matchCount int
	if err := upgraded.db.QueryRow(
		`SELECT COUNT(*) FROM memory_items WHERE project_id = ?`, projID,
	).Scan(&matchCount); err != nil {
		t.Fatalf("backfilled count: %v", err)
	}
	if matchCount != 3 {
		t.Fatalf("expected 3 backfilled rows, got %d", matchCount)
	}

	// Index on memory_items.project_id must exist for query planner.
	var idxName string
	err = upgraded.db.QueryRow(
		`SELECT name FROM sqlite_master
		 WHERE type='index' AND tbl_name='memory_items' AND name='idx_memory_items_project_id'`,
	).Scan(&idxName)
	if err != nil {
		t.Fatalf("expected idx_memory_items_project_id index: %v", err)
	}

	// Idempotent re-run.
	if err := upgraded.Init(); err != nil {
		t.Fatalf("Init idempotent: %v", err)
	}
	if err := upgraded.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		t.Fatalf("count projects post idempotent: %v", err)
	}
	if count != 1 {
		t.Fatalf("idempotent Init duplicated default project: count=%d", count)
	}
}

// TestInit_V5_SnapshotFailureAbortsMigration covers spec scenario
// "Pre-migration safety": when the pre-migration snapshot fails, migration
// MUST abort atomically and the DB MUST remain at its pre-v5 state.
func TestInit_V5_SnapshotFailureAbortsMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	// Seed a legacy DB at v4 surface so snapshotForMigration is invoked.
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
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := old.db.Exec(
		`INSERT INTO memory_items(content, created_at, updated_at) VALUES('x', ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old: %v", err)
	}

	// Point backupDir at a path that cannot be created (a regular file).
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	bogusBackupDir := filepath.Join(blocker, "backups") // mkdir under a file → ENOTDIR

	upgraded, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	upgraded.SetBackupDir(bogusBackupDir)
	defer upgraded.Close()

	err = upgraded.Init()
	if err == nil {
		t.Fatalf("expected Init to fail when snapshot path is unwriteable")
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("expected snapshot error, got: %v", err)
	}

	// projects table MUST NOT exist — migration aborted before any DDL.
	var name string
	row := upgraded.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='projects'`,
	)
	if scanErr := row.Scan(&name); scanErr == nil {
		t.Fatalf("projects table must not exist after aborted migration")
	}
	// schema_version must NOT be 5.
	v, _ := upgraded.SchemaVersion()
	if v == CurrentSchemaVersion {
		t.Fatalf("schema_version must not advance to %d on aborted migration", CurrentSchemaVersion)
	}
}
