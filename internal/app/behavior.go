package app

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func parseObservationMarker(marker string) (category, field, value string, confidence int, err error) {
	hasConfidence := false
	raw := strings.TrimSpace(marker)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "BEHAVIORAL_OBSERVATION:") {
		return "", "", "", 0, errors.New("missing BEHAVIORAL_OBSERVATION prefix")
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "BEHAVIORAL_OBSERVATION:"))
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		kv := strings.TrimSpace(p)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return "", "", "", 0, fmt.Errorf("invalid marker segment %q", kv)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "category":
			category = v
		case "field":
			field = v
		case "value":
			value = v
		case "confidence":
			n, convErr := strconv.Atoi(v)
			if convErr != nil {
				return "", "", "", 0, fmt.Errorf("invalid confidence %q", v)
			}
			confidence = n
			hasConfidence = true
		}
	}
	if strings.TrimSpace(category) == "" || strings.TrimSpace(field) == "" || strings.TrimSpace(value) == "" || !hasConfidence {
		return "", "", "", 0, errors.New("marker missing required keys")
	}
	if confidence < 0 || confidence > 100 {
		return "", "", "", 0, errors.New("confidence must be between 0 and 100")
	}
	return category, field, value, confidence, nil
}

func runBehavior(svc *Service, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: nt-cli behavior <list|show|dismiss|preview>")
		return 1
	}
	bs := svc.BehavioralStore()
	if bs == nil {
		fmt.Fprintln(stderr, "behavior failed: store does not support behavioral operations")
		return 1
	}
	switch args[0] {
	case "list":
		statuses := []string{"observed", "candidate"}
		for _, a := range args[1:] {
			if a == "--candidates" {
				statuses = []string{"candidate"}
				continue
			}
			fmt.Fprintln(stderr, "usage: nt-cli behavior list [--candidates]")
			return 1
		}
		rows, err := bs.ListObservations(statuses)
		if err != nil {
			fmt.Fprintf(stderr, "behavior list failed: %v\n", err)
			return 1
		}
		for _, r := range rows {
			fmt.Fprintf(stdout, "#%d [%s/%s=%s] confidence=%d count=%d status=%s\n", r.ID, r.Category, r.Field, r.Value, r.Confidence, r.OccurrenceCount, r.Status)
		}
		return 0

	case "show":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli behavior show <id>")
			return 1
		}
		id, convErr := strconv.ParseInt(strings.TrimSpace(args[1]), 10, 64)
		if convErr != nil || id <= 0 {
			fmt.Fprintln(stderr, "id must be a positive integer")
			return 1
		}
		obs, err := bs.GetObservation(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
				fmt.Fprintf(stderr, "behavior observation #%d not found\n", id)
				return 1
			}
			fmt.Fprintf(stderr, "behavior show failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "id: %d\n", obs.ID)
		fmt.Fprintf(stdout, "category: %s\n", obs.Category)
		fmt.Fprintf(stdout, "field: %s\n", obs.Field)
		fmt.Fprintf(stdout, "value: %s\n", obs.Value)
		fmt.Fprintf(stdout, "confidence: %d\n", obs.Confidence)
		fmt.Fprintf(stdout, "occurrence_count: %d\n", obs.OccurrenceCount)
		fmt.Fprintf(stdout, "status: %s\n", obs.Status)
		fmt.Fprintf(stdout, "last_seen: %s\n", obs.LastSeen.Format("2006-01-02 15:04"))
		fmt.Fprintf(stdout, "created_at: %s\n", obs.CreatedAt.Format("2006-01-02 15:04"))
		return 0

	case "dismiss":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: nt-cli behavior dismiss <id>")
			return 1
		}
		id, convErr := strconv.ParseInt(strings.TrimSpace(args[1]), 10, 64)
		if convErr != nil || id <= 0 {
			fmt.Fprintln(stderr, "id must be a positive integer")
			return 1
		}
		if err := bs.DismissObservation(id); err != nil {
			fmt.Fprintf(stderr, "behavior dismiss failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "dismissed observation #%d\n", id)
		return 0

	case "preview":
		rows, err := bs.Candidates()
		if err != nil {
			fmt.Fprintf(stderr, "behavior preview failed: %v\n", err)
			return 1
		}
		if len(rows) == 0 {
			fmt.Fprintln(stdout, "No candidates to preview")
			return 0
		}
		fmt.Fprintln(stdout, "<!-- nt-cli:behavioral-candidates -->")
		fmt.Fprintln(stdout, "## Behavioral Candidates")
		fmt.Fprintln(stdout)
		for _, r := range rows {
			fmt.Fprintf(stdout, "- [%s/%s=%s] confidence=%d, seen %d times\n", r.Category, r.Field, r.Value, r.Confidence, r.OccurrenceCount)
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "<!-- /nt-cli:behavioral-candidates -->")
		return 0
	default:
		fmt.Fprintf(stderr, "unknown behavior subcommand %q (expected list|show|dismiss|preview)\n", args[0])
		return 1
	}
}
