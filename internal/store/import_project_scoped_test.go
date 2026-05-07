package store

import (
	"testing"

	"nt-cli/internal/app"
)

// TestImportStore_DedupeIsolatedAcrossProjects verifies dedupe isolation by
// project_id: same topic_key + content_hash imported in two different active
// projects must insert once per project.
func TestImportStore_DedupeIsolatedAcrossProjects(t *testing.T) {
	s := newTestStore(t)

	proj2, err := s.CreateProject(app.ProjectInput{Name: "import-proj2", RootPath: "/import/p2", Fingerprint: "fp-import-p2"})
	if err != nil {
		t.Fatalf("create proj2: %v", err)
	}

	rows := []app.ImportRecord{{Content: "same body", TopicKey: "arch/import"}}

	if err := s.SetActive(1); err != nil {
		t.Fatalf("set active default: %v", err)
	}
	r1, err := s.ImportRecords(rows)
	if err != nil {
		t.Fatalf("import default: %v", err)
	}
	if r1.Inserted != 1 || r1.Skipped != 0 {
		t.Fatalf("unexpected default import result: %+v", r1)
	}

	if err := s.SetActive(proj2.ID); err != nil {
		t.Fatalf("set active proj2: %v", err)
	}
	r2, err := s.ImportRecords(rows)
	if err != nil {
		t.Fatalf("import proj2: %v", err)
	}
	if r2.Inserted != 1 || r2.Skipped != 0 {
		t.Fatalf("expected insert in second project (no cross-project dedupe), got %+v", r2)
	}
}
