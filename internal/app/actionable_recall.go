package app

import (
	"fmt"
	"regexp"
	"strings"
)

// ActionableRecallResponse is the wire shape returned by recall callers
// that have opted into PR5's actionable contract. It is a *superset* of
// the legacy `[]MemoryItem` payload: matches still carries the raw rows
// so existing parsers can keep working, plus three derived fields that
// reduce time-to-resume on a session resume.
//
// Field invariants (validated by ActionableRecall.schema.json):
//   - Matches: original items, in rank order. Empty when no hits.
//   - NextAction: a single human-readable cue derived from the top
//     match. Empty string when the top match's type is not in
//     {decision, bugfix, pattern} or when there are zero matches.
//     Callers should treat empty as JSON null.
//   - Checklist: up to 5 items extracted from the top match's content.
//     Always non-nil so the JSON encoder emits `[]` instead of `null`.
//   - InferredPaths: unique file paths mentioned in the top THREE
//     matches' content, first-seen order. Always non-nil.
type ActionableRecallResponse struct {
	Matches       []MemoryItem `json:"matches"`
	NextAction    string       `json:"next_action"`
	Checklist     []string     `json:"checklist"`
	InferredPaths []string     `json:"inferred_paths"`
}

// actionableTypes is the closed whitelist defined by the spec — only
// these top-match types yield a non-empty next_action because they are
// the rows operators are most likely to act on after a recall.
var actionableTypes = map[string]struct{}{
	"decision": {},
	"bugfix":   {},
	"pattern":  {},
}

// bulletPrefix matches the leading marker of a checklist line. We keep
// the regex narrow on purpose (no soft-bullets / em-dashes) so we don't
// accidentally promote prose into checklist items.
var bulletPrefix = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)

// pathRegex finds path-like tokens. We require at least one slash plus
// a recognizable file extension so that prose words ("path/way" without
// extension) don't get mistaken for files. Limits the alphabet to
// chars common in source trees.
var pathRegex = regexp.MustCompile(`[A-Za-z0-9_./-]+\.[A-Za-z0-9]{1,8}`)

// BuildActionableRecall is a pure function that derives the actionable
// recall response from an ordered slice of recall matches. It does no
// I/O and treats `nil` and `[]` identically.
//
// The function is intentionally pure so it can be unit-tested without
// any store/service plumbing and reused by both MCP and CLI surfaces.
func BuildActionableRecall(items []MemoryItem) ActionableRecallResponse {
	resp := ActionableRecallResponse{
		Matches:       items,
		Checklist:     []string{},
		InferredPaths: []string{},
	}
	if len(items) == 0 {
		return resp
	}
	top := items[0]
	resp.NextAction = nextActionFromTop(top)
	resp.Checklist = checklistFromTop(top)
	resp.InferredPaths = inferredPathsFromTop3(items)
	return resp
}

// nextActionFromTop renders the next_action string. Returns "" when
// the top type is not in actionableTypes.
func nextActionFromTop(top MemoryItem) string {
	if _, ok := actionableTypes[strings.ToLower(strings.TrimSpace(top.Type))]; !ok {
		return ""
	}
	title := strings.TrimSpace(top.Title)
	if title == "" {
		title = fmt.Sprintf("note #%d", top.ID)
	}
	// Verb-led, type-aware cue. The "decision/bugfix/pattern" word is
	// embedded so the test can assert it round-trips.
	switch strings.ToLower(top.Type) {
	case "decision":
		return fmt.Sprintf("Apply decision: %s", title)
	case "bugfix":
		return fmt.Sprintf("Reapply bugfix: %s", title)
	case "pattern":
		return fmt.Sprintf("Reuse pattern: %s", title)
	}
	return ""
}

// checklistFromTop extracts up to 5 bulleted items from top.Content,
// trimming the bullet marker and surrounding whitespace.
func checklistFromTop(top MemoryItem) []string {
	out := make([]string, 0, 5)
	for _, line := range strings.Split(top.Content, "\n") {
		if !bulletPrefix.MatchString(line) {
			continue
		}
		clean := strings.TrimSpace(bulletPrefix.ReplaceAllString(line, ""))
		if clean == "" {
			continue
		}
		out = append(out, clean)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

// inferredPathsFromTop3 returns unique file paths mentioned in the
// content of the top 3 matches, preserving first-seen order.
func inferredPathsFromTop3(items []MemoryItem) []string {
	cap3 := items
	if len(cap3) > 3 {
		cap3 = cap3[:3]
	}
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, 8)
	for _, it := range cap3 {
		for _, match := range pathRegex.FindAllString(it.Content, -1) {
			if _, dup := seen[match]; dup {
				continue
			}
			seen[match] = struct{}{}
			out = append(out, match)
		}
	}
	return out
}
