package store

import (
	"testing"
	"time"

	"nt-cli/internal/app"
)

// TestSaveWithMeta_ProjectScopedUpsertIsolation proves the project-scoping
// upsert contract: same (topic_key, scope) in different projects MUST NOT
// collide, while repeated saves in the same project still update in place.
func TestSaveWithMeta_ProjectScopedUpsertIsolation(t *testing.T) {
	s, _ := newMetaStore(t)

	active, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	projB, err := s.CreateProject(app.ProjectInput{Name: "proj-b", RootPath: "/tmp/proj-b"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	t1 := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	t3 := t2.Add(time.Minute)

	idA, err := s.SaveWithMeta(app.SaveRequest{
		Content:   "default project body",
		TopicKey:  "ops/runbook",
		Scope:     "ad373971",
		ProjectID: active.ID,
		CreatedAt: t1,
	})
	if err != nil {
		t.Fatalf("save A: %v", err)
	}
	idB1, err := s.SaveWithMeta(app.SaveRequest{
		Content:   "project b body",
		TopicKey:  "ops/runbook",
		Scope:     "ad373971",
		ProjectID: projB.ID,
		CreatedAt: t2,
	})
	if err != nil {
		t.Fatalf("save B1: %v", err)
	}
	if idA == idB1 {
		t.Fatalf("cross-project save collided: idA=%d idB1=%d", idA, idB1)
	}

	idB2, err := s.SaveWithMeta(app.SaveRequest{
		Content:   "project b updated",
		TopicKey:  "ops/runbook",
		Scope:     "ad373971",
		ProjectID: projB.ID,
		CreatedAt: t3,
	})
	if err != nil {
		t.Fatalf("save B2: %v", err)
	}
	if idB2 != idB1 {
		t.Fatalf("same-project upsert must reuse id: idB1=%d idB2=%d", idB1, idB2)
	}

	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_items
		 WHERE topic_key = ? AND COALESCE(scope, '') = ?`,
		"ops/runbook", "ad373971",
	).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows (one per project), got %d", count)
	}
}
