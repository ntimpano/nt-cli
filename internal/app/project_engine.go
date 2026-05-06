package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrNoActiveProject is returned by ProjectStore.GetActive when no active
// project pointer is set (should never happen after v5 migration, but
// guards the path for stubbed environments).
var ErrNoActiveProject = errors.New("no active project")

// ProbeResult is the read-only proposal returned by Probe. It never mutates
// state — callers invoke Confirm or Switch explicitly to change the active
// project.
type ProbeResult struct {
	Status     string // "known" | "new" | "ambiguous" | "none"
	Candidate  string // project name (existing or proposed)
	Confidence string // "high" | "low"
	Reason     string
}

// FingerprintLookup is the subset of the project store needed by ProbeWithResolver.
// Kept narrow so unit tests only need to stub one method.
type FingerprintLookup interface {
	FindByFingerprint(fp string) (*Project, error)
}

// ProjectStore is the full interface the engine needs to read/write projects.
type ProjectStore interface {
	FingerprintLookup
	ListProjects() ([]Project, error)
	GetActive() (Project, error)
	SetActive(id int64) error
	CreateProject(in ProjectInput) (Project, error)
}

// ProjectEngine is the application-layer interface for project context
// management. All state-mutating operations are explicit — no silent switches.
type ProjectEngine interface {
	Probe(cwd string) (ProbeResult, error)
	Confirm(candidate string) error
	List() ([]Project, error)
	Current() (Project, error)
	Switch(projectID int64) error
}

// projectEngineImpl is the concrete implementation of ProjectEngine.
type projectEngineImpl struct {
	store    ProjectStore
	resolver gitInfoResolver
}

// newProjectEngine constructs a projectEngineImpl with the given store and
// git resolver. In production, pass realGitResolver; in tests, pass a stub.
func newProjectEngine(store ProjectStore, resolver gitInfoResolver) *projectEngineImpl {
	return &projectEngineImpl{store: store, resolver: resolver}
}

// Probe inspects cwd and returns a ProbeResult without mutating state.
func (e *projectEngineImpl) Probe(cwd string) (ProbeResult, error) {
	return ProbeWithResolver(cwd, e.store, e.resolver)
}

// Confirm creates (if new) or switches to the candidate project identified by
// the most recent Probe call for cwd. Callers must hold the candidate name
// from ProbeResult.Candidate. For now it switches by name lookup.
func (e *projectEngineImpl) Confirm(candidate string) error {
	projects, err := e.store.ListProjects()
	if err != nil {
		return fmt.Errorf("confirm: list projects: %w", err)
	}
	for _, p := range projects {
		if p.Name == candidate {
			return e.store.SetActive(p.ID)
		}
	}
	return fmt.Errorf("confirm: no project named %q", candidate)
}

// List returns all projects from the store.
func (e *projectEngineImpl) List() ([]Project, error) {
	return e.store.ListProjects()
}

// Current returns the active project.
func (e *projectEngineImpl) Current() (Project, error) {
	return e.store.GetActive()
}

// Switch atomically sets the active project to the given id.
// The caller is responsible for taking a backup before calling Switch
// (enforced at the CLI/MCP layer per spec task 2.8).
func (e *projectEngineImpl) Switch(projectID int64) error {
	return e.store.SetActive(projectID)
}

// ---------------------------------------------------------------------------
// Task 2.2: Fingerprint computation
// ---------------------------------------------------------------------------

// ComputeFingerprint returns a stable hex-encoded SHA-256 fingerprint for a
// project, derived from "<gitRoot>|<remoteURL>". When remoteURL is empty the
// fingerprint uses the root path only. Empty gitRoot always returns "".
func ComputeFingerprint(gitRoot, remoteURL string) string {
	if gitRoot == "" {
		return ""
	}
	raw := gitRoot + "|" + remoteURL
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Task 2.3: ProbeWithResolver — testable pure probe logic
// ---------------------------------------------------------------------------

// ProbeWithResolver runs the probe logic with an injected git resolver,
// making it deterministic in tests (no real git calls).
func ProbeWithResolver(cwd string, lookup FingerprintLookup, resolver gitInfoResolver) (ProbeResult, error) {
	gitRoot, remoteURL := resolver(cwd)
	if gitRoot == "" {
		return ProbeResult{
			Status:     "none",
			Confidence: "low",
			Reason:     "not inside a git repository",
		}, nil
	}

	fp := ComputeFingerprint(gitRoot, remoteURL)
	existing, err := lookup.FindByFingerprint(fp)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe: lookup fingerprint: %w", err)
	}

	name := proposeName(remoteURL, gitRoot)
	if existing != nil {
		return ProbeResult{
			Status:     "known",
			Candidate:  existing.Name,
			Confidence: "high",
			Reason:     "fingerprint matched existing project",
		}, nil
	}

	return ProbeResult{
		Status:     "new",
		Candidate:  name,
		Confidence: "high",
		Reason:     "no existing project matches this fingerprint",
	}, nil
}

// proposeName derives a human-friendly project name from a remote URL or,
// failing that, from the git root directory basename.
// It strips ".git" suffixes and SSH-style "git@host:org/name" prefixes.
func proposeName(remoteURL, gitRoot string) string {
	if remoteURL != "" {
		// Strip SSH prefix "git@host:org/" → keep "name"
		u := remoteURL
		if idx := strings.LastIndex(u, "/"); idx >= 0 {
			u = u[idx+1:]
		} else if idx := strings.Index(u, ":"); idx >= 0 {
			u = u[idx+1:]
			if idx2 := strings.LastIndex(u, "/"); idx2 >= 0 {
				u = u[idx2+1:]
			}
		}
		u = strings.TrimSuffix(u, ".git")
		if u != "" {
			return u
		}
	}
	if gitRoot != "" {
		return filepath.Base(gitRoot)
	}
	return "unknown"
}

// realGitResolver calls the real git executable to resolve gitRoot and
// remoteURL from cwd. Used in production (cmd/nt-cli/main.go).
// Returns ("", "") when cwd is not inside a git repository.
func realGitResolver(cwd string) (gitRoot, remoteURL string) {
	// Delegate to the git helpers in git_detect.go (same package).
	return detectGitContext(cwd)
}
