package mcp

import "testing"

// TestMCP_ToolsCall_InvalidArgs_ReturnsRPC32602 covers handlers that
// previously ignored json.Unmarshal errors and proceeded with zero values.
// Contract: malformed/typed-wrong arguments MUST return JSON-RPC -32602.
func TestMCP_ToolsCall_InvalidArgs_ReturnsRPC32602(t *testing.T) {
	f := newProjectMCPFixture(t)

	cases := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{
			name: "local_context wrong n type",
			tool: "local_context",
			args: map[string]interface{}{"n": "five"},
		},
		{
			name: "project_probe wrong cwd type",
			tool: "project_probe",
			args: map[string]interface{}{"cwd": 42},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, rpcErr := callTool(t, f.svc, tc.tool, tc.args)
			if rpcErr == nil {
				t.Fatalf("expected rpc error -32602 for %s", tc.tool)
			}
			if rpcErr.Code != -32602 || rpcErr.Message != "invalid arguments" {
				t.Fatalf("expected -32602 invalid arguments, got code=%d message=%q", rpcErr.Code, rpcErr.Message)
			}
		})
	}
}
