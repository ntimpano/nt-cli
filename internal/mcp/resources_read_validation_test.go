package mcp

import (
	"encoding/json"
	"testing"
)

// TestMCP_ResourcesRead_InvalidParams_ReturnsRPC32602 ensures malformed
// params payloads return JSON-RPC invalid arguments errors.
func TestMCP_ResourcesRead_InvalidParams_ReturnsRPC32602(t *testing.T) {
	f := newProjectMCPFixture(t)

	req := request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri": 123}`),
	}
	payload, _ := json.Marshal(req)
	resp, ok := handleRequest(payload, f.svc)
	if !ok {
		t.Fatal("expected response")
	}
	if resp.Error == nil {
		t.Fatalf("expected rpc error -32602, got nil")
	}
	if resp.Error.Code != -32602 || resp.Error.Message != "invalid arguments" {
		t.Fatalf("expected -32602 invalid arguments, got code=%d message=%q", resp.Error.Code, resp.Error.Message)
	}
}
