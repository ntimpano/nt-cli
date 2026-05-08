package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeConfigSave_Writes0600(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := RuntimeConfig{Runtime: RuntimeOpenCode, Models: ModelTiers{Thinking: "a", Execution: "b", Fast: "c"}}
	if err := cfg.Save(); err != nil {
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
}

func TestLoadRuntimeConfig_LegacyInlineKeysRemainCompatible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".nt-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	body := []byte("{\n  \"runtime\": \"opencode\",\n  \"models\": {\n    \"thinking\": \"claude-opus-api:secret\",\n    \"execution\": \"claude-sonnet-api:secret\",\n    \"fast\": \"claude-haiku-api:secret\"\n  }\n}\n")
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
}
