package store

import (
	"path/filepath"
	"testing"
	"time"

	"nt-cli/internal/app"
)

func TestInit_ProjectBackfillSafetyScenarios(t *testing.T) {
	t.Run("new db creates default project and active pointer", func(t *testing.T) {
		s, _ := newMetaStore(t)

		var defaultCount int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE name = 'default'`).Scan(&defaultCount); err != nil {
			t.Fatalf("count default project: %v", err)
		}
		if defaultCount != 1 {
			t.Fatalf("expected exactly one default project, got %d", defaultCount)
		}

		var nullCount int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM memory_items WHERE project_id IS NULL`).Scan(&nullCount); err != nil {
			t.Fatalf("count null project rows: %v", err)
		}
		if nullCount != 0 {
			t.Fatalf("new db must not leave NULL project_id rows, got %d", nullCount)
		}

		var activeCount int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM active_project WHERE id = 1`).Scan(&activeCount); err != nil {
			t.Fatalf("count active_project row: %v", err)
		}
		if activeCount != 1 {
			t.Fatalf("expected active_project singleton row, got %d", activeCount)
		}
	})

	t.Run("legacy null rows backfill to default", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "legacy-null.db")

		old, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("open legacy db: %v", err)
		}
		if _, err := old.db.Exec(`
			CREATE TABLE memory_items (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				content TEXT NOT NULL,
				created_at DATETIME NOT NULL,
				updated_at DATETIME,
				title TEXT,
				type TEXT,
				topic_key TEXT,
				scope TEXT,
				project_id INTEGER
			);
		`); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
		stamp := time.Now().UTC().Format(time.RFC3339)
		if _, err := old.db.Exec(
			`INSERT INTO memory_items(content, created_at, updated_at, topic_key, scope, project_id)
			 VALUES(?, ?, ?, ?, ?, NULL)`,
			"legacy row", stamp, stamp, "legacy/topic", "ad373971",
		); err != nil {
			t.Fatalf("insert legacy null row: %v", err)
		}
		if err := old.Close(); err != nil {
			t.Fatalf("close legacy db: %v", err)
		}

		upgraded, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("reopen upgraded db: %v", err)
		}
		defer upgraded.Close()
		if err := upgraded.Init(); err != nil {
			t.Fatalf("Init: %v", err)
		}

		var defaultID int64
		if err := upgraded.db.QueryRow(`SELECT id FROM projects WHERE name = 'default'`).Scan(&defaultID); err != nil {
			t.Fatalf("read default id: %v", err)
		}

		var rowProjectID int64
		if err := upgraded.db.QueryRow(
			`SELECT project_id FROM memory_items WHERE topic_key = 'legacy/topic'`,
		).Scan(&rowProjectID); err != nil {
			t.Fatalf("read row project_id: %v", err)
		}
		if rowProjectID != defaultID {
			t.Fatalf("expected backfill to default project id %d, got %d", defaultID, rowProjectID)
		}
	})

	t.Run("already migrated rerun is idempotent and backfills late null rows", func(t *testing.T) {
		s, _ := newMetaStore(t)

		legacyNullID, err := s.SaveWithMeta(app.SaveRequest{
			Content:   "late null row",
			TopicKey:  "migration/idempotent",
			Scope:     "ad373971",
			ProjectID: 0,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("insert late null row: %v", err)
		}

		if err := s.Init(); err != nil {
			t.Fatalf("first rerun Init: %v", err)
		}
		if err := s.Init(); err != nil {
			t.Fatalf("second rerun Init: %v", err)
		}

		var defaultID int64
		if err := s.db.QueryRow(`SELECT id FROM projects WHERE name = 'default'`).Scan(&defaultID); err != nil {
			t.Fatalf("read default id: %v", err)
		}

		var rowProjectID int64
		if err := s.db.QueryRow(`SELECT project_id FROM memory_items WHERE id = ?`, legacyNullID).Scan(&rowProjectID); err != nil {
			t.Fatalf("read backfilled id: %v", err)
		}
		if rowProjectID != defaultID {
			t.Fatalf("expected late null row backfilled to default id %d, got %d", defaultID, rowProjectID)
		}

		var projectsCount int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE name = 'default'`).Scan(&projectsCount); err != nil {
			t.Fatalf("count default projects: %v", err)
		}
		if projectsCount != 1 {
			t.Fatalf("idempotent reruns duplicated default project rows: %d", projectsCount)
		}
	})
}
