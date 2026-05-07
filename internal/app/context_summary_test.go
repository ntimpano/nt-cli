package app_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nt-cli/internal/app"
)

// TestContextSummary_FlagParsing proves --summary and --json parse correctly.
func TestContextSummary_FlagParsing(t *testing.T) {
	cases := []struct {
		args        []string
		wantSummary bool
		wantJSON    bool
	}{
		{[]string{"context"}, false, false},
		{[]string{"context", "--summary"}, true, false},
		{[]string{"context", "--summary=true"}, true, false},
		{[]string{"context", "--summary=false"}, false, false},
		{[]string{"context", "--json"}, false, true},
		{[]string{"context", "--summary", "--json"}, true, true},
	}
	for _, tc := range cases {
		store := newFilterMemStore()
		code, stdout, stderr := runCLIFilter(t, store, tc.args...)
		if code != 0 {
			t.Fatalf("args=%v: expected exit 0, got %d (stderr=%q)", tc.args, code, stderr)
		}
		hasSummary := strings.Contains(stdout, "PROJECT") || strings.Contains(stdout, `"recent_notes"`)
		if hasSummary != tc.wantSummary {
			t.Fatalf("args=%v: wantSummary=%v but summaryMode output=%v (stdout=%q)", tc.args, tc.wantSummary, hasSummary, stdout)
		}
	}
}

// TestContextSummary_JSONWithoutSummary_IsNoop proves --json alone does not trigger summary.
func TestContextSummary_JSONWithoutSummary_IsNoop(t *testing.T) {
	store := newFilterMemStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.SaveWithMeta(app.SaveRequest{Content: "hello", CreatedAt: base}); err != nil {
		t.Fatal(err)
	}

	_, outPlain, _ := runCLIFilter(t, store, "context")
	_, outJSON, _ := runCLIFilter(t, store, "context", "--json")

	if outPlain != outJSON {
		t.Fatalf("--json without --summary should produce identical output\nplain=%q\njson=%q", outPlain, outJSON)
	}
	if strings.Contains(outJSON, "PROJECT") {
		t.Fatalf("expected no summary section without --summary flag, got %q", outJSON)
	}
}

// TestContextSummary_TopicDerivation proves deduplication and empty exclusion.
func TestContextSummary_TopicDerivation(t *testing.T) {
	store := newFilterMemStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	notes := []struct {
		topic   string
		content string
	}{
		{"alpha", "note1"},
		{"beta", "note2"},
		{"alpha", "note3"}, // duplicate
		{"", "note4"},      // empty — excluded
		{"gamma", "note5"},
	}
	for i, n := range notes {
		if _, err := store.SaveWithMeta(app.SaveRequest{
			Content:   n.content,
			TopicKey:  n.topic,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	_, stdout, stderr := runCLIFilter(t, store, "context", "--summary", "--json")
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var v struct {
		Topics []string `json:"topics"`
	}
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("json decode: %v (stdout=%q)", err, stdout)
	}

	// "alpha" must appear exactly once
	count := 0
	for _, tp := range v.Topics {
		if tp == "alpha" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected alpha once, got %d times in %v", count, v.Topics)
	}

	// empty string must not appear
	for _, tp := range v.Topics {
		if tp == "" {
			t.Fatalf("empty topic_key leaked into topics: %v", v.Topics)
		}
	}

	// gamma must be present
	found := false
	for _, tp := range v.Topics {
		if tp == "gamma" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected gamma in topics, got %v", v.Topics)
	}
}

// TestContextSummary_JSONStructure proves --summary --json keys and slice types.
func TestContextSummary_JSONStructure(t *testing.T) {
	store := newFilterMemStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.SaveWithMeta(app.SaveRequest{
		Content:   "test note",
		TopicKey:  "my/topic",
		CreatedAt: base,
	}); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLIFilter(t, store, "context", "--summary", "--json")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}

	var v map[string]any
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("json decode: %v (stdout=%q)", err, stdout)
	}

	for _, key := range []string{"project", "memory", "recent_notes", "topics"} {
		if _, ok := v[key]; !ok {
			t.Fatalf("expected key %q in JSON output, got keys: %v", key, v)
		}
	}

	// recent_notes must be a slice (not null)
	notes, ok := v["recent_notes"].([]any)
	if !ok {
		t.Fatalf("expected recent_notes to be an array, got %T", v["recent_notes"])
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one note in recent_notes")
	}
}

// TestContextSummary_EmptyStore proves graceful output when no notes and no project.
func TestContextSummary_EmptyStore(t *testing.T) {
	store := newFilterMemStore()

	code, stdout, stderr := runCLIFilter(t, store, "context", "--summary")
	if code != 0 {
		t.Fatalf("expected exit 0 on empty store, got %d (stderr=%q)", code, stderr)
	}

	if !strings.Contains(stdout, "PROJECT") {
		t.Fatalf("expected PROJECT section, got %q", stdout)
	}
	// No active project — expect "none"
	if !strings.Contains(stdout, "none") {
		t.Fatalf("expected 'none' for missing project, got %q", stdout)
	}
	if !strings.Contains(stdout, "no notes found") {
		t.Fatalf("expected 'no notes found' for empty store, got %q", stdout)
	}
}
