package app

import (
	"path/filepath"
	"testing"
)

func TestFlintPaths_RuntimeConfigProfileAndDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	runtimePath, err := RuntimeConfigPath()
	if err != nil {
		t.Fatalf("RuntimeConfigPath: %v", err)
	}
	if got, want := runtimePath, filepath.Join(home, ".flint", "config.json"); got != want {
		t.Fatalf("runtime path mismatch: got %q want %q", got, want)
	}

	profilePath, err := ProfilePath()
	if err != nil {
		t.Fatalf("ProfilePath: %v", err)
	}
	if got, want := profilePath, filepath.Join(home, ".flint", "profile.json"); got != want {
		t.Fatalf("profile path mismatch: got %q want %q", got, want)
	}

	dbPath, err := DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath: %v", err)
	}
	if got, want := dbPath, filepath.Join(home, ".flint", "flint.db"); got != want {
		t.Fatalf("db path mismatch: got %q want %q", got, want)
	}
}
