package mcp

import (
	"strings"
	"testing"

	"flint/internal/app"
)

// TestMCP_LocalList_ScopedByActiveProject verifies local_list follows active
// project scoping by default.
func TestMCP_LocalList_ScopedByActiveProject(t *testing.T) {
	f := newProjectMCPFixture(t)

	proj2, err := f.sqlRepo.CreateProject(app.ProjectInput{Name: "list-scope-p2", RootPath: "/scope/list/p2"})
	if err != nil {
		t.Fatalf("create proj2: %v", err)
	}

	f.svc.SetActiveProject(1)
	_, _ = callTool(t, f.svc, "local_save", map[string]interface{}{"content": "list-only-default"})

	f.svc.SetActiveProject(proj2.ID)
	result, rpcErr := callTool(t, f.svc, "local_list", map[string]interface{}{"limit": 20})
	if rpcErr != nil {
		t.Fatalf("local_list rpc error: %+v", rpcErr)
	}
	text := toolResultText(t, result)
	if strings.Contains(text, "list-only-default") {
		t.Fatalf("local_list must be scoped by active project, got %q", text)
	}
}

// TestMCP_LocalList_AllProjectsBypass verifies all_projects=true bypasses
// project scoping and returns rows from other projects.
func TestMCP_LocalList_AllProjectsBypass(t *testing.T) {
	f := newProjectMCPFixture(t)

	proj2, err := f.sqlRepo.CreateProject(app.ProjectInput{Name: "list-bypass-p2", RootPath: "/bypass/list/p2"})
	if err != nil {
		t.Fatalf("create proj2: %v", err)
	}

	f.svc.SetActiveProject(1)
	_, _ = callTool(t, f.svc, "local_save", map[string]interface{}{"content": "list-bypass-default"})

	f.svc.SetActiveProject(proj2.ID)
	result, rpcErr := callTool(t, f.svc, "local_list", map[string]interface{}{
		"limit":        20,
		"all_projects": true,
	})
	if rpcErr != nil {
		t.Fatalf("local_list rpc error: %+v", rpcErr)
	}
	text := toolResultText(t, result)
	if !strings.Contains(text, "list-bypass-default") {
		t.Fatalf("local_list all_projects=true must include cross-project rows, got %q", text)
	}
}
