package foundation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadmeAndGitignoreReferenceFlint(t *testing.T) {
	root := repoRoot(t)
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	for _, needle := range []string{"# flint", "~/.flint/flint.db", "flint init", "flint mcp"} {
		if !strings.Contains(string(readme), needle) {
			t.Fatalf("README missing %q", needle)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), "/flint") {
		t.Fatalf(".gitignore must ignore /flint binary")
	}
}
