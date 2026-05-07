package mcp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"nt-cli/internal/app"
)

func TestMCP_ProjectSwitch_PreSwitchBackup_UsesUniqueNames(t *testing.T) {
	f := newProjectMCPFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	backupDir := filepath.Join(home, ".nt-cli", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}

	p2, err := f.sqlRepo.CreateProject(app.ProjectInput{Name: "p2", RootPath: "/tmp/p2"})
	if err != nil {
		t.Fatalf("create project p2: %v", err)
	}

	for _, id := range []int64{p2.ID, 1} {
		result, rpcErr := callTool(t, f.svc, "project_switch", map[string]interface{}{"id": id})
		if rpcErr != nil {
			t.Fatalf("switch rpc error: %+v", rpcErr)
		}
		if result["isError"] == true {
			t.Fatalf("project_switch returned tool error: %v", toolResultText(t, result))
		}
	}

	files, err := filepath.Glob(filepath.Join(backupDir, "pre-switch-*.db"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 unique pre-switch backups, got %d (%v)", len(files), files)
	}
}

func TestMCP_ProjectSwitch_PreSwitchBackup_ErrorIsSurfaced(t *testing.T) {
	f := newProjectMCPFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".nt-cli"), []byte("block"), 0o644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	result, rpcErr := callTool(t, f.svc, "project_switch", map[string]interface{}{"id": 1})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if result["isError"] != true {
		t.Fatalf("expected tool error when backup fails, got %+v", result)
	}
	msg := strings.ToLower(toolResultText(t, result))
	if !strings.Contains(msg, "pre-switch backup") {
		t.Fatalf("expected surfaced backup failure message, got %q", msg)
	}
}

func TestMCP_ProjectSwitch_PreSwitchBackup_KeepLastFivePerProject(t *testing.T) {
	f := newProjectMCPFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	backupDir := filepath.Join(home, ".nt-cli", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}

	targetProjectID := int64(1)
	otherProjectID := int64(99)
	for i := 0; i < 6; i++ {
		p := filepath.Join(backupDir, "pre-switch-"+strconv.FormatInt(targetProjectID, 10)+"-legacy-"+strconv.Itoa(i)+".db")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed backup %d: %v", i, err)
		}
	}
	otherPath := filepath.Join(backupDir, "pre-switch-"+strconv.FormatInt(otherProjectID, 10)+"-keep.db")
	if err := os.WriteFile(otherPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed other project backup: %v", err)
	}

	result, rpcErr := callTool(t, f.svc, "project_switch", map[string]interface{}{"id": targetProjectID})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %s", toolResultText(t, result))
	}

	kept, err := filepath.Glob(filepath.Join(backupDir, "pre-switch-1-*.db"))
	if err != nil {
		t.Fatalf("glob kept backups: %v", err)
	}
	if len(kept) != 5 {
		t.Fatalf("retention mismatch for target project: got %d want 5 (%v)", len(kept), kept)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("other project backup should not be deleted: %v", err)
	}
}
