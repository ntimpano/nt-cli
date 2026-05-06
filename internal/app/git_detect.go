package app

import (
	"os/exec"
	"strings"
)

// gitInfoResolver is a function type injected into projectEngineImpl so tests
// can provide deterministic git info without spawning real processes.
type gitInfoResolver func(cwd string) (gitRoot, remoteURL string)

// detectGitContext runs `git rev-parse --show-toplevel` and
// `git remote get-url origin` in cwd. Returns empty strings when cwd is not
// inside a git repo or git is not available.
func detectGitContext(cwd string) (gitRoot, remoteURL string) {
	// Resolve git root
	out, err := runGitCmd(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ""
	}
	gitRoot = strings.TrimSpace(out)
	if gitRoot == "" {
		return "", ""
	}

	// Best-effort remote URL — not fatal if the repo has no remote
	remote, err := runGitCmd(cwd, "remote", "get-url", "origin")
	if err == nil {
		remoteURL = strings.TrimSpace(remote)
	}
	return gitRoot, remoteURL
}

// runGitCmd runs git with the given args inside dir and returns stdout.
// Returns an error when git exits non-zero (e.g. not a git repo).
func runGitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
