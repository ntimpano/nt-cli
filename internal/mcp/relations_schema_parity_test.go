package mcp

import (
	"testing"

	"flint/internal/app"
)

func TestMCP_RelateSchema_ParityWithAllowedRelationTypes(t *testing.T) {
	withGraphFlag(t, "1")

	tools := advertisedTools(t)
	var relate map[string]interface{}
	for _, tool := range tools {
		if tool["name"] == "relate" {
			relate = tool
			break
		}
	}
	if relate == nil {
		t.Fatalf("relate tool not advertised")
	}

	schema, _ := relate["inputSchema"].(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})
	rawRelationType, ok := props["relation_type"].(map[string]interface{})
	if !ok {
		t.Fatalf("relate schema missing relation_type property")
	}

	enumRaw, ok := rawRelationType["enum"]
	if !ok {
		t.Fatalf("relation_type must define enum to match service whitelist")
	}
	enum := toStringSlice(enumRaw)
	if !sameStringSet(enum, app.AllowedRelationTypes) {
		t.Fatalf("relation_type enum mismatch: got=%v want=%v", enum, app.AllowedRelationTypes)
	}
}
