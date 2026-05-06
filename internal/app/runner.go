package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"nt-cli/internal/parity"
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

	case "import":
		return runImport(svc, args[1:], stdout, stderr)

	case "backup":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli backup <path>")
			return 1
		}
		if err := svc.Backup(args[1]); err != nil {
			fmt.Fprintf(stderr, "backup failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "backup written to %s\n", args[1])
		return 0

	case "restore":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli restore <path>")
			return 1
		}
		if err := svc.Restore(args[1]); err != nil {
			fmt.Fprintf(stderr, "restore failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "restored from %s\n", args[1])
		return 0

	case "doctor":
		// Phase 6: doctor accepts an optional `--json` flag that
		// emits a structured report including autopilot.session_close_rate
		// and autopilot.threshold. Default surface is unchanged.
		// Any other extra arg is rejected so genuine typos still
		// surface instead of being silently ignored.
		jsonOut := false
		for _, a := range args[1:] {
			switch a {
			case "--json":
				jsonOut = true
			default:
				fmt.Fprintln(stderr, "usage: nt-cli doctor [--json]")
				return 1
			}
		}
		report, err := svc.Doctor()
		if err != nil {
			fmt.Fprintf(stderr, "doctor failed: %v\n", err)
			return 1
		}
		if jsonOut {
			if err := writeDoctorJSON(stdout, report); err != nil {
				fmt.Fprintf(stderr, "doctor --json failed: %v\n", err)
				return 1
			}
			return 0
		}
		fmt.Fprintln(stdout, report.Summary)
		// Surface non-ok integrity messages verbatim so users can act
		// on them. Healthy stores produce zero extra lines.
		for _, msg := range report.IntegrityMessages {
			fmt.Fprintf(stdout, "  integrity: %s\n", msg)
		}
		return 0

	case "parity":
		return runParity(svc, args, stdout, stderr)

	default:
		printUsage(stdout)
		return 1
	}
}

// runSession dispatches `nt-cli session <start|end|summary> <id> [text...]`.
// Kept as a helper so the main switch stays scannable. Validation lives at
// the service layer; the CLI is just an args parser + presenter.
// runImport dispatches `nt-cli import [--dry-run] <file.json>`. Currently
// JSON-only; format is detected by extension when MD/CSV are added.
func runImport(svc *Service, args []string, stdout, stderr io.Writer) int {
	dryRun := false
	var path string
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			key, val, hasVal := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			switch key {
			case "dry-run":
				if hasVal && strings.ToLower(strings.TrimSpace(val)) == "false" {
					dryRun = false
				} else {
					dryRun = true
				}
			default:
				fmt.Fprintf(stderr, "unknown flag --%s\n", key)
				return 1
			}
			continue
		}
		if path != "" {
			fmt.Fprintln(stderr, "import takes a single file path")
			return 1
		}
		path = a
	}
	if strings.TrimSpace(path) == "" {
		fmt.Fprintln(stderr, "usage: nt-cli import [--dry-run] <file.json>")
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "import read failed: %v\n", err)
		return 1
	}
	res, err := svc.ImportJSON(data, dryRun)
	if err != nil {
		fmt.Fprintf(stderr, "import failed: %v\n", err)
		return 1
	}
	prefix := "import"
	if dryRun {
		prefix = "import (dry-run)"
	}
	fmt.Fprintf(stdout, "%s: inserted=%d skipped=%d\n", prefix, res.Inserted, res.Skipped)
	return 0
}

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
			fmt.Fprintln(stderr, "usage: nt-cli session end <id> [--force]")
			return 1
		}
		id := strings.TrimSpace(args[1])
		// Phase 6 / autopilot: --force bypasses the summary guard.
		// Accept the flag in any position after id so users don't
		// have to think about ordering.
		force := false
		for _, a := range args[2:] {
			if a == "--force" {
				force = true
			}
		}
		var err error
		if force {
			err = svc.SessionEndForce(id)
		} else {
			err = svc.SessionEnd(id)
		}
		if err != nil {
			// Map the autopilot sentinel to exit code 2 so scripts
			// can distinguish "summary missing, you should write
			// one" from generic failures (exit 1).
			if errors.Is(err, ErrSummaryRequired) {
				fmt.Fprintf(stderr, "session end failed: %v\n", err)
				return 2
			}
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
	fmt.Fprintln(w, "  nt-cli import [--dry-run] <file.json>")
	fmt.Fprintln(w, "  nt-cli backup <path>")
	fmt.Fprintln(w, "  nt-cli restore <path>")
	fmt.Fprintln(w, "  nt-cli doctor")
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

// runParity dispatches `nt-cli parity <subcommand>`. Subcommands:
//   - scorecard:  computes the parity verdict from supplied dimension
//     signals and prints the canonical JSON contract.
//   - continuity: replays the knowledge-continuity fixture against the
//     live store, writes baseline.json, and prints the same JSON.
//
// Each subcommand keeps its own flag parser so unknown flags fail
// loudly (a silent skip would let a typo zero a dimension and quietly
// skew the scorecard verdict).
func runParity(svc *Service, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: nt-cli parity <scorecard|continuity>")
		return 1
	}
	switch args[1] {
	case "scorecard":
		return runParityScorecard(args[2:], stdout, stderr)
	case "continuity":
		return runParityContinuity(svc, args[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown parity subcommand %q (valid: scorecard, continuity)\n", args[1])
		return 1
	}
}

// parityScorecardFlags is the set of int flags accepted by
// `nt-cli parity scorecard`. They map 1:1 to parity.ScorecardSignals
// fields so changes to the contract surface here as compile errors.
type parityScorecardFlags struct {
	coreOps                int
	metadataRetrieval      int
	sessionWorkflow        int
	importExportBackup     int
	reliabilityOperability int
	knowledgeContinuity    int
	uxAPIContract          int
	soakDays               int
}

// parseParityScorecardFlags parses the supported `--key=value` flags into
// a parityScorecardFlags value. Unknown flags surface a usage error so
// typos fail loudly instead of silently scoring 0 and skewing the verdict.
func parseParityScorecardFlags(args []string) (parityScorecardFlags, error) {
	f := parityScorecardFlags{}
	known := map[string]*int{
		"--core-ops":                &f.coreOps,
		"--metadata-retrieval":      &f.metadataRetrieval,
		"--session-workflow":        &f.sessionWorkflow,
		"--import-export-backup":    &f.importExportBackup,
		"--reliability-operability": &f.reliabilityOperability,
		"--knowledge-continuity":    &f.knowledgeContinuity,
		"--ux-api-contract":         &f.uxAPIContract,
		"--soak-days":               &f.soakDays,
	}
	for _, raw := range args {
		key, val, ok := strings.Cut(raw, "=")
		if !ok {
			return f, fmt.Errorf("flag %q must use --key=value form", raw)
		}
		ptr, found := known[key]
		if !found {
			return f, fmt.Errorf("unknown flag %q", key)
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return f, fmt.Errorf("flag %s expects integer, got %q", key, val)
		}
		*ptr = n
	}
	return f, nil
}

func runParityScorecard(args []string, stdout, stderr io.Writer) int {
	flags, err := parseParityScorecardFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	v := parity.ComputeScorecard(parity.ScorecardSignals{
		CoreOps:                flags.coreOps,
		MetadataRetrieval:      flags.metadataRetrieval,
		SessionWorkflow:        flags.sessionWorkflow,
		ImportExportBackup:     flags.importExportBackup,
		ReliabilityOperability: flags.reliabilityOperability,
		KnowledgeContinuity:    flags.knowledgeContinuity,
		UXAPIContract:          flags.uxAPIContract,
		SoakDays:               flags.soakDays,
	})
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(stderr, "encode scorecard: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

// runParityContinuity executes `nt-cli parity continuity --fixture=<path> --out=<path>`.
// Replays the fixture suite through the live store, writes baseline.json
// to --out, and prints the same baseline as JSON on stdout so the
// runbook can pipe it into jq or diff against a previous baseline.
//
// Both flags are required: a missing fixture should fail loudly rather
// than silently producing a zero-row baseline that would skew the
// scorecard's KnowledgeContinuity dimension to zero.
func runParityContinuity(svc *Service, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("parity continuity", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fixture := fs.String("fixture", "", "path to fixture queries.json (required)")
	out := fs.String("out", "", "path to write baseline.json (required)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *fixture == "" {
		fmt.Fprintln(stderr, "parity continuity: --fixture is required")
		return 1
	}
	if *out == "" {
		fmt.Fprintln(stderr, "parity continuity: --out is required")
		return 1
	}
	baseline, err := svc.RunContinuityHarness(*fixture, *out)
	if err != nil {
		fmt.Fprintf(stderr, "parity continuity: %v\n", err)
		return 1
	}
	body, err := json.Marshal(baseline)
	if err != nil {
		fmt.Fprintf(stderr, "encode baseline: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(body))
	return 0
}
