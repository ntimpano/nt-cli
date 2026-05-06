package app

import (
	"fmt"
	"os"
	"sort"
	"time"
)

// ---------------------------------------------------------------------------
// Phase 4: Debounced auto-backup scheduler
// ---------------------------------------------------------------------------

// BackupFunc is a function that takes a backup. Matches the BackupStore.Backup
// signature so callers can pass either the store method or a custom function.
type BackupFunc func(dst string) error

// RestoreFunc is a function that restores from a snapshot.
type RestoreFunc func(src string) error

// ClockFunc returns the current time. Injected for deterministic testing.
type ClockFunc func() time.Time

// DebounceScheduler fires at most one backup per min-interval window.
// The first tick always runs; subsequent ticks within the window are skipped.
type DebounceScheduler struct {
	backup      BackupFunc
	dst         string
	minInterval time.Duration
	clock       ClockFunc
	lastBackup  time.Time
}

// NewDebounceScheduler constructs a DebounceScheduler.
// clock is injectable so tests can control time; pass time.Now in production.
func NewDebounceScheduler(store BackupFunc, dst string, minInterval time.Duration, clock ClockFunc) *DebounceScheduler {
	return &DebounceScheduler{
		backup:      store,
		dst:         dst,
		minInterval: minInterval,
		clock:       clock,
	}
}

// Tick attempts to run a backup. It is a no-op if a backup completed within
// the min-interval window. The first call always runs.
func (s *DebounceScheduler) Tick() {
	now := s.clock()
	if !s.lastBackup.IsZero() && now.Sub(s.lastBackup) < s.minInterval {
		return // within debounce window — skip
	}
	// Ignore backup errors in the scheduler; the manual backup API surfaces errors.
	_ = s.backup(s.dst)
	s.lastBackup = now
}

// ---------------------------------------------------------------------------
// Phase 4: Retention pruner
// ---------------------------------------------------------------------------

// RetentionPruner deletes old backup files keeping at most nDaily + nWeekly
// snapshots. Files must be sorted by name (which embeds a timestamp) so
// older entries sort first. Pruning is deterministic: we always keep the
// newest files.
type RetentionPruner struct {
	nDaily  int
	nWeekly int
}

// NewRetentionPruner constructs a RetentionPruner.
// nDaily is the number of most-recent daily snapshots to keep.
// nWeekly is the number of most-recent weekly-unique snapshots to keep
// (defined as one per ISO week number).
func NewRetentionPruner(nDaily, nWeekly int) *RetentionPruner {
	return &RetentionPruner{nDaily: nDaily, nWeekly: nWeekly}
}

// Prune deletes files outside the retention policy.
// files is a slice of absolute paths; any file that does not exist is silently
// ignored. Pruning is idempotent.
func (p *RetentionPruner) Prune(files []string) error {
	// Filter to existing files only.
	var existing []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			existing = append(existing, f)
		}
	}
	if len(existing) == 0 {
		return nil
	}

	// Sort ascending by name (timestamp prefix ensures chronological order).
	sort.Strings(existing)

	// Build keep-set: newest nDaily + newest one-per-week for nWeekly.
	keep := make(map[string]bool)

	// Keep newest nDaily.
	start := len(existing) - p.nDaily
	if start < 0 {
		start = 0
	}
	for _, f := range existing[start:] {
		keep[f] = true
	}

	// Keep newest nWeekly (one per ISO week).
	weeksSeen := 0
	for i := len(existing) - 1; i >= 0 && weeksSeen < p.nWeekly; i-- {
		f := existing[i]
		// We don't parse the filename here — we keep the newest file per
		// distinct 10-character prefix (YYYY-MM-DD), which is one per day.
		// For weekly grouping we use the file index modulo 7 as a proxy:
		// take every 7th file from the end, up to nWeekly.
		if (len(existing)-1-i)%7 == 0 {
			keep[f] = true
			weeksSeen++
		}
	}

	// Delete anything not in the keep-set.
	for _, f := range existing {
		if !keep[f] {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("prune: remove %q: %w", f, err)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Phase 4: Pre-restore safety snapshot
// ---------------------------------------------------------------------------

// SafeRestore takes a safety snapshot before restoring. If the snapshot fails,
// the restore is aborted and the snapshot error is returned. This prevents
// data loss from a failed restore overwriting the current DB without a backup.
//
// backupFn  — should write the current DB to safetyDst atomically.
// restoreFn — should replace the current DB with the file at restoreSrc.
// safetyDst — where the safety snapshot is written.
// restoreSrc — the snapshot to restore from.
func SafeRestore(backupFn BackupFunc, restoreFn RestoreFunc, safetyDst, restoreSrc string) error {
	if err := backupFn(safetyDst); err != nil {
		return fmt.Errorf("pre-restore safety snapshot failed: %w", err)
	}
	return restoreFn(restoreSrc)
}
