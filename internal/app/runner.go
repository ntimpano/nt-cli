package app

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RunCLI dispatches an nt-cli command using the provided Service and writes
// human output to stdout / errors to stderr. It returns the process exit code
// (0 on success, non-zero on validation, not-found, or unknown command).
//
// This function is the testable seam used in place of an inline `func main`
// switch so that strict TDD can cover dispatch, validation, exit codes, and
// CLI/MCP behaviour parity end-to-end.
//
// The caller is responsible for constructing the Service (and the underlying
// store / DB), so RunCLI itself never touches the filesystem.
func RunCLI(svc *Service, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stdout)
		return 1
	}

	switch args[0] {
	case "init":
		if err := svc.Init(); err != nil {
			fmt.Fprintf(stderr, "init failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "initialized")
		return 0

	case "save":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli save \"your note\"")
			return 1
		}
		note := strings.TrimSpace(strings.Join(args[1:], " "))
		if note == "" {
			fmt.Fprintln(stderr, "note cannot be empty")
			return 1
		}
		id, err := svc.Save(note)
		if err != nil {
			fmt.Fprintf(stderr, "save failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "saved #%d\n", id)
		return 0

	case "recall":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli recall \"query\"")
			return 1
		}
		query := strings.TrimSpace(strings.Join(args[1:], " "))
		if query == "" {
			fmt.Fprintln(stderr, "query cannot be empty")
			return 1
		}
		items, err := svc.Recall(query, 10)
		if err != nil {
			fmt.Fprintf(stderr, "recall failed: %v\n", err)
			return 1
		}
		if len(items) == 0 {
			fmt.Fprintln(stdout, "no results")
			return 0
		}
		for _, it := range items {
			fmt.Fprintf(stdout, "#%d [%s] %s\n", it.ID, it.CreatedAt.Format("2006-01-02 15:04"), it.Content)
		}
		return 0

	case "list":
		limit := 10
		if len(args) >= 2 {
			n, err := strconv.Atoi(strings.TrimSpace(args[1]))
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "usage: nt-cli list [positive-limit]")
				return 1
			}
			limit = n
		}
		items, err := svc.List(limit)
		if err != nil {
			fmt.Fprintf(stderr, "list failed: %v\n", err)
			return 1
		}
		if len(items) == 0 {
			fmt.Fprintln(stdout, "no results")
			return 0
		}
		for _, it := range items {
			fmt.Fprintf(stdout, "#%d [%s] %s\n", it.ID, it.CreatedAt.Format("2006-01-02 15:04"), it.Content)
		}
		return 0

	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli delete <id>")
			return 1
		}
		id, err := strconv.ParseInt(strings.TrimSpace(args[1]), 10, 64)
		if err != nil || id <= 0 {
			fmt.Fprintln(stderr, "id must be a positive integer")
			return 1
		}
		deleted, err := svc.Delete(id)
		if err != nil {
			fmt.Fprintf(stderr, "delete failed: %v\n", err)
			return 1
		}
		if !deleted {
			fmt.Fprintf(stdout, "note #%d not found\n", id)
			return 0
		}
		fmt.Fprintf(stdout, "deleted #%d\n", id)
		return 0

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli get <id>")
			return 1
		}
		id, err := ParsePositiveID(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		item, err := svc.Get(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				fmt.Fprintf(stderr, "note #%d not found\n", id)
				return 1
			}
			fmt.Fprintf(stderr, "get failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, FormatNote(item))
		return 0

	case "update":
		if len(args) < 3 {
			fmt.Fprintln(stderr, "usage: nt-cli update <id> \"new content\"")
			return 1
		}
		id, err := ParsePositiveID(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		content := strings.TrimSpace(strings.Join(args[2:], " "))
		if content == "" {
			fmt.Fprintln(stderr, "content cannot be empty")
			return 1
		}
		ok, err := svc.Update(id, content)
		if err != nil {
			fmt.Fprintf(stderr, "update failed: %v\n", err)
			return 1
		}
		if !ok {
			fmt.Fprintf(stderr, "note #%d not found\n", id)
			return 1
		}
		fmt.Fprintf(stdout, "updated #%d\n", id)
		return 0

	default:
		printUsage(stdout)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "nt-cli commands:")
	fmt.Fprintln(w, "  nt-cli init")
	fmt.Fprintln(w, "  nt-cli save \"note\"")
	fmt.Fprintln(w, "  nt-cli recall \"query\"")
	fmt.Fprintln(w, "  nt-cli list [limit]")
	fmt.Fprintln(w, "  nt-cli get <id>")
	fmt.Fprintln(w, "  nt-cli update <id> \"new content\"")
	fmt.Fprintln(w, "  nt-cli delete <id>")
	fmt.Fprintln(w, "  nt-cli mcp")
}
