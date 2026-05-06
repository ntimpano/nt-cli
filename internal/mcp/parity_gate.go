package mcp

import (
	"fmt"
	"sort"
	"strings"
)

// Parity readiness gates for the nt-cli rollout. These helpers are pure
// functions over already-collected data so they can be unit-tested in
// isolation and reused by integration tests that drive the live MCP
// surface. Each gate returns (ok, message): when ok is false, message
// MUST name the failing gate (G1 or G2) so operators can act on it.

// checkParityGateG1 enforces spec gate G1 ("Tool parity"): every tool in
// `required` MUST appear in `advertised`. On miss, the failure message
// names gate G1 and lists every missing tool by name so operators know
// exactly which contract the host broke.
func checkParityGateG1(advertised, required []string) (bool, string) {
	have := map[string]struct{}{}
	for _, t := range advertised {
		have[t] = struct{}{}
	}
	missing := []string{}
	for _, want := range required {
		if _, ok := have[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return true, ""
	}
	sort.Strings(missing)
	return false, fmt.Sprintf("G1 tool parity failed: missing tools %s", strings.Join(missing, ", "))
}

// opResult captures the outcome of one CLI-vs-MCP smoke operation. The
// gate-level test inspects these to compute an aggregate verdict; the
// detail string aids debugging without requiring the test to re-run.
type opResult struct {
	tool   string
	ok     bool
	detail string
}

// g2Report is the aggregate outcome of the operation-parity smoke run.
// It is exposed via runG2Smoke so the test can assert both the operation
// count (N>=10) and per-tool coverage (every required tool exercised).
type g2Report struct {
	total          int
	perTool        map[string]int
	ok             bool
	failureMessage string
}

// checkParityGateG2 enforces spec gate G2 ("Operation parity"): every op
// result MUST be ok. On any failure the message names gate G2 and the
// failing tool plus its detail so operators can pinpoint the divergence.
func checkParityGateG2(results []opResult) (bool, string) {
	failed := []string{}
	for _, r := range results {
		if !r.ok {
			failed = append(failed, fmt.Sprintf("%s (%s)", r.tool, r.detail))
		}
	}
	if len(failed) == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("G2 operation parity failed: %s", strings.Join(failed, "; "))
}
