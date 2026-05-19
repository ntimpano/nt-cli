package main

import (
	"fmt"
	"os"

	"flint/internal/app"
	"flint/internal/mcp"
	"flint/internal/store"
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

	if cmd == "init" {
		os.Exit(app.RunInitOrProfile(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
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

	// Resolve active project at boot and inject into service so all
	// read/write paths are automatically scoped (tasks 2.4–2.6).
	if activeProj, err := repo.GetActive(); err == nil {
		svc.SetActiveProject(activeProj.ID)
	}

	os.Exit(app.RunCLIWithStdin(svc, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
