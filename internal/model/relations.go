package model

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
