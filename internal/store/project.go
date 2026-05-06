package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nt-cli/internal/app"
)

// ErrProjectNotFound is returned by SetActive when the target project id
// is unknown. The store keeps the active pointer unchanged in that case.
var ErrProjectNotFound = errors.New("project not found")

// CreateProject inserts a new project row and returns the populated record.
// The caller chooses the name and fingerprint; created_at is stamped UTC.
//
// Uniqueness is enforced at the schema level on `name` — duplicate names
// surface the underlying SQLite constraint error to the caller.
func (s *SQLiteStore) CreateProject(in app.ProjectInput) (app.Project, error) {
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO projects(name, root_path, fingerprint, created_at)
		 VALUES(?, ?, ?, ?)`,
		in.Name, in.RootPath, in.Fingerprint, stamp,
	)
	if err != nil {
		return app.Project{}, fmt.Errorf("insert project: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return app.Project{}, err
	}
	return app.Project{
		ID:          id,
		Name:        in.Name,
		RootPath:    in.RootPath,
		Fingerprint: in.Fingerprint,
		CreatedAt:   now,
	}, nil
}

// GetActive returns the currently active project as recorded by the
// active_project singleton. The v5 migration guarantees this row exists
// (it is seeded to "default" during Init) so callers can rely on a
// non-zero result on a healthy DB.
func (s *SQLiteStore) GetActive() (app.Project, error) {
	var p app.Project
	var createdRaw string
	err := s.db.QueryRow(
		`SELECT p.id, p.name, p.root_path, p.fingerprint, p.created_at
		 FROM active_project ap
		 JOIN projects p ON p.id = ap.project_id
		 WHERE ap.id = 1`,
	).Scan(&p.ID, &p.Name, &p.RootPath, &p.Fingerprint, &createdRaw)
	if err != nil {
		return app.Project{}, fmt.Errorf("read active project: %w", err)
	}
	if t, perr := time.Parse(time.RFC3339, createdRaw); perr == nil {
		p.CreatedAt = t
	}
	return p, nil
}

// SetActive switches the active_project pointer to the given project id.
// Returns ErrProjectNotFound when the id does not exist; the pointer is
// left untouched in that case (the validation happens before the update).
func (s *SQLiteStore) SetActive(projectID int64) error {
	var exists int
	err := s.db.QueryRow(
		`SELECT 1 FROM projects WHERE id = ?`, projectID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProjectNotFound
	}
	if err != nil {
		return fmt.Errorf("verify project: %w", err)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE active_project SET project_id = ?, updated_at = ? WHERE id = 1`,
		projectID, stamp,
	); err != nil {
		return fmt.Errorf("update active project: %w", err)
	}
	return nil
}

// ListProjects returns all projects ordered by id ASC for deterministic
// iteration. The default project (id stamped at v5 migration) sorts first.
func (s *SQLiteStore) ListProjects() ([]app.Project, error) {
	rows, err := s.db.Query(
		`SELECT id, name, root_path, fingerprint, created_at
		 FROM projects ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []app.Project
	for rows.Next() {
		var p app.Project
		var createdRaw string
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.Fingerprint, &createdRaw); err != nil {
			return nil, err
		}
		if t, perr := time.Parse(time.RFC3339, createdRaw); perr == nil {
			p.CreatedAt = t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FindByRootPath returns all projects whose root_path is a prefix of (or equal
// to) cwd. This allows the probe engine to detect the "ambiguous" case where
// the working directory could belong to more than one registered project.
// An empty cwd returns an empty slice (no match).
func (s *SQLiteStore) FindByRootPath(cwd string) ([]app.Project, error) {
	if cwd == "" {
		return nil, nil
	}
	// Match projects where root_path is a path-prefix of cwd.
	// We use LIKE with a "/" separator to avoid false prefix matches
	// (e.g. "/foo" should not match "/foobar").
	// Two patterns cover: exact match OR prefix with trailing slash.
	rows, err := s.db.Query(
		`SELECT id, name, root_path, fingerprint, created_at
		 FROM projects
		 WHERE root_path != ''
		   AND (root_path = ? OR ? LIKE root_path || '/%')
		 ORDER BY id ASC`,
		cwd, cwd,
	)
	if err != nil {
		return nil, fmt.Errorf("find by root path: %w", err)
	}
	defer rows.Close()
	var out []app.Project
	for rows.Next() {
		var p app.Project
		var createdRaw string
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.Fingerprint, &createdRaw); err != nil {
			return nil, err
		}
		if t, perr := time.Parse(time.RFC3339, createdRaw); perr == nil {
			p.CreatedAt = t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FindByFingerprint returns the project whose fingerprint matches, or nil
// if no such project exists. An empty fingerprint is treated as "no match"
// to avoid colliding with the default project's empty fingerprint.
func (s *SQLiteStore) FindByFingerprint(fp string) (*app.Project, error) {
	if fp == "" {
		return nil, nil
	}
	var p app.Project
	var createdRaw string
	err := s.db.QueryRow(
		`SELECT id, name, root_path, fingerprint, created_at
		 FROM projects WHERE fingerprint = ? LIMIT 1`,
		fp,
	).Scan(&p.ID, &p.Name, &p.RootPath, &p.Fingerprint, &createdRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find project by fingerprint: %w", err)
	}
	if t, perr := time.Parse(time.RFC3339, createdRaw); perr == nil {
		p.CreatedAt = t
	}
	return &p, nil
}
