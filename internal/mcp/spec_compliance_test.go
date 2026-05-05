package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runbookPath returns the absolute path to docs/engram-offramp.md regardless
// of the package's working directory at test time.
func runbookPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve caller path")
	}
	// internal/mcp/spec_compliance_test.go → ../../docs/engram-offramp.md
	return filepath.Join(filepath.Dir(file), "..", "..", "docs", "engram-offramp.md")
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
//   - "Default profile is Engram-off": Engram tools MUST NOT be registered
//     when no user override is set.
//   - "Opt-in restores Engram": setting the documented opt-in flag MUST
//     re-register Engram alongside nt-cli.
//
// The runbook is the authoritative artifact that encodes the cutover
// contract for operators; this test asserts it documents BOTH halves of
// the contract (off-by-default AND explicit opt-in to restore).
func TestRunbook_FullCutoverDefault(t *testing.T) {
	doc := loadFile(t, runbookPath(t))

	t.Run("default profile ships with Engram off", func(t *testing.T) {
		// The Full cutover phase MUST be described and MUST commit to
		// Engram tools being off in the default host profile.
		containsAllNoCase(t, doc, []string{
			"full cutover",
			"default host profile",
			"engram",
		}, "runbook full-cutover description")

		// The runbook MUST state Engram is "off" / "not registered" /
		// equivalent in the default profile. Accept any of the canonical
		// phrasings to allow doc edits without breaking the test.
		lower := strings.ToLower(doc)
		offPhrasings := []string{
			"engram memory tools off",
			"engram memory tools are off",
			"engram tools off",
			"engram off",
			"not registered",
		}
		hit := false
		for _, p := range offPhrasings {
			if strings.Contains(lower, p) {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatalf("runbook MUST state Engram is off/not-registered in the default profile; none of %v found", offPhrasings)
		}
	})

	t.Run("explicit opt-in restores Engram", func(t *testing.T) {
		// The runbook MUST document the opt-in path (explicit flag /
		// config change) that re-enables Engram.
		lower := strings.ToLower(doc)
		// "opt-in" must appear AND it must be tied to restoring/enabling Engram.
		if !strings.Contains(lower, "opt-in") {
			t.Fatalf("runbook MUST document an explicit opt-in path to restore Engram; phrase not found")
		}
		// The runbook must also mention that the toggle is reversible /
		// re-enables Engram, not just that it's "off forever".
		restorePhrasings := []string{
			"re-enabled",
			"re-enable",
			"restored",
			"restore engram",
			"opt-in flag",
			"explicit opt-in",
		}
		hit := false
		for _, p := range restorePhrasings {
			if strings.Contains(lower, p) {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatalf("runbook MUST document how the opt-in restores/re-enables Engram; none of %v found", restorePhrasings)
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

// TestRunbook_RollbackTriggersAndNonDestructive proves the spec scenarios
// under "Rollback Semantics":
//   - "Rollback trigger fires": each documented trigger condition is named.
//   - "DB corruption suspected": snapshot restore steps are present.
//   - "Rollback is non-destructive to Engram": the runbook commits to it.
func TestRunbook_RollbackTriggersAndNonDestructive(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	t.Run("all four trigger conditions named", func(t *testing.T) {
		// Map each spec trigger to a substring that MUST appear in the
		// rollback section. This is the property the test enforces.
		triggers := map[string]string{
			"parity test failure":        "parity_test.go",
			"data loss":                  "data loss",
			"mcp tool registration error": "tool registration error",
			"soak window error rate":     "soak",
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

	t.Run("rollback is non-destructive to Engram", func(t *testing.T) {
		// MUST contain an explicit non-destructive guarantee.
		// Accept several canonical phrasings.
		nonDestructive := []string{
			"non-destructive to engram",
			"no engram data",
			"no engram data is",
			"no engram data must be modified",
			"does not modify or delete",
		}
		hit := false
		for _, p := range nonDestructive {
			if strings.Contains(lower, p) {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatalf("runbook MUST state rollback is non-destructive to Engram; none of %v found", nonDestructive)
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
	if !strings.Contains(doc, "engram-offramp.md#rollback-runbook") {
		t.Fatalf("README rollback section MUST link to docs/engram-offramp.md#rollback-runbook")
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

// TestRunbook_ShadowSampleConfigShowsBothBackends proves the spec scenario
// "Both backends registered": the runbook MUST publish a sample host
// config snippet in which BOTH Engram and nt-cli are registered (so an
// operator copy-pasting the snippet ends up in shadow mode by default,
// not silently in pilot).
func TestRunbook_ShadowSampleConfigShowsBothBackends(t *testing.T) {
	doc := loadFile(t, runbookPath(t))
	lower := strings.ToLower(doc)

	// The sample snippet block MUST register both servers.
	if !strings.Contains(lower, "\"engram\"") {
		t.Fatalf("runbook sample config MUST register an \"engram\" MCP server entry")
	}
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
	// "the toggle to restore Engram MUST be a single documented config change".
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
