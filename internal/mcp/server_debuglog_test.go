package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDebugLogPath_DefaultDisabled(t *testing.T) {
	t.Setenv("NTCLI_MCP_DEBUG", "")
	if got := resolveDebugLogPath(); got != "" {
		t.Fatalf("debug log must be disabled by default, got path %q", got)
	}
}

func TestResolveDebugLogPath_OptInEnabledUsesHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NTCLI_MCP_DEBUG", "1")

	want := filepath.Join(home, ".nt-cli", "logs", "mcp.log")
	if got := resolveDebugLogPath(); got != want {
		t.Fatalf("unexpected debug log path: got %q want %q", got, want)
	}
}

func TestDebugLog_EnabledCreates0600File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.log")
	debugLog(path, "hello")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected debug log file to exist: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("unexpected permissions: got %o want %o", got, want)
	}
}
