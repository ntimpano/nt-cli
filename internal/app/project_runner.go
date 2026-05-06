package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// runProject dispatches `nt-cli project <detect|current|list|switch>`.
func runProject(svc *Service, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: nt-cli project <detect|current|list|switch>")
		return 1
	}
	switch args[0] {
	case "current":
		return runProjectCurrent(svc, stdout, stderr)
	case "list":
		return runProjectList(svc, stdout, stderr)
	case "switch":
		return runProjectSwitch(svc, args[1:], stdout, stderr)
	case "detect":
		return runProjectDetect(svc, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown project subcommand %q (valid: detect|current|list|switch)\n", args[0])
		return 1
	}
}

func runProjectCurrent(svc *Service, stdout, stderr io.Writer) int {
	eng := svc.ProjectEng
	if eng == nil {
		fmt.Fprintln(stderr, "project commands not available")
		return 1
	}
	p, err := eng.Current()
	if err != nil {
		fmt.Fprintf(stderr, "project current failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "#%d %s (active)\n", p.ID, p.Name)
	return 0
}

func runProjectList(svc *Service, stdout, stderr io.Writer) int {
	eng := svc.ProjectEng
	if eng == nil {
		fmt.Fprintln(stderr, "project commands not available")
		return 1
	}
	projects, err := eng.List()
	if err != nil {
		fmt.Fprintf(stderr, "project list failed: %v\n", err)
		return 1
	}
	active, _ := eng.Current()
	for _, p := range projects {
		marker := " "
		if p.ID == active.ID {
			marker = "*"
		}
		fmt.Fprintf(stdout, "%s #%d %s\n", marker, p.ID, p.Name)
	}
	return 0
}

func runProjectSwitch(svc *Service, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: nt-cli project switch <id>")
		return 1
	}
	id, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil || id <= 0 {
		fmt.Fprintln(stderr, "id must be a positive integer")
		return 1
	}
	eng := svc.ProjectEng
	if eng == nil {
		fmt.Fprintln(stderr, "project commands not available")
		return 1
	}

	// Task 2.8: take a pre-switch backup snapshot before committing the switch.
	backupPath := preBackupPath()
	if backupErr := svc.Backup(backupPath); backupErr != nil {
		// Non-fatal: warn but proceed — the backup directory may not exist yet
		// on a first run; the spec says "take a backup before the switch commits",
		// not "abort if backup fails". We surface the warning so the user knows.
		fmt.Fprintf(stderr, "warning: pre-switch backup failed: %v\n", backupErr)
	}

	if err := eng.Switch(id); err != nil {
		fmt.Fprintf(stderr, "project switch failed: %v\n", err)
		return 1
	}
	// Update in-memory active project for this session
	svc.SetActiveProject(id)
	fmt.Fprintf(stdout, "switched to project #%d\n", id)
	return 0
}

func runProjectDetect(svc *Service, stdout, stderr io.Writer) int {
	eng := svc.ProjectEng
	if eng == nil {
		fmt.Fprintln(stderr, "project commands not available")
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "project detect: %v\n", err)
		return 1
	}
	result, err := eng.Probe(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "project detect failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "status=%s candidate=%s confidence=%s\n%s\n",
		result.Status, result.Candidate, result.Confidence, result.Reason)
	return 0
}

// preBackupPath returns a timestamped path inside the default backup directory
// for the pre-switch snapshot.
func preBackupPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.TempDir()
	}
	backupDir := filepath.Join(home, ".nt-cli", "backups")
	_ = os.MkdirAll(backupDir, 0o755)
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return filepath.Join(backupDir, "pre-switch-"+stamp+".db")
}
