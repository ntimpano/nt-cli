package mcp

import (
	"testing"

	"flint/internal/app"
)

// TestMCP_ProjectConfirm_SyncsActiveProjectID verifies project_confirm
// updates Service.activeProjectID so subsequent local_save writes are
// scoped to the newly confirmed project.
func TestMCP_ProjectConfirm_SyncsActiveProjectID(t *testing.T) {
	f := newProjectMCPFixture(t)

	active, err := f.sqlRepo.GetActive()
	if err != nil {
		t.Fatalf("get active project: %v", err)
	}
	f.svc.SetActiveProject(active.ID)

	proj2, err := f.sqlRepo.CreateProject(app.ProjectInput{Name: "confirm-sync-proj", RootPath: "/tmp/confirm-sync"})
	if err != nil {
		t.Fatalf("create proj2: %v", err)
	}

	result, rpcErr := callTool(t, f.svc, "project_confirm", map[string]interface{}{
		"candidate": proj2.Name,
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %+v", result)
	}

	noteContent := "project-confirm-sync-note"
	_, rpcErr = callTool(t, f.svc, "local_save", map[string]interface{}{
		"content":   noteContent,
		"topic_key": "sdd/project-confirm/sync",
		"scope":     "ad373971",
	})
	if rpcErr != nil {
		t.Fatalf("save error: %+v", rpcErr)
	}

	itemsNew, err := f.sqlRepo.ListFiltered(app.ListOptions{Limit: 10, ProjectID: proj2.ID})
	if err != nil {
		t.Fatalf("list new project: %v", err)
	}
	foundNew := false
	for _, it := range itemsNew {
		if it.Content == noteContent {
			foundNew = true
			break
		}
	}

	itemsOld, err := f.sqlRepo.ListFiltered(app.ListOptions{Limit: 10, ProjectID: active.ID})
	if err != nil {
		t.Fatalf("list old project: %v", err)
	}
	foundOld := false
	for _, it := range itemsOld {
		if it.Content == noteContent {
			foundOld = true
			break
		}
	}

	if !foundNew {
		t.Fatalf("expected note saved under new project id=%d", proj2.ID)
	}
	if foundOld {
		t.Fatalf("note must not be saved under old project id=%d", active.ID)
	}
}
