package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nt-cli/internal/app"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Init() error {
	// Create the base table for fresh installs. Includes updated_at so new
	// databases never need the migration path.
	create := `
	CREATE TABLE IF NOT EXISTS memory_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_memory_items_created_at ON memory_items(created_at DESC);
	`
	if _, err := s.db.Exec(create); err != nil {
		return err
	}

	// Backward-compatible migration: legacy databases created before
	// updated_at existed need an additive ALTER TABLE. SQLite has no
	// "ADD COLUMN IF NOT EXISTS", so we ignore the duplicate-column error.
	if _, err := s.db.Exec(`ALTER TABLE memory_items ADD COLUMN updated_at DATETIME`); err != nil {
		if !isDuplicateColumnErr(err) {
			return fmt.Errorf("migrate updated_at: %w", err)
		}
	}

	// Backfill: any pre-existing row with NULL updated_at gets created_at so
	// every read path can rely on a non-null timestamp.
	if _, err := s.db.Exec(
		`UPDATE memory_items SET updated_at = created_at WHERE updated_at IS NULL`,
	); err != nil {
		return fmt.Errorf("backfill updated_at: %w", err)
	}
	return nil
}

// isDuplicateColumnErr matches SQLite's "duplicate column name" error so
// repeated Init() calls remain idempotent.
func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

func (s *SQLiteStore) Save(content string, createdAt time.Time) (int64, error) {
	stamp := createdAt.Format(time.RFC3339)
	res, err := s.db.Exec(
		"INSERT INTO memory_items(content, created_at, updated_at) VALUES(?, ?, ?)",
		content,
		stamp,
		stamp,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// scanRow scans a single row in (id, content, created_at, updated_at) order.
func scanRow(scanner interface {
	Scan(dest ...any) error
}) (app.MemoryItem, error) {
	var (
		it          app.MemoryItem
		createdRaw  string
		updatedRaw  sql.NullString
	)
	if err := scanner.Scan(&it.ID, &it.Content, &createdRaw, &updatedRaw); err != nil {
		return app.MemoryItem{}, err
	}
	created, err := time.Parse(time.RFC3339, createdRaw)
	if err != nil {
		return app.MemoryItem{}, fmt.Errorf("invalid created_at in db: %w", err)
	}
	it.CreatedAt = created

	// Defensive: if a row somehow lacks updated_at after migration, fall back
	// to created_at so callers always get a usable timestamp.
	if updatedRaw.Valid && updatedRaw.String != "" {
		updated, err := time.Parse(time.RFC3339, updatedRaw.String)
		if err != nil {
			return app.MemoryItem{}, fmt.Errorf("invalid updated_at in db: %w", err)
		}
		it.UpdatedAt = updated
	} else {
		it.UpdatedAt = created
	}
	return it, nil
}

func (s *SQLiteStore) Get(id int64) (app.MemoryItem, error) {
	row := s.db.QueryRow(
		`SELECT id, content, created_at, updated_at
		 FROM memory_items
		 WHERE id = ?`,
		id,
	)
	it, err := scanRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return app.MemoryItem{}, app.ErrNotFound
		}
		return app.MemoryItem{}, err
	}
	return it, nil
}

func (s *SQLiteStore) Update(id int64, content string, updatedAt time.Time) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE memory_items SET content = ?, updated_at = ? WHERE id = ?`,
		content,
		updatedAt.Format(time.RFC3339),
		id,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *SQLiteStore) Recall(query string, limit int) ([]app.MemoryItem, error) {
	like := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(
		`SELECT id, content, created_at, updated_at
		 FROM memory_items
		 WHERE LOWER(content) LIKE ?
		 ORDER BY datetime(created_at) DESC
		 LIMIT ?`,
		like,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

func (s *SQLiteStore) List(limit int) ([]app.MemoryItem, error) {
	rows, err := s.db.Query(
		`SELECT id, content, created_at, updated_at
		 FROM memory_items
		 ORDER BY datetime(created_at) DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

func scanAll(rows *sql.Rows) ([]app.MemoryItem, error) {
	var items []app.MemoryItem
	for rows.Next() {
		it, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteStore) Delete(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM memory_items WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
