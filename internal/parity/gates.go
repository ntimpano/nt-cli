package parity

import "fmt"

// ActionableDeltaThresholdPct is the spec floor (-35%) for the
// actionable-delta gate. A post-feature replay MUST show median resume
// time at least 35% faster than baseline; otherwise CI fails.
//
// Sign convention: delta_pct = (current-baseline)/baseline * 100.
// Improvements are negative numbers (faster). The gate passes when
// delta_pct ≤ -35 (i.e. ≥35% improvement).
const ActionableDeltaThresholdPct = -35.0

// ContinuityUpliftPoints is the spec floor (+5 points on a 0..100
// scale, equivalent to +0.05 on TopKHitRate's 0..1 scale) for the
// graph-aware boost. Without ≥+5 uplift, the boost is not actionable
// and CI fails.
const ContinuityUpliftPoints = 0.05

// AssertActionableDeltaGate enforces "delta_pct ≤ -35 is reported and
// CI fails on regression" (capability: parity-scorecard, requirement:
// MUST gate on actionable delta).
//
// Returns nil when current.MedianResumeMs is at least 35% faster than
// baseline.MedianResumeMs. Returns an error otherwise — including the
// regression case (positive delta).
func AssertActionableDeltaGate(baseline, current ContinuityBaseline) error {
	if baseline.MedianResumeMs <= 0 {
		return fmt.Errorf("baseline median_resume_ms must be > 0; got %d", baseline.MedianResumeMs)
	}
	delta := float64(current.MedianResumeMs-baseline.MedianResumeMs) / float64(baseline.MedianResumeMs) * 100.0
	if delta > ActionableDeltaThresholdPct {
		return fmt.Errorf("actionable delta gate: delta=%.2f%% does not meet threshold %.2f%% (baseline=%dms, current=%dms)",
			delta, ActionableDeltaThresholdPct, baseline.MedianResumeMs, current.MedianResumeMs)
	}
	return nil
}

// AssertContinuityUplift enforces "Graph-aware boost improves
// continuity score" (capability: memory-graph, requirement: graph
// boost MUST raise knowledge-continuity by ≥5 points).
//
// Compares TopKHitRate (the score's primary input) on the 0..1 scale,
// requiring current ≥ baseline + 0.05.
func AssertContinuityUplift(baseline, current ContinuityBaseline) error {
	uplift := current.TopKHitRate - baseline.TopKHitRate
	// Tolerate floating-point dust: an exactly-+0.05 case would
	// otherwise fail by ~1e-17 on some hosts.
	const eps = 1e-9
	if uplift+eps < ContinuityUpliftPoints {
		return fmt.Errorf("continuity uplift gate: uplift=%.4f does not meet threshold %.4f (baseline=%.4f, current=%.4f)",
			uplift, ContinuityUpliftPoints, baseline.TopKHitRate, current.TopKHitRate)
	}
	return nil
}
