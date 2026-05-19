package store

import (
	"testing"

	"flint/internal/app"
)

// TestProjectStore_CRUD covers task 1.6: project store exposes
// CreateProject, GetActive, SetActive, ListProjects, FindByFingerprint
// with correct semantics on a freshly-migrated v5 DB.
func TestProjectStore_CRUD(t *testing.T) {
	s := newTestStore(t)

	// After Init on a fresh DB the default project must exist and be active.
	active, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "default" {
		t.Fatalf("expected default active project, got %q", active.Name)
	}
	if active.ID <= 0 {
		t.Fatalf("expected positive id, got %d", active.ID)
	}

	// CreateProject inserts and returns a populated record.
	p, err := s.CreateProject(app.ProjectInput{
		Name:        "alpha",
		RootPath:    "/repos/alpha",
		Fingerprint: "fp-alpha",
	})
	if err != nil {
		t.Fatalf("CreateProject alpha: %v", err)
	}
	if p.ID <= 0 || p.Name != "alpha" || p.RootPath != "/repos/alpha" {
		t.Fatalf("CreateProject returned %+v", p)
	}

	// Triangulation: a second distinct project produces a distinct id.
	p2, err := s.CreateProject(app.ProjectInput{
		Name:        "beta",
		RootPath:    "/repos/beta",
		Fingerprint: "fp-beta",
	})
	if err != nil {
		t.Fatalf("CreateProject beta: %v", err)
	}
	if p2.ID == p.ID {
		t.Fatalf("expected distinct ids, got %d == %d", p.ID, p2.ID)
	}

	// FindByFingerprint must locate by fingerprint, miss on unknown.
	hit, err := s.FindByFingerprint("fp-beta")
	if err != nil {
		t.Fatalf("FindByFingerprint hit: %v", err)
	}
	if hit == nil || hit.ID != p2.ID {
		t.Fatalf("expected beta hit, got %+v", hit)
	}
	miss, err := s.FindByFingerprint("nope")
	if err != nil {
		t.Fatalf("FindByFingerprint miss: %v", err)
	}
	if miss != nil {
		t.Fatalf("expected nil on miss, got %+v", miss)
	}

	// ListProjects must include default + alpha + beta.
	all, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 projects, got %d (%+v)", len(all), all)
	}
	names := map[string]bool{}
	for _, pr := range all {
		names[pr.Name] = true
	}
	for _, want := range []string{"default", "alpha", "beta"} {
		if !names[want] {
			t.Fatalf("ListProjects missing %q: %+v", want, all)
		}
	}

	// SetActive switches and GetActive reflects the change.
	if err := s.SetActive(p2.ID); err != nil {
		t.Fatalf("SetActive beta: %v", err)
	}
	active2, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive after switch: %v", err)
	}
	if active2.ID != p2.ID || active2.Name != "beta" {
		t.Fatalf("expected active=beta, got %+v", active2)
	}

	// SetActive on unknown id must error and not mutate state.
	if err := s.SetActive(9999); err == nil {
		t.Fatalf("SetActive unknown id must error")
	}
	active3, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive post-error: %v", err)
	}
	if active3.ID != p2.ID {
		t.Fatalf("active changed despite SetActive error: %+v", active3)
	}
}

// TestFindByRootPath_SingleMatch verifies FindByRootPath returns the single
// project whose root_path is a prefix of cwd.
// RED: FindByRootPath not implemented yet → build fails.
func TestFindByRootPath_SingleMatch(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateProject(app.ProjectInput{
		Name:        "alpha",
		RootPath:    "/repos/alpha",
		Fingerprint: "fp-alpha",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// cwd is a subdirectory of /repos/alpha — should match.
	matches, err := s.FindByRootPath("/repos/alpha/src")
	if err != nil {
		t.Fatalf("FindByRootPath: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(matches), matches)
	}
	if matches[0].Name != "alpha" {
		t.Errorf("expected project 'alpha', got %q", matches[0].Name)
	}
}

// TestFindByRootPath_AmbiguousMatch verifies FindByRootPath returns multiple
// projects when cwd is inside more than one registered root_path.
func TestFindByRootPath_AmbiguousMatch(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateProject(app.ProjectInput{Name: "outer", RootPath: "/repos", Fingerprint: "fp-outer"})
	if err != nil {
		t.Fatalf("create outer: %v", err)
	}
	_, err = s.CreateProject(app.ProjectInput{Name: "inner", RootPath: "/repos/inner", Fingerprint: "fp-inner"})
	if err != nil {
		t.Fatalf("create inner: %v", err)
	}

	// cwd is inside both "/repos" and "/repos/inner".
	matches, err := s.FindByRootPath("/repos/inner/src")
	if err != nil {
		t.Fatalf("FindByRootPath ambiguous: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected ≥2 matches for ambiguous cwd, got %d: %+v", len(matches), matches)
	}
}

// TestFindByRootPath_NoMatch verifies FindByRootPath returns empty slice
// when cwd is not inside any registered root_path.
func TestFindByRootPath_NoMatch(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.CreateProject(app.ProjectInput{Name: "myproj", RootPath: "/repos/myproj", Fingerprint: "fp1"})

	matches, err := s.FindByRootPath("/unrelated/path")
	if err != nil {
		t.Fatalf("FindByRootPath no-match: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d: %+v", len(matches), matches)
	}
}
