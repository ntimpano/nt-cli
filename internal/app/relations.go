package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

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

// Relate validates and persists a typed directed edge between two memory
// items. Validation runs at the service layer so every surface (CLI,
// MCP, future REST) returns the same friendly errors regardless of the
// backing store. The DB CHECK constraints remain the source of truth —
// these guards just catch the common cases earlier with better text.
//
// Errors:
//   - source/target id ≤0  -> "source id must be positive" / "target id must be positive"
//   - source == target     -> "self-loop is not allowed"
//   - empty/unknown type   -> "relation type ... must be one of <whitelist>"
//   - capability missing   -> "store does not support graph operations"
//   - store error          -> wrapped verbatim so callers can distinguish layers
func (s *Service) Relate(sourceID, targetID int64, relationType string) error {
	if sourceID <= 0 {
		return errors.New("source id must be positive")
	}
	if targetID <= 0 {
		return errors.New("target id must be positive")
	}
	if sourceID == targetID {
		return errors.New("self-loop is not allowed")
	}
	clean := strings.TrimSpace(relationType)
	if clean == "" {
		return fmt.Errorf("relation type is empty; must be one of %s", strings.Join(AllowedRelationTypes, ", "))
	}
	if !IsAllowedRelationType(clean) {
		return fmt.Errorf("relation type %q is not allowed; must be one of %s", clean, strings.Join(AllowedRelationTypes, ", "))
	}
	rs, ok := s.repo.(RelationStore)
	if !ok {
		return errors.New("store does not support graph operations")
	}
	return rs.CreateRelation(sourceID, targetID, clean, time.Now().UTC())
}

// Neighbors returns the typed edges incident to id in the requested
// direction. id must be positive; an unknown id is not an error and
// returns an empty slice (a node simply may have zero neighbors).
//
// Same defensive capability guard as Relate — a store that doesn't
// implement RelationStore fails fast rather than silently returning nil.
func (s *Service) Neighbors(id int64, dir RelationDirection) ([]MemoryRelation, error) {
	if id <= 0 {
		return nil, errors.New("id must be positive")
	}
	rs, ok := s.repo.(RelationStore)
	if !ok {
		return nil, errors.New("store does not support graph operations")
	}
	return rs.Neighbors(id, dir)
}

// SuggestRelations implements the spec scenario "Suggestions returned,
// none auto-applied" (capability: memory-graph, requirement: Auto-link
// MUST suggest on save). Given a candidate save request with a non-empty
// TopicKey, it returns up to 3 candidate edges (`relation_type=related`)
// pointing at existing rows that share topic / FTS overlap with the
// content.
//
// Contract:
//   - TopicKey empty            -> no candidates, no Recall call
//   - Returned MemoryRelation has ID=0 (proposal — not persisted) and
//     SourceID=0 (caller fills it in after the host save commits)
//   - The DB is NEVER mutated here; CreateRelation is not called
//   - Up to 3 suggestions, ranked by Recall order, deduped by TargetID
//
// Stronger relation types (supersedes/refines/depends_on/conflicts_with)
// stay opt-in; auto-link defaults to the safest verb so confirmation is
// a low-risk Yes/No for the operator.
func (s *Service) SuggestRelations(req SaveRequest) ([]MemoryRelation, error) {
	topic := strings.TrimSpace(req.TopicKey)
	if topic == "" {
		return nil, nil
	}
	// Use TopicKey as the dominant signal; fall back to Content when
	// the topic is too narrow to score anything (Recall handles the
	// FTS5 query — empty hits return [] and the slice stays empty).
	query := topic
	if c := strings.TrimSpace(req.Content); c != "" {
		query = topic + " " + c
	}
	// Over-fetch slightly so dedupe (when a hit shares an id) does not
	// starve the 3-cap. 5 keeps the cost bounded under p95.
	const fetchN = 5
	hits, err := s.repo.Recall(query, fetchN)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryRelation, 0, 3)
	seen := map[int64]struct{}{}
	for _, h := range hits {
		if h.ID <= 0 {
			continue
		}
		if _, dup := seen[h.ID]; dup {
			continue
		}
		seen[h.ID] = struct{}{}
		out = append(out, MemoryRelation{
			// ID=0 marks this as an unsaved proposal. SourceID=0
			// because the caller's row id is not known yet (the host
			// save commits AFTER suggestion review per spec).
			ID:           0,
			SourceID:     0,
			TargetID:     h.ID,
			RelationType: "related",
		})
		if len(out) == 3 {
			break
		}
	}
	return out, nil
}
