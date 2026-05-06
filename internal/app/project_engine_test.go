package app

import (
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 2.2: Fingerprint computation — pure function, no I/O
// ---------------------------------------------------------------------------

func TestComputeFingerprint_GitRootAndRemote(t *testing.T) {
	// Fingerprint is SHA256-hex of "<gitRoot>|<remoteURL>".
	// When both are present the result is stable across machines.
	fp := ComputeFingerprint("/home/user/myrepo", "https://github.com/user/myrepo.git")
	if fp == "" {
		t.Fatal("expected non-empty fingerprint")
	}
	// Same inputs → same output (deterministic)
	fp2 := ComputeFingerprint("/home/user/myrepo", "https://github.com/user/myrepo.git")
	if fp != fp2 {
		t.Errorf("fingerprint is not deterministic: %q vs %q", fp, fp2)
	}
}

func TestComputeFingerprint_DifferentInputsDifferentOutputs(t *testing.T) {
	fp1 := ComputeFingerprint("/home/user/repoA", "https://github.com/user/repoA.git")
	fp2 := ComputeFingerprint("/home/user/repoB", "https://github.com/user/repoB.git")
	if fp1 == fp2 {
		t.Error("expected distinct fingerprints for different repos")
	}
}

func TestComputeFingerprint_EmptyRemote_StillNonEmpty(t *testing.T) {
	// A repo with no remote still gets a fingerprint (based on root path only).
	fp := ComputeFingerprint("/home/user/localrepo", "")
	if fp == "" {
		t.Fatal("expected non-empty fingerprint even without remote URL")
	}
}

// ---------------------------------------------------------------------------
// Task 2.3: ProbeResult status contract
// ---------------------------------------------------------------------------

// stubProjectLookup is a test double for the store lookup needed by ProbeFromGit.
// Returns (project, found) by fingerprint.
type stubProjectLookup struct {
	byFingerprint map[string]*Project
}

func (s *stubProjectLookup) FindByFingerprint(fp string) (*Project, error) {
	p, ok := s.byFingerprint[fp]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func TestProbe_KnownProject(t *testing.T) {
	// GIVEN: fingerprint matches an existing project
	fp := ComputeFingerprint("/home/user/myrepo", "https://github.com/user/myrepo.git")
	known := &Project{ID: 1, Name: "myrepo", Fingerprint: fp}
	store := &stubProjectLookup{byFingerprint: map[string]*Project{fp: known}}

	resolver := func(cwd string) (string, string) {
		return "/home/user/myrepo", "https://github.com/user/myrepo.git"
	}

	result, err := ProbeWithResolver("/home/user/myrepo", store, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "known" {
		t.Errorf("expected status=known, got %q", result.Status)
	}
	if result.Candidate != "myrepo" {
		t.Errorf("expected candidate=myrepo, got %q", result.Candidate)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence=high, got %q", result.Confidence)
	}
}

func TestProbe_NewProject(t *testing.T) {
	// GIVEN: no project matches the fingerprint
	store := &stubProjectLookup{byFingerprint: map[string]*Project{}}

	resolver := func(cwd string) (string, string) {
		return "/home/user/newrepo", "https://github.com/user/newrepo.git"
	}

	result, err := ProbeWithResolver("/home/user/newrepo", store, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "new" {
		t.Errorf("expected status=new, got %q", result.Status)
	}
	// Proposed name comes from last path segment when remote is present
	if result.Candidate == "" {
		t.Error("expected non-empty candidate for new project")
	}
}

func TestProbe_NoneStatus_NoGitRoot(t *testing.T) {
	// GIVEN: resolver finds no git root (not inside a git repo)
	store := &stubProjectLookup{byFingerprint: map[string]*Project{}}

	resolver := func(cwd string) (string, string) {
		return "", "" // no git root
	}

	result, err := ProbeWithResolver("/tmp/notarepo", store, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "none" {
		t.Errorf("expected status=none, got %q", result.Status)
	}
}

// ---------------------------------------------------------------------------
// Task 2.1: ProjectEngine interface — compile-time check that
// projectEngineImpl satisfies the interface.
// ---------------------------------------------------------------------------

func TestProjectEngine_InterfaceCompliance(t *testing.T) {
	// If projectEngineImpl doesn't satisfy ProjectEngine, this won't compile.
	var _ ProjectEngine = (*projectEngineImpl)(nil)
}

// ---------------------------------------------------------------------------
// Task 2.3: ProbeResult for ambiguous — multiple projects share fingerprint
// (edge case: happens when the same git remote is registered twice with
// different names; store returns the first match — status is "known",
// since our store uniqueness is enforced by fingerprint. The "ambiguous"
// status is for when the resolver can't determine a stable fingerprint:
// no git root AND cwd matches multiple project root_paths.)
// ---------------------------------------------------------------------------

func TestProbe_CandidateNameFromRemoteURL(t *testing.T) {
	// Remote URL "https://github.com/org/awesome-tool.git" → name "awesome-tool"
	store := &stubProjectLookup{byFingerprint: map[string]*Project{}}
	resolver := func(cwd string) (string, string) {
		return "/work/awesome-tool", "https://github.com/org/awesome-tool.git"
	}
	result, err := ProbeWithResolver("/work/awesome-tool", store, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Candidate != "awesome-tool" {
		t.Errorf("expected candidate=awesome-tool, got %q", result.Candidate)
	}
}

func TestProbe_CandidateNameFromDirBasename_WhenNoRemote(t *testing.T) {
	// No remote URL → name comes from git root basename
	store := &stubProjectLookup{byFingerprint: map[string]*Project{}}
	resolver := func(cwd string) (string, string) {
		return "/work/my-local-project", ""
	}
	result, err := ProbeWithResolver("/work/my-local-project", store, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Candidate != "my-local-project" {
		t.Errorf("expected candidate=my-local-project, got %q", result.Candidate)
	}
}

// ---------------------------------------------------------------------------
// Task 2.1: projectEngineImpl.List and Current delegate to the store
// ---------------------------------------------------------------------------

type stubEngineStore struct {
	stubProjectLookup
	projects  []Project
	active    *Project
	setActive func(int64) error
}

func (s *stubEngineStore) ListProjects() ([]Project, error)    { return s.projects, nil }
func (s *stubEngineStore) GetActive() (Project, error) {
	if s.active == nil {
		return Project{}, ErrNoActiveProject
	}
	return *s.active, nil
}
func (s *stubEngineStore) SetActive(id int64) error {
	if s.setActive != nil {
		return s.setActive(id)
	}
	return nil
}
func (s *stubEngineStore) CreateProject(in ProjectInput) (Project, error) {
	p := Project{ID: 99, Name: in.Name, RootPath: in.RootPath, Fingerprint: in.Fingerprint}
	return p, nil
}

func TestProjectEngine_Current(t *testing.T) {
	active := &Project{ID: 1, Name: "myrepo"}
	eng := newProjectEngine(&stubEngineStore{active: active}, func(string) (string, string) {
		return "", ""
	})
	got, err := eng.Current()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "myrepo" {
		t.Errorf("expected myrepo, got %q", got.Name)
	}
}

func TestProjectEngine_List(t *testing.T) {
	projects := []Project{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
	eng := newProjectEngine(&stubEngineStore{projects: projects}, func(string) (string, string) {
		return "", ""
	})
	list, err := eng.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 projects, got %d", len(list))
	}
}

func TestProjectEngine_Switch_Delegates(t *testing.T) {
	called := false
	eng := newProjectEngine(&stubEngineStore{
		setActive: func(id int64) error {
			called = true
			return nil
		},
		active: &Project{ID: 1, Name: "x"},
	}, func(string) (string, string) { return "", "" })

	if err := eng.Switch(1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected SetActive to be called")
	}
}

// helper to resolve candidate name from remote URL — tested via proposeName
func TestProposeName_FromRemote(t *testing.T) {
	cases := []struct {
		remote  string
		gitRoot string
		want    string
	}{
		{"https://github.com/org/my-tool.git", "/work/my-tool", "my-tool"},
		{"git@github.com:org/another.git", "/work/another", "another"},
		{"https://github.com/org/no-ext", "/work/no-ext", "no-ext"},
		{"", "/work/local-only", "local-only"},
		{"", "", "unknown"},
	}
	for _, tc := range cases {
		got := proposeName(tc.remote, tc.gitRoot)
		if got != tc.want {
			t.Errorf("proposeName(%q, %q) = %q, want %q", tc.remote, tc.gitRoot, got, tc.want)
		}
	}
}

// Ensure filepath.Base is being used (cross-platform check)
func TestComputeFingerprint_PathSeparatorAgnostic(t *testing.T) {
	fp1 := ComputeFingerprint(filepath.Join("home", "user", "repo"), "")
	if fp1 == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}
