package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitHappyPathInteractive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	input := strings.Join([]string{
		"",  // runtime: default OpenCode
		"1", // model profile: free tier
		"2", // domain: creative
		"3", // language: english
		"1", // tone: warm/direct
		"2", // verbosity: detailed
		"n", // ask before mutation
		"y", // context autoswitch
	}, "\n") + "\n"

	var stdout, stderr bytes.Buffer
	err := runInit(nil, strings.NewReader(input), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "✓ profile.json saved") {
		t.Fatalf("expected profile saved message, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "✓ config.json saved") {
		t.Fatalf("expected config saved message, got %q", stdout.String())
	}

	p := LoadProfile()
	if p.PrimaryDomain != "creative" {
		t.Fatalf("expected primary domain creative, got %q", p.PrimaryDomain)
	}
	if p.Language != "en" || p.Tone != "english" || p.Verbosity != "verbose" {
		t.Fatalf("unexpected profile values: %+v", p)
	}
	if p.AskBeforeMutation || !p.ContextAutoswitch {
		t.Fatalf("unexpected profile bools: %+v", p)
	}

	rc, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig error: %v", err)
	}
	if rc.Runtime != RuntimeOpenCode {
		t.Fatalf("expected runtime opencode, got %q", rc.Runtime)
	}
}

func TestRunInitNonInteractiveWritesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	var stdout, stderr bytes.Buffer
	err := runInit([]string{"--non-interactive"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	p := LoadProfile()
	d := Defaults()
	if p != d {
		t.Fatalf("expected defaults profile, got %+v", p)
	}
	rc, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig error: %v", err)
	}
	if rc != DefaultRuntimeConfig() {
		t.Fatalf("expected default runtime config, got %+v", rc)
	}
}

func TestRunInitForceOverwritesExistingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	custom := RuntimeConfig{Runtime: RuntimeUnknown, Models: ModelTiers{Thinking: "x", Execution: "y", Fast: "z"}}
	if err := custom.Save(); err != nil {
		t.Fatalf("custom.Save error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runInit([]string{"--force", "--non-interactive"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	rc, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig error: %v", err)
	}
	if rc != DefaultRuntimeConfig() {
		t.Fatalf("expected overwritten defaults, got %+v", rc)
	}
}

func TestRunInitOpenCodeMissingReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var stdout, stderr bytes.Buffer
	err := runInit(nil, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error when OpenCode missing")
	}
	msg := stderr.String()
	if !strings.Contains(msg, "OpenCode runtime not detected") || !strings.Contains(msg, "https://opencode.ai") {
		t.Fatalf("expected actionable install message, got %q", msg)
	}
}

func TestRunInitRerunWithoutForceNoOverwrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	initial := RuntimeConfig{Runtime: RuntimeUnknown, Models: ModelTiers{Thinking: "keep", Execution: "keep", Fast: "keep"}}
	if err := initial.Save(); err != nil {
		t.Fatalf("initial.Save error: %v", err)
	}

	path, err := runtimeConfigPath()
	if err != nil {
		t.Fatalf("runtimeConfigPath error: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = runInit([]string{"--non-interactive"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after error: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("config should be unchanged without --force")
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestHasNTCLIMCPEntry(t *testing.T) {
	body := []byte(`{"mcpServers":{"nt-cli":{"command":"nt-cli"}}}`)
	if !hasNTCLIMCPEntry(body) {
		t.Fatal("expected nt-cli entry to be detected")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
}
