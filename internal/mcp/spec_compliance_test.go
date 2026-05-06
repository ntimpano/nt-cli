package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runbookPath returns the absolute path to docs/rollout-runbook.md regardless
// of the package's working directory at test time.
func runbookPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve caller path")
	}
	// internal/mcp/spec_compliance_test.go → ../../docs/rollout-runbook.md
	return filepath.Join(filepath.Dir(file), "..", "..", "docs", "rollout-runbook.md")
}

func readmePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve caller path")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "README.md")
}

func loadFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// containsAllNoCase asserts every needle appears in haystack (case-insensitive).
// On miss, it fails reporting which needles are absent so the failure pinpoints
// the missing spec property — not just "doc changed".
func containsAllNoCase(t *testing.T, hay string, needles []string, label string) {
	t.Helper()
	lower := strings.ToLower(hay)
	missing := []string{}
	for _, n := range needles {
		if !strings.Contains(lower, strings.ToLower(n)) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%s: missing required phrases %v", label, missing)
	}
}

// TestRunbook_FullCutoverDefault proves the spec scenarios under
// "Full Cutover Default":
//   - "Default profile makes nt-cli the canonical backend": the runbook
//     MUST commit to nt-cli being the default in the full-cutover phase.
//   - "Explicit opt-in path documented": setting the documented opt-in
//     toggle MUST roll back to the shadow profile.
//
// The runbook is the authoritative artifact that encodes the cutover
// contract for operators; this test asserts it documents BOTH halves of
// the contract (default-to-nt-cli AND explicit opt-in to roll back).
func TestRunbook_FullCutoverDefault(t *testing.T) {
	doc := loadFile(t, runbookPath(t))

	t.Run("default profile ships with nt-cli as the canonical backend", func(t *testing.T) {
		// The Full cutover phase MUST be described and MUST commit to
		// nt-cli being the canonical backend in the default host profile.
		containsAllNoCase(t, doc, []string{
			"full cutover",
			"default host profile",
			"nt-cli",
			"canonical",
		}, "runbook full-cutover description")
	})

	t.Run("explicit opt-in documents the rollback toggle", func(t *testing.T) {
		// The runbook MUST document the opt-in toggle that rolls the
		// host back to the shadow configuration.
		lower := strings.ToLower(doc)
		if !strings.Contains(lower, "opt-in") {
			t.Fatalf("runbook MUST document an explicit opt-in path; phrase not found")
		}
		// The toggle MUST be tied to the shadow / pilot profile so
		// operators understand it is reversible, not "off forever".
		togglePhrasings := []string{
			"shadow",
			"pilot",
			"toggle",
		}
		hits := 0
		for _, p := range togglePhrasings {
			if strings.Contains(lower, p) {
				hits++
			}
		}
		if hits < 2 {
			t.Fatalf("runbook MUST document the opt-in via the shadow/pilot toggle; needed at least 2 of %v", togglePhrasings)
		}
	})
}

// TestRunbook_GateFailureBlocksAdvancement proves the spec scenario
// "Gate failure blocks advancement": the runbook MUST instruct the
// operator to hold at the current phase AND MUST require naming the
// failing gate in the hold notice.
func TestRunbook_GateFailureBlocksAdvancement(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	// Hold instruction
	holdPhrasings := []string{"hold at the current phase", "do not advance"}
	holdHit := false
	for _, p := range holdPhrasings {
		if strings.Contains(lower, p) {
			holdHit = true
			break
		}
	}
	if !holdHit {
		t.Fatalf("runbook MUST instruct hold-at-current-phase on gate failure; none of %v found", holdPhrasings)
	}

	// Naming the failing gate. The runbook may wrap "name the failing
	// gate" across lines, so normalise whitespace before searching.
	flat := strings.Join(strings.Fields(lower), " ")
	if !strings.Contains(flat, "name the failing gate") &&
		!strings.Contains(flat, "name the gate") {
		t.Fatalf("runbook MUST require naming the failing gate in the hold notice")
	}

	// All gates G1..G6 must be enumerated so the operator can identify
	// which one fired.
	for _, gate := range []string{"G1", "G2", "G3", "G4", "G5", "G6"} {
		if !strings.Contains(doc, gate) {
			t.Fatalf("runbook MUST enumerate gate %s", gate)
		}
	}
}

// TestRunbook_RollbackTriggersAndScope proves the spec scenarios
// under "Rollback Semantics":
//   - "Rollback trigger fires": each documented trigger condition is named.
//   - "DB corruption suspected": snapshot restore steps are present.
//   - "Rollback is scoped to nt-cli": the runbook commits to it.
func TestRunbook_RollbackTriggersAndScope(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	t.Run("all four trigger conditions named", func(t *testing.T) {
		// Map each spec trigger to a substring that MUST appear in the
		// rollback section. This is the property the test enforces.
		triggers := map[string]string{
			"parity test failure":         "parity_test.go",
			"data loss":                   "data loss",
			"mcp tool registration error": "tool registration error",
			"soak window error rate":      "soak",
		}
		for label, needle := range triggers {
			if !strings.Contains(lower, strings.ToLower(needle)) {
				t.Fatalf("runbook MUST document trigger %q (looking for %q)", label, needle)
			}
		}
	})

	t.Run("DB snapshot restore steps documented", func(t *testing.T) {
		// Snapshot restoration MUST mention the snapshot path, cp, and
		// re-running parity to confirm restoration.
		needles := []string{
			"~/.nt-cli/data.db",
			"snapshot",
			"cp ",
			"parity_test.go",
		}
		for _, n := range needles {
			if !strings.Contains(doc, n) {
				t.Fatalf("DB restore steps MUST mention %q", n)
			}
		}
	})

	t.Run("rollback is scoped to nt-cli", func(t *testing.T) {
		// MUST contain an explicit scoping/non-destructive guarantee
		// expressed in nt-cli-local terms (no external backend named).
		// Accept several canonical phrasings.
		scopePhrasings := []string{
			"scoped to nt-cli",
			"does not modify or delete data outside",
			"reversible in a single edit",
			"~/.nt-cli/",
		}
		hit := false
		for _, p := range scopePhrasings {
			if strings.Contains(lower, p) {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatalf("runbook MUST state the rollback is scoped to nt-cli; none of %v found", scopePhrasings)
		}
	})

	t.Run("post-mortem note via nt-cli is required", func(t *testing.T) {
		// Spec: "a post-mortem note MUST be saved via nt-cli".
		if !strings.Contains(lower, "post-mortem") {
			t.Fatalf("runbook MUST require a post-mortem note after rollback")
		}
		if !strings.Contains(lower, "nt-cli save") {
			t.Fatalf("runbook MUST direct the post-mortem to be saved via `nt-cli save`")
		}
	})
}

// TestRunbook_ObservabilityNTCLIMCPDebug proves the spec scenarios under
// "Observability and Reporting":
//   - "Debug path documented": NTCLI_MCP_DEBUG activation steps are present.
//   - "structured logs sufficient to identify the failing tool": example log
//     output identifies the failing tool.
func TestRunbook_ObservabilityNTCLIMCPDebug(t *testing.T) {
	doc := loadFile(t, runbookPath(t))

	// Activation steps must be present and use the documented env var.
	if !strings.Contains(doc, "NTCLI_MCP_DEBUG") {
		t.Fatalf("runbook MUST document NTCLI_MCP_DEBUG")
	}
	if !strings.Contains(doc, "NTCLI_MCP_DEBUG=1") {
		t.Fatalf("runbook MUST show how to activate NTCLI_MCP_DEBUG (=1)")
	}

	// Structured log example MUST identify the failing tool by name.
	// Accept any of the documented tool names since the example may rotate.
	knownTools := []string{
		"local_save", "local_recall", "local_list",
		"local_get", "local_update", "local_delete",
	}
	lower := strings.ToLower(doc)
	hit := false
	for _, tool := range knownTools {
		if strings.Contains(lower, "tool="+tool) {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatalf("runbook example log output MUST identify the failing tool (tool=<name>); none of %v present", knownTools)
	}

	// Example MUST show a status=error line so operators see what failure
	// telemetry looks like.
	if !strings.Contains(lower, "status=error") {
		t.Fatalf("runbook example log MUST include a status=error line")
	}
}

// TestREADME_PhaseStatusAndRollbackTriggers proves the spec scenario
// "Phase status reported": README discloses the current phase AND the
// rollback trigger list MUST be linked from the same place.
func TestREADME_PhaseStatusAndRollbackTriggers(t *testing.T) {
	doc := loadFile(t, readmePath(t))
	lower := strings.ToLower(doc)

	// Phase indicator
	if !strings.Contains(lower, "fase actual") && !strings.Contains(lower, "current phase") {
		t.Fatalf("README MUST surface the current cutover phase (Fase actual / Current phase)")
	}
	// Phase value MUST be one of shadow|partial|full so it's meaningful.
	hasPhaseValue := false
	for _, p := range []string{"shadow", "partial", "full"} {
		if strings.Contains(lower, p) {
			hasPhaseValue = true
			break
		}
	}
	if !hasPhaseValue {
		t.Fatalf("README MUST name the active phase (shadow|partial|full)")
	}

	// Rollback triggers MUST be listed (or linked) from README.
	if !strings.Contains(lower, "rollback") {
		t.Fatalf("README MUST surface rollback information")
	}
	if !strings.Contains(lower, "trigger") {
		t.Fatalf("README MUST list rollback triggers (or call them out by name)")
	}
	// The rollback section must link to the runbook's rollback anchor so
	// "the rollback trigger list MUST be linked from the same place" holds.
	if !strings.Contains(doc, "rollout-runbook.md#rollback-runbook") {
		t.Fatalf("README rollback section MUST link to docs/rollout-runbook.md#rollback-runbook")
	}
}

// TestRunbook_SoakWindowDocumented proves the spec scenario
// "Soak window measured": the runbook MUST document a finite soak window
// and tie its clean completion to the readiness to advance to full.
func TestRunbook_SoakWindowDocumented(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	if !strings.Contains(lower, "soak") {
		t.Fatalf("runbook MUST document a soak window")
	}
	// Soak must have a concrete duration (e.g. "7 calendar days") so it's
	// measurable, not hand-wavy.
	durationPhrasings := []string{
		"calendar day", "calendar days",
		"business day", "business days",
		"hours", "weeks",
	}
	hit := false
	for _, p := range durationPhrasings {
		if strings.Contains(lower, p) {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatalf("soak window MUST have a measurable duration; none of %v found", durationPhrasings)
	}
	// Clean completion path MUST tie to advancement.
	advance := []string{"advance to full", "advance to **full**", "advance"}
	advanceHit := false
	for _, p := range advance {
		if strings.Contains(lower, p) {
			advanceHit = true
			break
		}
	}
	if !advanceHit {
		t.Fatalf("runbook MUST tie soak completion to advancement")
	}
}

// TestRunbook_ShadowSampleConfigShowsToggle proves the spec scenario
// "Sample config publishes shadow-by-default toggle": the runbook MUST
// publish a sample host config snippet that registers `nt-cli`, names
// `NTCLI_PROFILE=shadow` as the default, and illustrates the pilot
// variant as the single-config-change rollback path.
func TestRunbook_ShadowSampleConfigShowsToggle(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	// The sample snippet block MUST register the ntcli MCP entry.
	if !strings.Contains(lower, "\"ntcli\"") {
		t.Fatalf("runbook sample config MUST register an \"ntcli\" MCP server entry")
	}

	// And the snippet MUST identify shadow as the default profile in the
	// "environment" section so the copy-paste path is shadow-by-default.
	if !strings.Contains(lower, "ntcli_profile") || !strings.Contains(lower, "shadow") {
		t.Fatalf("runbook sample config MUST set NTCLI_PROFILE=shadow as the default")
	}

	// And the toggle to flip to pilot MUST be a single-config-change
	// affordance (commented variant or equivalent), per spec
	// "the toggle MUST be a single documented config change".
	if !strings.Contains(lower, "pilot") {
		t.Fatalf("runbook sample config MUST illustrate the pilot variant as the single-toggle path")
	}
}

// TestParityGate_RollbackTriggerDetection_Triangulation hardens
// IsRollbackTrigger against false positives by asserting case-insensitivity
// and the empty / whitespace-only safety property.
func TestParityGate_RollbackTriggerDetection_Triangulation(t *testing.T) {
	t.Run("case-insensitive matching", func(t *testing.T) {
		if !IsRollbackTrigger("PARITY_TEST.GO failed") {
			t.Errorf("matching MUST be case-insensitive")
		}
		if !IsRollbackTrigger("Data Loss reported by user") {
			t.Errorf("matching MUST be case-insensitive (data loss)")
		}
	})

	t.Run("whitespace and empty are not triggers", func(t *testing.T) {
		for _, in := range []string{"", "   ", "\t\n"} {
			if IsRollbackTrigger(in) {
				t.Errorf("IsRollbackTrigger(%q) must be false", in)
			}
		}
	})

	t.Run("unrelated noise is not a trigger", func(t *testing.T) {
		// An unrelated incident report MUST NOT be classified as a
		// rollback trigger so alerting cannot misfire.
		for _, in := range []string{
			"user requested a feature",
			"build cache invalidated",
			"linter reported style warning",
		} {
			if IsRollbackTrigger(in) {
				t.Errorf("IsRollbackTrigger(%q) must be false", in)
			}
		}
	})

	t.Run("returned trigger set is independent (defensive copy)", func(t *testing.T) {
		// Mutating the returned slice MUST NOT affect future calls,
		// otherwise a misbehaving caller could silently break the
		// canonical set for the rest of the process.
		first := RollbackTriggers()
		if len(first) == 0 {
			t.Fatal("RollbackTriggers() returned empty set")
		}
		first[0] = RollbackTrigger{Name: "tampered", RunbookKeyword: "tampered"}
		second := RollbackTriggers()
		if second[0].Name == "tampered" {
			t.Fatalf("RollbackTriggers() MUST return a defensive copy")
		}
	})
}

// TestParityGate_RollbackTriggerDetection proves the spec scenario
// "Rollback trigger fires" at the code level: a pure helper MUST classify
// whether a given condition is a documented rollback trigger so that
// future automation (alerts, dashboards) can act on the same contract
// the runbook commits to. This is intentionally a small, pure function:
// the value is in proving the trigger SET is encoded once and matches
// the runbook.
func TestParityGate_RollbackTriggerDetection(t *testing.T) {
	t.Run("documented triggers are recognised", func(t *testing.T) {
		cases := []struct {
			name      string
			condition string
			want      bool
		}{
			{"parity test failure", "parity_test.go failed after a previously-green run", true},
			{"data loss report", "user reported data loss attributable to nt-cli", true},
			{"mcp registration error", "MCP tool registration error on host startup", true},
			{"soak threshold exceeded", "soak-window error rate above threshold", true},
			{"unrelated noise", "user prefers dark mode", false},
			{"empty", "", false},
		}
		for _, tc := range cases {
			got := IsRollbackTrigger(tc.condition)
			if got != tc.want {
				t.Errorf("IsRollbackTrigger(%q) = %v, want %v", tc.condition, got, tc.want)
			}
		}
	})

	t.Run("trigger set matches runbook content", func(t *testing.T) {
		// Every canonical trigger label returned by RollbackTriggers()
		// MUST be discoverable in the runbook so the code-level contract
		// and the operator-facing contract cannot drift.
		doc := strings.ToLower(loadFile(t, runbookPath(t)))
		triggers := RollbackTriggers()
		if len(triggers) < 4 {
			t.Fatalf("expected at least 4 documented triggers, got %d", len(triggers))
		}
		for _, trig := range triggers {
			if !strings.Contains(doc, strings.ToLower(trig.RunbookKeyword)) {
				t.Errorf("runbook missing keyword %q for trigger %q", trig.RunbookKeyword, trig.Name)
			}
		}
	})
}

// TestRunbook_ParityScorecardContract proves task 1.6 of ntcli-singularity:
// the runbook MUST document the parity scorecard contract — its 7 weighted
// dimensions, the critical-floor concept, the 14-day soak window, and the
// fact that the scorecard verdict supersedes binary G1/G2 while G3–G6
// remain independent preconditions. Locking the wording here prevents
// silent contract drift between code and operator docs.
func TestRunbook_ParityScorecardContract(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	// The runbook MUST introduce the scorecard concept.
	containsAllNoCase(t, doc, []string{
		"parity scorecard",
	}, "runbook scorecard introduction")

	// All 7 dimension names MUST appear so operators recognise the
	// vocabulary surfaced by the CLI/MCP verdict payload.
	for _, dim := range []string{
		"core-ops",
		"metadata-retrieval",
		"session-workflow",
		"import-export-backup",
		"reliability-operability",
		"knowledge-continuity",
		"ux-api-contract",
	} {
		if !strings.Contains(lower, dim) {
			t.Fatalf("runbook MUST document scorecard dimension %q", dim)
		}
	}

	// The 95-threshold and 14-day soak window MUST be discoverable.
	if !strings.Contains(lower, "95") {
		t.Fatalf("runbook MUST document the >=95 total threshold for verdict=pass")
	}
	if !strings.Contains(lower, "14") {
		t.Fatalf("runbook MUST document the 14-day soak window")
	}

	// The runbook MUST commit that the scorecard verdict supersedes
	// binary G1/G2 while G3–G6 stay independent preconditions.
	for _, phrase := range []string{
		"supersedes",
		"g3",
		"g6",
	} {
		if !strings.Contains(lower, phrase) {
			t.Fatalf("runbook MUST document scorecard relationship to gates; missing %q", phrase)
		}
	}
}

// TestRunbook_ContinuityHarness proves task 2.5 of ntcli-singularity:
// the runbook MUST document how operators record and replay the
// knowledge-continuity baseline that feeds the scorecard's
// KnowledgeContinuity dimension and is consumed by PR5 to assert
// `delta_pct ≤ -35`. Locking the wording here keeps the harness
// contract (fixture path, baseline.json output, CLI command) from
// drifting silently between code and operator docs.
func TestRunbook_ContinuityHarness(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	// The runbook MUST introduce the harness as a discoverable section.
	containsAllNoCase(t, doc, []string{
		"knowledge-continuity harness",
	}, "runbook continuity harness section")

	// Operators MUST be able to find the CLI command, the fixture
	// path, and the baseline output filename. These are the three
	// nouns the runbook needs so a new on-call can run the harness
	// without reading the code.
	for _, phrase := range []string{
		"parity continuity",
		"testdata/parity/queries.json",
		"baseline.json",
	} {
		if !strings.Contains(lower, phrase) {
			t.Fatalf("runbook MUST document continuity harness artifact %q", phrase)
		}
	}

	// The harness output feeds the KnowledgeContinuity scorecard
	// dimension and is replayed by PR5 to assert delta_pct ≤ -35.
	// Both relationships MUST be discoverable so operators understand
	// why the baseline matters.
	for _, phrase := range []string{
		"knowledge-continuity",
		"delta_pct",
	} {
		if !strings.Contains(lower, phrase) {
			t.Fatalf("runbook MUST document continuity harness consumer %q", phrase)
		}
	}
}
