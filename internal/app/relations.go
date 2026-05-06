package app

import "time"

// RelationDirection selects how Neighbors traverses memory_relations.
// Outbound returns edges where the queried id is the source; Inbound
// returns edges where it is the target. Kept as an explicit enum (not a
// bool) so future call sites — e.g. an "any" or "both" mode added in a
// later milestone — extend without breaking existing callers.
type RelationDirection int

const (
	// RelationDirectionOutbound lists edges where source_id == queried id.
	RelationDirectionOutbound RelationDirection = iota
	// RelationDirectionInbound lists edges where target_id == queried id.
	RelationDirectionInbound
)

// MemoryRelation is one row of the memory_relations table — a typed,
// directed edge between two memory_items. Mirrors the store-layer
// projection so callers consume a single shape regardless of traversal
// direction. CreatedAt is UTC.
type MemoryRelation struct {
	ID           int64
	SourceID     int64
	TargetID     int64
	RelationType string
	CreatedAt    time.Time
}

// AllowedRelationTypes is the curated whitelist of relation_type values
// accepted by the v4 schema CHECK constraint. Service- and MCP-layer
// validation reuse this list so all rejection paths return the same
// human-friendly error text.
//
// Order is stable for deterministic error messages and registry output.
var AllowedRelationTypes = []string{
	"related",
	"supersedes",
	"conflicts_with",
	"refines",
	"depends_on",
}

// IsAllowedRelationType reports whether t is a member of
// AllowedRelationTypes. Centralising the check keeps service/MCP guards
// in lockstep with the DB constraint.
func IsAllowedRelationType(t string) bool {
	for _, v := range AllowedRelationTypes {
		if v == t {
			return true
		}
	}
	return false
}

// RelationStore extends Store with the v4 typed-graph operations.
// Optional capability — same defensive type-assert pattern as
// MetadataStore / SessionStore so legacy fakes that don't implement
// the graph surface still compile against Store.
type RelationStore interface {
	CreateRelation(sourceID, targetID int64, relationType string, at time.Time) error
	Neighbors(id int64, dir RelationDirection) ([]MemoryRelation, error)
}
