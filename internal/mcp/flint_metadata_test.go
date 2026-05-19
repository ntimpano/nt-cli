package mcp

import (
	"encoding/json"
	"testing"
)

func TestInitialize_ServerInfoUsesFlintName(t *testing.T) {
	req := request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize"}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal initialize request: %v", err)
	}

	resp, ok := handleRequest(payload, nil)
	if !ok {
		t.Fatalf("expected initialize response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	serverInfo, _ := result["serverInfo"].(map[string]interface{})
	if got := serverInfo["name"]; got != "flint" {
		t.Fatalf("initialize.serverInfo.name = %v, want flint", got)
	}
}

func TestResourcesList_UsesFlintResourceURI(t *testing.T) {
	f := newProjectMCPFixture(t)
	req := request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "resources/list"}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal resources/list request: %v", err)
	}

	resp, ok := handleRequest(payload, f.svc)
	if !ok {
		t.Fatalf("expected resources/list response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	resources, _ := result["resources"].([]map[string]interface{})
	if len(resources) == 0 {
		t.Fatalf("expected at least one resource")
	}
	if got := resources[0]["uri"]; got != "flint://project/active" {
		t.Fatalf("resource uri = %v, want flint://project/active", got)
	}
}
