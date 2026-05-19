package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flint/internal/model"
)

const (
	initWorkflowSourcePath = "/opt/nt-cli/workflows.json"
)

type initState struct {
	profile model.ProfileConfig
	runtime model.RuntimeConfig
}

type initRunner struct {
	stdin          io.Reader
	stdout         io.Writer
	stderr         io.Writer
	scanner        *bufio.Scanner
	nonInteractive bool
	force          bool

	state initState
}

// RunInit executes the redesigned 7-step init flow.
func RunInit(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if err := runInit(args, stdin, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "init failed: %v\n", err)
		return 1
	}
	return 0
}

func runInit(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	r, err := newInitRunner(args, stdin, stdout, stderr)
	if err != nil {
		return err
	}
	return r.run()
}

func newInitRunner(args []string, stdin io.Reader, stdout, stderr io.Writer) (*initRunner, error) {
	force, nonInteractive, err := parseInitFlags(args)
	if err != nil {
		return nil, err
	}
	r := &initRunner{
		stdin:          stdin,
		stdout:         stdout,
		stderr:         stderr,
		nonInteractive: nonInteractive,
		force:          force,
		state: initState{
			profile: Defaults(),
			runtime: DefaultRuntimeConfig(),
		},
	}
	if !nonInteractive {
		r.scanner = bufio.NewScanner(stdin)
	}
	r.loadExistingDefaults()
	return r, nil
}

func (r *initRunner) run() error {
	fmt.Fprintln(r.stdout, "Step 1/7 — Prerequisites check")
	if err := r.checkPrerequisites(); err != nil {
		return err
	}
	if !r.force && r.alreadyConfigured() {
		fmt.Fprintln(r.stdout, "Init already configured (use --force or -f to re-run all steps)")
		return nil
	}

	fmt.Fprintln(r.stdout, "Step 2/7 — Runtime selection")
	detected := detectAvailableRuntimes()
	runtime := r.selectRuntime(detected)

	fmt.Fprintln(r.stdout, "Step 3/7 — AI model selection")
	models := r.configureModels(runtime)

	fmt.Fprintln(r.stdout, "Step 4/7 — Primary domain")
	domain := r.selectDomain()

	fmt.Fprintln(r.stdout, "Step 5/7 — Tone & persona")
	profile := r.configureProfile()
	profile.PrimaryDomain = domain

	fmt.Fprintln(r.stdout, "Step 6/7 — Write configs")
	if err := r.writeConfigs(runtime, models, domain, profile); err != nil {
		return err
	}

	fmt.Fprintln(r.stdout, "Step 7/7 — Verify MCP connection")
	if err := r.verifyMCP(runtime); err != nil {
		return err
	}
	return nil
}

func (r *initRunner) loadExistingDefaults() {
	if existing := LoadProfile(); ValidateProfile(existing) == nil {
		r.state.profile = existing
	}
	if existing, err := LoadRuntimeConfig(); err == nil {
		r.state.runtime = existing
	}
	if strings.TrimSpace(r.state.profile.PrimaryDomain) == "" {
		r.state.profile.PrimaryDomain = r.state.runtime.PrimaryDomain
	}
	if strings.TrimSpace(r.state.profile.PrimaryDomain) == "" {
		r.state.profile.PrimaryDomain = "dev"
	}
}

func (r *initRunner) alreadyConfigured() bool {
	profilePath, err := ProfilePath()
	if err != nil {
		return false
	}
	runtimePath, err := RuntimeConfigPath()
	if err != nil {
		return false
	}
	return pathExists(profilePath) || pathExists(runtimePath)
}

func (r *initRunner) checkPrerequisites() error {
	if _, err := os.Executable(); err != nil {
		return fmt.Errorf("unable to resolve nt-cli binary: %w", err)
	}
	if len(detectAvailableRuntimes()) == 0 {
		return fmt.Errorf("no supported runtime detected")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	opencodeDir := filepath.Join(home, ".config", "opencode")
	st, err := os.Stat(opencodeDir)
	if err != nil || !st.IsDir() {
		fmt.Fprintln(r.stderr, "OpenCode runtime not detected.")
		fmt.Fprintln(r.stderr, "Install/setup OpenCode and then re-run init.")
		fmt.Fprintln(r.stderr, "Expected directory: ~/.config/opencode/")
		fmt.Fprintln(r.stderr, "Docs: https://opencode.ai")
		return fmt.Errorf("missing OpenCode runtime")
	}
	return nil
}

func (r *initRunner) selectRuntime(detected []Runtime) Runtime {
	if len(detected) == 0 {
		return OpenCodeRuntime{}
	}
	if r.nonInteractive {
		fmt.Fprintln(r.stdout, "Auto-selected runtime: OpenCode")
		return detected[0]
	}
	printRuntimeMenu(r.stdout, detected)
	defaultChoice := runtimeIndexByName(detected, r.state.runtime.RuntimeDef.Type) + 1
	if defaultChoice <= 0 {
		defaultChoice = 1
	}
	fmt.Fprintf(r.stdout, "Runtime [%d]: ", defaultChoice)
	if !r.scanner.Scan() {
		return detected[defaultChoice-1]
	}
	choice := strings.TrimSpace(r.scanner.Text())
	if choice == "" {
		return detected[defaultChoice-1]
	}
	if choice == "1" {
		return detected[0]
	}
	fmt.Fprintln(r.stdout, "That runtime is coming soon; using OpenCode for now.")
	return detected[0]
}

func (r *initRunner) configureModels(runtime Runtime) model.ModelTiers {
	_ = runtime
	current := r.state.runtime.Models
	if r.nonInteractive {
		fmt.Fprintln(r.stdout, "non-interactive mode: using free tier defaults")
		return DefaultRuntimeConfig().Models
	}
	fmt.Fprintln(r.stdout, "Select model profile:")
	fmt.Fprintln(r.stdout, "  [1] Keep current")
	fmt.Fprintln(r.stdout, "  [2] Free tier defaults")
	fmt.Fprintf(r.stdout, "Model option [1]: ")
	if !r.scanner.Scan() {
		return current
	}
	switch strings.TrimSpace(r.scanner.Text()) {
	case "", "1":
		return current
	case "2":
		return DefaultRuntimeConfig().Models
	default:
		fmt.Fprintln(r.stdout, "Invalid model option; keeping current.")
		return current
	}
}

func (r *initRunner) selectDomain() string {
	current := r.state.profile.PrimaryDomain
	if strings.TrimSpace(current) == "" {
		current = r.state.runtime.PrimaryDomain
	}
	if strings.TrimSpace(current) == "" {
		current = "dev"
	}
	if r.nonInteractive {
		fmt.Fprintln(r.stdout, "non-interactive mode: primary domain = software development")
		return "dev"
	}
	fmt.Fprintln(r.stdout, "Select primary domain:")
	fmt.Fprintln(r.stdout, "  [1] Software development")
	fmt.Fprintln(r.stdout, "  [2] Creative work")
	fmt.Fprintln(r.stdout, "  [3] Strategy & planning")
	fmt.Fprintln(r.stdout, "  [4] Research & analysis")
	defaultIndex := domainIndex(current)
	fmt.Fprintf(r.stdout, "Domain [%d]: ", defaultIndex)
	if !r.scanner.Scan() {
		return current
	}
	switch strings.TrimSpace(r.scanner.Text()) {
	case "":
		return current
	case "1":
		return "dev"
	case "2":
		return "creative"
	case "3":
		return "strategy"
	case "4":
		return "research"
	default:
		fmt.Fprintln(r.stdout, "Invalid domain; keeping current.")
		return current
	}
}

func (r *initRunner) configureProfile() model.ProfileConfig {
	if r.nonInteractive {
		fmt.Fprintln(r.stdout, "non-interactive mode: using profile defaults")
		return Defaults()
	}
	cfg := r.state.profile
	defaultLang := "1"
	if cfg.Language == "en" {
		defaultLang = "3"
	} else if cfg.Tone == "neutral" {
		defaultLang = "2"
	}
	fmt.Fprintln(r.stdout, "Language:")
	fmt.Fprintln(r.stdout, "  [1] Spanish (Argentina)")
	fmt.Fprintln(r.stdout, "  [2] Spanish (neutral)")
	fmt.Fprintln(r.stdout, "  [3] English")
	fmt.Fprintf(r.stdout, "Language [%s]: ", defaultLang)
	if r.scanner.Scan() {
		switch strings.TrimSpace(r.scanner.Text()) {
		case "", defaultLang:
			// keep current
		case "1":
			cfg.Language = "es"
			cfg.Tone = "argentino"
		case "2":
			cfg.Language = "es"
			cfg.Tone = "neutral"
		case "3":
			cfg.Language = "en"
			cfg.Tone = "english"
		}
	}

	fmt.Fprintln(r.stdout, "Tone:")
	fmt.Fprintln(r.stdout, "  [1] Warm and direct")
	fmt.Fprintln(r.stdout, "  [2] Formal")
	fmt.Fprintln(r.stdout, "  [3] Concise")
	fmt.Fprint(r.stdout, "Tone [1]: ")
	if r.scanner.Scan() {
		switch strings.TrimSpace(r.scanner.Text()) {
		case "", "1":
			if cfg.Language == "en" {
				cfg.Tone = "english"
			} else if cfg.Tone != "neutral" {
				cfg.Tone = "argentino"
			}
		case "2", "3":
			cfg.Tone = "neutral"
		}
	}

	defaultVerbosity := "1"
	if cfg.Verbosity == "verbose" {
		defaultVerbosity = "2"
	}
	fmt.Fprintln(r.stdout, "Verbosity:")
	fmt.Fprintln(r.stdout, "  [1] Short")
	fmt.Fprintln(r.stdout, "  [2] Detailed")
	fmt.Fprintf(r.stdout, "Verbosity [%s]: ", defaultVerbosity)
	if r.scanner.Scan() {
		switch strings.TrimSpace(r.scanner.Text()) {
		case "", defaultVerbosity:
			// keep current
		case "1":
			cfg.Verbosity = "short"
		case "2":
			cfg.Verbosity = "verbose"
		}
	}
	if v, err := promptBool(r.scanner, r.stdout, "ask_before_mutation", cfg.AskBeforeMutation); err == nil {
		cfg.AskBeforeMutation = v
	}
	if v, err := promptBool(r.scanner, r.stdout, "context_autoswitch", cfg.ContextAutoswitch); err == nil {
		cfg.ContextAutoswitch = v
	}
	return cfg
}

func (r *initRunner) writeConfigs(runtime Runtime, models model.ModelTiers, domain string, profile model.ProfileConfig) error {
	runtimeCfg := r.state.runtime
	runtimeCfg.Runtime = RuntimeType(runtime.Name())
	runtimeCfg.RuntimeDef.Type = runtime.Name()
	runtimeCfg.Models = models
	runtimeCfg.PrimaryDomain = domain
	if strings.TrimSpace(runtimeCfg.RuntimeDef.AgentConfigPath) == "" {
		runtimeCfg.RuntimeDef.AgentConfigPath = runtime.AgentConfigPath()
	}
	if err := SaveRuntimeConfig(runtimeCfg); err != nil {
		return fmt.Errorf("runtime config save failed: %w", err)
	}
	fmt.Fprintln(r.stdout, "✓ config.json saved")

	if err := SaveProfile(profile); err != nil {
		return fmt.Errorf("profile save failed: %w", err)
	}
	r.state.runtime = runtimeCfg
	r.state.profile = profile
	fmt.Fprintln(r.stdout, "✓ profile.json saved")

	if err := copyWorkflowCatalog(); err != nil {
		return err
	}
	fmt.Fprintln(r.stdout, "✓ workflows.json copied")

	if err := mergeOpencodeAgents(); err != nil {
		return err
	}
	fmt.Fprintln(r.stdout, "✓ opencode.json merged")
	return nil
}

func (r *initRunner) verifyMCP(runtime Runtime) error {
	if err := r.verifyMCPScript(); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(r.stdout, "⚠ unable to resolve home directory — run: nt-cli setup-opencode")
		return nil
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(r.stdout, "⚠ opencode.json not found — run: nt-cli setup-opencode")
		return nil
	}
	if hasNTCLIMCPEntry(body) {
		fmt.Fprintln(r.stdout, "✓ MCP connection verified")
		return nil
	}
	fmt.Fprintf(r.stdout, "⚠ nt-cli MCP server not configured for runtime %s — run: nt-cli setup-opencode\n", runtime.Name())
	return nil
}

func (r *initRunner) verifyMCPScript() error {
	script := strings.TrimSpace(r.state.runtime.RuntimeDef.MCPScript)
	if script == "" {
		return nil
	}

	parts := strings.Fields(script)
	if len(parts) == 0 {
		return nil
	}

	prog, err := expandHome(parts[0])
	if err != nil {
		return fmt.Errorf("mcp verification failed: %w", err)
	}
	if prog == "" {
		return nil
	}
	parts[0] = prog

	if _, err := exec.LookPath(parts[0]); err != nil {
		// Keep init backward-compatible when the configured script is not present yet.
		return nil
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		failure := strings.TrimSpace(stderr.String())
		if failure == "" {
			failure = err.Error()
		}
		return fmt.Errorf("mcp verification failed: %s", failure)
	}

	return nil
}

func parseInitFlags(args []string) (force bool, nonInteractive bool, err error) {
	for _, a := range args {
		switch strings.TrimSpace(a) {
		case "--force", "-f":
			force = true
		case "--non-interactive":
			nonInteractive = true
		case "", "--legacy", "--profile":
			// handled by runner dispatch; ignored here
		default:
			if strings.HasPrefix(a, "-") {
				return false, false, fmt.Errorf("unknown init flag: %s", a)
			}
		}
	}
	return force, nonInteractive, nil
}

func printRuntimeMenu(stdout io.Writer, detected []Runtime) {
	fmt.Fprintln(stdout, "Select runtime:")
	for i, rt := range detected {
		name := rt.Name()
		if name == runtimeTypeOpenCode {
			name = "OpenCode"
		}
		fmt.Fprintf(stdout, "  [%d] %s\n", i+1, name)
	}
	fmt.Fprintln(stdout, "  [2] Claude Code (coming soon)")
	fmt.Fprintln(stdout, "  [3] Cursor (coming soon)")
}

func runtimeIndexByName(detected []Runtime, name string) int {
	for i, rt := range detected {
		if rt.Name() == name {
			return i
		}
	}
	return 0
}

func domainIndex(domain string) int {
	switch strings.TrimSpace(domain) {
	case "creative":
		return 2
	case "strategy":
		return 3
	case "research":
		return 4
	default:
		return 1
	}
}

func copyWorkflowCatalog() error {
	body, err := os.ReadFile(initWorkflowSourcePath)
	if err != nil {
		return fmt.Errorf("workflow catalog read failed: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".flint", "workflows.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return fmt.Errorf("workflow catalog write failed: %w", err)
	}
	return nil
}

func mergeOpencodeAgents() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bundlePath := filepath.Join("/opt/nt-cli", ".nt-cli-agents.json")
	bundleBytes, err := os.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("agent bundle read failed: %w", err)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal(bundleBytes, &bundle); err != nil {
		return fmt.Errorf("agent bundle parse failed: %w", err)
	}

	opencodePath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(opencodePath), 0o755); err != nil {
		return err
	}

	existing := map[string]interface{}{}
	if data, err := os.ReadFile(opencodePath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("opencode.json parse failed: %w", err)
		}
	}

	agents := map[string]interface{}{}
	if raw, ok := existing["agent"].(map[string]interface{}); ok {
		for k, v := range raw {
			agents[k] = v
		}
	}
	for k, v := range bundle {
		if strings.HasPrefix(k, "nt-") {
			agents[k] = v
		}
	}
	existing["agent"] = agents

	body, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(opencodePath, body, 0o644)
}

func hasNTCLIMCPEntry(body []byte) bool {
	if strings.Contains(string(body), "nt-cli") {
		return true
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	for _, key := range []string{"mcpServers", "mcp_servers", "servers"} {
		v, ok := raw[key]
		if !ok {
			continue
		}
		if m, ok := v.(map[string]interface{}); ok {
			if _, found := m["nt-cli"]; found {
				return true
			}
		}
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
