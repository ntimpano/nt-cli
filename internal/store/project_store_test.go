package store

import (
	"testing"

	"nt-cli/internal/app"
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
