package foundation_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func TestGoModuleRenamedToFlint(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !bytes.HasPrefix(body, []byte("module flint\n")) {
		t.Fatalf("expected go.mod module flint, got:\n%s", string(body))
	}
}

func TestFlintEntrypointExists(t *testing.T) {
	newPath := filepath.Join(repoRoot(t), "cmd", "flint", "main.go")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected flint entrypoint at %s: %v", newPath, err)
	}
}

func TestLegacyNTCLIEntrypointRemoved(t *testing.T) {
	oldPath := filepath.Join(repoRoot(t), "cmd", "nt-cli", "main.go")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy entrypoint removed at %s", oldPath)
	}
}

func TestNoStaleNTCLIImportPathsInInternal(t *testing.T) {
	root := filepath.Join(repoRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if filepath.Base(path) == "flint_foundation_test.go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		stale := "\"nt-cli" + "/internal/"
		if strings.Contains(string(body), stale) {
			t.Fatalf("stale internal import found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}
