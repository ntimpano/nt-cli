package mcp

import "strings"

// RollbackTrigger names a documented condition under which the operator
// MUST execute the rollback runbook. The Name is the short label used in
// alerts and post-mortems; RunbookKeyword is the substring guaranteed to
// appear in docs/engram-offramp.md so code and runbook stay in lockstep.
//
// Adding a new trigger requires updating BOTH the runbook AND the slice
// returned by RollbackTriggers(); the spec_compliance_test asserts the
// two sources never drift.
type RollbackTrigger struct {
	Name           string
	RunbookKeyword string
	matchKeywords  []string
}

// canonicalRollbackTriggers is the single source of truth for rollback
// trigger conditions. The set mirrors the spec ("Rollback Semantics →
// Rollback trigger fires") and the runbook's "Triggers" list.
var canonicalRollbackTriggers = []RollbackTrigger{
	{
		Name:           "parity test failure",
		RunbookKeyword: "parity_test.go",
		matchKeywords:  []string{"parity_test.go", "parity test failure"},
	},
	{
		Name:           "data loss report",
		RunbookKeyword: "data loss",
		matchKeywords:  []string{"data loss", "data-loss"},
	},
	{
		Name:           "mcp tool registration error",
		RunbookKeyword: "tool registration error",
		matchKeywords:  []string{"tool registration error", "registration error"},
	},
	{
		Name:           "soak window error rate threshold",
		RunbookKeyword: "soak",
		matchKeywords:  []string{"soak-window error rate", "soak window error rate", "error rate above"},
	},
}

// RollbackTriggers returns the documented rollback trigger contract. The
// slice is freshly allocated so callers cannot mutate the canonical set.
func RollbackTriggers() []RollbackTrigger {
	out := make([]RollbackTrigger, len(canonicalRollbackTriggers))
	copy(out, canonicalRollbackTriggers)
	return out
}

// IsRollbackTrigger reports whether the given condition string matches any
// documented rollback trigger. The match is case-insensitive and substring
// based to keep the contract robust to phrasing variations in alerts and
// log lines while still rejecting unrelated noise.
func IsRollbackTrigger(condition string) bool {
	if strings.TrimSpace(condition) == "" {
		return false
	}
	c := strings.ToLower(condition)
	for _, t := range canonicalRollbackTriggers {
		for _, kw := range t.matchKeywords {
			if strings.Contains(c, strings.ToLower(kw)) {
				return true
			}
		}
	}
	return false
}
