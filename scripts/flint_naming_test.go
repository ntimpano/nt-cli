package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseAndScriptsUseFlintNaming(t *testing.T) {
	root := filepath.Join("..")
	checks := []struct {
		path     string
		contains []string
	}{
		{path: filepath.Join(root, "scripts", "install.sh"), contains: []string{"flint", "flint_"}},
		{path: filepath.Join(root, "scripts", "build-release.sh"), contains: []string{"flint_", "./cmd/flint", "binary_name=\"flint\""}},
		{path: filepath.Join(root, "scripts", "opencode-mcp-dev.sh"), contains: []string{"cmd/flint", "flint MCP"}},
		{path: filepath.Join(root, ".github", "workflows", "release.yml"), contains: []string{"flint_*.tar.gz"}},
	}

	for _, tc := range checks {
		body, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		for _, needle := range tc.contains {
			if !strings.Contains(string(body), needle) {
				t.Fatalf("expected %s to contain %q", tc.path, needle)
			}
		}
	}
}
