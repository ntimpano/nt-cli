package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"nt-cli/internal/app"
	"nt-cli/internal/store"
)

// ---------------------------------------------------------------------------
// Test store helpers: projectMemStoreMCP layers ProjectStore capability on
// top of the existing memStore so Service auto-wires a ProjectEngine.
// ---------------------------------------------------------------------------

// projectMemStoreMCP is an in-memory ProjectStore for MCP project tool tests.
// It satisfies app.Store AND app.ProjectStore (via store.ProjectStore interface)
// by embedding a real SQLite store under a temp path.  We use an in-process
// SQLite so migration and project-engine wiring happen exactly as in production
// without any network I/O.
type projectMCPFixture struct {
	svc     *app.Service
	sqlRepo *store.SQLiteStore
}

func newProjectMCPFixture(t *testing.T) *projectMCPFixture {
	t.Helper()
	dir := t.TempDir()
	dbPath := dir + "/test.db"
	repo, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	svc := app.NewService(repo)
	if err := svc.Init(); err != nil {
		t.Fatalf("svc init: %v", err)
	}
	return &projectMCPFixture{svc: svc, sqlRepo: repo}
}

// ---------------------------------------------------------------------------
// Task 3.1 — project_probe (read-only)
// ---------------------------------------------------------------------------

// TestMCP_ProjectProbe_Returns_Status verifies project_probe returns a
// result with status="none" when called from a non-git directory.
// RED: tool not registered yet → expect "tool not found" error.
func TestMCP_ProjectProbe_ReturnsStatus(t *testing.T) {
	f := newProjectMCPFixture(t)
	// Passing an empty cwd so git resolution returns "none"
	result, rpcErr := callTool(t, f.svc, "project_probe", map[string]interface{}{
		"cwd": "/tmp/not-a-git-dir-zzzz",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	// The tool must return a result (isError=false) with a "status" field.
	if result["isError"] == true {
		t.Fatalf("unexpected tool error result: %+v", result)
	}
	text := toolResultText(t, result)
	if !strings.Contains(text, "status") {
		t.Fatalf("expected 'status' in response, got %q", text)
	}
}

// TestMCP_ProjectProbe_DoesNotMutate verifies calling project_probe
// twice with the same cwd does not change active project (task 3.6 — probe
// is ambiguous → no mutation; only confirm commits).
func TestMCP_ProjectProbe_DoesNotMutate(t *testing.T) {
	f := newProjectMCPFixture(t)

	// Get initial active project id.
	beforeResult, _ := callTool(t, f.svc, "project_current", nil)
	beforeText := toolResultText(t, beforeResult)

	// Probe twice — must not change active project.
	callTool(t, f.svc, "project_probe", map[string]interface{}{"cwd": "/tmp/z"})
	callTool(t, f.svc, "project_probe", map[string]interface{}{"cwd": "/tmp/z"})

	afterResult, _ := callTool(t, f.svc, "project_current", nil)
	afterText := toolResultText(t, afterResult)

	if beforeText != afterText {
		t.Fatalf("project_probe must not mutate active project; before=%q after=%q",
			beforeText, afterText)
	}
}

// ---------------------------------------------------------------------------
// Task 3.2 — project_confirm, project_current, project_list, project_switch
// ---------------------------------------------------------------------------

// TestMCP_ProjectCurrent_ReturnsDefaultProject verifies project_current
// returns the default project created by v5 migration.
func TestMCP_ProjectCurrent_ReturnsDefaultProject(t *testing.T) {
	f := newProjectMCPFixture(t)
	result, rpcErr := callTool(t, f.svc, "project_current", nil)
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %+v", result)
	}
	text := toolResultText(t, result)
	// v5 migration always creates a "default" project.
	if !strings.Contains(strings.ToLower(text), "default") {
		t.Fatalf("expected default project, got %q", text)
	}
}

// TestMCP_ProjectList_ContainsDefault verifies project_list returns at
// least the default project.
func TestMCP_ProjectList_ContainsDefault(t *testing.T) {
	f := newProjectMCPFixture(t)
	result, rpcErr := callTool(t, f.svc, "project_list", nil)
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %+v", result)
	}
	text := toolResultText(t, result)
	if !strings.Contains(strings.ToLower(text), "default") {
		t.Fatalf("expected default project in list, got %q", text)
	}
}

// TestMCP_ProjectSwitch_UpdatesActive implements task 3.7: sidebar flow
// project_list → project_switch(id) updates active and returns new record.
func TestMCP_ProjectSwitch_UpdatesActive(t *testing.T) {
	f := newProjectMCPFixture(t)

	// Get current list to find default project id.
	listResult, _ := callTool(t, f.svc, "project_list", nil)
	listText := toolResultText(t, listResult)

	// The list should have at least one entry with id=1.
	if !strings.Contains(listText, "1") {
		t.Fatalf("expected project with id=1 in list, got %q", listText)
	}

	// Switch to project id=1 (default). Must succeed (no error).
	switchResult, rpcErr := callTool(t, f.svc, "project_switch", map[string]interface{}{
		"id": 1,
	})
	if rpcErr != nil {
		t.Fatalf("rpc error on switch: %+v", rpcErr)
	}
	if switchResult["isError"] == true {
		t.Fatalf("unexpected tool error on switch: %+v", switchResult)
	}
	switchText := toolResultText(t, switchResult)

	// Response must contain the updated project record.
	if !strings.Contains(strings.ToLower(switchText), "default") {
		t.Fatalf("switch response must include new active project, got %q", switchText)
	}
}

// TestMCP_ProjectConfirm_SetsActive verifies project_confirm with a known
// candidate name sets it as the active project.
func TestMCP_ProjectConfirm_SetsActive(t *testing.T) {
	f := newProjectMCPFixture(t)

	// Confirm "default" — it already exists.
	result, rpcErr := callTool(t, f.svc, "project_confirm", map[string]interface{}{
		"candidate": "default",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %+v", result)
	}
	// Active project must still be "default".
	cur, _ := callTool(t, f.svc, "project_current", nil)
	if !strings.Contains(strings.ToLower(toolResultText(t, cur)), "default") {
		t.Fatalf("expected default to remain active after confirm")
	}
}

// TestMCP_ProjectConfirm_UnknownReturnsError verifies project_confirm
// with a non-existent candidate returns a tool error (not a crash).
func TestMCP_ProjectConfirm_UnknownReturnsError(t *testing.T) {
	f := newProjectMCPFixture(t)
	result, rpcErr := callTool(t, f.svc, "project_confirm", map[string]interface{}{
		"candidate": "no-such-project",
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result["isError"] != true {
		t.Fatalf("expected tool error for unknown candidate, got %+v", result)
	}
}

// ---------------------------------------------------------------------------
// Task 3.3 — active-project resource
// ---------------------------------------------------------------------------

// TestMCP_ResourcesList_IncludesActiveProject verifies that resources/list
// advertises the active_project resource when project feature is available.
func TestMCP_ResourcesList_IncludesActiveProject(t *testing.T) {
	f := newProjectMCPFixture(t)

	if f.svc.ProjectEng == nil {
		t.Fatal("ProjectEng must not be nil — SQLiteStore must satisfy ProjectStore")
	}

	reqJSON, _ := json.Marshal(request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/list",
	})
	resp, ok := handleRequest(reqJSON, f.svc)
	if !ok {
		t.Fatal("expected response for resources/list")
	}
	m, _ := resp.Result.(map[string]interface{})
	// resources may be []map[string]interface{} or []interface{} depending on how
	// the server serialised it. Handle both.
	var resources []interface{}
	switch rv := m["resources"].(type) {
	case []interface{}:
		resources = rv
	case []map[string]interface{}:
		for _, r := range rv {
			resources = append(resources, r)
		}
	}
	if len(resources) == 0 {
		t.Fatal("expected at least one resource (active_project), got empty list")
	}
	found := false
	for _, r := range resources {
		rm, _ := r.(map[string]interface{})
		if rm["uri"] == "nt-cli://project/active" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected active_project resource with uri=nt-cli://project/active, got %+v", resources)
	}
}

// ---------------------------------------------------------------------------
// Task 3.4 — legacy tools (save/recall) operate transparently
// ---------------------------------------------------------------------------

// TestMCP_LegacyTools_TransparentWithActiveProject verifies that local_save
// and local_recall still work when a project is active (backward compat).
func TestMCP_LegacyTools_TransparentWithActiveProject(t *testing.T) {
	f := newProjectMCPFixture(t)

	// Save a note.
	saveResult, rpcErr := callTool(t, f.svc, "local_save", map[string]interface{}{
		"content": "hello project context",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error on save: %+v", rpcErr)
	}
	if saveResult["isError"] == true {
		t.Fatalf("save error: %+v", saveResult)
	}

	// Recall the note.
	recallResult, rpcErr := callTool(t, f.svc, "local_recall", map[string]interface{}{
		"query": "hello project context",
		"limit": 5,
	})
	if rpcErr != nil {
		t.Fatalf("rpc error on recall: %+v", rpcErr)
	}
	if recallResult["isError"] == true {
		t.Fatalf("recall error: %+v", recallResult)
	}
	recallText := toolResultText(t, recallResult)
	if !strings.Contains(recallText, "hello project context") {
		t.Fatalf("recall must return saved note, got %q", recallText)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func toolResultText(t *testing.T, result map[string]interface{}) string {
	t.Helper()
	content, _ := result["content"].([]interface{})
	if len(content) == 0 {
		// try typed slice
		if cs, ok := result["content"].([]map[string]string); ok {
			if len(cs) > 0 {
				return cs[0]["text"]
			}
		}
		return ""
	}
	m, _ := content[0].(map[string]interface{})
	text, _ := m["text"].(string)
	return text
}
