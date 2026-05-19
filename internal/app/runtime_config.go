package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flint/internal/model"
)

const (
	runtimeTypeOpenCode = "opencode"
)

// Runtime represents a supported runtime host.
type Runtime interface {
	Name() string
	SupportsAgents() bool
	AgentConfigPath() string
}

// OpenCodeRuntime is the built-in runtime host.
type OpenCodeRuntime struct{}

func (OpenCodeRuntime) Name() string { return runtimeTypeOpenCode }

func (OpenCodeRuntime) SupportsAgents() bool { return true }

func (OpenCodeRuntime) AgentConfigPath() string { return "~/.config/opencode/opencode.json" }

// DefaultRuntimeConfig returns safe defaults for OpenCode
func DefaultRuntimeConfig() model.RuntimeConfig {
	agentPath := "~/.config/opencode/opencode.json"
	mcpScript := "~/.local/bin/nt-cli mcp serve"
	if expanded, err := expandHome(agentPath); err == nil && strings.TrimSpace(expanded) != "" {
		agentPath = expanded
	}
	if expanded, err := expandHome(mcpScript); err == nil && strings.TrimSpace(expanded) != "" {
		mcpScript = expanded
	}

	return model.RuntimeConfig{
		Version: "v1",
		Runtime: model.RuntimeOpenCode,
		RuntimeDef: model.RuntimeDef{
			Type:            runtimeTypeOpenCode,
			AgentConfigPath: agentPath,
			MCPScript:       mcpScript,
		},
		Models: model.ModelTiers{
			Thinking:  "claude-opus-4-5",
			Execution: "claude-sonnet-4-5",
			Fast:      "claude-haiku-4-5",
			Planning:  "claude-sonnet-4-5",
		},
		PrimaryDomain: "dev",
	}
}

// RuntimeConfigPath returns ~/.flint/config.json.
func RuntimeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("home directory is empty")
	}
	return filepath.Join(home, ".flint", "config.json"), nil
}

func runtimeConfigPath() (string, error) { return RuntimeConfigPath() }

type runtimeConfigDisk struct {
	Version       string          `json:"version"`
	Runtime       json.RawMessage `json:"runtime"`
	Models        model.ModelTiers `json:"models"`
	PrimaryDomain string          `json:"primary_domain"`
}

// LoadRuntimeConfig reads ~/.flint/config.json; returns defaults if missing
func LoadRuntimeConfig() (model.RuntimeConfig, error) {
	path, err := RuntimeConfigPath()
	if err != nil {
		return model.RuntimeConfig{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultRuntimeConfig(), nil
		}
		return model.RuntimeConfig{}, err
	}

	var disk runtimeConfigDisk
	if err := json.Unmarshal(body, &disk); err != nil {
		return DefaultRuntimeConfig(), nil
	}

	cfg := DefaultRuntimeConfig()
	if strings.TrimSpace(disk.Version) != "" {
		cfg.Version = strings.TrimSpace(disk.Version)
	}
	if disk.Models.Thinking != "" {
		cfg.Models.Thinking = disk.Models.Thinking
	}
	if disk.Models.Execution != "" {
		cfg.Models.Execution = disk.Models.Execution
	}
	if disk.Models.Fast != "" {
		cfg.Models.Fast = disk.Models.Fast
	}
	if disk.Models.Planning != "" {
		cfg.Models.Planning = disk.Models.Planning
	}
	if strings.TrimSpace(disk.PrimaryDomain) != "" {
		cfg.PrimaryDomain = strings.TrimSpace(disk.PrimaryDomain)
	}

	if len(disk.Runtime) != 0 {
		var legacyRuntime string
		if err := json.Unmarshal(disk.Runtime, &legacyRuntime); err == nil {
			cfg.Runtime = model.RuntimeType(strings.TrimSpace(legacyRuntime))
			cfg.RuntimeDef.Type = strings.TrimSpace(legacyRuntime)
		} else {
			var def model.RuntimeDef
			if err := json.Unmarshal(disk.Runtime, &def); err != nil {
				return DefaultRuntimeConfig(), nil
			}
			if strings.TrimSpace(def.Type) != "" {
				cfg.RuntimeDef.Type = strings.TrimSpace(def.Type)
				cfg.Runtime = model.RuntimeType(strings.TrimSpace(def.Type))
			}
			if strings.TrimSpace(def.AgentConfigPath) != "" {
				cfg.RuntimeDef.AgentConfigPath = def.AgentConfigPath
			}
			if strings.TrimSpace(def.MCPScript) != "" {
				cfg.RuntimeDef.MCPScript = def.MCPScript
			}
		}
	}

	expanded, err := expandRuntimePaths(cfg.RuntimeDef)
	if err != nil {
		return model.RuntimeConfig{}, err
	}
	cfg.RuntimeDef = expanded

	if cfg.RuntimeDef.Type != string(model.RuntimeOpenCode) {
		cfg.Runtime = model.RuntimeType(cfg.RuntimeDef.Type)
	}
	if cfg.Runtime == "" {
		cfg.Runtime = model.RuntimeType(cfg.RuntimeDef.Type)
	}
	if err := cfg.Validate(); err != nil {
		return model.RuntimeConfig{}, err
	}
	return cfg, nil
}

// Save writes config to ~/.flint/config.json
func SaveRuntimeConfig(r model.RuntimeConfig) error {
	if r.Version == "" {
		r.Version = "v1"
	}
	if strings.TrimSpace(r.RuntimeDef.Type) == "" {
		r.RuntimeDef.Type = strings.TrimSpace(string(r.Runtime))
	}
	if r.Runtime == "" {
		r.Runtime = model.RuntimeType(strings.TrimSpace(r.RuntimeDef.Type))
	}
	if strings.TrimSpace(r.PrimaryDomain) == "" {
		r.PrimaryDomain = "dev"
	}
	if strings.TrimSpace(r.Models.Planning) == "" {
		r.Models.Planning = r.Models.Execution
	}
	if r.RuntimeDef.Type == runtimeTypeOpenCode {
		runtime := OpenCodeRuntime{}
		if strings.TrimSpace(r.RuntimeDef.AgentConfigPath) == "" {
			r.RuntimeDef.AgentConfigPath = runtime.AgentConfigPath()
		}
		if strings.TrimSpace(r.RuntimeDef.MCPScript) == "" {
			r.RuntimeDef.MCPScript = "~/.local/bin/nt-cli mcp serve"
		}
	}

	path, err := RuntimeConfigPath()
	if err != nil {
		return err
	}
	home := filepath.Dir(filepath.Dir(path))
	compressed, err := compressRuntimePaths(r.RuntimeDef, home)
	if err != nil {
		return err
	}
	r.RuntimeDef = compressed
	return r.Save()
}

func detectAvailableRuntimes() []Runtime {
	return []Runtime{OpenCodeRuntime{}}
}

func expandRuntimePaths(def model.RuntimeDef) (model.RuntimeDef, error) {
	agentPath, err := expandHome(def.AgentConfigPath)
	if err != nil {
		return model.RuntimeDef{}, fmt.Errorf("expand runtime.agent_config_path: %w", err)
	}
	mcp, err := expandHome(def.MCPScript)
	if err != nil {
		return model.RuntimeDef{}, fmt.Errorf("expand runtime.mcp_script: %w", err)
	}
	def.AgentConfigPath = agentPath
	def.MCPScript = mcp
	return def, nil
}

func compressRuntimePaths(def model.RuntimeDef, home string) (model.RuntimeDef, error) {
	h := strings.TrimSpace(home)
	if h == "" {
		return model.RuntimeDef{}, fmt.Errorf("home directory is empty")
	}
	def.AgentConfigPath = compressHome(def.AgentConfigPath, h)
	def.MCPScript = compressHome(def.MCPScript, h)
	return def, nil
}

func expandHome(value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", nil
	}
	if v == "~" || strings.HasPrefix(v, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(home) == "" {
			return "", errors.New("home directory is empty")
		}
		if v == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(v, "~/")), nil
	}
	return v, nil
}

func compressHome(value, home string) string {
	v := strings.TrimSpace(value)
	h := strings.TrimSpace(home)
	if v == "" || h == "" {
		return v
	}
	if v == h {
		return "~"
	}
	prefix := h + string(filepath.Separator)
	if strings.HasPrefix(v, prefix) {
		return "~/" + strings.TrimPrefix(v, prefix)
	}
	return v
}
