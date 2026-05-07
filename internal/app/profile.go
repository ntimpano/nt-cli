package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ProfileConfig struct {
	Language          string `json:"language"`
	Tone              string `json:"tone"`
	Verbosity         string `json:"verbosity"`
	AskBeforeMutation bool   `json:"ask_before_mutation"`
	ContextAutoswitch bool   `json:"context_autoswitch"`
	PrimaryDomain     string `json:"primary_domain,omitempty"`
}

func Defaults() ProfileConfig {
	return ProfileConfig{
		Language:          "es",
		Tone:              "argentino",
		Verbosity:         "short",
		AskBeforeMutation: true,
		ContextAutoswitch: true,
		PrimaryDomain:     "dev",
	}
}

var (
	validLanguages = []string{"es", "en"}
	validTones     = []string{"argentino", "neutral", "english"}
	validVerbosity = []string{"short", "balanced", "verbose"}
	validDomains   = []string{"dev", "creative", "strategy", "research"}
)

func ProfilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("home directory is empty")
	}
	return filepath.Join(home, ".nt-cli", "profile.json"), nil
}

func isOneOf(value string, valid []string) bool {
	for _, v := range valid {
		if value == v {
			return true
		}
	}
	return false
}

func (c ProfileConfig) Validate() error {
	if !isOneOf(c.Language, validLanguages) {
		return fmt.Errorf("invalid language %q (valid: %s)", c.Language, strings.Join(validLanguages, ", "))
	}
	if !isOneOf(c.Tone, validTones) {
		return fmt.Errorf("invalid tone %q (valid: %s)", c.Tone, strings.Join(validTones, ", "))
	}
	if !isOneOf(c.Verbosity, validVerbosity) {
		return fmt.Errorf("invalid verbosity %q (valid: %s)", c.Verbosity, strings.Join(validVerbosity, ", "))
	}
	if !isOneOf(c.PrimaryDomain, validDomains) {
		return fmt.Errorf("invalid primary domain %q (valid: %s)", c.PrimaryDomain, strings.Join(validDomains, ", "))
	}
	return nil
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func LoadProfile() ProfileConfig {
	path, err := ProfilePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: profile path: %v\n", err)
		return Defaults()
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Defaults()
		}
		fmt.Fprintf(os.Stderr, "warning: profile read failed: %v\n", err)
		return Defaults()
	}
	var cfg ProfileConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: profile parse failed: %v\n", err)
		return Defaults()
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: profile validate failed: %v\n", err)
		return Defaults()
	}
	return cfg
}

func (c ProfileConfig) Save() error {
	if err := c.Validate(); err != nil {
		return err
	}
	path, err := ProfilePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".profile-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func parseBoolInput(raw string, current bool) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return current, nil
	}
	switch v {
	case "y", "yes", "true", "1":
		return true, nil
	case "n", "no", "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes/no")
	}
}

func promptString(scanner *bufio.Scanner, stdout io.Writer, label, current string) (string, error) {
	fmt.Fprintf(stdout, "%s [%s]: ", label, current)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return current, nil
	}
	value := strings.TrimSpace(scanner.Text())
	if value == "" {
		return current, nil
	}
	return value, nil
}

func promptBool(scanner *bufio.Scanner, stdout io.Writer, label string, current bool) (bool, error) {
	defaultLabel := "y/N"
	if current {
		defaultLabel = "Y/n"
	}
	fmt.Fprintf(stdout, "%s [%s]: ", label, defaultLabel)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return current, nil
	}
	return parseBoolInput(scanner.Text(), current)
}

func profileDisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}
	if path == filepath.Join(home, ".nt-cli", "profile.json") {
		return "~/.nt-cli/profile.json"
	}
	return path
}

func RunInitProfile(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runInitProfile(args, stdin, stdout, stderr, isInteractive())
}

func runInitProfile(args []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) int {
	cfg := Defaults()
	force := false
	nonInteractive := false

	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			continue
		}
		keyVal := strings.TrimPrefix(a, "--")
		key, val, hasVal := strings.Cut(keyVal, "=")
		switch key {
		case "force":
			force = true
		case "non-interactive":
			nonInteractive = true
		case "language":
			if !hasVal || strings.TrimSpace(val) == "" {
				fmt.Fprintln(stderr, "invalid flag --language (expected --language=<value>)")
				return 1
			}
			cfg.Language = strings.TrimSpace(val)
		case "tone":
			if !hasVal || strings.TrimSpace(val) == "" {
				fmt.Fprintln(stderr, "invalid flag --tone (expected --tone=<value>)")
				return 1
			}
			cfg.Tone = strings.TrimSpace(val)
		case "verbosity":
			if !hasVal || strings.TrimSpace(val) == "" {
				fmt.Fprintln(stderr, "invalid flag --verbosity (expected --verbosity=<value>)")
				return 1
			}
			cfg.Verbosity = strings.TrimSpace(val)
		}
	}

	path, err := ProfilePath()
	if err != nil {
		fmt.Fprintf(stderr, "profile path failed: %v\n", err)
		return 1
	}
	displayPath := profileDisplayPath(path)
	if _, err := os.Stat(path); err == nil && !force {
		fmt.Fprintf(stdout, "Profile already exists at %s (use --force to overwrite)\n", displayPath)
		return 0
	}

	if !interactive || nonInteractive {
		if err := cfg.Validate(); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(stderr, "profile save failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stderr, "non-interactive mode: profile written with defaults")
		fmt.Fprintf(stdout, "wrote %s\n", displayPath)
		return 0
	}

	scanner := bufio.NewScanner(stdin)
	if v, err := promptString(scanner, stdout, "language", cfg.Language); err != nil {
		fmt.Fprintf(stderr, "prompt failed: %v\n", err)
		return 1
	} else {
		cfg.Language = v
	}
	if v, err := promptString(scanner, stdout, "tone", cfg.Tone); err != nil {
		fmt.Fprintf(stderr, "prompt failed: %v\n", err)
		return 1
	} else {
		cfg.Tone = v
	}
	if v, err := promptString(scanner, stdout, "verbosity", cfg.Verbosity); err != nil {
		fmt.Fprintf(stderr, "prompt failed: %v\n", err)
		return 1
	} else {
		cfg.Verbosity = v
	}
	if v, err := promptBool(scanner, stdout, "ask_before_mutation", cfg.AskBeforeMutation); err != nil {
		fmt.Fprintf(stderr, "prompt failed: %v\n", err)
		return 1
	} else {
		cfg.AskBeforeMutation = v
	}
	if v, err := promptBool(scanner, stdout, "context_autoswitch", cfg.ContextAutoswitch); err != nil {
		fmt.Fprintf(stderr, "prompt failed: %v\n", err)
		return 1
	} else {
		cfg.ContextAutoswitch = v
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(stderr, "profile save failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", displayPath)
	return 0
}
