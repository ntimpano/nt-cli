package parity

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"flint/internal/model"
)

// Knowledge-Continuity Harness contract (spec capability:
// actionable-recall + MODIFIED Recall/knowledge-continuity gate).
// PR2 ships the read-only harness that records a deterministic
// baseline.json. PR5 will replay it post-feature and assert
// `delta_pct ≤ -35`. The harness is intentionally pure: callers
// supply a Recaller and a Clock so tests are byte-deterministic.

// Recaller is the minimal recall surface the harness needs. The app
// Service satisfies it via a tiny adapter (string-only result), which
// keeps the parity package free of any store/service import cycle.
type Recaller interface {
	Recall(query string, limit int) ([]string, error)
}

// Clock is the deterministic time source used to measure resume
// latency. Production wires `time.Now`; tests inject a sequence so
// replay is byte-stable.
type Clock interface {
	Now() time.Time
}

// LoadQueries parses the JSON fixture suite at path. Errors surface
// loudly because a missing fixture would silently zero the harness
// and corrupt the scorecard's knowledge-continuity dimension.
func LoadQueries(path string) ([]model.ContinuityQuery, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var queries []model.ContinuityQuery
	if err := json.Unmarshal(data, &queries); err != nil {
		return nil, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	return queries, nil
}

// ComputeContinuity replays every fixture row through the Recaller,
// measures resume latency from the supplied Clock, and aggregates
// hit-rate / median / p95. The function is pure: same inputs →
// identical baseline (used by the idempotent-replay test).
//
// Top-k window: the spec requires ≥10 queries with expected top hits.
// We request top 3 and count a hit when ANY of the returned content
// strings contains the expected_marker (case-insensitive). Top-1
// strict matching is too brittle for a fixture that must survive
// store reseeding; "expected_marker in top-3" is the spec's intent.
func ComputeContinuity(queries []model.ContinuityQuery, r Recaller, clk Clock) (model.ContinuityBaseline, error) {
	results := make([]model.ContinuityQueryResult, 0, len(queries))
	latencies := make([]int64, 0, len(queries))
	hits := 0

	for _, q := range queries {
		start := clk.Now()
		got, err := r.Recall(q.Query, 3)
		end := clk.Now()
		if err != nil {
			return model.ContinuityBaseline{}, fmt.Errorf("recall %q: %w", q.Query, err)
		}
		latencyMs := end.Sub(start).Milliseconds()
		hit := containsMarker(got, q.ExpectedMarker)
		if hit {
			hits++
		}
		topContent := ""
		if len(got) > 0 {
			topContent = got[0]
		}
		results = append(results, model.ContinuityQueryResult{
			Query:          q.Query,
			ExpectedMarker: q.ExpectedMarker,
			Hit:            hit,
			LatencyMs:      latencyMs,
			TopKContent:    topContent,
		})
		latencies = append(latencies, latencyMs)
	}

	hitRate := 0.0
	if len(queries) > 0 {
		hitRate = float64(hits) / float64(len(queries))
	}

	// GeneratedAt is sourced from the same Clock for determinism. We
	// take a fresh tick here AFTER the replay loop so the timestamp
	// orders strictly after every measured query.
	generatedAt := clk.Now()

	return model.ContinuityBaseline{
		Version:        model.ContinuityContractVersion,
		GeneratedAt:    generatedAt.UTC(),
		Count:          len(queries),
		TopKHitRate:    hitRate,
		MedianResumeMs: medianMs(latencies),
		P95ResumeMs:    p95Ms(latencies),
		Queries:        results,
	}, nil
}

// containsMarker reports whether any string in tops contains marker
// (case-insensitive). Empty marker is treated as "no expectation",
// which counts as a miss — fixture rows MUST declare a marker.
func containsMarker(tops []string, marker string) bool {
	if marker == "" {
		return false
	}
	low := strings.ToLower(marker)
	for _, t := range tops {
		if strings.Contains(strings.ToLower(t), low) {
			return true
		}
	}
	return false
}

// medianMs returns the median of a latency slice. Empty input → 0.
// Sorts a copy so callers' input is untouched.
func medianMs(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int64(nil), xs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	// Even count: lower of the two middle samples (we deal in
	// integer ms — averaging would introduce a fractional baseline
	// that breaks byte-for-byte determinism on replay).
	return sorted[n/2-1]
}

// p95Ms returns the nearest-rank 95th percentile of a latency slice.
// nearest-rank: index = ceil(0.95*n)-1 over the sorted samples. This
// is the same definition used by the runbook's SLO checks.
func p95Ms(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int64(nil), xs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	idx := int(math.Ceil(0.95*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// ScoreKnowledgeContinuity converts a baseline into the 0..100
// knowledge-continuity dimension score (task 2.3 — wires harness
// output into ComputeScorecard's KnowledgeContinuity signal).
//
// Score = round(hit_rate * 100) * latency_factor, where latency_factor
// is 1.0 at or below the budget, decaying linearly to 0.5 at 2×
// budget and clamped to 0.5 thereafter. Rationale: a perfect hit-rate
// with catastrophic latency is NOT continuity — operators feel the
// stall — but we never zero the dimension on latency alone, because
// a slow correct answer still beats a fast wrong one.
func ScoreKnowledgeContinuity(b model.ContinuityBaseline) int {
	if b.TopKHitRate <= 0 {
		return 0
	}
	hitScore := b.TopKHitRate * 100.0

	factor := 1.0
	if b.P95ResumeMs > model.ContinuityLatencyBudgetMs {
		over := float64(b.P95ResumeMs - model.ContinuityLatencyBudgetMs)
		// Decay: 0% over budget → 1.0; 100% over (2× budget) → 0.5.
		decay := over / float64(model.ContinuityLatencyBudgetMs) // 1.0 at 2× budget
		if decay > 1.0 {
			decay = 1.0
		}
		factor = 1.0 - 0.5*decay
	}
	score := int(hitScore * factor)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
