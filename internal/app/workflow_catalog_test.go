package app

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkflowCatalog_MissingSystemFallsBackToDevOnly(t *testing.T) {
	tmp := t.TempDir()
	systemPath := filepath.Join(tmp, "missing-workflows.json")
	userPath := filepath.Join(tmp, "missing-user-workflows.json")

	var logs bytes.Buffer
	origWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(origWriter) })

	catalog, err := LoadWorkflowCatalog(systemPath, userPath)
	if err != nil {
		t.Fatalf("expected fallback catalog, got error: %v", err)
	}

	wf, ok := catalog.Workflows["dev"]
	if !ok {
		t.Fatalf("expected dev workflow in fallback catalog, got %+v", catalog.Workflows)
	}
	if len(wf.Phases) == 0 {
		t.Fatalf("expected fallback dev workflow to define phases")
	}
	if !strings.Contains(logs.String(), "warning") {
		t.Fatalf("expected warning log when system catalog missing, got %q", logs.String())
	}
}

func TestLoadWorkflowCatalog_MalformedSystemReturnsError(t *testing.T) {
	tmp := t.TempDir()
	systemPath := filepath.Join(tmp, "workflows.json")
	if err := os.WriteFile(systemPath, []byte("{ malformed"), 0o644); err != nil {
		t.Fatalf("write system catalog: %v", err)
	}

	_, err := LoadWorkflowCatalog(systemPath, filepath.Join(tmp, "user-workflows.json"))
	if err == nil {
		t.Fatalf("expected malformed system catalog to fail")
	}
}

func TestLoadWorkflowCatalog_MergesUserOverrides(t *testing.T) {
	tmp := t.TempDir()
	systemPath := filepath.Join(tmp, "workflows.json")
	userPath := filepath.Join(tmp, "user-workflows.json")

	system := WorkflowCatalog{
		Workflows: map[string]WorkflowDef{
			"dev": {
				Description: "system dev",
				Phases:      []string{"spark", "explore"},
			},
			"creative": {
				Description: "creative",
				Phases:      []string{"expand"},
			},
		},
		Phases: map[string]PhaseDef{
			"spark":   {Agent: "nt-spark", Capability: "route"},
			"explore": {Agent: "nt-sdd-explore", Capability: "explore"},
			"expand":  {Agent: "nt-creative-expand", Capability: "expand"},
		},
	}
	writeCatalogJSON(t, systemPath, system)

	user := WorkflowCatalog{
		Workflows: map[string]WorkflowDef{
			"dev": {
				Description: "user dev",
				Phases:      []string{"spark", "apply"},
			},
		},
		Phases: map[string]PhaseDef{
			"apply": {Agent: "nt-sdd-apply", Capability: "implement"},
		},
	}
	writeCatalogJSON(t, userPath, user)

	got, err := LoadWorkflowCatalog(systemPath, userPath)
	if err != nil {
		t.Fatalf("LoadWorkflowCatalog error: %v", err)
	}

	if got.Workflows["dev"].Description != "user dev" {
		t.Fatalf("expected user override for dev workflow, got %+v", got.Workflows["dev"])
	}
	if _, ok := got.Workflows["creative"]; !ok {
		t.Fatalf("expected untouched system workflow to remain")
	}
	if got.Phases["apply"].Agent != "nt-sdd-apply" {
		t.Fatalf("expected user phase override to be merged, got %+v", got.Phases["apply"])
	}
}

func TestValidateWorkflowCatalog_MissingPhaseMapping(t *testing.T) {
	c := &WorkflowCatalog{
		Workflows: map[string]WorkflowDef{
			"dev": {Phases: []string{"spark", "explore"}},
		},
		Phases: map[string]PhaseDef{
			"spark": {Agent: "nt-spark", Capability: "route"},
		},
	}

	err := ValidateWorkflowCatalog(c)
	if err == nil {
		t.Fatalf("expected missing mapping validation error")
	}
	if err.Error() != `invalid catalog: phase "explore" has no mapping` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWorkflowCatalog_EmptyAgent(t *testing.T) {
	c := &WorkflowCatalog{
		Workflows: map[string]WorkflowDef{
			"dev": {Phases: []string{"spark"}},
		},
		Phases: map[string]PhaseDef{
			"spark": {Agent: "", Capability: "route"},
		},
	}

	err := ValidateWorkflowCatalog(c)
	if err == nil {
		t.Fatalf("expected empty agent validation error")
	}
	if err.Error() != `invalid catalog: phase "spark" has empty agent in phases map` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeCatalogJSON(t *testing.T, path string, c WorkflowCatalog) {
	t.Helper()
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}
