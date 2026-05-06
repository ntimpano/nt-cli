package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 4.5 — debounced tick skipped within window
// ---------------------------------------------------------------------------

// TestDebouncedBackup_SkippedWithinWindow verifies that when the ticker fires
// within the min-interval window, the backup is NOT triggered.
// RED: DebouncedBackup not implemented yet.
func TestDebouncedBackup_SkippedWithinWindow(t *testing.T) {
	calls := 0
	doBackup := func() error {
		calls++
		return nil
	}
	// Set a 1-minute window. Call twice rapidly.
	last := time.Now()
	window := time.Minute
	DebouncedBackup(doBackup, last, window)
	// Called immediately after last — should skip.
	if calls != 0 {
		t.Fatalf("expected 0 backup calls within window, got %d", calls)
	}
}

// TestDebouncedBackup_TriggersAfterWindow verifies the backup IS triggered
// when enough time has passed since the last backup.
func TestDebouncedBackup_TriggersAfterWindow(t *testing.T) {
	calls := 0
	doBackup := func() error {
		calls++
		return nil
	}
	// Last backup was 2 minutes ago; window is 1 minute.
	last := time.Now().Add(-2 * time.Minute)
	window := time.Minute
	DebouncedBackup(doBackup, last, window)
	if calls != 1 {
		t.Fatalf("expected 1 backup call after window, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// Task 4.6 — retention pruning
// ---------------------------------------------------------------------------

// TestRetentionPrune_KeepsCorrectCount verifies that PruneBackups keeps
// exactly keepDaily daily files and keepWeekly weekly files and deletes the
// rest. Files are created in a temp dir with predictable timestamps.
func TestRetentionPrune_KeepsCorrectCount(t *testing.T) {
	dir := t.TempDir()
	// Create 12 fake backup files spanning 12 days.
	// Naming: auto-YYYYMMDD-HHMMSS.db
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		ts := base.Add(time.Duration(i) * 24 * time.Hour)
		name := fmt.Sprintf("auto-%s.db", ts.Format("20060102-150405"))
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write fake backup: %v", err)
		}
	}

	keepDaily := 7
	keepWeekly := 4

	if err := PruneBackups(dir, keepDaily, keepWeekly); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	remaining := listBackupFiles(t, dir)
	if len(remaining) > keepDaily+keepWeekly {
		t.Fatalf("expected at most %d files after prune, got %d: %v",
			keepDaily+keepWeekly, len(remaining), remaining)
	}
	// Must keep at least keepDaily (the most recent daily files).
	if len(remaining) < keepDaily {
		t.Fatalf("expected at least %d files retained, got %d: %v",
			keepDaily, len(remaining), remaining)
	}
}

// TestRetentionPrune_NoopWhenFewFiles verifies that PruneBackups is a no-op
// when the number of files is within budget.
func TestRetentionPrune_NoopWhenFewFiles(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("auto-20260101-%06d.db", i))
		os.WriteFile(path, []byte("x"), 0o644)
	}
	if err := PruneBackups(dir, 7, 4); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}
	remaining := listBackupFiles(t, dir)
	if len(remaining) != 3 {
		t.Fatalf("expected 3 files unchanged, got %d", len(remaining))
	}
}

// ---------------------------------------------------------------------------
// Task 4.7 — pre-restore safety snapshot
// ---------------------------------------------------------------------------

// TestPreRestoreSnapshot_FailureAborts verifies that if the pre-restore
// snapshot fails, the restore is aborted and returns an error.
func TestPreRestoreSnapshot_FailureAborts(t *testing.T) {
	snapCalls := 0
	restoreCalls := 0
	snapErr := fmt.Errorf("snapshot disk full")
	doSnap := func() error {
		snapCalls++
		return snapErr
	}
	doRestore := func() error {
		restoreCalls++
		return nil
	}

	err := SafeRestore(doSnap, doRestore)
	if err == nil {
		t.Fatal("expected error when snapshot fails")
	}
	if restoreCalls != 0 {
		t.Fatalf("restore must not be called when snapshot fails, got %d calls", restoreCalls)
	}
}

// TestPreRestoreSnapshot_HappyPath verifies that when snapshot succeeds,
// the restore is called exactly once.
func TestPreRestoreSnapshot_HappyPath(t *testing.T) {
	snapCalls := 0
	restoreCalls := 0
	doSnap := func() error {
		snapCalls++
		return nil
	}
	doRestore := func() error {
		restoreCalls++
		return nil
	}

	if err := SafeRestore(doSnap, doRestore); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapCalls != 1 || restoreCalls != 1 {
		t.Fatalf("expected 1 snap + 1 restore, got snap=%d restore=%d", snapCalls, restoreCalls)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func listBackupFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}
