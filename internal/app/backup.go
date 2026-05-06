package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DebouncedBackup calls doBackup only if at least `window` time has elapsed
// since `lastBackup`. It is a pure function — callers manage state.
// This design keeps the function deterministic and trivially testable.
func DebouncedBackup(doBackup func() error, lastBackup time.Time, window time.Duration) error {
	if time.Since(lastBackup) < window {
		return nil
	}
	return doBackup()
}

// PruneBackups deletes old backup files in dir, keeping the most recent
// `keepDaily` daily files and the most recent `keepWeekly` weekly files.
// Files are matched by the pattern "auto-YYYYMMDD-*.db".
// Weekly de-dup: only the latest file per ISO week is counted toward the
// weekly budget; others are eligible for deletion once daily budget is met.
func PruneBackups(dir string, keepDaily, keepWeekly int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("prune backups: readdir %s: %w", dir, err)
	}

	type backupFile struct {
		name string
		ts   time.Time
	}

	var files []backupFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "auto-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		// Parse timestamp from "auto-YYYYMMDD-HHMMSS.db"
		inner := strings.TrimPrefix(name, "auto-")
		inner = strings.TrimSuffix(inner, ".db")
		// Accept "YYYYMMDD-HHMMSS" or "YYYYMMDD-NNNNNN" (any 6-digit suffix)
		ts, err := time.Parse("20060102-150405", inner)
		if err != nil {
			// Try partial: just date part
			if len(inner) >= 8 {
				ts, err = time.Parse("20060102", inner[:8])
			}
		}
		if err != nil {
			continue // ignore unrecognised format
		}
		files = append(files, backupFile{name: name, ts: ts})
	}

	if len(files) <= keepDaily {
		return nil // within budget — nothing to do
	}

	// Sort newest-first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].ts.After(files[j].ts)
	})

	keep := map[string]bool{}

	// Keep the most recent keepDaily files unconditionally.
	for i := 0; i < keepDaily && i < len(files); i++ {
		keep[files[i].name] = true
	}

	// Keep one file per ISO week (the newest in that week) up to keepWeekly.
	weekSeen := map[string]bool{}
	weekKept := 0
	for _, f := range files {
		year, week := f.ts.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", year, week)
		if !weekSeen[key] {
			weekSeen[key] = true
			if weekKept < keepWeekly {
				keep[f.name] = true
				weekKept++
			}
		}
	}

	// Delete everything not in keep set.
	for _, f := range files {
		if keep[f.name] {
			continue
		}
		path := filepath.Join(dir, f.name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("prune backups: remove %s: %w", path, err)
		}
	}
	return nil
}

// SafeRestore takes a pre-restore snapshot then runs the restore operation.
// If the snapshot fails, the restore is aborted and the snapshot error is
// returned. This is a pure orchestration function — implementations of
// snapshot and restore are injected by callers.
func SafeRestore(doSnapshot func() error, doRestore func() error) error {
	if err := doSnapshot(); err != nil {
		return fmt.Errorf("pre-restore snapshot failed, restore aborted: %w", err)
	}
	return doRestore()
}

// DefaultBackupDir returns the default directory for auto backups.
func DefaultBackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".nt-cli", "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	return dir, nil
}

// AutoBackupPath returns a timestamped path inside the default backup dir.
func AutoBackupPath() (string, error) {
	dir, err := DefaultBackupDir()
	if err != nil {
		return "", err
	}
	name := "auto-" + time.Now().UTC().Format("20060102-150405") + ".db"
	return filepath.Join(dir, name), nil
}
