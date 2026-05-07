package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

	backupPath, pathErr := preBackupPath(id)
	if pathErr != nil {
		fmt.Fprintf(stderr, "project switch failed: pre-switch backup path: %v\n", pathErr)
		return 1
	}
	if backupErr := svc.Backup(backupPath); backupErr != nil {
		fmt.Fprintf(stderr, "project switch failed: pre-switch backup: %v\n", backupErr)
		return 1
	}
	cleanupPreSwitchBackups(id)

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

// preBackupPath returns a unique path inside ~/.nt-cli/backups with pattern
// pre-switch-<projectID>-<unix>.db.
func preBackupPath(projectID int64) (string, error) {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.TempDir()
	}
	backupDir := filepath.Join(home, ".nt-cli", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().UnixNano()
	return filepath.Join(backupDir, fmt.Sprintf("pre-switch-%d-%d.db", projectID, stamp)), nil
}

func cleanupPreSwitchBackups(projectID int64) {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.TempDir()
	}
	backupDir := filepath.Join(home, ".nt-cli", "backups")
	pattern := filepath.Join(backupDir, fmt.Sprintf("pre-switch-%d-*.db", projectID))
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= 5 {
		return
	}
	type entry struct {
		path string
		mod  time.Time
	}
	entries := make([]entry, 0, len(matches))
	for _, p := range matches {
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		entries = append(entries, entry{path: p, mod: info.ModTime()})
	}
	if len(entries) <= 5 {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mod.After(entries[j].mod)
	})
	for _, e := range entries[5:] {
		_ = os.Remove(e.path)
	}
}
