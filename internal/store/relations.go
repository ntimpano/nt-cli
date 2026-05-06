package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nt-cli/internal/app"
)

// CreateRelation inserts a typed directed edge from sourceID to targetID.
// The CHECK + FK constraints in the v4 migration enforce:
//   - source != target          (no self-loops)
//   - relation_type in whitelist (curated semantics)
//   - both ids must reference existing memory_items rows
//
// We deliberately let the DB raise these errors rather than duplicating
// the rules in Go — the schema is the single source of truth, and the
// service layer wraps the raw error with friendlier text for callers.
func (s *SQLiteStore) CreateRelation(sourceID, targetID int64, relationType string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO memory_relations(source_id, target_id, relation_type, created_at)
		 VALUES(?, ?, ?, ?)`,
		sourceID, targetID, relationType, at.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create relation: %w", err)
	}
	return nil
}

// Neighbors lists every edge incident to id in the requested direction,
// ordered by created_at ASC, id ASC for deterministic traversal.
//
// Outbound returns rows where source_id == id (forward links).
// Inbound  returns rows where target_id == id (back-links).
//
// An unknown id returns an empty slice — callers MUST NOT treat that
// as a not-found error (a node simply may have zero neighbors).
func (s *SQLiteStore) Neighbors(id int64, dir app.RelationDirection) ([]app.MemoryRelation, error) {
	var col string
	switch dir {
	case app.RelationDirectionOutbound:
		col = "source_id"
	case app.RelationDirectionInbound:
		col = "target_id"
	default:
		return nil, errors.New("unknown relation direction")
	}
	// Direct interpolation of `col` is safe here because the only values
	// it can take are the two literal column names above — never user
	// input. Keeping it as a constant string keeps the prepared-statement
	// path optimisable by SQLite.
	rows, err := s.db.Query(
		`SELECT id, source_id, target_id, relation_type, created_at
		 FROM memory_relations
		 WHERE `+col+` = ?
		 ORDER BY datetime(created_at) ASC, id ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []app.MemoryRelation
	for rows.Next() {
		var (
			r          app.MemoryRelation
			createdRaw string
		)
		if err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.RelationType, &createdRaw); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, createdRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid memory_relations.created_at: %w", err)
		}
		r.CreatedAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}

// DropRelationsSchema removes the memory_relations table and its indexes
// without touching memory_items. It is the documented down-migration
// path for v4 → v3: callers who need to roll back the graph schema run
// this and then either accept that schema_version stays at 4 (the row
// in schema_version is informational, not enforced) or manually
// `DELETE FROM schema_version WHERE version=4` outside this API.
//
// Re-running Init() afterwards re-creates the table additively, which
// is the round-trip property the migration test exercises.
//
// We do NOT delete the v4 row from schema_version here. schema_version
// is an audit log of what migrations have been applied; rewriting it
// would lose that history. A future "re-up" call sees max(version)=4
// already, finds the table missing, and CREATE TABLE IF NOT EXISTS
// recreates it as part of the v3 baseline migration step (Init() runs
// every CREATE TABLE IF NOT EXISTS unconditionally; only the
// schema_version stamp is gated on current<target).
func (s *SQLiteStore) DropRelationsSchema() error {
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_memory_relations_source`,
		`DROP INDEX IF EXISTS idx_memory_relations_target`,
		`DROP TABLE IF EXISTS memory_relations`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("drop relations schema: %w", err)
		}
	}
	return nil
}

// Compile-time guarantee: SQLiteStore satisfies the optional graph
// capability interface. If the method set ever drifts, the build breaks
// here instead of at the (rarer) MCP/service call site.
var _ app.RelationStore = (*SQLiteStore)(nil)

// Suppress unused-import lint when sql.ErrNoRows isn't referenced
// directly — kept available for future refinements (e.g. surfacing
// FK target-missing as ErrNotFound).
var _ = sql.ErrNoRows
