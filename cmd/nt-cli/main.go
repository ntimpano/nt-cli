package main

import (
	"fmt"
	"os"

	"nt-cli/internal/app"
	"nt-cli/internal/mcp"
	"nt-cli/internal/store"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		os.Exit(app.RunCLI(nil, nil, os.Stdout, os.Stderr))
	}

	cmd := os.Args[1]

	if cmd == "mcp" {
		srv := mcp.NewServer(os.Stdin, os.Stdout)
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "mcp error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	dbPath, err := app.DefaultDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	repo, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		os.Exit(1)
	}
	defer repo.Close()

	svc := app.NewService(repo)

	// init prints the canonical path; everything else flows through RunCLI.
	if cmd == "init" {
		if err := svc.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("initialized at %s\n", dbPath)
		if containsFlag(os.Args[2:], "--legacy") {
			if code := app.RunInitProfile(os.Args[2:], os.Stdin, os.Stdout, os.Stderr); code != 0 {
				os.Exit(code)
			}
			return
		}
		if err := app.RunInit(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Resolve active project at boot and inject into service so all
	// read/write paths are automatically scoped (tasks 2.4–2.6).
	if activeProj, err := repo.GetActive(); err == nil {
		svc.SetActiveProject(activeProj.ID)
	}

	os.Exit(app.RunCLIWithStdin(svc, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func containsFlag(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}
