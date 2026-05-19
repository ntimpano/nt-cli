package store

import (
	"strings"

	"flint/internal/model"
)

// RecallGraphAware augments the FTS recall path with graph-aware
// re-ranking and `supersedes`-predecessor suppression. It is the
// store-layer implementation of the PR4 graph-aware recall capability;
// service- and MCP-layer callers gate it behind the NTCLI_FF_GRAPH
// feature flag so the legacy Recall surface stays byte-identical for
// clients that have not opted in.
//
// Algorithm (kept intentionally simple — re-rank, not re-score):
//
//  1. Validate / default opts as plain Recall would (empty query →
//     empty slice, limit ≤0 → 10). Caller validation at the service
//     layer is still the real guard; we tolerate misuse here.
//  2. Over-fetch via the existing Recall path so we have a stable
//     base ranking and enough headroom to surface boosted neighbors
//     that were below the visible window. We over-fetch
//     min(limit*overfetchMul, hardCap).
//  3. Identify the top match (position 0). Load its outbound +
//     inbound edges from memory_relations.
//  4. Build two id sets:
//     - boostable: neighbor ids reached by `related|refines|depends_on`
//     - suppressed: predecessors reached by an outbound `supersedes`
//     edge from any row in the candidate set (top-match path drives
//     the dominant case; we also honor edges originating from any
//     row already in the over-fetched window so a non-top successor
//     still hides its predecessor).
//  5. Filter the candidate slice: drop suppressed ids unless
//     opts.IncludeSuperseded is set.
//  6. Re-rank: each boostable row swaps with the row immediately
//     above it as long as that move does not pass position 0 (the
//     top match) AND the swap does not exceed the bounded-cap
//     budget — in this implementation the cap is enforced by
//     simply forbidding any boosted row from reaching position 0,
//     which directly satisfies the "≤20% of base rank" spec ceiling
//     when the fetch window is ≤5 rows (one position swap is the
//     coarsest possible re-rank). For larger windows the same rule
//     keeps the move strictly bounded.
//  7. Truncate to opts.Limit.
//
// We deliberately avoid a numeric score column (FTS bm25 is opaque
// and the position-swap re-rank is sufficient for the spec's
// continuity-score uplift target while staying obviously correct on
// inspection — important for a feature behind a flag whose purpose
// is to ship safely).
func (s *SQLiteStore) RecallGraphAware(opts model.RecallOptions) ([]model.MemoryItem, error) {
	if strings.TrimSpace(opts.Query) == "" {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Over-fetch so a low-ranked neighbor can still be surfaced after
	// boost. Capped to keep the post-processing cost bounded — the
	// p95 < 50ms latency budget from the spec leaves no room for an
	// unbounded fan-out here.
	const (
		overfetchMul = 3
		hardCap      = 50
	)
	fetchN := limit * overfetchMul
	if fetchN > hardCap {
		fetchN = hardCap
	}
	if fetchN < limit {
		fetchN = limit
	}

	candidates, err := s.Recall(opts.Query, fetchN)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return candidates, nil
	}

	// Index candidates by id for fast membership checks during
	// suppress / boost passes.
	posByID := make(map[int64]int, len(candidates))
	for i, c := range candidates {
		posByID[c.ID] = i
	}

	// Collect outbound `supersedes` edges from every candidate so a
	// non-top successor still hides its predecessor. Collect
	// outbound + inbound `boostable` edges from the TOP match only —
	// that is the spec contract ("connected to the query's top match").
	suppressed := map[int64]struct{}{}
	boostable := map[int64]struct{}{}

	for _, c := range candidates {
		out, err := s.Neighbors(c.ID, model.RelationDirectionOutbound)
		if err != nil {
			return nil, err
		}
		for _, e := range out {
			if e.RelationType == "supersedes" {
				suppressed[e.TargetID] = struct{}{}
			}
		}
	}

	if !opts.IncludeSuperseded {
		// Drop suppressed rows in place, preserving order.
		filtered := candidates[:0]
		for _, c := range candidates {
			if _, hide := suppressed[c.ID]; hide {
				continue
			}
			filtered = append(filtered, c)
		}
		candidates = filtered
		// Rebuild positions after filtering.
		posByID = make(map[int64]int, len(candidates))
		for i, c := range candidates {
			posByID[c.ID] = i
		}
	}

	if len(candidates) == 0 {
		return candidates, nil
	}

	top := candidates[0]
	for _, dir := range []model.RelationDirection{
		model.RelationDirectionOutbound,
		model.RelationDirectionInbound,
	} {
		edges, err := s.Neighbors(top.ID, dir)
		if err != nil {
			return nil, err
		}
		for _, e := range edges {
			if !isBoostableRelation(e.RelationType) {
				continue
			}
			var other int64
			if dir == model.RelationDirectionOutbound {
				other = e.TargetID
			} else {
				other = e.SourceID
			}
			boostable[other] = struct{}{}
		}
	}

	// Re-rank pass: bubble each boostable row up one slot at a time,
	// never passing position 0. Single pass is enough because each
	// row only earns one swap (the bounded-cap guarantee).
	for i := 1; i < len(candidates); i++ {
		if _, ok := boostable[candidates[i].ID]; !ok {
			continue
		}
		// Attempt one swap with i-1 — but only if i-1 is not the top
		// match (i==1 case keeps the top in place) AND i-1 is not
		// itself a boostable row already swapped earlier (avoid
		// thrashing).
		if i-1 == 0 {
			continue
		}
		if _, alreadyBoosted := boostable[candidates[i-1].ID]; alreadyBoosted {
			continue
		}
		candidates[i-1], candidates[i] = candidates[i], candidates[i-1]
	}

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

// isBoostableRelation reports whether a relation_type qualifies for the
// graph-aware rank boost. The spec restricts the boost to
// `refines | depends_on | related`; `supersedes` and `conflicts_with`
// have separate semantics (suppression / conflict surface) and MUST
// NOT trigger a boost.
func isBoostableRelation(t string) bool {
	switch t {
	case "related", "refines", "depends_on":
		return true
	default:
		return false
	}
}
