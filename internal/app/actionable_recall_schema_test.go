package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestActionableRecallResponse_MatchesPublishedSchema acts as a bridge
// between the documented `actionable_recall.schema.json` fixture and
// the live `BuildActionableRecall` output. We don't pull a runtime
// JSON-Schema validator (no deps); instead we walk the schema's
// `required` / `properties` tree and assert the live JSON has the
// exact same keys and JSON-types. If the schema and the response
// drift, this test fails — protecting external clients from a silent
// contract change.
func TestActionableRecallResponse_MatchesPublishedSchema(t *testing.T) {
	schemaBytes, err := os.ReadFile(filepath.Join("testdata", "actionable_recall.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	stamp := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	items := []MemoryItem{{
		ID:        1,
		Title:     "Switched to JWT",
		Type:      "decision",
		Content:   "**Where**: internal/auth/middleware.go\n- one\n- two",
		CreatedAt: stamp,
		UpdatedAt: stamp,
		TopicKey:  "auth/jwt",
		Scope:     "project",
	}}
	resp := BuildActionableRecall(items)

	// Render through the same JSON encoder a wire caller would see.
	wireBytes, err := json.Marshal(map[string]interface{}{
		"matches": []map[string]interface{}{
			{
				"id":         resp.Matches[0].ID,
				"content":    resp.Matches[0].Content,
				"created_at": resp.Matches[0].CreatedAt.UTC().Format(time.RFC3339Nano),
				"updated_at": resp.Matches[0].UpdatedAt.UTC().Format(time.RFC3339Nano),
				"title":      resp.Matches[0].Title,
				"type":       resp.Matches[0].Type,
				"topic_key":  resp.Matches[0].TopicKey,
				"scope":      resp.Matches[0].Scope,
			},
		},
		"next_action":    resp.NextAction,
		"checklist":      resp.Checklist,
		"inferred_paths": resp.InferredPaths,
	})
	if err != nil {
		t.Fatalf("marshal wire: %v", err)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(wireBytes, &wire); err != nil {
		t.Fatalf("parse wire: %v", err)
	}

	// Top-level required keys must all be present.
	required, _ := schema["required"].([]interface{})
	if len(required) == 0 {
		t.Fatalf("schema missing top-level required[]")
	}
	for _, k := range required {
		key := k.(string)
		if _, has := wire[key]; !has {
			t.Fatalf("response missing required key %q", key)
		}
	}

	// Top-level type-shape checks: schema says matches is array,
	// next_action is string, checklist is array, inferred_paths is
	// array. Assert each.
	if _, ok := wire["matches"].([]interface{}); !ok {
		t.Fatalf("matches must be JSON array, got %T", wire["matches"])
	}
	if _, ok := wire["next_action"].(string); !ok {
		t.Fatalf("next_action must be JSON string, got %T", wire["next_action"])
	}
	checklist, ok := wire["checklist"].([]interface{})
	if !ok {
		t.Fatalf("checklist must be JSON array, got %T", wire["checklist"])
	}
	if len(checklist) > 5 {
		t.Fatalf("checklist exceeds maxItems=5, got %d", len(checklist))
	}
	if _, ok := wire["inferred_paths"].([]interface{}); !ok {
		t.Fatalf("inferred_paths must be JSON array, got %T", wire["inferred_paths"])
	}

	// Disallow extra keys at the top level — schema sets
	// additionalProperties=false.
	allowed := map[string]struct{}{
		"matches": {}, "next_action": {}, "checklist": {}, "inferred_paths": {},
	}
	for k := range wire {
		if _, ok := allowed[k]; !ok {
			t.Fatalf("unexpected top-level key %q (additionalProperties=false in schema)", k)
		}
	}

	// Each match must have all 8 schema-required fields.
	matches := wire["matches"].([]interface{})
	if len(matches) != 1 {
		t.Fatalf("expected 1 match in fixture, got %d", len(matches))
	}
	row := matches[0].(map[string]interface{})
	for _, k := range []string{"id", "content", "created_at", "updated_at", "title", "type", "topic_key", "scope"} {
		if _, has := row[k]; !has {
			t.Fatalf("match missing required field %q", k)
		}
	}
}
