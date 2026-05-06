package app

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
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
			fmt.Fprintln(stderr, "usage: nt-cli save [--title=...] [--type=...] [--topic-key=...] [--scope=...] \"your note\"")
			return 1
		}
		// Parse leading `--key=value` flags, stop at the first non-flag arg
		// so callers can save content that itself contains `--`-prefixed
		// words (e.g. `nt-cli save --type=decision "use --foo flag"`).
		var meta SaveRequest
		hasMeta := false
		i := 1
		for ; i < len(args); i++ {
			a := args[i]
			if !strings.HasPrefix(a, "--") {
				break
			}
			key, val, ok := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if !ok {
				fmt.Fprintf(stderr, "invalid flag %q (expected --key=value)\n", a)
				return 1
			}
			switch key {
			case "title":
				meta.Title = val
			case "type":
				meta.Type = val
			case "topic-key":
				meta.TopicKey = val
			case "scope":
				meta.Scope = val
			default:
				fmt.Fprintf(stderr, "unknown flag --%s\n", key)
				return 1
			}
			hasMeta = true
		}
		if i >= len(args) {
			fmt.Fprintln(stderr, "usage: nt-cli save [flags] \"your note\"")
			return 1
		}
		note := strings.TrimSpace(strings.Join(args[i:], " "))
		if note == "" {
			fmt.Fprintln(stderr, "note cannot be empty")
			return 1
		}
		var (
			id  int64
			err error
		)
		if hasMeta {
			meta.Content = note
			id, err = svc.SaveWithMeta(meta)
		} else {
			id, err = svc.Save(note)
		}
		if err != nil {
			fmt.Fprintf(stderr, "save failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "saved #%d\n", id)
		return 0

	case "recall":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli recall [--type=...] [--since=YYYY-MM-DD] [--until=YYYY-MM-DD] \"query\"")
			return 1
		}
		// Parse leading `--key=value` filter flags. Stop at the first
		// non-flag arg so the query itself can contain `--`-prefixed
		// words (mirrors the `save` parser).
		var (
			filterType string
			since      time.Time
			until      time.Time
			hasFilter  bool
		)
		i := 1
		for ; i < len(args); i++ {
			a := args[i]
			if !strings.HasPrefix(a, "--") {
				break
			}
			key, val, ok := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if !ok {
				fmt.Fprintf(stderr, "invalid flag %q (expected --key=value)\n", a)
				return 1
			}
			switch key {
			case "type":
				filterType = val
				hasFilter = true
			case "since":
				t, err := parseDateFlag(val)
				if err != nil {
					fmt.Fprintf(stderr, "invalid --since: %v\n", err)
					return 1
				}
				since = t
				hasFilter = true
			case "until":
				t, err := parseDateFlag(val)
				if err != nil {
					fmt.Fprintf(stderr, "invalid --until: %v\n", err)
					return 1
				}
				until = t
				hasFilter = true
			default:
				fmt.Fprintf(stderr, "unknown flag --%s\n", key)
				return 1
			}
		}
		if i >= len(args) {
			fmt.Fprintln(stderr, "usage: nt-cli recall [flags] \"query\"")
			return 1
		}
		query := strings.TrimSpace(strings.Join(args[i:], " "))
		if query == "" {
			fmt.Fprintln(stderr, "query cannot be empty")
			return 1
		}
		var (
			items []MemoryItem
			err   error
		)
		if hasFilter {
			items, err = svc.RecallWithOptions(RecallOptions{
				Query: query,
				Type:  filterType,
				Since: since,
				Until: until,
				Limit: 10,
			})
		} else {
			items, err = svc.Recall(query, 10)
		}
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

	case "context":
		// `context` exposes the most-recent-N view, optionally narrowed
		// by scope. Defaults: n=10, no scope filter.
		n := 10
		scope := ""
		for i := 1; i < len(args); i++ {
			a := args[i]
			if !strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "unexpected positional arg %q (context only takes flags)\n", a)
				return 1
			}
			key, val, ok := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if !ok {
				fmt.Fprintf(stderr, "invalid flag %q (expected --key=value)\n", a)
				return 1
			}
			switch key {
			case "n":
				parsed, err := strconv.Atoi(strings.TrimSpace(val))
				if err != nil || parsed <= 0 {
					fmt.Fprintln(stderr, "--n must be a positive integer")
					return 1
				}
				n = parsed
			case "scope":
				scope = val
			default:
				fmt.Fprintf(stderr, "unknown flag --%s\n", key)
				return 1
			}
		}
		items, err := svc.Context(n, scope)
		if err != nil {
			fmt.Fprintf(stderr, "context failed: %v\n", err)
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

	case "session":
		return runSession(svc, args[1:], stdout, stderr)

	default:
		printUsage(stdout)
		return 1
	}
}

// runSession dispatches `nt-cli session <start|end|summary> <id> [text...]`.
// Kept as a helper so the main switch stays scannable. Validation lives at
// the service layer; the CLI is just an args parser + presenter.
func runSession(svc *Service, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: nt-cli session <start|end|summary> <id> [text]")
		return 1
	}
	switch args[0] {
	case "start":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli session start <id>")
			return 1
		}
		id := strings.TrimSpace(args[1])
		if err := svc.SessionStart(id); err != nil {
			fmt.Fprintf(stderr, "session start failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "session started %s\n", id)
		return 0
	case "end":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli session end <id>")
			return 1
		}
		id := strings.TrimSpace(args[1])
		if err := svc.SessionEnd(id); err != nil {
			fmt.Fprintf(stderr, "session end failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "session ended %s\n", id)
		return 0
	case "summary":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli session summary <id> \"text\"")
			return 1
		}
		if len(args) < 3 {
			fmt.Fprintln(stderr, "usage: nt-cli session summary <id> \"text\"")
			return 1
		}
		id := strings.TrimSpace(args[1])
		text := strings.TrimSpace(strings.Join(args[2:], " "))
		if text == "" {
			fmt.Fprintln(stderr, "summary text cannot be empty")
			return 1
		}
		if err := svc.SessionSummary(id, text); err != nil {
			fmt.Fprintf(stderr, "session summary failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "session summary %s\n", id)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown session subcommand %q (expected start|end|summary)\n", args[0])
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "nt-cli commands:")
	fmt.Fprintln(w, "  nt-cli init")
	fmt.Fprintln(w, "  nt-cli save \"note\"")
	fmt.Fprintln(w, "  nt-cli recall [--type=...] [--since=YYYY-MM-DD] [--until=YYYY-MM-DD] \"query\"")
	fmt.Fprintln(w, "  nt-cli context [--n=10] [--scope=...]")
	fmt.Fprintln(w, "  nt-cli list [limit]")
	fmt.Fprintln(w, "  nt-cli get <id>")
	fmt.Fprintln(w, "  nt-cli update <id> \"new content\"")
	fmt.Fprintln(w, "  nt-cli delete <id>")
	fmt.Fprintln(w, "  nt-cli session <start|end|summary> <id> [text]")
	fmt.Fprintln(w, "  nt-cli mcp")
}

// parseDateFlag accepts YYYY-MM-DD (interpreted as UTC midnight) or RFC3339.
// The shorter form is the documented surface — RFC3339 is accepted as an
// escape hatch for tests/automation that need wall-clock precision.
func parseDateFlag(raw string) (time.Time, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, errors.New("empty date")
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339, got %q", v)
	}
	return t.UTC(), nil
}
