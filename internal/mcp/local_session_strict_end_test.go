package mcp

import (
	"strings"
	"testing"
)

// TestMCP_LocalSessionEnd_StrictContract enforces BUG-09: local_session_end
// must route to SessionEndStrict, so ending without a prior summary fails
// with summary_required while ending after summary passes.
func TestMCP_LocalSessionEnd_StrictContract(t *testing.T) {
	f := newProjectMCPFixture(t)

	_, _ = callTool(t, f.svc, "local_session_start", map[string]interface{}{
		"session_id": "strict-1",
	})

	withoutSummary, rpcErr := callTool(t, f.svc, "local_session_end", map[string]interface{}{
		"session_id": "strict-1",
	})
	if rpcErr != nil {
		t.Fatalf("expected tool error, got rpc error: %+v", rpcErr)
	}
	if withoutSummary["isError"] != true {
		t.Fatalf("expected isError=true without summary, got %+v", withoutSummary)
	}
	if !strings.Contains(toolResultText(t, withoutSummary), "summary_required") {
		t.Fatalf("expected summary_required error, got %q", toolResultText(t, withoutSummary))
	}

	_, _ = callTool(t, f.svc, "local_session_summary", map[string]interface{}{
		"session_id": "strict-1",
		"summary":    "session wrapped",
	})

	withSummary, rpcErr := callTool(t, f.svc, "local_session_end", map[string]interface{}{
		"session_id": "strict-1",
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error with summary: %+v", rpcErr)
	}
	if withSummary["isError"] == true {
		t.Fatalf("expected strict end to pass after summary, got %+v", withSummary)
	}
}
