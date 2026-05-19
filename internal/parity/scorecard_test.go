package parity

import (
	"strings"
	"testing"

	"flint/internal/model"
)

// TestParityScorecard_WeightedSumMath proves spec scenario "Score produced
// from live signals": the total MUST equal Σ(score×weight)/Σ(weight) rounded
// to 0.1 over the 7 hardcoded dimensions with weights 25/15/10/15/15/10/10.
//
// We exercise the math with two distinct signal sets (triangulation) so a
// hardcoded "return 100" or "return 95" implementation cannot pass.
func TestParityScorecard_WeightedSumMath(t *testing.T) {
	// All-100 signals → total MUST be exactly 100.0.
	allGreen := model.ScorecardSignals{
		CoreOps:                100,
		MetadataRetrieval:      100,
		SessionWorkflow:        100,
		ImportExportBackup:     100,
		ReliabilityOperability: 100,
		KnowledgeContinuity:    100,
		UXAPIContract:          100,
		SoakDays:               30,
	}
	v := ComputeScorecard(allGreen)
	if v.Total != 100.0 {
		t.Fatalf("all-100 signals must yield total=100.0, got %.2f", v.Total)
	}

	// Mixed signals → total MUST equal the weighted average.
	// Weights: core 25, meta 15, session 10, ieb 15, rel 15, kc 10, ux 10 → sum 100.
	// Scores: 80,90,70,100,60,50,40
	// Weighted: 80*25+90*15+70*10+100*15+60*15+50*10+40*10
	//         = 2000+1350+700+1500+900+500+400 = 7350 → /100 = 73.5
	mixed := model.ScorecardSignals{
		CoreOps:                80,
		MetadataRetrieval:      90,
		SessionWorkflow:        70,
		ImportExportBackup:     100,
		ReliabilityOperability: 60,
		KnowledgeContinuity:    50,
		UXAPIContract:          40,
		SoakDays:               20,
	}
	v2 := ComputeScorecard(mixed)
	if v2.Total != 73.5 {
		t.Fatalf("mixed signals must yield total=73.5, got %.2f", v2.Total)
	}
}

// TestParityScorecard_DimensionsExposed proves spec scenario requirement:
// "Each dimension MUST expose its raw score and pass/fail flag."
// The verdict result MUST list all 7 dimensions with name, score, weight, pass.
func TestParityScorecard_DimensionsExposed(t *testing.T) {
	v := ComputeScorecard(model.ScorecardSignals{
		CoreOps:                100,
		MetadataRetrieval:      100,
		SessionWorkflow:        100,
		ImportExportBackup:     100,
		ReliabilityOperability: 100,
		KnowledgeContinuity:    100,
		UXAPIContract:          100,
		SoakDays:               30,
	})
	if len(v.Dimensions) != 7 {
		t.Fatalf("verdict must expose 7 dimensions, got %d", len(v.Dimensions))
	}
	wantWeights := map[string]int{
		"core-ops":                25,
		"metadata-retrieval":      15,
		"session-workflow":        10,
		"import-export-backup":    15,
		"reliability-operability": 15,
		"knowledge-continuity":    10,
		"ux-api-contract":         10,
	}
	gotWeights := map[string]int{}
	for _, d := range v.Dimensions {
		gotWeights[d.Name] = d.Weight
		if d.Score != 100 {
			t.Errorf("dimension %s: expected score=100, got %d", d.Name, d.Score)
		}
		if !d.Pass {
			t.Errorf("dimension %s: expected pass=true on score=100", d.Name)
		}
	}
	for name, w := range wantWeights {
		if gotWeights[name] != w {
			t.Errorf("dimension %s: expected weight=%d, got %d", name, w, gotWeights[name])
		}
	}
}

// TestParityScorecard_PassRequiresAllConditions proves spec requirement:
// "Pass MUST require ≥95 AND critical green AND soak_days ≥ 14".
// Triangulation: only the all-green-with-soak case yields pass.
func TestParityScorecard_PassRequiresAllConditions(t *testing.T) {
	pass := ComputeScorecard(model.ScorecardSignals{
		CoreOps: 98, MetadataRetrieval: 96, SessionWorkflow: 95,
		ImportExportBackup: 97, ReliabilityOperability: 96,
		KnowledgeContinuity: 95, UXAPIContract: 95,
		SoakDays: 14,
	})
	if pass.Verdict != model.VerdictPass {
		t.Fatalf("expected verdict=pass for total≥95+critical-green+soak=14, got %q (total=%.2f)", pass.Verdict, pass.Total)
	}
	if pass.HoldReason != "" {
		t.Fatalf("pass verdict must have empty hold_reason, got %q", pass.HoldReason)
	}
}

// TestParityScorecard_CriticalRedForcesFail proves spec scenario "Critical
// dimension red forces overall fail": even with total ≥ 95, if any of
// core-ops / reliability-operability / knowledge-continuity is below 95
// (pass=false), verdict MUST be fail.
func TestParityScorecard_CriticalRedForcesFail(t *testing.T) {
	// core-ops red, everything else green → total still ≥ 95.
	v := ComputeScorecard(model.ScorecardSignals{
		CoreOps:                80, // critical red (below 95)
		MetadataRetrieval:      100,
		SessionWorkflow:        100,
		ImportExportBackup:     100,
		ReliabilityOperability: 100,
		KnowledgeContinuity:    100,
		UXAPIContract:          100,
		SoakDays:               30,
	})
	if v.Total < 95 {
		t.Fatalf("test setup error: total must remain ≥95 to prove critical-red overrides total, got %.2f", v.Total)
	}
	if v.Verdict != model.VerdictFail {
		t.Fatalf("critical red MUST force verdict=fail despite total≥95, got %q (total=%.2f)", v.Verdict, v.Total)
	}
	if !strings.Contains(v.HoldReason, "core-ops") {
		t.Fatalf("fail verdict on critical-red MUST name the failing critical dimension, got hold_reason=%q", v.HoldReason)
	}
}

// TestParityScorecard_SoakUnder14Holds proves spec scenario "Soak under 14d
// holds verdict": total=97, critical green, soak_days=10 → verdict=hold with
// reason `soak_window`.
func TestParityScorecard_SoakUnder14Holds(t *testing.T) {
	v := ComputeScorecard(model.ScorecardSignals{
		CoreOps: 97, MetadataRetrieval: 97, SessionWorkflow: 97,
		ImportExportBackup: 97, ReliabilityOperability: 97,
		KnowledgeContinuity: 97, UXAPIContract: 97,
		SoakDays: 10,
	})
	if v.Verdict != model.VerdictHold {
		t.Fatalf("soak<14 with otherwise-green signals MUST produce verdict=hold, got %q", v.Verdict)
	}
	if v.HoldReason != "soak_window" {
		t.Fatalf("soak<14 hold_reason MUST be 'soak_window', got %q", v.HoldReason)
	}
}

// TestParityScorecard_HoldNamesLowestCriticalDimension proves the spec
// scenario "Scorecard hold names dimension": hold notice MUST include the
// lowest-scoring critical dimension.
//
// We construct signals where total<95 (forces hold path, not pass) but
// critical dimensions are green (so it is hold not fail). Among the three
// critical dimensions, knowledge-continuity has the lowest score → the
// hold reason MUST name it.
func TestParityScorecard_HoldNamesLowestCriticalDimension(t *testing.T) {
	v := ComputeScorecard(model.ScorecardSignals{
		CoreOps:                99, // critical, green
		MetadataRetrieval:      50, // non-critical drag
		SessionWorkflow:        50,
		ImportExportBackup:     50,
		ReliabilityOperability: 98, // critical, green
		KnowledgeContinuity:    95, // critical, green, lowest of the 3 criticals
		UXAPIContract:          50,
		SoakDays:               30,
	})
	if v.Verdict == model.VerdictPass {
		t.Fatalf("test setup expected non-pass verdict (total<95), got pass total=%.2f", v.Total)
	}
	if v.Verdict != model.VerdictHold {
		t.Fatalf("expected verdict=hold (total<95, critical green), got %q", v.Verdict)
	}
	if !strings.Contains(v.HoldReason, "knowledge-continuity") {
		t.Fatalf("hold_reason must name lowest critical dimension 'knowledge-continuity', got %q", v.HoldReason)
	}
}

// TestParityScorecard_VersionStamp proves the verdict carries a non-empty
// version string so contract changes are detectable. The MCP/CLI surface
// (task 1.4) returns this field; locking it here prevents accidental drift.
func TestParityScorecard_VersionStamp(t *testing.T) {
	v := ComputeScorecard(model.ScorecardSignals{SoakDays: 14})
	if strings.TrimSpace(v.Version) == "" {
		t.Fatalf("verdict.Version must be a non-empty contract version, got %q", v.Version)
	}
}
