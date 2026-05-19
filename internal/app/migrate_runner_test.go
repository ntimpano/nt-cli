package app

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func seedSQLiteDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE demo(id INTEGER PRIMARY KEY, name TEXT);`); err != nil {
		t.Fatalf("create demo table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO demo(name) VALUES('migrated');`); err != nil {
		t.Fatalf("insert demo row: %v", err)
	}
}

func countDemoRows(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM demo`).Scan(&n); err != nil {
		t.Fatalf("count demo rows: %v", err)
	}
	return n
}

func TestRunMigrate_CopiesDBConfigAndProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldDir := filepath.Join(home, ".nt-cli")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old dir: %v", err)
	}
	seedSQLiteDB(t, filepath.Join(oldDir, "data.db"))
	if err := os.WriteFile(filepath.Join(oldDir, "config.json"), []byte(`{"version":"v1"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "profile.json"), []byte(`{"language":"es"}`), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	var out, errOut bytes.Buffer
	code := RunMigrate(nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected migrate success, code=%d stderr=%q", code, errOut.String())
	}

	newDir := filepath.Join(home, ".flint")
	if got := countDemoRows(t, filepath.Join(newDir, "flint.db")); got != 1 {
		t.Fatalf("expected migrated row count 1, got %d", got)
	}
	if _, err := os.Stat(filepath.Join(newDir, "config.json")); err != nil {
		t.Fatalf("expected copied config.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "profile.json")); err != nil {
		t.Fatalf("expected copied profile.json: %v", err)
	}
}

func TestRunMigrate_RefusesOverwriteWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldDir := filepath.Join(home, ".nt-cli")
	newDir := filepath.Join(home, ".flint")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}
	seedSQLiteDB(t, filepath.Join(oldDir, "data.db"))
	if err := os.WriteFile(filepath.Join(newDir, "flint.db"), []byte("occupied"), 0o644); err != nil {
		t.Fatalf("seed target db: %v", err)
	}

	var out, errOut bytes.Buffer
	code := RunMigrate(nil, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero without --force")
	}
}
