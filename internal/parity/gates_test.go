package parity

import (
	"strings"
	"testing"

	"flint/internal/model"
)

// TestActionableDeltaGate_FailsOnRegression — spec scenario:
// "delta_pct ≤ -35 is reported and CI fails on regression". When the
// post-feature replay reports a slower median resume time than the
// baseline (regression), the gate MUST fail.
func TestActionableDeltaGate_FailsOnRegression(t *testing.T) {
	baseline := model.ContinuityBaseline{MedianResumeMs: 100}
	current := model.ContinuityBaseline{MedianResumeMs: 120} // +20% slower

	if err := AssertActionableDeltaGate(baseline, current); err == nil {
		t.Fatalf("expected gate failure on regression; got nil")
	} else if !strings.Contains(err.Error(), "delta") {
		t.Fatalf("expected error mentioning delta; got %q", err.Error())
	}
}

// TestActionableDeltaGate_PassesOnUplift — triangulation with the spec's
// minimum acceptable improvement: a 35% drop in median time must pass.
func TestActionableDeltaGate_PassesOnUplift(t *testing.T) {
	baseline := model.ContinuityBaseline{MedianResumeMs: 100}
	current := model.ContinuityBaseline{MedianResumeMs: 65} // -35% exactly

	if err := AssertActionableDeltaGate(baseline, current); err != nil {
		t.Fatalf("expected gate pass at -35%% delta; got %v", err)
	}
}

// TestActionableDeltaGate_FailsOnInsufficientUplift — a 20% drop is
// movement in the right direction but does not meet the spec's -35%
// floor. CI MUST still fail so the gate enforces the budget, not the
// vibe.
func TestActionableDeltaGate_FailsOnInsufficientUplift(t *testing.T) {
	baseline := model.ContinuityBaseline{MedianResumeMs: 100}
	current := model.ContinuityBaseline{MedianResumeMs: 80} // -20% only

	if err := AssertActionableDeltaGate(baseline, current); err == nil {
		t.Fatalf("expected gate failure when delta > -35; got nil")
	}
}

// TestContinuityUplift_FailsWhenScoreNotPlus5 — spec scenario:
// "Graph-aware boost improves continuity score". GIVEN graph flag ON,
// THEN knowledge-continuity dimension score MUST be ≥ baseline + 5.
func TestContinuityUplift_FailsWhenScoreNotPlus5(t *testing.T) {
	baseline := model.ContinuityBaseline{TopKHitRate: 0.80, P95ResumeMs: 10}
	current := model.ContinuityBaseline{TopKHitRate: 0.82, P95ResumeMs: 10} // only +2

	if err := AssertContinuityUplift(baseline, current); err == nil {
		t.Fatalf("expected uplift failure when current < baseline+5; got nil")
	}
}

// TestContinuityUplift_PassesWhenScoreAtLeastPlus5 — triangulation with
// the minimum acceptable uplift (+5 exactly).
func TestContinuityUplift_PassesWhenScoreAtLeastPlus5(t *testing.T) {
	baseline := model.ContinuityBaseline{TopKHitRate: 0.80, P95ResumeMs: 10}
	current := model.ContinuityBaseline{TopKHitRate: 0.85, P95ResumeMs: 10} // +5

	if err := AssertContinuityUplift(baseline, current); err != nil {
		t.Fatalf("expected uplift pass at exactly +5; got %v", err)
	}
}
