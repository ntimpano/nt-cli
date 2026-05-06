package parity

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Parity scorecard contract for the nt-cli rollout (spec capability:
// parity-scorecard). The scorecard quantifies "100% practical parity"
// as a weighted score over 7 dimensions with critical-floor enforcement
// and a 14-day soak window. The version field stamps the contract so
// CLI/MCP consumers can detect drift between releases. Weights MUST sum
// to 100 and are embedded in code (not config) by deliberate design:
// parity thresholds must be immovable and strictly enforced.

// ScorecardContractVersion stamps the verdict payload so CLI/MCP clients
// can pin a contract version. Bump this when dimension weights, critical
// floors, or pass conditions change.
const ScorecardContractVersion = "1.0.0"

// MinSoakDays is the minimum 14-day soak window required for verdict=pass.
const MinSoakDays = 14

// MinTotalToPass is the minimum weighted total required for verdict=pass.
const MinTotalToPass = 95.0

// DimensionPassThreshold is the per-dimension threshold for pass=true.
// Below it, the dimension is flagged red and (if critical) overrides the
// overall verdict.
const DimensionPassThreshold = 95

// Verdict is the overall scorecard outcome.
type Verdict string

const (
	VerdictPass Verdict = "pass"
	VerdictHold Verdict = "hold"
	VerdictFail Verdict = "fail"
)

// ScorecardSignals carries the raw 0..100 dimension scores collected
// from the live system plus the soak counter. It is a pure value type —
// the scorecard math has zero side effects.
type ScorecardSignals struct {
	CoreOps                int
	MetadataRetrieval      int
	SessionWorkflow        int
	ImportExportBackup     int
	ReliabilityOperability int
	KnowledgeContinuity    int
	UXAPIContract          int
	SoakDays               int
}

// ScorecardDimension is one row of the scorecard verdict, exposing the
// raw score, weight, and pass flag the spec requires.
type ScorecardDimension struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Weight int    `json:"weight"`
	Pass   bool   `json:"pass"`
}

// ScorecardVerdict is the full scorecard result returned to callers.
// It is shaped to match the JSON contract used by CLI/MCP surfaces.
type ScorecardVerdict struct {
	Total      float64              `json:"total"`
	Dimensions []ScorecardDimension `json:"dimensions"`
	Version    string               `json:"version"`
	Verdict    Verdict              `json:"verdict"`
	HoldReason string               `json:"hold_reason"`
}

// dimensionDef carries the static metadata for one scorecard dimension.
// criticalDims is the subset whose pass=false MUST force overall fail.
type dimensionDef struct {
	name     string
	weight   int
	critical bool
}

// scorecardDimensions encodes the spec's 7 dimensions in the canonical
// order the verdict will expose. Weights sum to 100. Critical dimensions:
// core-ops, reliability-operability, knowledge-continuity (per spec).
var scorecardDimensions = []dimensionDef{
	{name: "core-ops", weight: 25, critical: true},
	{name: "metadata-retrieval", weight: 15, critical: false},
	{name: "session-workflow", weight: 10, critical: false},
	{name: "import-export-backup", weight: 15, critical: false},
	{name: "reliability-operability", weight: 15, critical: true},
	{name: "knowledge-continuity", weight: 10, critical: true},
	{name: "ux-api-contract", weight: 10, critical: false},
}

// scoreFor returns the raw 0..100 score for a given dimension name from
// the supplied signals. Centralising the lookup keeps ComputeScorecard
// scannable and avoids a parallel switch each time we iterate dimensions.
func scoreFor(name string, s ScorecardSignals) int {
	switch name {
	case "core-ops":
		return s.CoreOps
	case "metadata-retrieval":
		return s.MetadataRetrieval
	case "session-workflow":
		return s.SessionWorkflow
	case "import-export-backup":
		return s.ImportExportBackup
	case "reliability-operability":
		return s.ReliabilityOperability
	case "knowledge-continuity":
		return s.KnowledgeContinuity
	case "ux-api-contract":
		return s.UXAPIContract
	default:
		return 0
	}
}

// ComputeScorecard turns a set of raw dimension signals into a verdict.
//
// Rules (spec capability: parity-scorecard):
//  1. total = Σ(score×weight)/Σ(weight) rounded to 0.1 (weights sum to 100).
//  2. Each dimension exposes {score, weight, pass}. pass = score ≥ 95.
//  3. Verdict logic:
//     - Any critical dimension red    → fail (names the failing critical).
//     - total ≥ 95 AND criticals green AND soak_days ≥ 14 → pass.
//     - Otherwise                      → hold (reason names the gap).
//  4. Hold reason for soak_days < 14 is exactly "soak_window" (spec wording).
//  5. Hold reason for low total names the lowest-scoring critical dimension
//     so operators know where to focus remediation.
//
// The function is pure: same input → same output, no side effects.
func ComputeScorecard(s ScorecardSignals) ScorecardVerdict {
	dims := make([]ScorecardDimension, 0, len(scorecardDimensions))
	weightedSum := 0
	weightSum := 0
	for _, def := range scorecardDimensions {
		score := scoreFor(def.name, s)
		dims = append(dims, ScorecardDimension{
			Name:   def.name,
			Score:  score,
			Weight: def.weight,
			Pass:   score >= DimensionPassThreshold,
		})
		weightedSum += score * def.weight
		weightSum += def.weight
	}
	total := 0.0
	if weightSum > 0 {
		total = float64(weightedSum) / float64(weightSum)
	}
	total = math.Round(total*10) / 10 // round to 0.1 per spec

	verdict, hold := classifyVerdict(total, dims, s.SoakDays)
	return ScorecardVerdict{
		Total:      total,
		Dimensions: dims,
		Version:    ScorecardContractVersion,
		Verdict:    verdict,
		HoldReason: hold,
	}
}

// classifyVerdict applies the pass/hold/fail rules over the computed
// dimensions. It is split out so the math in ComputeScorecard stays
// linear and the verdict logic is unit-testable independently.
func classifyVerdict(total float64, dims []ScorecardDimension, soakDays int) (Verdict, string) {
	failedCriticals := []ScorecardDimension{}
	criticals := []ScorecardDimension{}
	for _, d := range dims {
		if !isCritical(d.Name) {
			continue
		}
		criticals = append(criticals, d)
		if !d.Pass {
			failedCriticals = append(failedCriticals, d)
		}
	}

	// Critical red overrides total: even total≥95 yields fail.
	if len(failedCriticals) > 0 {
		names := make([]string, 0, len(failedCriticals))
		for _, d := range failedCriticals {
			names = append(names, d.Name)
		}
		sort.Strings(names)
		return VerdictFail, fmt.Sprintf("critical_red: %s", strings.Join(names, ","))
	}

	// Soak window holds verdict regardless of total/criticals.
	if soakDays < MinSoakDays {
		return VerdictHold, "soak_window"
	}

	// Low total holds; name the lowest critical so operators target it.
	if total < MinTotalToPass {
		lowest := lowestCritical(criticals)
		return VerdictHold, fmt.Sprintf("total_below_threshold: lowest critical=%s score=%d", lowest.Name, lowest.Score)
	}

	return VerdictPass, ""
}

// isCritical reports whether the named dimension is in the critical set.
// The list is short, so a linear scan is fine and keeps a single source
// of truth (scorecardDimensions) for both weighting and criticality.
func isCritical(name string) bool {
	for _, d := range scorecardDimensions {
		if d.name == name {
			return d.critical
		}
	}
	return false
}

// lowestCritical returns the lowest-scoring critical dimension. Ties are
// broken by name (alphabetical) for deterministic hold reasons.
func lowestCritical(criticals []ScorecardDimension) ScorecardDimension {
	if len(criticals) == 0 {
		return ScorecardDimension{}
	}
	lowest := criticals[0]
	for _, d := range criticals[1:] {
		if d.Score < lowest.Score || (d.Score == lowest.Score && d.Name < lowest.Name) {
			lowest = d
		}
	}
	return lowest
}
