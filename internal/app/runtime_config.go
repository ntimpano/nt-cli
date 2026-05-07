package app

import (
	"encoding/json"
	"errors"
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

// ModelTiers defines model preferences per task complexity
type ModelTiers struct {
	Thinking  string `json:"thinking"`  // complex reasoning tasks
	Execution string `json:"execution"` // standard implementation
	Fast      string `json:"fast"`      // quick lookups, summaries
}

// RuntimeConfig is stored at ~/.nt-cli/config.json
type RuntimeConfig struct {
	Runtime RuntimeType `json:"runtime"`
	Models  ModelTiers  `json:"models"`
}

// DefaultRuntimeConfig returns safe defaults for OpenCode
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Runtime: RuntimeOpenCode,
		Models: ModelTiers{
			Thinking:  "claude-opus-4-5",
			Execution: "claude-sonnet-4-5",
			Fast:      "claude-haiku-4-5",
		},
	}
}

func runtimeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("home directory is empty")
	}
	return filepath.Join(home, ".nt-cli", "config.json"), nil
}

// LoadRuntimeConfig reads ~/.nt-cli/config.json; returns defaults if missing
func LoadRuntimeConfig() (RuntimeConfig, error) {
	path, err := runtimeConfigPath()
	if err != nil {
		return RuntimeConfig{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultRuntimeConfig(), nil
		}
		return RuntimeConfig{}, err
	}

	var cfg RuntimeConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return RuntimeConfig{}, err
	}
	return cfg, nil
}

// Save writes config to ~/.nt-cli/config.json
func (r RuntimeConfig) Save() error {
	path, err := runtimeConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}
