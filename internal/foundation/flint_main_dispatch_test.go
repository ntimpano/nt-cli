package foundation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlintMainDispatchIncludesMigrate(t *testing.T) {
	mainPath := filepath.Join(repoRoot(t), "cmd", "flint", "main.go")
	body, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}
	want := `if cmd == "migrate" {
		os.Exit(app.RunMigrate(os.Args[2:], os.Stdout, os.Stderr))
	}`
	if !strings.Contains(string(body), want) {
		t.Fatalf("migrate dispatch missing in cmd/flint/main.go")
	}
}
