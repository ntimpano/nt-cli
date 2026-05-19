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
	if err := SaveRuntimeConfig(custom); err != nil {
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
	if err := SaveRuntimeConfig(initial); err != nil {
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

func TestRunInitForceUsesExistingValuesAsPromptDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	seedProfile := Defaults()
	seedProfile.Language = "en"
	seedProfile.Tone = "english"
	seedProfile.Verbosity = "verbose"
	seedProfile.AskBeforeMutation = false
	seedProfile.ContextAutoswitch = false
	seedProfile.PrimaryDomain = "strategy"
	if err := SaveProfile(seedProfile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	seedRuntime := DefaultRuntimeConfig()
	seedRuntime.PrimaryDomain = "strategy"
	seedRuntime.Models = ModelTiers{
		Thinking:  "seed-thinking",
		Execution: "seed-execution",
		Fast:      "seed-fast",
		Planning:  "seed-planning",
	}
	if err := SaveRuntimeConfig(seedRuntime); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}

	// Empty answers should keep existing values when --force is used.
	input := strings.Repeat("\n", 8)
	var stdout, stderr bytes.Buffer
	err := runInit([]string{"--force"}, strings.NewReader(input), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	gotProfile := LoadProfile()
	if gotProfile != seedProfile {
		t.Fatalf("expected profile defaults from existing config, got %+v", gotProfile)
	}
	gotRuntime, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig error: %v", err)
	}
	if gotRuntime.Models != seedRuntime.Models {
		t.Fatalf("expected model defaults from existing config, got %+v", gotRuntime.Models)
	}
	if gotRuntime.PrimaryDomain != "strategy" {
		t.Fatalf("expected existing primary domain preserved, got %q", gotRuntime.PrimaryDomain)
	}
}

func TestRunInitWritesWorkflowAndMergesOpencodeAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "opencode")
	mustMkdirAll(t, configDir)

	existing := []byte(`{"theme":"dark","agent":{"other-agent":{"model":"other"}}}`)
	opencodePath := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(opencodePath, existing, 0o644); err != nil {
		t.Fatalf("seed opencode.json: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runInit([]string{"--non-interactive", "--force"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	workflowDst := filepath.Join(home, ".nt-cli", "workflows.json")
	wf, err := os.ReadFile(workflowDst)
	if err != nil {
		t.Fatalf("expected workflows.json copy, read failed: %v", err)
	}
	if !strings.Contains(string(wf), `"workflows"`) {
		t.Fatalf("copied workflows.json looks invalid: %s", string(wf))
	}

	body, err := os.ReadFile(opencodePath)
	if err != nil {
		t.Fatalf("read merged opencode.json: %v", err)
	}
	var merged map[string]interface{}
	if err := json.Unmarshal(body, &merged); err != nil {
		t.Fatalf("merged opencode.json invalid json: %v", err)
	}
	if merged["theme"] != "dark" {
		t.Fatalf("expected existing top-level keys preserved, got theme=%v", merged["theme"])
	}
	agentMap, ok := merged["agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected merged agent object")
	}
	if _, ok := agentMap["other-agent"]; !ok {
		t.Fatalf("expected pre-existing agent preserved")
	}
	if _, ok := agentMap["nt-leader"]; !ok {
		t.Fatalf("expected nt-leader merged from bundle")
	}
}

func TestRunInitMCPFailureReturnsErrorWithScriptStderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	scriptPath := writeFailingMCPScript(t, home, "mcp smoke failed")
	cfg := DefaultRuntimeConfig()
	cfg.RuntimeDef.MCPScript = scriptPath
	if err := SaveRuntimeConfig(cfg); err != nil {
		t.Fatalf("seed runtime config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runInit([]string{"--non-interactive", "--force"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected mcp verification failure")
	}
	if !strings.Contains(err.Error(), "mcp smoke failed") {
		t.Fatalf("expected script stderr in error, got %q", err.Error())
	}

	runtimePath, err := runtimeConfigPath()
	if err != nil {
		t.Fatalf("runtimeConfigPath error: %v", err)
	}
	if _, err := os.Stat(runtimePath); err != nil {
		t.Fatalf("expected runtime config to remain on disk after MCP failure: %v", err)
	}

	profilePath, err := ProfilePath()
	if err != nil {
		t.Fatalf("ProfilePath error: %v", err)
	}
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("expected profile config to remain on disk after MCP failure: %v", err)
	}
}

func TestRunInitMCPFailurePropagatesNonZeroExit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	scriptPath := writeFailingMCPScript(t, home, "forced mcp error")
	cfg := DefaultRuntimeConfig()
	cfg.RuntimeDef.MCPScript = scriptPath
	if err := SaveRuntimeConfig(cfg); err != nil {
		t.Fatalf("seed runtime config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := RunInit([]string{"--non-interactive", "--force"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit when mcp script fails; stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "forced mcp error") {
		t.Fatalf("expected propagated stderr from failing script, got %q", stderr.String())
	}
}

func TestRunInitNonInteractiveForceRepairsPartialConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	profilePath, err := ProfilePath()
	if err != nil {
		t.Fatalf("ProfilePath error: %v", err)
	}
	mustMkdirAll(t, filepath.Dir(profilePath))
	if err := os.WriteFile(profilePath, []byte(`{"language":"es","tone":"argentino","verbosity":"short","ask_before_mutation":true,"context_autoswitch":true,"primary_domain":"research"}`), 0o644); err != nil {
		t.Fatalf("write seeded profile: %v", err)
	}

	runtimePath, err := runtimeConfigPath()
	if err != nil {
		t.Fatalf("runtimeConfigPath error: %v", err)
	}
	mustMkdirAll(t, filepath.Dir(runtimePath))
	partial := `{
		"version":"v1",
		"runtime":{"type":"opencode"},
		"models":{},
		"primary_domain":"research"
	}`
	if err := os.WriteFile(runtimePath, []byte(partial), 0o644); err != nil {
		t.Fatalf("write partial runtime config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = runInit([]string{"--non-interactive", "--force"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runInit error: %v stderr=%q", err, stderr.String())
	}

	rc, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig error: %v", err)
	}
	if rc != DefaultRuntimeConfig() {
		t.Fatalf("expected force non-interactive to normalize partial runtime config to defaults, got %+v", rc)
	}

	p := LoadProfile()
	if p != Defaults() {
		t.Fatalf("expected force non-interactive profile defaults, got %+v", p)
	}
}

func TestInitRunnerWriteConfigsRejectsInvalidDomain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))

	r := &initRunner{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		state: initState{
			profile: Defaults(),
			runtime: DefaultRuntimeConfig(),
		},
	}
	err := r.writeConfigs(OpenCodeRuntime{}, DefaultRuntimeConfig().Models, "sales", Defaults())
	if err == nil {
		t.Fatalf("expected invalid domain validation error")
	}
	if !strings.Contains(err.Error(), "primary domain") {
		t.Fatalf("expected primary-domain validation message, got %v", err)
	}
}

func writeFailingMCPScript(t *testing.T, dir, message string) string {
	t.Helper()
	path := filepath.Join(dir, "fail-mcp.sh")
	body := "#!/bin/sh\necho '" + message + "' 1>&2\nexit 23\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write failing script: %v", err)
	}
	return path
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
