package store

import (
	"database/sql"
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
	query := `
	CREATE TABLE IF NOT EXISTS memory_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_memory_items_created_at ON memory_items(created_at DESC);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *SQLiteStore) Save(content string, createdAt time.Time) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO memory_items(content, created_at) VALUES(?, ?)",
		content,
		createdAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) Recall(query string, limit int) ([]app.MemoryItem, error) {
	like := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(
		`SELECT id, content, created_at
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

	var items []app.MemoryItem
	for rows.Next() {
		var it app.MemoryItem
		var createdRaw string
		if err := rows.Scan(&it.ID, &it.Content, &createdRaw); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339, createdRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp in db: %w", err)
		}
		it.CreatedAt = parsed
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteStore) List(limit int) ([]app.MemoryItem, error) {
	rows, err := s.db.Query(
		`SELECT id, content, created_at
		 FROM memory_items
		 ORDER BY datetime(created_at) DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []app.MemoryItem
	for rows.Next() {
		var it app.MemoryItem
		var createdRaw string
		if err := rows.Scan(&it.ID, &it.Content, &createdRaw); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339, createdRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp in db: %w", err)
		}
		it.CreatedAt = parsed
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
