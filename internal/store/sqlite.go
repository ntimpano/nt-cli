package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nt-cli/internal/app"

	_ "modernc.org/sqlite"
)

// CurrentSchemaVersion is the migration target enforced by Init. Increment
// this constant only when adding a new forward-only migration step.
//
// v1: M1 structured-memory columns (title/type/topic_key/scope) + indexes.
// v2: M2 ranked-recall — memory_fts virtual table + sync triggers.
// v3: M3 session lifecycle log — `sessions` table for session_workflow.
const CurrentSchemaVersion = 3

type SQLiteStore struct {
	db        *sql.DB
	dbPath    string
	backupDir string
	// useFTS is set at Init() time to true when the FTS5 module is
	// available AND the memory_fts table is healthy. Recall() checks this
	// flag once per call and downgrades to LIKE on the fly if a query
	// against memory_fts fails (e.g. corruption, manual DROP).
	useFTS bool
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &SQLiteStore{
		db:        db,
		dbPath:    path,
		backupDir: defaultBackupDir(),
	}, nil
}

// SetBackupDir overrides the directory used for pre-migration snapshots.
// Tests use this to keep snapshots inside a t.TempDir; the production
// default is `~/.nt-cli/backups/`.
func (s *SQLiteStore) SetBackupDir(dir string) {
	s.backupDir = dir
}

// defaultBackupDir resolves `~/.nt-cli/backups` lazily so tests that never
// hit migration do not need a writable HOME.
func defaultBackupDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		if h := strings.TrimSpace(os.Getenv("HOME")); h != "" {
			home = h
		} else {
			home = os.TempDir()
		}
	}
	return filepath.Join(home, ".nt-cli", "backups")
}

func (s *SQLiteStore) Init() error {
	// Bootstrap: ensure schema_version exists so migration state is queryable
	// regardless of whether this is a fresh install or a legacy DB.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	current, err := s.SchemaVersion()
	if err != nil {
		return err
	}

	// Detect whether this DB pre-existed M1 (legacy memory_items present but
	// schema_version still 0). Snapshot the file BEFORE applying migrations
	// so a failed run is recoverable from disk.
	legacy, err := s.hasLegacyTable()
	if err != nil {
		return err
	}
	needsMigration := current < CurrentSchemaVersion && legacy
	if needsMigration {
		if err := s.snapshotForMigration(); err != nil {
			return fmt.Errorf("pre-migration snapshot: %w", err)
		}
	}

	// Apply M1 schema in a transaction so a mid-migration failure leaves
	// schema_version unchanged. The CREATE TABLE IF NOT EXISTS keeps fresh
	// installs additive too.
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	commit := false
	defer func() {
		if !commit {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS memory_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME,
			title TEXT,
			type TEXT,
			topic_key TEXT,
			scope TEXT
		);
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`CREATE INDEX IF NOT EXISTS idx_memory_items_created_at ON memory_items(created_at DESC)`,
	); err != nil {
		return err
	}

	// Drop any stale FTS sync triggers BEFORE running the column backfill
	// UPDATE below. If a previous process left triggers behind but
	// memory_fts was dropped (corruption, manual recovery, downgrade),
	// the backfill UPDATE would fire those triggers and crash with
	// "no such table: memory_fts". Doing this here keeps the migration
	// reentrant in the face of partially broken installs.
	for _, drop := range []string{
		`DROP TRIGGER IF EXISTS memory_items_ai`,
		`DROP TRIGGER IF EXISTS memory_items_ad`,
		`DROP TRIGGER IF EXISTS memory_items_au`,
	} {
		if _, err := tx.Exec(drop); err != nil {
			return fmt.Errorf("clean stale fts triggers: %w", err)
		}
	}

	// Backwards-compat ALTERs: SQLite has no IF NOT EXISTS for ADD COLUMN,
	// so we tolerate the duplicate-column error per migration step.
	for _, alter := range []string{
		`ALTER TABLE memory_items ADD COLUMN updated_at DATETIME`,
		`ALTER TABLE memory_items ADD COLUMN title TEXT`,
		`ALTER TABLE memory_items ADD COLUMN type TEXT`,
		`ALTER TABLE memory_items ADD COLUMN topic_key TEXT`,
		`ALTER TABLE memory_items ADD COLUMN scope TEXT`,
		`ALTER TABLE memory_items ADD COLUMN content_hash TEXT`,
	} {
		if _, err := tx.Exec(alter); err != nil {
			if !isDuplicateColumnErr(err) {
				return fmt.Errorf("migrate columns: %w", err)
			}
		}
	}

	if _, err := tx.Exec(
		`UPDATE memory_items SET updated_at = created_at WHERE updated_at IS NULL`,
	); err != nil {
		return fmt.Errorf("backfill updated_at: %w", err)
	}

	// Topic-key lookup index — used by the application-level upsert path.
	if _, err := tx.Exec(
		`CREATE INDEX IF NOT EXISTS idx_memory_items_topic ON memory_items(topic_key, scope)`,
	); err != nil {
		return err
	}

	// M3 — session lifecycle log. Each row is one of "start" / "summary"
	// / "end" tagged with a session_id. Kept as an append-only log
	// (no UPDATE) so re-running session start/end doesn't mutate
	// historical rows. Indexed by session_id for fast SessionEvents().
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			summary TEXT,
			created_at DATETIME NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("create sessions: %w", err)
	}
	if _, err := tx.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sessions_session_id ON sessions(session_id, id)`,
	); err != nil {
		return err
	}

	// M3 — content_hash column on memory_items powers the import dedupe
	// key `(topic_key, content_hash)`. Column was added with the rest of
	// the additive ALTERs above; the index is created here so it lives
	// inside the v3 migration step alongside the sessions table.
	if _, err := tx.Exec(
		`CREATE INDEX IF NOT EXISTS idx_memory_items_import_dedupe
		 ON memory_items(topic_key, content_hash)`,
	); err != nil {
		return err
	}

	// M2 — FTS5 ranked recall. The virtual table mirrors (content, title)
	// and is kept in sync by AFTER INSERT/UPDATE/DELETE triggers. Triggers
	// reference the rowid as `memory_items.id` so bm25 ranks map cleanly
	// back to the canonical row id without an extra join.
	//
	// We tolerate FTS5 not being compiled into the SQLite build: a CREATE
	// VIRTUAL TABLE failure flips useFTS=false and Recall() degrades to
	// LIKE for the lifetime of the process. Other migration steps must
	// still succeed so the rest of the M1 surface keeps working.
	ftsHealthy := true
	if _, err := tx.Exec(
		`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			content,
			title,
			content='memory_items',
			content_rowid='id',
			tokenize='unicode61'
		)`,
	); err != nil {
		ftsHealthy = false
	}
	if ftsHealthy {
		// Sync triggers. content_rowid='id' makes memory_fts a contentless
		// (external-content) FTS table; we use the documented insert/delete
		// dance for UPDATEs to keep the index aligned with row revisions.
		for _, stmt := range []string{
			`CREATE TRIGGER IF NOT EXISTS memory_items_ai AFTER INSERT ON memory_items BEGIN
				INSERT INTO memory_fts(rowid, content, title) VALUES (new.id, new.content, COALESCE(new.title, ''));
			END`,
			`CREATE TRIGGER IF NOT EXISTS memory_items_ad AFTER DELETE ON memory_items BEGIN
				INSERT INTO memory_fts(memory_fts, rowid, content, title) VALUES('delete', old.id, old.content, COALESCE(old.title, ''));
			END`,
			`CREATE TRIGGER IF NOT EXISTS memory_items_au AFTER UPDATE ON memory_items BEGIN
				INSERT INTO memory_fts(memory_fts, rowid, content, title) VALUES('delete', old.id, old.content, COALESCE(old.title, ''));
				INSERT INTO memory_fts(rowid, content, title) VALUES (new.id, new.content, COALESCE(new.title, ''));
			END`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				ftsHealthy = false
				break
			}
		}
	}
	if ftsHealthy {
		// Backfill: rebuild the FTS index from memory_items so any rows
		// inserted under v1 (or imported via DB swap) become searchable
		// without requiring callers to re-save them.
		if _, err := tx.Exec(
			`INSERT INTO memory_fts(memory_fts) VALUES('rebuild')`,
		); err != nil {
			ftsHealthy = false
		}
	}
	s.useFTS = ftsHealthy

	// Stamp schema_version exactly once per upgrade so re-running Init is
	// idempotent (no duplicate snapshot, no row churn).
	if current < CurrentSchemaVersion {
		if _, err := tx.Exec(
			`INSERT INTO schema_version(version, applied_at) VALUES(?, ?)`,
			CurrentSchemaVersion, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("record schema_version: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	commit = true
	return nil
}

// SchemaVersion returns the highest applied schema version, or 0 if no
// migrations have ever been recorded.
func (s *SQLiteStore) SchemaVersion() (int, error) {
	var v sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("read schema_version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// hasLegacyTable reports whether memory_items already exists, which signals
// "this is a pre-M1 install" when combined with schema_version = 0.
func (s *SQLiteStore) hasLegacyTable() (bool, error) {
	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='memory_items'`,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return name == "memory_items", nil
}

// snapshotForMigration copies the current DB file into backupDir under a
// timestamped name BEFORE the M1 migration runs. The copy uses a streaming
// io.Copy so large databases are handled without loading them into memory.
func (s *SQLiteStore) snapshotForMigration() error {
	if err := os.MkdirAll(s.backupDir, 0o755); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	dst := filepath.Join(s.backupDir, fmt.Sprintf("pre-migration-%s.db", stamp))

	// Flush WAL/journal state so the copied file is a complete snapshot.
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		// Non-fatal: not all journal modes support checkpoint. Continue.
		_ = err
	}

	src, err := os.Open(s.dbPath)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return err
	}
	return out.Sync()
}

// isDuplicateColumnErr matches SQLite's "duplicate column name" error so
// repeated ALTER TABLE calls remain idempotent across Init() runs.
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

// SaveWithMeta persists a row with optional structured metadata. When
// req.TopicKey is non-empty the call performs an application-level upsert:
// the latest matching (scope, topic_key) row is UPDATEd in place; otherwise
// a new row is INSERTed. The application-level path was chosen over a unique
// DB constraint to keep future history/versioning flexibility open per
// design.md §FTS+Topic strategy.
func (s *SQLiteStore) SaveWithMeta(req app.SaveRequest) (int64, error) {
	stamp := req.CreatedAt.Format(time.RFC3339)

	if strings.TrimSpace(req.TopicKey) != "" {
		var existingID int64
		err := s.db.QueryRow(
			`SELECT id FROM memory_items
			 WHERE topic_key = ? AND COALESCE(scope, '') = COALESCE(?, '')
			 ORDER BY datetime(updated_at) DESC, id DESC
			 LIMIT 1`,
			req.TopicKey, req.Scope,
		).Scan(&existingID)
		if err == nil {
			if _, err := s.db.Exec(
				`UPDATE memory_items
				 SET content = ?, updated_at = ?, title = ?, type = ?
				 WHERE id = ?`,
				req.Content, stamp, req.Title, req.Type, existingID,
			); err != nil {
				return 0, err
			}
			return existingID, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}

	res, err := s.db.Exec(
		`INSERT INTO memory_items(content, created_at, updated_at, title, type, topic_key, scope)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		req.Content, stamp, stamp,
		req.Title, req.Type, req.TopicKey, req.Scope,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// scanRow scans a single row in
// (id, content, created_at, updated_at, title, type, topic_key, scope) order.
func scanRow(scanner interface {
	Scan(dest ...any) error
}) (app.MemoryItem, error) {
	var (
		it         app.MemoryItem
		createdRaw string
		updatedRaw sql.NullString
		title      sql.NullString
		typ        sql.NullString
		topicKey   sql.NullString
		scope      sql.NullString
	)
	if err := scanner.Scan(
		&it.ID, &it.Content, &createdRaw, &updatedRaw,
		&title, &typ, &topicKey, &scope,
	); err != nil {
		return app.MemoryItem{}, err
	}
	created, err := time.Parse(time.RFC3339, createdRaw)
	if err != nil {
		return app.MemoryItem{}, fmt.Errorf("invalid created_at in db: %w", err)
	}
	it.CreatedAt = created

	if updatedRaw.Valid && updatedRaw.String != "" {
		updated, err := time.Parse(time.RFC3339, updatedRaw.String)
		if err != nil {
			return app.MemoryItem{}, fmt.Errorf("invalid updated_at in db: %w", err)
		}
		it.UpdatedAt = updated
	} else {
		it.UpdatedAt = created
	}
	it.Title = title.String
	it.Type = typ.String
	it.TopicKey = topicKey.String
	it.Scope = scope.String
	return it, nil
}

const selectColumns = `id, content, created_at, updated_at, title, type, topic_key, scope`

func (s *SQLiteStore) Get(id int64) (app.MemoryItem, error) {
	row := s.db.QueryRow(
		`SELECT `+selectColumns+`
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
	if s.useFTS {
		items, err := s.recallFTS(query, limit)
		if err == nil {
			return items, nil
		}
		// FTS query failed mid-flight (e.g. memory_fts dropped or
		// corrupted after Init). Mark FTS unhealthy for the rest of the
		// process and fall through to LIKE so the caller still gets a
		// result instead of an error per spec scenario "FTS unavailable
		// falls back to LIKE".
		s.useFTS = false
	}
	return s.recallLIKE(query, limit)
}

// RecallFiltered runs a recall with optional metadata + date-range
// filters. Empty/zero-valued filter fields are treated as unbounded so the
// same code path supports plain queries and fully-faceted queries without
// branching at the call site.
//
// Filter semantics (all AND-combined):
//   - Type: exact match on memory_items.type when non-empty.
//   - Since/Until: inclusive bounds on created_at; zero-time = unbounded.
//   - Limit: SQL LIMIT clause; the caller is responsible for defaults.
//
// FTS path is preferred when healthy; on FTS failure we transparently
// downgrade to LIKE and disable useFTS for the rest of the process —
// same resilience contract as Recall().
func (s *SQLiteStore) RecallFiltered(opts app.RecallOptions) ([]app.MemoryItem, error) {
	if s.useFTS {
		items, err := s.recallFTSFiltered(opts)
		if err == nil {
			return items, nil
		}
		s.useFTS = false
	}
	return s.recallLIKEFiltered(opts)
}

// Context returns the most recent N rows newest-first, optionally narrowed
// by scope. Empty scope disables the scope filter. Ordering is
// created_at DESC, falling back to id DESC for ties — matching the legacy
// List() ordering so callers see consistent recency semantics.
func (s *SQLiteStore) Context(n int, scope string) ([]app.MemoryItem, error) {
	scope = strings.TrimSpace(scope)
	var (
		rows *sql.Rows
		err  error
	)
	if scope == "" {
		rows, err = s.db.Query(
			`SELECT `+selectColumns+`
			 FROM memory_items
			 ORDER BY datetime(created_at) DESC, id DESC
			 LIMIT ?`,
			n,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT `+selectColumns+`
			 FROM memory_items
			 WHERE COALESCE(scope, '') = ?
			 ORDER BY datetime(created_at) DESC, id DESC
			 LIMIT ?`,
			scope, n,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// UseFTS reports whether ranked recall is currently active. It returns
// false when the FTS5 module was unavailable at Init() OR when an earlier
// recall failed and the store downgraded itself to LIKE for the rest of
// the process. Doctor/health surfaces (M3) read this flag.
func (s *SQLiteStore) UseFTS() bool {
	return s.useFTS
}

// recallFTS executes a bm25-ranked query against memory_fts and joins back
// to memory_items for full row reconstruction. The MATCH expression is
// rebuilt from the user query as a prefix-OR clause so casual keyword input
// (e.g. "fts5 ranking") still matches without forcing callers to learn
// FTS5 query syntax.
func (s *SQLiteStore) recallFTS(query string, limit int) ([]app.MemoryItem, error) {
	match := buildFTSMatch(query)
	if match == "" {
		// Nothing tokenizable — defer to LIKE so the caller still gets a
		// meaningful behavior on punctuation-only input.
		return s.recallLIKE(query, limit)
	}
	rows, err := s.db.Query(
		`SELECT mi.id, mi.content, mi.created_at, mi.updated_at,
		        mi.title, mi.type, mi.topic_key, mi.scope
		 FROM memory_fts
		 JOIN memory_items mi ON mi.id = memory_fts.rowid
		 WHERE memory_fts MATCH ?
		 ORDER BY bm25(memory_fts)
		 LIMIT ?`,
		match, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// recallLIKE is the resilient fallback path used when FTS is unavailable
// or returns an error. It preserves the legacy created_at-DESC ordering
// because LIKE has no relevance signal — newest-first is the least
// surprising default.
func (s *SQLiteStore) recallLIKE(query string, limit int) ([]app.MemoryItem, error) {
	like := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(
		`SELECT `+selectColumns+`
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

// recallFTSFiltered is recallFTS plus optional metadata + date filters
// applied as additional WHERE clauses against memory_items. The MATCH
// stays inside memory_fts so bm25 ranking is preserved; type/since/until
// are joined-table predicates.
func (s *SQLiteStore) recallFTSFiltered(opts app.RecallOptions) ([]app.MemoryItem, error) {
	match := buildFTSMatch(opts.Query)
	if match == "" {
		return s.recallLIKEFiltered(opts)
	}
	clauses := []string{"memory_fts MATCH ?"}
	args := []any{match}
	clauses, args = appendMetadataFilters(clauses, args, opts)

	q := `SELECT mi.id, mi.content, mi.created_at, mi.updated_at,
	             mi.title, mi.type, mi.topic_key, mi.scope
	      FROM memory_fts
	      JOIN memory_items mi ON mi.id = memory_fts.rowid
	      WHERE ` + strings.Join(clauses, " AND ") + `
	      ORDER BY bm25(memory_fts)
	      LIMIT ?`
	args = append(args, opts.Limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// recallLIKEFiltered is the resilient fallback for filter-aware recall.
// Uses LOWER(content) LIKE plus the same metadata predicates. Order is
// created_at DESC so newest-first matches the unfiltered LIKE path.
func (s *SQLiteStore) recallLIKEFiltered(opts app.RecallOptions) ([]app.MemoryItem, error) {
	like := "%" + strings.ToLower(opts.Query) + "%"
	clauses := []string{"LOWER(content) LIKE ?"}
	args := []any{like}
	clauses, args = appendMetadataFilters(clauses, args, opts)

	q := `SELECT ` + selectColumns + `
	      FROM memory_items
	      WHERE ` + strings.Join(clauses, " AND ") + `
	      ORDER BY datetime(created_at) DESC
	      LIMIT ?`
	args = append(args, opts.Limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// appendMetadataFilters appends type/since/until predicates to the given
// WHERE clause list. The column references are unqualified so callers
// MUST ensure memory_items is the only table where those columns exist
// in the surrounding query (true for both filtered recall paths).
func appendMetadataFilters(clauses []string, args []any, opts app.RecallOptions) ([]string, []any) {
	if t := strings.TrimSpace(opts.Type); t != "" {
		clauses = append(clauses, "COALESCE(type, '') = ?")
		args = append(args, t)
	}
	if !opts.Since.IsZero() {
		clauses = append(clauses, "datetime(created_at) >= datetime(?)")
		args = append(args, opts.Since.UTC().Format(time.RFC3339))
	}
	if !opts.Until.IsZero() {
		clauses = append(clauses, "datetime(created_at) <= datetime(?)")
		args = append(args, opts.Until.UTC().Format(time.RFC3339))
	}
	return clauses, args
}

// buildFTSMatch sanitizes free-form user input into a safe FTS5 MATCH
// expression. Each whitespace-separated token is wrapped in double quotes
// and given a `*` prefix-match suffix so partial words still hit. Quotes
// in the input are stripped — FTS5 treats `"` as a phrase delimiter and
// passing it raw would let user input break the parser.
func buildFTSMatch(query string) string {
	fields := strings.Fields(strings.ReplaceAll(query, `"`, ""))
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf(`"%s"*`, f))
	}
	return strings.Join(parts, " OR ")
}

func (s *SQLiteStore) List(limit int) ([]app.MemoryItem, error) {
	rows, err := s.db.Query(
		`SELECT `+selectColumns+`
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

// SessionStart appends a "start" lifecycle row tagged with id. Empty/
// whitespace-only ids are rejected so unrelated sessions cannot be
// silently merged under an empty key.
func (s *SQLiteStore) SessionStart(id string, at time.Time) error {
	return s.appendSessionEvent(id, "start", "", at)
}

// SessionSummary appends a "summary" lifecycle row carrying free-form
// text. Multiple summaries per session are allowed (append-only log).
func (s *SQLiteStore) SessionSummary(id, summary string, at time.Time) error {
	return s.appendSessionEvent(id, "summary", summary, at)
}

// SessionEnd appends an "end" lifecycle row. Multiple ends are tolerated;
// the log captures whatever the caller wrote — interpretation is the
// reader's responsibility.
func (s *SQLiteStore) SessionEnd(id string, at time.Time) error {
	return s.appendSessionEvent(id, "end", "", at)
}

func (s *SQLiteStore) appendSessionEvent(id, kind, summary string, at time.Time) error {
	clean := strings.TrimSpace(id)
	if clean == "" {
		return errors.New("session id is empty")
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions(session_id, kind, summary, created_at) VALUES(?, ?, ?, ?)`,
		clean, kind, summary, at.UTC().Format(time.RFC3339),
	)
	return err
}

// SessionEvents returns every lifecycle row tagged to id, ordered by id
// ASC (insertion order). Empty result is not an error — callers see an
// empty slice for unknown session ids.
func (s *SQLiteStore) SessionEvents(id string) ([]app.SessionEvent, error) {
	clean := strings.TrimSpace(id)
	if clean == "" {
		return nil, errors.New("session id is empty")
	}
	rows, err := s.db.Query(
		`SELECT session_id, kind, COALESCE(summary, ''), created_at
		 FROM sessions
		 WHERE session_id = ?
		 ORDER BY id ASC`,
		clean,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []app.SessionEvent
	for rows.Next() {
		var ev app.SessionEvent
		var createdRaw string
		if err := rows.Scan(&ev.SessionID, &ev.Kind, &ev.Summary, &createdRaw); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, createdRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid sessions.created_at: %w", err)
		}
		ev.CreatedAt = t
		out = append(out, ev)
	}
	return out, rows.Err()
}

// ImportRecords inserts a batch of records, deduping each row on the
// composite key `(topic_key, sha256(content))`. Rows that match an
// existing key are counted as Skipped and not written. Empty content
// is also skipped (defensive — input parsers should already filter).
//
// The whole batch runs in a single transaction so a mid-batch failure
// rolls back cleanly. CreatedAt for new rows is stamped with time.Now
// at the store layer to avoid clock skew between caller and DB.
func (s *SQLiteStore) ImportRecords(rows []app.ImportRecord) (app.ImportResult, error) {
	res := app.ImportResult{}
	if len(rows) == 0 {
		return res, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range rows {
		content := r.Content
		if strings.TrimSpace(content) == "" {
			res.Skipped++
			continue
		}
		hash := contentHash(content)
		// Dedupe lookup. COALESCE so empty/null topic_key both compare
		// against the literal empty string, keeping pure-content imports
		// idempotent (spec scenario "Re-import produces no duplicates").
		var existing int64
		err := tx.QueryRow(
			`SELECT id FROM memory_items
			 WHERE COALESCE(topic_key, '') = COALESCE(?, '')
			   AND content_hash = ?
			 LIMIT 1`,
			r.TopicKey, hash,
		).Scan(&existing)
		if err == nil {
			res.Skipped++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return res, err
		}

		typ := r.Type
		if strings.TrimSpace(typ) == "" {
			typ = "manual"
		}
		scope := r.Scope
		if strings.TrimSpace(scope) == "" {
			scope = "project"
		}
		if _, err := tx.Exec(
			`INSERT INTO memory_items(content, created_at, updated_at, title, type, topic_key, scope, content_hash)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			content, now, now, r.Title, typ, r.TopicKey, scope, hash,
		); err != nil {
			return res, err
		}
		res.Inserted++
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

// contentHash returns the lowercase hex sha256 of the input. Used as
// the second component of the import dedupe key.
func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Backup writes a portable, self-contained snapshot of the live database
// to dst using SQLite's `VACUUM INTO` — atomic, schema-aware, and safe
// to run while connections are open. The destination file MUST NOT exist
// (VACUUM INTO refuses to overwrite); callers should pick a fresh path
// with a timestamp suffix per `pre-migration-<ts>.db` convention.
//
// Errors:
//   - parent directory missing  -> bubble up the OS error verbatim
//   - destination already exists -> SQLite error (caller should rename)
//   - any I/O fault              -> rollback is automatic; live DB untouched
func (s *SQLiteStore) Backup(dst string) error {
	if strings.TrimSpace(dst) == "" {
		return errors.New("backup destination is empty")
	}
	// Validate parent dir exists so we surface a clear error instead of
	// SQLite's terse "unable to open database file".
	if _, err := os.Stat(filepath.Dir(dst)); err != nil {
		return fmt.Errorf("backup destination dir: %w", err)
	}
	// VACUUM INTO requires a string literal; use parameter substitution
	// at the Go layer (path can contain quotes only on weird filesystems
	// — we escape defensively).
	escaped := strings.ReplaceAll(dst, "'", "''")
	if _, err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escaped)); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}

// Restore replaces the live database file with the contents of src by
// closing the active connection, copying bytes, and reopening. The
// caller is responsible for re-running Init() afterwards if the schema
// version of the artifact is older than CurrentSchemaVersion — Init is
// forward-only and will migrate transparently.
//
// Errors:
//   - src missing      -> os.Stat error before mutation
//   - copy failure     -> live DB is restored from a temp side-copy taken
//                         before the swap, so a half-failed restore does
//                         not leave the user with no DB at all
func (s *SQLiteStore) Restore(src string) error {
	if strings.TrimSpace(src) == "" {
		return errors.New("restore source is empty")
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("restore source: %w", err)
	}
	live := s.dbPath
	if strings.TrimSpace(live) == "" {
		return errors.New("store has no on-disk path; cannot restore")
	}
	// Take a side-copy of the live DB so we can roll back if the restore
	// copy fails mid-way. Same dir to avoid cross-FS rename.
	side := live + ".restore-bak"
	if err := copyFile(live, side); err != nil {
		// If the live file doesn't exist yet, that's OK — we'll create
		// it fresh from the artifact below.
		if !os.IsNotExist(err) {
			return fmt.Errorf("snapshot live db: %w", err)
		}
	}
	// Close the active connection so the file handle releases the OS
	// lock before we overwrite it (Windows would refuse otherwise; UNIX
	// is more lenient but locks can still cause readback weirdness).
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close live db: %w", err)
	}
	if err := copyFile(src, live); err != nil {
		// Roll back: put the side-copy back in place so the user isn't
		// left with a broken DB. Best-effort — log via err return only.
		_ = copyFile(side, live)
		_ = reopenStore(s)
		return fmt.Errorf("copy artifact: %w", err)
	}
	_ = os.Remove(side)
	return reopenStore(s)
}

// copyFile copies src to dst using a buffered io.Copy. dst is created
// with 0o644; the caller is responsible for ensuring the parent dir
// exists.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// reopenStore re-opens the underlying *sql.DB on s.dbPath so the store
// is usable after a Restore. Kept private — Backup/Restore are the only
// callers.
func reopenStore(s *SQLiteStore) error {
	db, err := sql.Open("sqlite", s.dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return fmt.Errorf("reopen sqlite: %w", err)
	}
	s.db = db
	return nil
}
