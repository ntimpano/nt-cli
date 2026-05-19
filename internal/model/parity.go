package model

import "time"

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

// ContinuityContractVersion stamps baseline.json so PR5 / runbook
// readers can detect schema drift between releases. Bump when fields
// or scoring rules change.
const ContinuityContractVersion = "1.0.0"

// ContinuityLatencyBudgetMs is the spec's recall p95 budget (ms).
// At or below the budget the dimension keeps its full latency factor;
// above it the factor decays linearly to a 0.5 floor at 2× budget.
// This matches the spec's "preserves p95 < 50ms" SLO from PR4 design.
const ContinuityLatencyBudgetMs = 50

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

// ContinuityQuery is one fixture row: a query string and an
// `expected_marker` substring that MUST appear in the top-3 returned
// content for the row to count as a hit. Stable substring matching
// keeps the fixture portable across different stores (no DB IDs).
type ContinuityQuery struct {
	Query          string `json:"query"`
	ExpectedMarker string `json:"expected_marker"`
	Note           string `json:"note,omitempty"`
}

// ContinuityQueryResult is one replayed row in the baseline.
type ContinuityQueryResult struct {
	Query          string `json:"query"`
	ExpectedMarker string `json:"expected_marker"`
	Hit            bool   `json:"hit"`
	LatencyMs      int64  `json:"latency_ms"`
	TopKContent    string `json:"top_k_content"` // first match, for debugging
}

// ContinuityBaseline is the full output written to baseline.json.
// Field order is the JSON contract — do not reorder without bumping
// ContinuityContractVersion.
type ContinuityBaseline struct {
	Version        string                  `json:"version"`
	GeneratedAt    time.Time               `json:"generated_at"`
	Count          int                     `json:"count"`
	TopKHitRate    float64                 `json:"top_k_hit_rate"`
	MedianResumeMs int64                   `json:"median_resume_ms"`
	P95ResumeMs    int64                   `json:"p95_resume_ms"`
	Queries        []ContinuityQueryResult `json:"queries"`
}
