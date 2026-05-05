package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"nt-cli/internal/app"
	"nt-cli/internal/mcp"
	"nt-cli/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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

	switch cmd {
	case "init":
		if err := svc.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("initialized at %s\n", dbPath)
	case "save":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nt-cli save \"your note\"")
			os.Exit(1)
		}
		note := strings.TrimSpace(strings.Join(os.Args[2:], " "))
		if note == "" {
			fmt.Fprintln(os.Stderr, "note cannot be empty")
			os.Exit(1)
		}
		id, err := svc.Save(note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("saved #%d\n", id)
	case "recall":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nt-cli recall \"query\"")
			os.Exit(1)
		}
		query := strings.TrimSpace(strings.Join(os.Args[2:], " "))
		if query == "" {
			fmt.Fprintln(os.Stderr, "query cannot be empty")
			os.Exit(1)
		}
		items, err := svc.Recall(query, 10)
		if err != nil {
			fmt.Fprintf(os.Stderr, "recall failed: %v\n", err)
			os.Exit(1)
		}
		if len(items) == 0 {
			fmt.Println("no results")
			return
		}
		for _, it := range items {
			fmt.Printf("#%d [%s] %s\n", it.ID, it.CreatedAt.Format("2006-01-02 15:04"), it.Content)
		}
	case "list":
		limit := 10
		if len(os.Args) >= 3 {
			n, err := strconv.Atoi(strings.TrimSpace(os.Args[2]))
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "usage: nt-cli list [positive-limit]")
				os.Exit(1)
			}
			limit = n
		}
		items, err := svc.List(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "list failed: %v\n", err)
			os.Exit(1)
		}
		if len(items) == 0 {
			fmt.Println("no results")
			return
		}
		for _, it := range items {
			fmt.Printf("#%d [%s] %s\n", it.ID, it.CreatedAt.Format("2006-01-02 15:04"), it.Content)
		}
	case "delete":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nt-cli delete <id>")
			os.Exit(1)
		}
		id, err := strconv.ParseInt(strings.TrimSpace(os.Args[2]), 10, 64)
		if err != nil || id <= 0 {
			fmt.Fprintln(os.Stderr, "id must be a positive integer")
			os.Exit(1)
		}
		deleted, err := svc.Delete(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "delete failed: %v\n", err)
			os.Exit(1)
		}
		if !deleted {
			fmt.Printf("note #%d not found\n", id)
			return
		}
		fmt.Printf("deleted #%d\n", id)
	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nt-cli get <id>")
			os.Exit(1)
		}
		id, err := app.ParsePositiveID(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		item, err := svc.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "get failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(app.FormatNote(item))
	case "update":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: nt-cli update <id> \"new content\"")
			os.Exit(1)
		}
		id, err := app.ParsePositiveID(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		content := strings.TrimSpace(strings.Join(os.Args[3:], " "))
		if content == "" {
			fmt.Fprintln(os.Stderr, "content cannot be empty")
			os.Exit(1)
		}
		ok, err := svc.Update(id, content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "note #%d not found\n", id)
			os.Exit(1)
		}
		fmt.Printf("updated #%d\n", id)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("nt-cli commands:")
	fmt.Println("  nt-cli init")
	fmt.Println("  nt-cli save \"note\"")
	fmt.Println("  nt-cli recall \"query\"")
	fmt.Println("  nt-cli list [limit]")
	fmt.Println("  nt-cli get <id>")
	fmt.Println("  nt-cli update <id> \"new content\"")
	fmt.Println("  nt-cli delete <id>")
	fmt.Println("  nt-cli mcp")
}
