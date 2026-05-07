package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// memoryCommands is the set of CLI verbs that trigger autoswitch.
// Operational commands (init, session, import, backup, restore, doctor,
// parity, project, mcp) are intentionally excluded per spec.
var memoryCommands = map[string]bool{
	"save":    true,
	"recall":  true,
	"context": true,
	"list":    true,
	"get":     true,
	"update":  true,
	"delete":  true,
}

// IsMemoryCommand reports whether cmd is in the memory-command set.
// Exported for testing.
func IsMemoryCommand(cmd string) bool {
	return memoryCommands[cmd]
}

// AutoswitchPolicy carries the injectable dependencies for autoswitch.
// Keeping them explicit makes the policy deterministic and testable
// without hitting the real filesystem or real TTY.
type AutoswitchPolicy struct {
	// GetCwd returns the working directory to probe. Defaults to os.Getwd.
	GetCwd func() (string, error)
	// IsInteractive reports whether stdin is a real TTY. Defaults to
	// os.Stdin TTY detection via isatty.
	IsInteractive func() bool
	// Stdin is used to read the user's confirmation in interactive mode.
	Stdin io.Reader
	// Stderr is used to print the confirmation prompt. Interactive prompts
	// must not pollute stdout so that machine-readable output (e.g. --json)
	// remains clean.
	Stderr io.Writer
}

// defaultIsInteractive returns true when os.Stdin is a character device.
func defaultIsInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// NewDefaultAutoswitchPolicy constructs a production-ready policy wired to
// real os.Getwd, real TTY detection, and real os.Stdin.
func NewDefaultAutoswitchPolicy(stdin io.Reader, stderr io.Writer) AutoswitchPolicy {
	return AutoswitchPolicy{
		GetCwd:        os.Getwd,
		IsInteractive: defaultIsInteractive,
		Stdin:         stdin,
		Stderr:        stderr,
	}
}

// ApplyAutoswitch runs the context-autoswitch logic for the given memory
// command. It probes the current working directory and, depending on
// confidence and interactivity, either silently switches, prompts the
// user, or does nothing (keeping current context).
//
// The function is idempotent per invocation: it is called once before
// RunCLI dispatches the command, so anti-noise is implicit (one probe
// per command invocation).
//
// Returns true when the active project was changed (for logging/tests).
func ApplyAutoswitch(svc *Service, policy AutoswitchPolicy) (switched bool) {
	eng := svc.ProjectEng
	if eng == nil {
		return false
	}

	getCwd := policy.GetCwd
	if getCwd == nil {
		getCwd = os.Getwd
	}

	cwd, err := getCwd()
	if err != nil {
		return false
	}

	result, err := eng.Probe(cwd)
	if err != nil {
		return false
	}

	// "none" means we have no signal — leave context alone.
	if result.Status == "none" {
		return false
	}

	// Get active project to detect if a switch is needed.
	active, err := eng.Current()
	if err != nil {
		return false
	}

	switch result.Status {
	case "known":
		// High-confidence known project: auto-switch silently.
		if result.Confidence == "high" && result.Candidate != "" && result.Candidate != active.Name {
			return silentSwitch(svc, eng, result.Candidate)
		}
		// Low-confidence known: treat as ambiguous — prompt in interactive mode.
		if result.Confidence != "high" && result.Candidate != active.Name {
			return promptSwitch(svc, eng, result.Candidate, policy)
		}

	case "new":
		// New project = uncertain; prompt in interactive mode only.
		return promptSwitch(svc, eng, result.Candidate, policy)

	case "ambiguous":
		// Multiple candidates = uncertain; prompt in interactive mode only.
		candidates := make([]string, 0, len(result.Candidates))
		for _, c := range result.Candidates {
			candidates = append(candidates, c.Name)
		}
		if len(candidates) == 0 && result.Candidate != "" {
			candidates = append(candidates, result.Candidate)
		}
		if len(candidates) > 0 {
			return promptSwitch(svc, eng, candidates[0], policy)
		}
	}

	return false
}

// silentSwitch changes the active project by name without user interaction.
func silentSwitch(svc *Service, eng ProjectEngine, candidate string) bool {
	// Find project by name in list.
	projects, err := eng.List()
	if err != nil {
		return false
	}
	for _, p := range projects {
		if p.Name == candidate {
			if err := eng.Switch(p.ID); err != nil {
				return false
			}
			svc.SetActiveProject(p.ID)
			return true
		}
	}
	return false
}

// promptSwitch presents a confirmation prompt in interactive mode.
// In non-interactive mode it is a no-op (returns false).
func promptSwitch(svc *Service, eng ProjectEngine, candidate string, policy AutoswitchPolicy) bool {
	isInteractive := policy.IsInteractive
	if isInteractive == nil {
		isInteractive = defaultIsInteractive
	}
	if !isInteractive() {
		// Non-interactive: never prompt, never mutate on uncertain state.
		return false
	}

	stderr := policy.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := policy.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	fmt.Fprintf(stderr, "autoswitch: detected project %q — switch context? [y/N] ", candidate)
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		// EOF or read error: treat as decline.
		fmt.Fprintln(stderr, "")
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		// User declined: keep current context.
		return false
	}

	// User confirmed: attempt to switch. If the project is new (not in list),
	// call Confirm to create+switch; otherwise Switch by ID.
	projects, err := eng.List()
	if err != nil {
		return false
	}
	for _, p := range projects {
		if p.Name == candidate {
			if err := eng.Switch(p.ID); err != nil {
				return false
			}
			svc.SetActiveProject(p.ID)
			return true
		}
	}
	// Project doesn't exist yet — use Confirm (creates + switches).
	if err := eng.Confirm(candidate); err != nil {
		return false
	}
	// Re-read active project to update the service's in-memory ID.
	if active, err := eng.Current(); err == nil {
		svc.SetActiveProject(active.ID)
	}
	return true
}
