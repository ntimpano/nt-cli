package main

import (
	"fmt"
	"os"

	"nt-cli/internal/app"
	"nt-cli/internal/mcp"
	"nt-cli/internal/store"
)

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
		return
	}

	os.Exit(app.RunCLI(svc, os.Args[1:], os.Stdout, os.Stderr))
}
