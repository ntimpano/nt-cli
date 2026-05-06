package app

import (
	"testing"
)

// TestBuildActionableRecall_EmptyInputs proves the response shape is
// stable even when there are zero matches: matches MUST be empty,
// next_action MUST be the empty string (callers serialize as null),
// and checklist + inferred_paths MUST be empty slices (never nil) so
// JSON encoders emit `[]` instead of `null`.
func TestBuildActionableRecall_EmptyInputs(t *testing.T) {
	resp := BuildActionableRecall(nil)
	if len(resp.Matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(resp.Matches))
	}
	if resp.NextAction != "" {
		t.Fatalf("expected empty next_action, got %q", resp.NextAction)
	}
	if resp.Checklist == nil {
		t.Fatalf("checklist must be non-nil empty slice for stable JSON shape")
	}
	if len(resp.Checklist) != 0 {
		t.Fatalf("expected empty checklist, got %v", resp.Checklist)
	}
	if resp.InferredPaths == nil {
		t.Fatalf("inferred_paths must be non-nil empty slice")
	}
	if len(resp.InferredPaths) != 0 {
		t.Fatalf("expected empty inferred_paths, got %v", resp.InferredPaths)
	}
}

// TestBuildActionableRecall_NextAction_GatedByType proves the spec
// requirement: next_action is non-empty ONLY when the top match's
// type is one of {decision, bugfix, pattern}. Other types (manual,
// architecture, etc.) MUST yield an empty next_action so callers
// know there is no high-confidence guidance.
func TestBuildActionableRecall_NextAction_GatedByType(t *testing.T) {
	cases := []struct {
		topType        string
		wantNonEmpty   bool
		humanLabel     string
		mustContainSub string
	}{
		{topType: "decision", wantNonEmpty: true, humanLabel: "decision", mustContainSub: "decision"},
		{topType: "bugfix", wantNonEmpty: true, humanLabel: "bugfix", mustContainSub: "bugfix"},
		{topType: "pattern", wantNonEmpty: true, humanLabel: "pattern", mustContainSub: "pattern"},
		{topType: "manual", wantNonEmpty: false, humanLabel: "manual"},
		{topType: "architecture", wantNonEmpty: false, humanLabel: "architecture"},
		{topType: "", wantNonEmpty: false, humanLabel: "(empty)"},
	}
	for _, tc := range cases {
		t.Run(tc.humanLabel, func(t *testing.T) {
			items := []MemoryItem{{
				ID:      1,
				Title:   "Switched to JWT",
				Type:    tc.topType,
				Content: "**What**: Replaced sessions with JWT.\n**Where**: src/auth.go",
			}}
			resp := BuildActionableRecall(items)
			if tc.wantNonEmpty && resp.NextAction == "" {
				t.Fatalf("type=%q: expected non-empty next_action", tc.topType)
			}
			if !tc.wantNonEmpty && resp.NextAction != "" {
				t.Fatalf("type=%q: expected empty next_action, got %q", tc.topType, resp.NextAction)
			}
			if tc.mustContainSub != "" && !containsFold(resp.NextAction, tc.mustContainSub) {
				t.Fatalf("type=%q: next_action %q must reference type label", tc.topType, resp.NextAction)
			}
		})
	}
}

// TestBuildActionableRecall_NextAction_UsesTitle proves the rendered
// next_action references the top match's title so a caller can act
// without re-reading the body. Triangulates the previous test by
// changing only the title and asserting the output reflects it.
func TestBuildActionableRecall_NextAction_UsesTitle(t *testing.T) {
	items := []MemoryItem{{
		ID: 1, Title: "Fixed N+1 in user list", Type: "bugfix",
		Content: "**What**: Joined eagerly\n**Where**: internal/users/list.go",
	}}
	resp := BuildActionableRecall(items)
	if !containsFold(resp.NextAction, "Fixed N+1 in user list") {
		t.Fatalf("next_action must reference top title, got %q", resp.NextAction)
	}
}

// TestBuildActionableRecall_Checklist_FromBullets proves the helper
// extracts up to 5 bullet lines from the top match's content. Bullets
// can start with `- `, `* `, or `1.` style numbering.
func TestBuildActionableRecall_Checklist_FromBullets(t *testing.T) {
	items := []MemoryItem{{
		ID: 1, Title: "Migration plan", Type: "decision",
		Content: `**What**: Move auth to JWT.
**Steps**:
- Add JWT middleware
- Wire login route
- Rotate refresh tokens
- Document the change
- Run smoke
- (sixth, must be dropped)
`,
	}}
	resp := BuildActionableRecall(items)
	if len(resp.Checklist) > 5 {
		t.Fatalf("checklist must cap at 5, got %d", len(resp.Checklist))
	}
	if len(resp.Checklist) == 0 {
		t.Fatalf("expected at least one checklist item")
	}
	if resp.Checklist[0] != "Add JWT middleware" {
		t.Fatalf("first item must be cleaned of bullet marker, got %q", resp.Checklist[0])
	}
	for _, item := range resp.Checklist {
		if containsFold(item, "sixth") {
			t.Fatalf("sixth item must not appear, got %v", resp.Checklist)
		}
	}
}

// TestBuildActionableRecall_Checklist_EmptyWhenNoBullets triangulates
// the previous test: a top match without bullet lines must yield an
// empty checklist (not nil — empty slice for stable JSON shape).
func TestBuildActionableRecall_Checklist_EmptyWhenNoBullets(t *testing.T) {
	items := []MemoryItem{{
		ID: 1, Title: "Plain note", Type: "decision",
		Content: "Just a paragraph with no list items in it.",
	}}
	resp := BuildActionableRecall(items)
	if resp.Checklist == nil {
		t.Fatalf("checklist must be non-nil empty slice")
	}
	if len(resp.Checklist) != 0 {
		t.Fatalf("expected empty checklist, got %v", resp.Checklist)
	}
}

// TestBuildActionableRecall_InferredPaths_FromTop3 proves the helper
// extracts file paths from the top THREE matches' content (not all of
// them) and returns unique paths in first-seen order.
func TestBuildActionableRecall_InferredPaths_FromTop3(t *testing.T) {
	items := []MemoryItem{
		{ID: 1, Type: "decision", Content: "**Where**: internal/auth/middleware.go and src/routes/login.ts"},
		{ID: 2, Type: "manual", Content: "see internal/auth/middleware.go again (dedup)"},
		{ID: 3, Type: "bugfix", Content: "patch in pkg/util/strings.go"},
		{ID: 4, Type: "manual", Content: "MUST NOT appear: cmd/main.go"},
	}
	resp := BuildActionableRecall(items)
	want := []string{
		"internal/auth/middleware.go",
		"src/routes/login.ts",
		"pkg/util/strings.go",
	}
	if len(resp.InferredPaths) != len(want) {
		t.Fatalf("expected %d unique paths from top-3, got %d: %v", len(want), len(resp.InferredPaths), resp.InferredPaths)
	}
	for i, w := range want {
		if resp.InferredPaths[i] != w {
			t.Fatalf("path[%d]: want %q, got %q", i, w, resp.InferredPaths[i])
		}
	}
	for _, p := range resp.InferredPaths {
		if p == "cmd/main.go" {
			t.Fatalf("path from item #4 (beyond top-3) must NOT appear: %v", resp.InferredPaths)
		}
	}
}

// TestBuildActionableRecall_Matches_PreservesItems proves the helper
// embeds the original items verbatim so callers don't lose any data.
func TestBuildActionableRecall_Matches_PreservesItems(t *testing.T) {
	items := []MemoryItem{
		{ID: 1, Title: "A", Type: "decision", Content: "x"},
		{ID: 2, Title: "B", Type: "manual", Content: "y"},
	}
	resp := BuildActionableRecall(items)
	if len(resp.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(resp.Matches))
	}
	if resp.Matches[0].ID != 1 || resp.Matches[1].ID != 2 {
		t.Fatalf("matches must preserve order and IDs, got %+v", resp.Matches)
	}
}

// containsFold is a tiny case-insensitive substring helper, kept local
// to the test file to avoid pulling strings into a tiny dep.
func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	hl := []byte(haystack)
	nl := []byte(needle)
	for i := 0; i+len(nl) <= len(hl); i++ {
		match := true
		for j := 0; j < len(nl); j++ {
			a := hl[i+j]
			b := nl[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'B' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
