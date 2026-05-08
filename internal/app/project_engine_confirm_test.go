package app

import "testing"

type confirmStoreStub struct {
	projects      []Project
	setActiveID   int64
	setActiveHits int
	createInput   ProjectInput
	createHits    int
	created       Project
}

func (s *confirmStoreStub) FindByFingerprint(string) (*Project, error) { return nil, nil }
func (s *confirmStoreStub) ListProjects() ([]Project, error)           { return s.projects, nil }
func (s *confirmStoreStub) GetActive() (Project, error)                { return Project{}, nil }
func (s *confirmStoreStub) SetActive(id int64) error {
	s.setActiveID = id
	s.setActiveHits++
	return nil
}
func (s *confirmStoreStub) CreateProject(in ProjectInput) (Project, error) {
	s.createInput = in
	s.createHits++
	return s.created, nil
}

func TestProjectEngineConfirm_SwitchesExistingProject(t *testing.T) {
	store := &confirmStoreStub{
		projects: []Project{{ID: 7, Name: "existing"}},
	}
	eng := newProjectEngine(store, func(string) (string, string) { return "", "" })

	if err := eng.Confirm("existing"); err != nil {
		t.Fatalf("confirm existing: %v", err)
	}
	if store.createHits != 0 {
		t.Fatalf("expected no project creation for existing candidate, got %d", store.createHits)
	}
	if store.setActiveHits != 1 || store.setActiveID != 7 {
		t.Fatalf("expected SetActive(7) once, got hits=%d id=%d", store.setActiveHits, store.setActiveID)
	}
}

func TestProjectEngineConfirm_CreatesAndSwitchesWhenMissing(t *testing.T) {
	store := &confirmStoreStub{
		projects: []Project{{ID: 1, Name: "default"}},
		created:  Project{ID: 11, Name: "new-project"},
	}
	eng := newProjectEngine(store, func(string) (string, string) { return "", "" })

	if err := eng.Confirm("new-project"); err != nil {
		t.Fatalf("confirm new: %v", err)
	}
	if store.createHits != 1 {
		t.Fatalf("expected project creation for missing candidate, got %d", store.createHits)
	}
	if store.createInput.Name != "new-project" {
		t.Fatalf("expected CreateProject name=new-project, got %q", store.createInput.Name)
	}
	if store.setActiveHits != 1 || store.setActiveID != 11 {
		t.Fatalf("expected SetActive(11) once after create, got hits=%d id=%d", store.setActiveHits, store.setActiveID)
	}
}
