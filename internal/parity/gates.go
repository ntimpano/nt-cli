package parity

import (
	"fmt"

	"flint/internal/model"
)

// AssertActionableDeltaGate enforces "delta_pct ≤ -35 is reported and
// CI fails on regression" (capability: parity-scorecard, requirement:
// MUST gate on actionable delta).
//
// Returns nil when current.MedianResumeMs is at least 35% faster than
// baseline.MedianResumeMs. Returns an error otherwise — including the
// regression case (positive delta).
func AssertActionableDeltaGate(baseline, current model.ContinuityBaseline) error {
	if baseline.MedianResumeMs <= 0 {
		return fmt.Errorf("baseline median_resume_ms must be > 0; got %d", baseline.MedianResumeMs)
	}
	delta := float64(current.MedianResumeMs-baseline.MedianResumeMs) / float64(baseline.MedianResumeMs) * 100.0
	if delta > model.ActionableDeltaThresholdPct {
		return fmt.Errorf("actionable delta gate: delta=%.2f%% does not meet threshold %.2f%% (baseline=%dms, current=%dms)",
			delta, model.ActionableDeltaThresholdPct, baseline.MedianResumeMs, current.MedianResumeMs)
	}
	return nil
}

// AssertContinuityUplift enforces "Graph-aware boost improves
// continuity score" (capability: memory-graph, requirement: graph
// boost MUST raise knowledge-continuity by ≥5 points).
//
// Compares TopKHitRate (the score's primary input) on the 0..1 scale,
// requiring current ≥ baseline + 0.05.
func AssertContinuityUplift(baseline, current model.ContinuityBaseline) error {
	uplift := current.TopKHitRate - baseline.TopKHitRate
	// Tolerate floating-point dust: an exactly-+0.05 case would
	// otherwise fail by ~1e-17 on some hosts.
	const eps = 1e-9
	if uplift+eps < model.ContinuityUpliftPoints {
		return fmt.Errorf("continuity uplift gate: uplift=%.4f does not meet threshold %.4f (baseline=%.4f, current=%.4f)",
			uplift, model.ContinuityUpliftPoints, baseline.TopKHitRate, current.TopKHitRate)
	}
	return nil
}
