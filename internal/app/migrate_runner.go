package app

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func RunMigrate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "overwrite target ~/.flint/flint.db if it exists")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "migrate failed: %v\n", err)
		return 1
	}

	srcDir := filepath.Join(home, ".nt-cli")
	dstDir := filepath.Join(home, ".flint")
	srcDB := filepath.Join(srcDir, "data.db")
	dstDB := filepath.Join(dstDir, "flint.db")

	if _, err := os.Stat(srcDB); err != nil {
		fmt.Fprintf(stderr, "migrate failed: source db not found at %s\n", srcDB)
		return 1
	}
	if _, err := os.Stat(dstDB); err == nil && !*force {
		fmt.Fprintf(stderr, "migrate failed: target %s already exists (use --force to overwrite)\n", dstDB)
		return 1
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "migrate failed: %v\n", err)
		return 1
	}
	if *force {
		_ = os.Remove(dstDB)
	}

	db, err := sql.Open("sqlite", srcDB)
	if err != nil {
		fmt.Fprintf(stderr, "migrate failed: %v\n", err)
		return 1
	}
	defer db.Close()

	if _, err := db.Exec("VACUUM INTO ?", dstDB); err != nil {
		fmt.Fprintf(stderr, "migrate failed: vacuum into: %v\n", err)
		return 1
	}

	if err := copyIfExists(filepath.Join(srcDir, "config.json"), filepath.Join(dstDir, "config.json"), *force); err != nil {
		fmt.Fprintf(stderr, "migrate failed: %v\n", err)
		return 1
	}
	if err := copyIfExists(filepath.Join(srcDir, "profile.json"), filepath.Join(dstDir, "profile.json"), *force); err != nil {
		fmt.Fprintf(stderr, "migrate failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "migration complete: ~/.nt-cli -> ~/.flint")
	return 0
}

func copyIfExists(src, dst string, force bool) error {
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(dst); err == nil && !force {
		return fmt.Errorf("target %s already exists (use --force)", dst)
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return err
	}
	return nil
}
