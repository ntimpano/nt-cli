package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RunInit executes the redesigned 7-step init flow using process stdio.
func RunInit(args []string) error {
	return runInit(args, os.Stdin, os.Stdout, os.Stderr)
}

func runInit(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	force, nonInteractive, err := parseInitFlags(args)
	if err != nil {
		return err
	}

	var scanner *bufio.Scanner
	if !nonInteractive {
		scanner = bufio.NewScanner(stdin)
	}

	fmt.Fprintln(stdout, "Step 1/7 — Prerequisites check")
	if err := checkPrerequisites(stderr); err != nil {
		return err
	}

	profilePath, err := ProfilePath()
	if err != nil {
		return err
	}
	runtimePath, err := runtimeConfigPath()
	if err != nil {
		return err
	}
	if !force && (pathExists(profilePath) || pathExists(runtimePath)) {
		fmt.Fprintln(stdout, "Init already configured (use --force or -f to re-run all steps)")
		return nil
	}

	fmt.Fprintln(stdout, "Step 2/7 — Runtime selection")
	runtimeType := RuntimeOpenCode
	if nonInteractive {
		printRuntimeMenu(stdout)
		fmt.Fprintln(stdout, "Auto-selected runtime: OpenCode")
	} else {
		runtimeType = selectRuntime(scanner, stdout)
	}

	fmt.Fprintln(stdout, "Step 3/7 — AI model selection")
	modelCfg := DefaultRuntimeConfig().Models
	if nonInteractive {
		fmt.Fprintln(stdout, "non-interactive mode: using free tier defaults")
	} else {
		modelCfg = configureModels(scanner, stdout)
	}

	fmt.Fprintln(stdout, "Step 4/7 — Primary domain")
	domain := "dev"
	if nonInteractive {
		fmt.Fprintln(stdout, "non-interactive mode: primary domain = software development")
	} else {
		domain = selectDomain(scanner, stdout)
	}

	fmt.Fprintln(stdout, "Step 5/7 — Tone & persona")
	profile := Defaults()
	profile.PrimaryDomain = domain
	if nonInteractive {
		fmt.Fprintln(stdout, "non-interactive mode: using profile defaults")
	} else {
		profile = configurePersona(scanner, stdout, profile)
		profile.PrimaryDomain = domain
	}

	fmt.Fprintln(stdout, "Step 6/7 — Write configs")
	rc := RuntimeConfig{Runtime: runtimeType, Models: modelCfg}
	if err := profile.Save(); err != nil {
		return fmt.Errorf("profile save failed: %w", err)
	}
	fmt.Fprintln(stdout, "✓ profile.json saved")
	if err := rc.Save(); err != nil {
		return fmt.Errorf("runtime config save failed: %w", err)
	}
	fmt.Fprintln(stdout, "✓ config.json saved")

	fmt.Fprintln(stdout, "Step 7/7 — Verify MCP connection")
	verifyMCP(stdout)

	_ = runtimeType // reserved for future runtimes
	return nil
}

func parseInitFlags(args []string) (force bool, nonInteractive bool, err error) {
	for _, a := range args {
		switch strings.TrimSpace(a) {
		case "--force", "-f":
			force = true
		case "--non-interactive":
			nonInteractive = true
		case "", "--legacy":
			// handled by runner dispatch; ignored here
		default:
			if strings.HasPrefix(a, "-") {
				return false, false, fmt.Errorf("unknown init flag: %s", a)
			}
		}
	}
	return force, nonInteractive, nil
}

func checkPrerequisites(stderr io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	opencodeDir := filepath.Join(home, ".config", "opencode")
	st, err := os.Stat(opencodeDir)
	if err != nil || !st.IsDir() {
		fmt.Fprintln(stderr, "OpenCode runtime not detected.")
		fmt.Fprintln(stderr, "Install/setup OpenCode and then re-run init.")
		fmt.Fprintln(stderr, "Expected directory: ~/.config/opencode/")
		fmt.Fprintln(stderr, "Docs: https://opencode.ai")
		return fmt.Errorf("missing OpenCode runtime")
	}
	return nil
}

func printRuntimeMenu(stdout io.Writer) {
	fmt.Fprintln(stdout, "Select runtime:")
	fmt.Fprintln(stdout, "  [1] OpenCode (installed ✓)")
	fmt.Fprintln(stdout, "  [2] Claude Code (coming soon)")
	fmt.Fprintln(stdout, "  [3] Cursor (coming soon)")
}

func selectRuntime(scanner *bufio.Scanner, stdout io.Writer) RuntimeType {
	printRuntimeMenu(stdout)
	fmt.Fprint(stdout, "Runtime [1]: ")
	if !scanner.Scan() {
		return RuntimeOpenCode
	}
	choice := strings.TrimSpace(scanner.Text())
	switch choice {
	case "", "1":
		return RuntimeOpenCode
	case "2", "3":
		fmt.Fprintln(stdout, "That runtime is coming soon; using OpenCode for now.")
		return RuntimeOpenCode
	default:
		fmt.Fprintln(stdout, "Invalid runtime choice; using OpenCode.")
		return RuntimeOpenCode
	}
}

func configureModels(scanner *bufio.Scanner, stdout io.Writer) ModelTiers {
	current, err := LoadRuntimeConfig()
	if err != nil {
		current = DefaultRuntimeConfig()
	}
	defaults := DefaultRuntimeConfig()

	fmt.Fprintln(stdout, "Select model profile:")
	fmt.Fprintln(stdout, "  [1] Free tier (OpenCode built-in) — no API key needed")
	fmt.Fprintln(stdout, "  [2] Claude API (your own key) — best reasoning, recommended for complex tasks")
	fmt.Fprintln(stdout, "  [3] OpenAI API (your own key) — strong alternative")
	fmt.Fprintln(stdout, "  [4] Keep current / skip")
	fmt.Fprintln(stdout, "For complex reasoning tasks (architecture, design decisions), Claude Opus is best. For execution, Claude Sonnet. For quick lookups, Haiku or GPT-4o-mini.")
	fmt.Fprint(stdout, "Model option [1]: ")
	if !scanner.Scan() {
		return defaults.Models
	}
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "", "1":
		return defaults.Models
	case "2":
		fmt.Fprint(stdout, "Claude API key: ")
		if !scanner.Scan() {
			return defaults.Models
		}
		key := strings.TrimSpace(scanner.Text())
		if key == "" {
			return defaults.Models
		}
		return ModelTiers{
			Thinking:  "claude-opus-api:" + key,
			Execution: "claude-sonnet-api:" + key,
			Fast:      "claude-haiku-api:" + key,
		}
	case "3":
		fmt.Fprint(stdout, "OpenAI API key: ")
		if !scanner.Scan() {
			return defaults.Models
		}
		key := strings.TrimSpace(scanner.Text())
		if key == "" {
			return defaults.Models
		}
		return ModelTiers{
			Thinking:  "openai-gpt-4o-api:" + key,
			Execution: "openai-gpt-4.1-api:" + key,
			Fast:      "openai-gpt-4o-mini-api:" + key,
		}
	case "4":
		return current.Models
	default:
		fmt.Fprintln(stdout, "Invalid model option; using free tier defaults.")
		return defaults.Models
	}
}

func selectDomain(scanner *bufio.Scanner, stdout io.Writer) string {
	fmt.Fprintln(stdout, "Select primary domain:")
	fmt.Fprintln(stdout, "  [1] Software development")
	fmt.Fprintln(stdout, "  [2] Creative work")
	fmt.Fprintln(stdout, "  [3] Strategy & planning")
	fmt.Fprintln(stdout, "  [4] Research & analysis")
	fmt.Fprintln(stdout, "This sets defaults for tone and tools — you're not locked to one domain.")
	fmt.Fprint(stdout, "Domain [1]: ")
	if !scanner.Scan() {
		return "dev"
	}
	switch strings.TrimSpace(scanner.Text()) {
	case "", "1":
		return "dev"
	case "2":
		return "creative"
	case "3":
		return "strategy"
	case "4":
		return "research"
	default:
		fmt.Fprintln(stdout, "Invalid domain; using software development.")
		return "dev"
	}
}

func configurePersona(scanner *bufio.Scanner, stdout io.Writer, cfg ProfileConfig) ProfileConfig {
	fmt.Fprintln(stdout, "Language:")
	fmt.Fprintln(stdout, "  [1] Spanish (Argentina)")
	fmt.Fprintln(stdout, "  [2] Spanish (neutral)")
	fmt.Fprintln(stdout, "  [3] English")
	fmt.Fprint(stdout, "Language [1]: ")
	if scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "", "1":
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

	fmt.Fprintln(stdout, "Tone:")
	fmt.Fprintln(stdout, "  [1] Warm and direct")
	fmt.Fprintln(stdout, "  [2] Formal")
	fmt.Fprintln(stdout, "  [3] Concise")
	fmt.Fprint(stdout, "Tone [1]: ")
	if scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "", "1":
			if cfg.Language == "en" {
				cfg.Tone = "english"
			} else {
				cfg.Tone = "argentino"
			}
		case "2", "3":
			cfg.Tone = "neutral"
		}
	}

	fmt.Fprintln(stdout, "Verbosity:")
	fmt.Fprintln(stdout, "  [1] Short (default)")
	fmt.Fprintln(stdout, "  [2] Detailed")
	fmt.Fprint(stdout, "Verbosity [1]: ")
	if scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "", "1":
			cfg.Verbosity = "short"
		case "2":
			cfg.Verbosity = "verbose"
		}
	}

	if v, err := promptBool(scanner, stdout, "Ask before mutations", cfg.AskBeforeMutation); err == nil {
		cfg.AskBeforeMutation = v
	}
	if v, err := promptBool(scanner, stdout, "Context autoswitch", cfg.ContextAutoswitch); err == nil {
		cfg.ContextAutoswitch = v
	}

	return cfg
}

func verifyMCP(stdout io.Writer) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stdout, "⚠ unable to resolve home directory — run: nt-cli setup-opencode")
		return
	}
	opencodeConfig := filepath.Join(home, ".config", "opencode", "opencode.json")
	body, err := os.ReadFile(opencodeConfig)
	if err != nil {
		fmt.Fprintln(stdout, "⚠ opencode.json not found — run: nt-cli setup-opencode")
		return
	}
	if hasNTCLIMCPEntry(body) {
		fmt.Fprintln(stdout, "✓ MCP connection verified")
		return
	}
	fmt.Fprintln(stdout, "⚠ nt-cli MCP server not configured — run: nt-cli setup-opencode")
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
