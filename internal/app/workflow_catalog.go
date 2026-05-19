package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
)

// WorkflowCatalog is the runtime representation of workflows.json.
type WorkflowCatalog struct {
	Schema    string                 `json:"$schema,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Workflows map[string]WorkflowDef `json:"workflows"`
	Phases    map[string]PhaseDef    `json:"phases"`
}

// WorkflowDef describes one workflow entry in the catalog.
type WorkflowDef struct {
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Phases      []string `json:"phases"`
	Tags        []string `json:"tags,omitempty"`
	Agents      []string `json:"agents,omitempty"`
}

// PhaseDef maps a phase to its handling agent/capability.
type PhaseDef struct {
	Agent      string `json:"agent"`
	Capability string `json:"capability"`
}

// DefaultDevOnlyCatalog returns a minimal dev-only fallback catalog.
func DefaultDevOnlyCatalog() *WorkflowCatalog {
	return &WorkflowCatalog{
		Version: "1",
		Workflows: map[string]WorkflowDef{
			"dev": {
				Description: "Development (fallback)",
				Phases:      []string{"spark", "explore", "propose", "spec", "design", "tasks", "apply", "verify", "archive"},
			},
		},
		Phases: map[string]PhaseDef{
			"spark":   {Agent: "nt-spark", Capability: "intent routing"},
			"explore": {Agent: "nt-sdd-explore", Capability: "problem exploration"},
			"propose": {Agent: "nt-sdd-propose", Capability: "change proposal"},
			"spec":    {Agent: "nt-sdd-spec", Capability: "requirements specification"},
			"design":  {Agent: "nt-sdd-design", Capability: "technical design"},
			"tasks":   {Agent: "nt-sdd-tasks", Capability: "implementation planning"},
			"apply":   {Agent: "nt-sdd-apply", Capability: "implementation"},
			"verify":  {Agent: "nt-sdd-verify", Capability: "spec verification"},
			"archive": {Agent: "nt-sdd-archive", Capability: "change closure"},
		},
	}
}

// LoadWorkflowCatalog loads system and optional user catalog, merges them,
// and validates catalog invariants.
func LoadWorkflowCatalog(systemPath, userPath string) (*WorkflowCatalog, error) {
	system, err := readWorkflowCatalog(systemPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("warning: workflow catalog missing at %s; using dev-only fallback", systemPath)
			system = DefaultDevOnlyCatalog()
		} else {
			return nil, err
		}
	}

	merged := cloneWorkflowCatalog(system)

	if strings.TrimSpace(userPath) != "" {
		user, err := readWorkflowCatalog(userPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		} else {
			mergeWorkflowCatalog(merged, user)
		}
	}

	if err := ValidateWorkflowCatalog(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

// ValidateWorkflowCatalog validates phase mapping invariants.
func ValidateWorkflowCatalog(catalog *WorkflowCatalog) error {
	if catalog == nil {
		return fmt.Errorf("invalid catalog: nil")
	}
	for _, workflow := range catalog.Workflows {
		for _, phase := range workflow.Phases {
			mapped, ok := catalog.Phases[phase]
			if !ok {
				return fmt.Errorf("invalid catalog: phase %q has no mapping", phase)
			}
			if strings.TrimSpace(mapped.Agent) == "" {
				return fmt.Errorf("invalid catalog: phase %q has empty agent in phases map", phase)
			}
			if strings.TrimSpace(mapped.Capability) == "" {
				return fmt.Errorf("invalid catalog: phase %q has empty capability in phases map", phase)
			}
		}
	}
	return nil
}

func readWorkflowCatalog(path string) (*WorkflowCatalog, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c WorkflowCatalog
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, err
	}
	if c.Workflows == nil {
		c.Workflows = map[string]WorkflowDef{}
	}
	if c.Phases == nil {
		c.Phases = map[string]PhaseDef{}
	}
	return &c, nil
}

func cloneWorkflowCatalog(in *WorkflowCatalog) *WorkflowCatalog {
	out := &WorkflowCatalog{
		Schema:    in.Schema,
		Version:   in.Version,
		Workflows: map[string]WorkflowDef{},
		Phases:    map[string]PhaseDef{},
	}
	for k, v := range in.Workflows {
		out.Workflows[k] = v
	}
	for k, v := range in.Phases {
		out.Phases[k] = v
	}
	return out
}

func mergeWorkflowCatalog(dst, src *WorkflowCatalog) {
	if strings.TrimSpace(src.Schema) != "" {
		dst.Schema = src.Schema
	}
	if strings.TrimSpace(src.Version) != "" {
		dst.Version = src.Version
	}
	for k, v := range src.Workflows {
		dst.Workflows[k] = v
	}
	for k, v := range src.Phases {
		dst.Phases[k] = v
	}
}
