package model_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flint/internal/app"
	"flint/internal/model"
)

func TestRuntimeConfigValidate(t *testing.T) {
	base := model.RuntimeConfig{
		Version: "v1",
		Runtime: model.RuntimeOpenCode,
		RuntimeDef: model.RuntimeDef{
			Type:            string(model.RuntimeOpenCode),
			AgentConfigPath: "~/.config/opencode/opencode.json",
			MCPScript:       "~/.local/bin/nt-cli mcp serve",
		},
		Models: model.ModelTiers{
			Thinking:  "think",
			Execution: "exec",
			Fast:      "fast",
			Planning:  "plan",
		},
		PrimaryDomain: "dev",
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	t.Run("invalid runtime", func(t *testing.T) {
		cfg := base
		cfg.RuntimeDef.Type = "foo"
		cfg.Runtime = model.RuntimeType("foo")
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "unsupported runtime type: foo") {
			t.Fatalf("expected unsupported runtime type error, got %v", err)
		}
	})

	t.Run("empty model tier", func(t *testing.T) {
		cfg := base
		cfg.Models.Planning = ""
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "models.planning cannot be empty") {
			t.Fatalf("expected empty planning error, got %v", err)
		}
	})

	t.Run("invalid primary domain", func(t *testing.T) {
		cfg := base
		cfg.PrimaryDomain = "sales"
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "unsupported primary domain: sales") {
			t.Fatalf("expected invalid domain error, got %v", err)
		}
	})
}

func TestRuntimeConfigSaveAtomicBehavior(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := app.DefaultRuntimeConfig()
	if err := cfg.Save(); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	path := filepath.Join(home, ".flint", "config.json")
	original := []byte("{\n  \"version\": \"v1\"\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed original config: %v", err)
	}

	configDir := filepath.Dir(path)
	if err := os.Chmod(configDir, 0o555); err != nil {
		t.Fatalf("chmod read-only config dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o755) })

	err := cfg.Save()
	if err == nil {
		t.Fatalf("expected save failure when parent path is not a directory")
	}

	current, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read original file after failed save: %v", readErr)
	}
	if string(current) != string(original) {
		t.Fatalf("expected original file unchanged after failed save")
	}
}

func TestLoadRuntimeConfigFallbackBehavior(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".flint")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	missing, err := app.LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("missing config should fallback to defaults, got %v", err)
	}
	if missing != app.DefaultRuntimeConfig() {
		t.Fatalf("expected defaults on missing config, got %+v", missing)
	}

	path := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(path, []byte("{ malformed"), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	malformed, err := app.LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("malformed config should fallback to defaults, got %v", err)
	}
	if malformed != app.DefaultRuntimeConfig() {
		t.Fatalf("expected defaults on malformed config, got %+v", malformed)
	}
}
