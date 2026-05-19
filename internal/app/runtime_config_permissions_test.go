package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeConfig_DefaultsV1(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	if cfg.Version != "v1" {
		t.Fatalf("expected version v1, got %q", cfg.Version)
	}
	if cfg.Runtime != RuntimeOpenCode {
		t.Fatalf("expected runtime opencode, got %q", cfg.Runtime)
	}
	if cfg.RuntimeDef.Type != string(RuntimeOpenCode) {
		t.Fatalf("expected runtime.type opencode, got %q", cfg.RuntimeDef.Type)
	}
	if cfg.PrimaryDomain != "dev" {
		t.Fatalf("expected primary_domain dev, got %q", cfg.PrimaryDomain)
	}
	if strings.HasPrefix(cfg.RuntimeDef.AgentConfigPath, "~") || strings.HasPrefix(cfg.RuntimeDef.MCPScript, "~") {
		t.Fatalf("expected defaults with expanded runtime paths, got %+v", cfg.RuntimeDef)
	}
	if cfg.Models.Thinking == "" || cfg.Models.Execution == "" || cfg.Models.Fast == "" || cfg.Models.Planning == "" {
		t.Fatalf("expected all model tiers non-empty: %+v", cfg.Models)
	}
}

func TestRuntimeConfigValidate(t *testing.T) {
	base := DefaultRuntimeConfig()
	if err := base.Validate(); err != nil {
		t.Fatalf("expected default config to validate, got %v", err)
	}

	invalidRuntime := base
	invalidRuntime.RuntimeDef.Type = "foo"
	invalidRuntime.Runtime = RuntimeType("foo")
	if err := invalidRuntime.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported runtime type: foo") {
		t.Fatalf("expected unsupported runtime type error, got %v", err)
	}

	emptyModel := base
	emptyModel.Models.Fast = ""
	if err := emptyModel.Validate(); err == nil || !strings.Contains(err.Error(), "models.fast cannot be empty") {
		t.Fatalf("expected empty model validation error, got %v", err)
	}

	invalidDomain := base
	invalidDomain.PrimaryDomain = "sales"
	if err := invalidDomain.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported primary domain: sales") {
		t.Fatalf("expected invalid domain validation error, got %v", err)
	}
}

func TestRuntimeConfigSave_Writes0600AndAtomicOnFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultRuntimeConfig()
	if err := SaveRuntimeConfig(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path := filepath.Join(home, ".nt-cli", "config.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected config file: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("unexpected permissions: got %o want %o", got, want)
	}

	original := []byte("{\n  \"version\": \"v1\"\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed original config: %v", err)
	}
	configDir := filepath.Dir(path)
	if err := os.Chmod(configDir, 0o555); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o755) })

	err = cfg.Save()
	if err == nil {
		t.Fatal("expected save failure when directory is read-only")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read config after failed save: %v", readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("atomic write violated: expected original content unchanged\nwant:\n%s\ngot:\n%s", string(original), string(after))
	}
}

func TestLoadRuntimeConfig_FallbackAndCompatibility(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".nt-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	missingPath := filepath.Join(configDir, "config.json")
	if err := os.Remove(missingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ensure missing config: %v", err)
	}
	defaults, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("missing config should fallback to defaults: %v", err)
	}
	if defaults != DefaultRuntimeConfig() {
		t.Fatalf("expected defaults on missing file, got %+v", defaults)
	}

	if err := os.WriteFile(missingPath, []byte("{ this is malformed json"), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}
	malformedCfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("malformed config should fallback to defaults: %v", err)
	}
	if malformedCfg != DefaultRuntimeConfig() {
		t.Fatalf("expected defaults on malformed file, got %+v", malformedCfg)
	}

	body := []byte("{\n  \"runtime\": \"opencode\",\n  \"models\": {\n    \"thinking\": \"claude-opus-api:secret\",\n    \"execution\": \"claude-sonnet-api:secret\",\n    \"fast\": \"claude-haiku-api:secret\",\n    \"planning\": \"claude-sonnet-api:secret\"\n  },\n  \"primary_domain\": \"strategy\"\n}\n")
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("load must keep legacy compatibility: %v", err)
	}
	if cfg.Models.Thinking != "claude-opus-api:secret" {
		t.Fatalf("legacy model string changed unexpectedly: %q", cfg.Models.Thinking)
	}
	if cfg.PrimaryDomain != "strategy" {
		t.Fatalf("expected primary domain strategy, got %q", cfg.PrimaryDomain)
	}
}

func TestRuntimeConfigPath_DetectAvailableRuntimes_AndHomeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := RuntimeConfigPath()
	if err != nil {
		t.Fatalf("RuntimeConfigPath error: %v", err)
	}
	want := filepath.Join(home, ".nt-cli", "config.json")
	if path != want {
		t.Fatalf("unexpected runtime config path: got %q want %q", path, want)
	}

	runtimes := detectAvailableRuntimes()
	if len(runtimes) == 0 {
		t.Fatal("expected at least one runtime")
	}
	foundOpenCode := false
	for _, r := range runtimes {
		if r.Name() == string(RuntimeOpenCode) {
			foundOpenCode = true
			if !r.SupportsAgents() {
				t.Fatal("opencode runtime must support agents")
			}
			if strings.TrimSpace(r.AgentConfigPath()) == "" {
				t.Fatal("opencode runtime must expose agent config path")
			}
		}
	}
	if !foundOpenCode {
		t.Fatal("expected detectAvailableRuntimes to include opencode")
	}

	configDir := filepath.Join(home, ".nt-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	v1 := []byte("{\n  \"version\": \"v1\",\n  \"runtime\": {\n    \"type\": \"opencode\",\n    \"agent_config_path\": \"~/.config/opencode/opencode.json\",\n    \"mcp_script\": \"~/bin/nt-cli mcp serve\"\n  },\n  \"models\": {\n    \"thinking\": \"claude-opus-4-5\",\n    \"execution\": \"claude-sonnet-4-5\",\n    \"fast\": \"claude-haiku-4-5\",\n    \"planning\": \"claude-sonnet-4-5\"\n  },\n  \"primary_domain\": \"dev\"\n}\n")
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), v1, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig error: %v", err)
	}
	if strings.HasPrefix(loaded.RuntimeDef.AgentConfigPath, "~") || strings.HasPrefix(loaded.RuntimeDef.MCPScript, "~") {
		t.Fatalf("expected home expansion, got runtime=%+v", loaded.RuntimeDef)
	}
	if !strings.HasPrefix(loaded.RuntimeDef.AgentConfigPath, home) {
		t.Fatalf("expected expanded path to start with home, got %q", loaded.RuntimeDef.AgentConfigPath)
	}
	if !strings.HasPrefix(loaded.RuntimeDef.MCPScript, home) {
		t.Fatalf("expected expanded mcp_script to start with home, got %q", loaded.RuntimeDef.MCPScript)
	}
}
