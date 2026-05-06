package app

import (
	"os"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Phase 4, Task 4.1: Debounced backup scheduler
//
// The scheduler fires a backup action at most once per min-interval window.
// If a tick arrives while a backup completed within the window, it is skipped.
// ---------------------------------------------------------------------------

// tickScheduler wraps BackupScheduler's tick logic in a testable,
// clock-injectable form. Used in unit tests only.
type fakeBackupStore struct {
	mu    sync.Mutex
	calls []time.Time
}

func (f *fakeBackupStore) Backup(dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, time.Now())
	return nil
}

func (f *fakeBackupStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestDebounceScheduler_SkipsTickWithinWindow verifies that when a tick arrives
// within the min-interval window after a backup, it is skipped.
// RED: DebounceScheduler does not exist yet — build will fail.
func TestDebounceScheduler_SkipsTickWithinWindow(t *testing.T) {
	store := &fakeBackupStore{}
	now := time.Now()
	clock := &fakeClock{current: now}
	sched := NewDebounceScheduler(store.Backup, "/tmp/auto-backup.db", 10*time.Minute, clock.Now)

	// First tick: no prior backup — should run.
	sched.Tick()
	if store.callCount() != 1 {
		t.Fatalf("first tick must run backup, got %d calls", store.callCount())
	}

	// Second tick within window: must be skipped.
	clock.Advance(1 * time.Minute) // still within 10-min window
	sched.Tick()
	if store.callCount() != 1 {
		t.Fatalf("tick within window must be skipped, got %d calls", store.callCount())
	}
}

// TestDebounceScheduler_RunsAfterWindow verifies that a tick after the
// min-interval window fires a new backup.
// Triangulation: different window size and different advance.
func TestDebounceScheduler_RunsAfterWindow(t *testing.T) {
	store := &fakeBackupStore{}
	now := time.Now()
	clock := &fakeClock{current: now}
	sched := NewDebounceScheduler(store.Backup, "/tmp/auto-backup.db", 5*time.Minute, clock.Now)

	// First tick.
	sched.Tick()
	if store.callCount() != 1 {
		t.Fatalf("first tick must run, got %d calls", store.callCount())
	}

	// Advance past the window.
	clock.Advance(6 * time.Minute)
	sched.Tick()
	if store.callCount() != 2 {
		t.Fatalf("tick after window must run, got %d calls", store.callCount())
	}
}

// TestDebounceScheduler_FirstTickAlwaysRuns verifies that the very first tick
// always runs (no prior backup recorded).
func TestDebounceScheduler_FirstTickAlwaysRuns(t *testing.T) {
	store := &fakeBackupStore{}
	clock := &fakeClock{current: time.Now()}
	sched := NewDebounceScheduler(store.Backup, "/tmp/auto-backup.db", 60*time.Minute, clock.Now)

	sched.Tick()
	if store.callCount() != 1 {
		t.Fatalf("first tick must always run, got %d calls", store.callCount())
	}
}

// ---------------------------------------------------------------------------
// Phase 4, Task 4.3: Retention pruning
//
// Keep N daily + M weekly snapshots; prune the rest.
// ---------------------------------------------------------------------------

// TestRetentionPruner_KeepsNDailyAndMWeekly verifies the retention logic
// keeps exactly N daily + M weekly and prunes older files.
// RED: RetentionPruner does not exist yet.
func TestRetentionPruner_KeepsNDailyAndMWeekly(t *testing.T) {
	dir := t.TempDir()
	// Generate 10 daily snapshots (sequential days).
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var files []string
	for i := 0; i < 10; i++ {
		ts := base.AddDate(0, 0, i)
		name := dir + "/backup-" + ts.Format("2006-01-02T15-04-05") + ".db"
		files = append(files, name)
		// Create the file.
		if err := createEmptyFile(name); err != nil {
			t.Fatalf("create file: %v", err)
		}
	}

	// Keep 3 daily + 2 weekly.
	pruner := NewRetentionPruner(3, 2)
	if err := pruner.Prune(files); err != nil {
		t.Fatalf("prune: %v", err)
	}

	// Count remaining files.
	kept := countExistingFiles(files)
	// At most 3+2=5 files should remain (daily and weekly may overlap).
	if kept > 5 {
		t.Errorf("expected at most 5 files after pruning, got %d", kept)
	}
	// The newest file must always survive.
	newest := files[len(files)-1]
	if !fileExists(newest) {
		t.Errorf("newest snapshot must not be pruned, missing %q", newest)
	}
}

// TestRetentionPruner_DoesNotPruneWhenUnderLimit verifies no files are
// deleted when the total count is within the limit.
func TestRetentionPruner_DoesNotPruneWhenUnderLimit(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var files []string
	for i := 0; i < 3; i++ {
		ts := base.AddDate(0, 0, i)
		name := dir + "/backup-" + ts.Format("2006-01-02T15-04-05") + ".db"
		files = append(files, name)
		if err := createEmptyFile(name); err != nil {
			t.Fatalf("create file: %v", err)
		}
	}

	pruner := NewRetentionPruner(7, 4)
	if err := pruner.Prune(files); err != nil {
		t.Fatalf("prune: %v", err)
	}

	kept := countExistingFiles(files)
	if kept != 3 {
		t.Errorf("expected all 3 files kept (under limit), got %d", kept)
	}
}

// ---------------------------------------------------------------------------
// Phase 4, Task 4.4: Pre-restore safety snapshot
//
// Backup MUST be taken before restore; if backup fails, restore is aborted.
// ---------------------------------------------------------------------------

// TestPreRestoreSnapshot_AbortedOnBackupFailure verifies that when the
// pre-restore backup fails, restore is not attempted.
// RED: SafeRestore does not exist yet.
func TestPreRestoreSnapshot_AbortedOnBackupFailure(t *testing.T) {
	restoreCalled := false
	result := SafeRestore(
		func(dst string) error { return &mockError{"backup failed"} },
		func(src string) error { restoreCalled = true; return nil },
		"/tmp/safety.db",
		"/tmp/target.db",
	)
	if result == nil {
		t.Fatal("expected error when backup fails")
	}
	if restoreCalled {
		t.Fatal("restore must not be called when backup fails")
	}
}

// TestPreRestoreSnapshot_ProceedsWhenBackupSucceeds verifies normal flow.
func TestPreRestoreSnapshot_ProceedsWhenBackupSucceeds(t *testing.T) {
	restoreCalled := false
	result := SafeRestore(
		func(dst string) error { return nil },
		func(src string) error { restoreCalled = true; return nil },
		"/tmp/safety.db",
		"/tmp/target.db",
	)
	if result != nil {
		t.Fatalf("unexpected error: %v", result)
	}
	if !restoreCalled {
		t.Fatal("restore must be called when backup succeeds")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type fakeClock struct {
	mu      sync.Mutex
	current time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(d)
}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

func createEmptyFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func countExistingFiles(paths []string) int {
	n := 0
	for _, p := range paths {
		if fileExists(p) {
			n++
		}
	}
	return n
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
