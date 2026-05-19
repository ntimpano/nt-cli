package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeType identifies the AI runtime host
type RuntimeType string

const (
	RuntimeOpenCode RuntimeType = "opencode"
	RuntimeUnknown  RuntimeType = "unknown"
)

// RuntimeDef defines runtime-specific integration settings.
type RuntimeDef struct {
	Type            string `json:"type"`
	AgentConfigPath string `json:"agent_config_path"`
	MCPScript       string `json:"mcp_script"`
}

// ModelTiers defines model preferences per task complexity
type ModelTiers struct {
	Thinking  string `json:"thinking"`  // complex reasoning tasks
	Execution string `json:"execution"` // standard implementation
	Fast      string `json:"fast"`      // quick lookups, summaries
	Planning  string `json:"planning"`  // planning/coordination work
}

// RuntimeConfig is stored at ~/.nt-cli/config.json
type RuntimeConfig struct {
	Version       string     `json:"version,omitempty"`
	Runtime       RuntimeType `json:"-"` // backward-compatible field used by current app code
	RuntimeDef    RuntimeDef `json:"runtime"`
	Models        ModelTiers `json:"models"`
	PrimaryDomain string     `json:"primary_domain"`
}

func (c RuntimeConfig) Validate() error {
	rt := c.RuntimeDef.Type
	if rt == "" {
		rt = string(c.Runtime)
	}
	switch rt {
	case string(RuntimeOpenCode):
		// valid
	case string(RuntimeUnknown):
		// backward-compatible placeholder runtime
	default:
		return fmt.Errorf("unsupported runtime type: %s", rt)
	}

	if c.Models.Thinking == "" {
		return fmt.Errorf("models.thinking cannot be empty")
	}
	if c.Models.Execution == "" {
		return fmt.Errorf("models.execution cannot be empty")
	}
	if c.Models.Fast == "" {
		return fmt.Errorf("models.fast cannot be empty")
	}
	if c.Models.Planning == "" {
		return fmt.Errorf("models.planning cannot be empty")
	}

	switch c.PrimaryDomain {
	case "dev", "creative", "strategy", "research":
		return nil
	default:
		return fmt.Errorf("unsupported primary domain: %s", c.PrimaryDomain)
	}
}

// Save persists runtime config atomically at ~/.nt-cli/config.json.
func (c RuntimeConfig) Save() error {
	if c.Version == "" {
		c.Version = "v1"
	}
	if c.RuntimeDef.Type == "" {
		c.RuntimeDef.Type = string(c.Runtime)
	}
	if c.Runtime == "" {
		c.Runtime = RuntimeType(c.RuntimeDef.Type)
	}
	if err := c.Validate(); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if strings.TrimSpace(home) == "" {
		return fmt.Errorf("home directory is empty")
	}

	path := filepath.Join(home, ".nt-cli", "config.json")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}

	return nil
}

type ProfileConfig struct {
	Language          string `json:"language"`
	Tone              string `json:"tone"`
	Verbosity         string `json:"verbosity"`
	AskBeforeMutation bool   `json:"ask_before_mutation"`
	ContextAutoswitch bool   `json:"context_autoswitch"`
	PrimaryDomain     string `json:"primary_domain,omitempty"`
}
