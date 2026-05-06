package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestToolDescriptions_MarkLocalOnly proves the spec scenario
// "nt-cli tools MUST be marked as local-only in their metadata". Each
// advertised local_* tool description MUST identify the tool as
// local-only AND state it does not depend on any external backend so
// hosts running multiple memory backends can tell them apart.
func TestToolDescriptions_MarkLocalOnly(t *testing.T) {
	tools := advertisedTools(t)

	wantTools := []string{
		"local_save",
		"local_recall",
		"local_list",
		"local_get",
		"local_update",
		"local_delete",
	}

	byName := map[string]map[string]interface{}{}
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		byName[name] = tool
	}

	for _, name := range wantTools {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("expected tool %q in advertised set, got %v", name, toolNames(tools))
		}
		desc, _ := tool["description"].(string)
		lower := strings.ToLower(desc)

		// MUST mark as local-only. Accept any of the canonical markers.
		if !strings.Contains(lower, "local-only") &&
			!strings.Contains(lower, "local sqlite") {
			t.Fatalf("tool %q description must mark it as local-only or mention local SQLite, got %q", name, desc)
		}

		// MUST state the tool does not depend on any external backend so
		// shadow-mode hosts can tell nt-cli tools apart from sibling
		// memory tools.
		if !strings.Contains(lower, "backend externo") &&
			!strings.Contains(lower, "external backend") {
			t.Fatalf("tool %q description must disambiguate from external backends (mention backend externo/external backend), got %q", name, desc)
		}
	}
}

// TestToolSchemas_Unchanged proves the spec constraint that PR2/PR2b
// preserve schema continuity: every advertised tool MUST keep its
// existing required properties and MUST NOT silently drop any of the
// previously-advertised optional properties. Additive properties are
// allowed (PR2b adds optional filter fields to local_recall) but the
// original set is enforced as a non-shrinking superset.
func TestToolSchemas_Unchanged(t *testing.T) {
	tools := advertisedTools(t)

	want := map[string]struct {
		props    []string
		required []string
		// allowExtra=true means new additive properties beyond `props`
		// are permitted (PR2b adds type/since/until to local_recall).
		allowExtra bool
	}{
		"local_save":   {props: []string{"content", "title", "type", "topic_key", "scope"}, required: []string{"content"}},
		"local_recall": {props: []string{"query", "limit"}, required: []string{"query"}, allowExtra: true},
		"local_list":   {props: []string{"limit"}, required: nil},
		"local_get":    {props: []string{"id"}, required: []string{"id"}},
		"local_update": {props: []string{"id", "content"}, required: []string{"id", "content"}},
		"local_delete": {props: []string{"id"}, required: []string{"id"}},
	}

	byName := map[string]map[string]interface{}{}
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		byName[name] = tool
	}

	for name, exp := range want {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("missing tool %q in advertised set", name)
		}
		schema, ok := tool["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatalf("tool %q inputSchema not a map: %T", name, tool["inputSchema"])
		}
		if got, _ := schema["type"].(string); got != "object" {
			t.Fatalf("tool %q schema type must be %q, got %q", name, "object", got)
		}

		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("tool %q properties not a map: %T", name, schema["properties"])
		}
		for _, p := range exp.props {
			if _, ok := props[p]; !ok {
				t.Fatalf("tool %q schema must keep property %q, properties=%v", name, p, propertyNames(props))
			}
		}
		if !exp.allowExtra && len(props) != len(exp.props) {
			t.Fatalf("tool %q schema property count drifted: want=%v got=%v", name, exp.props, propertyNames(props))
		}

		if exp.required == nil {
			if _, present := schema["required"]; present {
				t.Fatalf("tool %q must keep no required field, got %v", name, schema["required"])
			}
			continue
		}
		gotRequiredAny, ok := schema["required"]
		if !ok {
			t.Fatalf("tool %q schema missing required field, want %v", name, exp.required)
		}
		gotRequired := toStringSlice(gotRequiredAny)
		if !sameStringSet(gotRequired, exp.required) {
			t.Fatalf("tool %q required mismatch: want=%v got=%v", name, exp.required, gotRequired)
		}
	}
}

// advertisedTools invokes tools/list through the real handler and returns
// the advertised tool list, mirroring how an MCP host would discover them.
func advertisedTools(t *testing.T) []map[string]interface{} {
	t.Helper()
	req := request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, ok := handleRequest(payload, nil)
	if !ok {
		t.Fatalf("expected response from tools/list")
	}
	m, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("tools/list result not a map: %T", resp.Result)
	}
	tools, ok := m["tools"].([]map[string]interface{})
	if !ok {
		t.Fatalf("tools field not a slice of maps: %T", m["tools"])
	}
	return tools
}

func toolNames(tools []map[string]interface{}) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		out = append(out, name)
	}
	return out
}

func propertyNames(props map[string]interface{}) []string {
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	return out
}

func toStringSlice(v interface{}) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]int{}
	for _, x := range a {
		set[x]++
	}
	for _, x := range b {
		set[x]--
		if set[x] < 0 {
			return false
		}
	}
	return true
}
