package store

import (
	"testing"
	"time"

	"flint/internal/app"
)

// TestProjectScoped_RecallFiltered_DefaultsToActiveProject verifies that
// RecallFiltered returns only rows belonging to the active project when
// ProjectID is set (no --all).
func TestProjectScoped_RecallFiltered_DefaultsToActiveProject(t *testing.T) {
	s := newTestStore(t)

	// Insert two rows: one for project 1, one for project 2.
	// v5 migration creates the "default" project with id=1.
	// We create a second project for isolation.
	projB, err := s.CreateProject(app.ProjectInput{Name: "projB", RootPath: "/b", Fingerprint: "fpB"})
	if err != nil {
		t.Fatalf("create projB: %v", err)
	}

	// Save row under default project (id=1) using SaveWithMeta
	_, err = s.SaveWithMeta(app.SaveRequest{
		Content:   "note for project A",
		Type:      "manual",
		Scope:     "project",
		ProjectID: 1,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("save projA note: %v", err)
	}

	// Save row under project B
	_, err = s.SaveWithMeta(app.SaveRequest{
		Content:   "note for project B",
		Type:      "manual",
		Scope:     "project",
		ProjectID: projB.ID,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("save projB note: %v", err)
	}

	// Recall filtered with project A filter — should return only A's note
	items, err := s.RecallFiltered(app.RecallOptions{
		Query:     "note",
		Limit:     10,
		ProjectID: 1,
	})
	if err != nil {
		t.Fatalf("recall filtered: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item for project A, got %d", len(items))
	}
	if items[0].Content != "note for project A" {
		t.Errorf("unexpected content: %q", items[0].Content)
	}
}

// TestProjectScoped_RecallFiltered_AllProjectsBypass verifies that
// AllProjects=true bypasses the project filter.
func TestProjectScoped_RecallFiltered_AllProjectsBypass(t *testing.T) {
	s := newTestStore(t)

	projB, err := s.CreateProject(app.ProjectInput{Name: "projB2", RootPath: "/b2", Fingerprint: "fpB2"})
	if err != nil {
		t.Fatalf("create projB2: %v", err)
	}

	now := time.Now().UTC()
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "alpha note A", Type: "manual", Scope: "project", ProjectID: 1, CreatedAt: now})
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "alpha note B", Type: "manual", Scope: "project", ProjectID: projB.ID, CreatedAt: now})

	// With AllProjects=true — both should come back
	items, err := s.RecallFiltered(app.RecallOptions{
		Query:       "alpha",
		Limit:       10,
		ProjectID:   1,
		AllProjects: true,
	})
	if err != nil {
		t.Fatalf("recall all: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items with AllProjects=true, got %d", len(items))
	}
}

// TestProjectScoped_Context_DefaultsToActiveProject checks that Context
// only returns rows for the active project when ProjectID is set.
func TestProjectScoped_Context_DefaultsToActiveProject(t *testing.T) {
	s := newTestStore(t)

	projB, err := s.CreateProject(app.ProjectInput{Name: "ctxB", RootPath: "/ctxB", Fingerprint: "fpCtxB"})
	if err != nil {
		t.Fatalf("create ctxB: %v", err)
	}

	now := time.Now().UTC()
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "ctx note A", Type: "manual", Scope: "project", ProjectID: 1, CreatedAt: now})
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "ctx note B", Type: "manual", Scope: "project", ProjectID: projB.ID, CreatedAt: now})

	items, err := s.ContextFiltered(app.ContextOptions{N: 10, ProjectID: 1})
	if err != nil {
		t.Fatalf("context filtered: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if items[0].Content != "ctx note A" {
		t.Errorf("unexpected content: %q", items[0].Content)
	}
}

// TestProjectScoped_List_FiltersByProject ensures List respects ProjectID.
func TestProjectScoped_List_FiltersByProject(t *testing.T) {
	s := newTestStore(t)

	projB, err := s.CreateProject(app.ProjectInput{Name: "listB", RootPath: "/listB", Fingerprint: "fpListB"})
	if err != nil {
		t.Fatalf("create listB: %v", err)
	}

	now := time.Now().UTC()
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "list note A", Type: "manual", Scope: "project", ProjectID: 1, CreatedAt: now})
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "list note B", Type: "manual", Scope: "project", ProjectID: projB.ID, CreatedAt: now})

	items, err := s.ListFiltered(app.ListOptions{Limit: 10, ProjectID: 1})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

// TestProjectScoped_Save_StampsProjectID verifies that SaveWithMeta stamps
// the active project_id on the row, and subsequent scoped recall sees it.
func TestProjectScoped_Save_StampsProjectID(t *testing.T) {
	s := newTestStore(t)

	id, err := s.SaveWithMeta(app.SaveRequest{
		Content:   "stamped note",
		Type:      "manual",
		Scope:     "project",
		ProjectID: 1,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	// Recall with ProjectID=1 must return this note
	items, err := s.RecallFiltered(app.RecallOptions{
		Query:     "stamped",
		Limit:     5,
		ProjectID: 1,
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// Integration test 2.9: project isolation end-to-end
// ---------------------------------------------------------------------------

func TestIntegration_ProjectIsolation(t *testing.T) {
	s := newTestStore(t)

	// Simulate two projects (default=1, and projX)
	projX, err := s.CreateProject(app.ProjectInput{Name: "projX", RootPath: "/x", Fingerprint: "fpX"})
	if err != nil {
		t.Fatalf("create projX: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_, _ = s.SaveWithMeta(app.SaveRequest{Content: "projA isolation note", Type: "manual", Scope: "project", ProjectID: 1, CreatedAt: now})
		_, _ = s.SaveWithMeta(app.SaveRequest{Content: "projX isolation note", Type: "manual", Scope: "project", ProjectID: projX.ID, CreatedAt: now})
	}

	// Switch active to projX
	if err := s.SetActive(projX.ID); err != nil {
		t.Fatalf("set active: %v", err)
	}
	active, _ := s.GetActive()
	if active.ID != projX.ID {
		t.Errorf("expected active=projX, got %v", active.Name)
	}

	// Recall scoped to projX — only projX notes
	items, err := s.RecallFiltered(app.RecallOptions{
		Query:     "isolation",
		Limit:     10,
		ProjectID: projX.ID,
	})
	if err != nil {
		t.Fatalf("recall projX: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 projX notes, got %d", len(items))
	}
	for _, it := range items {
		if it.Content != "projX isolation note" {
			t.Errorf("unexpected content from projX scope: %q", it.Content)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration test 2.10: --all cross-project read
// ---------------------------------------------------------------------------

func TestIntegration_AllProjectsRead(t *testing.T) {
	s := newTestStore(t)

	projY, err := s.CreateProject(app.ProjectInput{Name: "projY", RootPath: "/y", Fingerprint: "fpY"})
	if err != nil {
		t.Fatalf("create projY: %v", err)
	}

	now := time.Now().UTC()
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "cross-project note from default", Type: "manual", Scope: "project", ProjectID: 1, CreatedAt: now})
	_, _ = s.SaveWithMeta(app.SaveRequest{Content: "cross-project note from projY", Type: "manual", Scope: "project", ProjectID: projY.ID, CreatedAt: now})

	// Active project is default (1), --all bypasses
	items, err := s.RecallFiltered(app.RecallOptions{
		Query:       "cross-project",
		Limit:       10,
		ProjectID:   1,
		AllProjects: true,
	})
	if err != nil {
		t.Fatalf("recall all: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items with --all, got %d", len(items))
	}
}
