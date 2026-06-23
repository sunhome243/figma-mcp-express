package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// callToolResult dispatches a tool call through the server's full HandleMessage
// path and returns the parsed CallToolResult. Unlike the shared callTool helper
// (which only smoke-checks non-nil), this returns the result so a test can assert
// on IsError and the text content.
func callToolResult(t *testing.T, s *server.MCPServer, name string, args map[string]any) mcp.CallToolResult {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		name, argsJSON,
	)
	resp := s.HandleMessage(context.Background(), []byte(msg))
	if resp == nil {
		t.Fatalf("HandleMessage returned nil for tool %q", name)
	}

	// HandleMessage returns a JSONRPCMessage; for a successful tools/call it is a
	// JSONRPCResponse whose Result is the CallToolResult. Round-trip through JSON
	// to decode it without depending on mcp-go's internal response type.
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal JSON-RPC response: %v", err)
	}
	var envelope struct {
		Result *mcp.CallToolResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal JSON-RPC response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("tool %q returned a JSON-RPC protocol error (code=%d): %s — expected a tool-result error instead",
			name, envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil {
		t.Fatalf("tool %q returned neither result nor error: %s", name, string(raw))
	}
	return *envelope.Result
}

// resultText concatenates the text content of a CallToolResult.
func resultText(t *testing.T, res mcp.CallToolResult) string {
	t.Helper()
	var out string
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out
}

// listToolNames returns every registered tool name via the tools/list method.
func listToolNames(t *testing.T, s *server.MCPServer) []string {
	t.Helper()
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	resp := s.HandleMessage(context.Background(), []byte(msg))
	if resp == nil {
		t.Fatal("tools/list returned nil")
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	var envelope struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	names := make([]string, 0, len(envelope.Result.Tools))
	for _, tool := range envelope.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// ── Registration ──────────────────────────────────────────────────────────────

func TestRegisterBatchTools_ToolRegistered(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1")
	registerBatchTools(s, NewNode("127.0.0.1", 19940, "test"))

	names := listToolNames(t, s)
	found := false
	for _, n := range names {
		if n == "batch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tool %q not registered; got %v", "batch", names)
	}
}

// ── Schema / input validation (handler-level, before any bridge call) ─────────

// Node-target ops are forgiving when the target is nested in params (a common
// mistake because get_batch_op_spec lists nodeIds/nodeId under paramKeys): the
// normalizer hoists it to the op-level nodeIds field. Regression for the usability
// finding where a delete_nodes composed straight from its spec failed.
func TestHoistNodeIDsFromParams(t *testing.T) {
	// params.nodeIds (array) → op-level nodeIds; removed from params.
	op := map[string]interface{}{
		"type":   "delete_nodes",
		"params": map[string]interface{}{"nodeIds": []interface{}{"1:2", "3:4"}},
	}
	normalizeBatchNodeIDs([]interface{}{op})
	if got, ok := op["nodeIds"].([]interface{}); !ok || len(got) != 2 || got[0] != "1:2" || got[1] != "3:4" {
		t.Fatalf("expected hoisted nodeIds [1:2 3:4], got %#v", op["nodeIds"])
	}
	if op["params"].(map[string]interface{})["nodeIds"] != nil {
		t.Fatal("params.nodeIds should be removed after hoist")
	}

	// Singular `nodeId` is a legit ROOT param for read/scan ops — it must NOT be hoisted
	// (hoisting it would strip the scan root and break the op).
	op2 := map[string]interface{}{
		"type":   "scan_nodes_by_types",
		"params": map[string]interface{}{"nodeId": "5:6", "types": []interface{}{"FRAME"}},
	}
	normalizeBatchNodeIDs([]interface{}{op2})
	if _, hoisted := op2["nodeIds"]; hoisted {
		t.Fatalf("singular nodeId (scan root) must NOT be hoisted, got nodeIds=%#v", op2["nodeIds"])
	}
	if p := op2["params"].(map[string]interface{}); p["nodeId"] != "5:6" {
		t.Fatalf("scan root nodeId must be preserved in params, got %#v", p)
	}

	// op-level nodeIds already set → top-level wins, params target untouched.
	op3 := map[string]interface{}{
		"type":    "delete_nodes",
		"nodeIds": []interface{}{"7:8"},
		"params":  map[string]interface{}{"nodeIds": []interface{}{"9:10"}},
	}
	normalizeBatchNodeIDs([]interface{}{op3})
	if got := op3["nodeIds"].([]interface{}); len(got) != 1 || got[0] != "7:8" {
		t.Fatalf("op-level nodeIds must win, got %#v", op3["nodeIds"])
	}
}

// End-to-end: a delete_nodes composed with the target in params (as get_batch_op_spec
// suggests) now validates, because the hoist runs on the validateOnly path too.
func TestRegisterBatchTools_HoistsParamsNodeIDsValidateOnly(t *testing.T) {
	s, _ := newTestServer(t)
	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{"type": "delete_nodes", "params": map[string]any{"nodeIds": []any{"1:2"}}},
		},
	})
	if res.IsError {
		t.Fatalf("validateOnly with params.nodeIds should pass after hoist, got: %s", resultText(t, res))
	}
}

func TestRegisterBatchTools_OpsRequired(t *testing.T) {
	s, _ := newTestServer(t)

	// No `ops` at all → handler's non-empty-ops gate fires.
	res := callToolResult(t, s, "batch", map[string]any{})
	if !res.IsError {
		t.Fatal("expected IsError=true when ops is missing")
	}
	if txt := resultText(t, res); txt == "" {
		t.Fatal("expected a non-empty error message for missing ops")
	}
}

func TestRegisterBatchTools_OpsEmptyArray(t *testing.T) {
	s, _ := newTestServer(t)

	// Present but empty → same non-empty-ops gate.
	res := callToolResult(t, s, "batch", map[string]any{"ops": []any{}})
	if !res.IsError {
		t.Fatal("expected IsError=true when ops is an empty array")
	}
}

func TestRegisterBatchTools_OpsMustBeArray(t *testing.T) {
	s, _ := newTestServer(t)

	// Wrong type (string, not array) → the []interface{} type assertion fails
	// and the handler returns the same non-empty-ops error.
	res := callToolResult(t, s, "batch", map[string]any{"ops": "not-an-array"})
	if !res.IsError {
		t.Fatal("expected IsError=true when ops is a string instead of an array")
	}
}

func TestRegisterBatchTools_OpItemMustBeObject(t *testing.T) {
	s, _ := newTestServer(t)

	// An op that is not an object → per-op gate ops[0] must be an object.
	res := callToolResult(t, s, "batch", map[string]any{"ops": []any{"bogus"}})
	if !res.IsError {
		t.Fatal("expected IsError=true when an op is not an object")
	}
}

func TestRegisterBatchTools_OpTypeRequired(t *testing.T) {
	s, _ := newTestServer(t)

	// An op object missing `type` → per-op gate ops[0] missing string `type`.
	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{"params": map[string]any{"name": "Card"}}},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true when an op is missing its type")
	}
}

func TestRegisterBatchTools_NoNestedBatch(t *testing.T) {
	s, _ := newTestServer(t)

	// A nested batch op is rejected at the Go-side gate.
	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{"type": "batch"}},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for a nested batch op")
	}
}

func TestRegisterBatchTools_RejectsUnknownParamPerOp(t *testing.T) {
	s, _ := newTestServer(t)

	// A create_text op inside a batch using the Plugin-API name `characters`
	// (instead of `text`) must be rejected at the Go-side gate — otherwise the
	// silent-drop invisible-node bug survives inside batch (it bypasses ValidateRPC).
	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type":   "create_text",
			"params": map[string]any{"characters": "hi", "fontSize": float64(14)},
		}},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for create_text with `characters` inside batch")
	}
	if txt := resultText(t, res); !strings.Contains(txt, "characters") || !strings.Contains(txt, "text") {
		t.Errorf("error %q should name the bad param and the correct one", txt)
	}

	// A valid create_text op passes the gate (then reaches the bridge send, which
	// fails with no backend — that's a different, non-validation error path).
	okRes := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type":   "create_text",
			"params": map[string]any{"text": "hi", "fillColor": "#FFFFFF"},
		}},
	})
	if okRes.IsError {
		if txt := resultText(t, okRes); strings.Contains(txt, "unknown param") {
			t.Errorf("valid create_text params must not be rejected as unknown: %q", txt)
		}
	}
}

func TestRegisterBatchTools_RejectsUnknownOpType(t *testing.T) {
	s, _ := newTestServer(t)
	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{"type": "not_a_real_op"}},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for an unknown op type")
	}
	if txt := resultText(t, res); !strings.Contains(txt, "unknown op type") {
		t.Fatalf("expected unknown-op message, got %q", txt)
	}
}

func TestRegisterBatchTools_RejectsScriptLikeFields(t *testing.T) {
	s, _ := newTestServer(t)
	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type":   "create_frame",
			"params": map[string]any{"name": "Card", "script": "figma.createFrame()"},
		}},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for script-like params")
	}
	if txt := resultText(t, res); !strings.Contains(txt, "script-like") {
		t.Fatalf("expected script-like error, got %q", txt)
	}
}

func TestRegisterBatchTools_RejectsBadRefsBeforeBridge(t *testing.T) {
	s, _ := newTestServer(t)
	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type":    "set_fills",
			"nodeIds": []any{"$0.id"},
			"params":  map[string]any{"color": "#fff"},
		}},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for a self ref")
	}
	if txt := resultText(t, res); !strings.Contains(txt, "has not run yet") {
		t.Fatalf("expected ref error, got %q", txt)
	}
}

func TestRegisterBatchTools_RejectsMalformedPlanShapesBeforeBridge(t *testing.T) {
	cases := []struct {
		name      string
		args      map[string]any
		wantError string
	}{
		{
			name: "nodeIds must be an array",
			args: map[string]any{
				"ops": []any{map[string]any{
					"type":    "set_fills",
					"nodeIds": "1:1",
					"params":  map[string]any{"color": "#fff"},
				}},
			},
			wantError: "nodeIds",
		},
		{
			name: "params must be an object",
			args: map[string]any{
				"ops": []any{map[string]any{
					"type":   "create_frame",
					"params": []any{},
				}},
			},
			wantError: "params",
		},
		{
			name: "unknown op field rejected",
			args: map[string]any{
				"ops": []any{map[string]any{
					"type":   "create_frame",
					"params": map[string]any{"name": "Card"},
					"extra":  true,
				}},
			},
			wantError: "unknown op field",
		},
		{
			name: "nested script-like key rejected",
			args: map[string]any{
				"ops": []any{map[string]any{
					"type": "create_frame",
					"params": map[string]any{
						"name": "Card",
						"meta": map[string]any{"code": "figma.createFrame()"},
					},
				}},
			},
			wantError: "script-like",
		},
		{
			name: "case-insensitive script-like key rejected",
			args: map[string]any{
				"ops": []any{map[string]any{
					"type": "create_frame",
					"params": map[string]any{
						"meta": map[string]any{"Function": "return figma.currentPage"},
					},
				}},
			},
			wantError: "script-like",
		},
		{
			name: "script-like key nested in array rejected",
			args: map[string]any{
				"ops": []any{map[string]any{
					"type": "create_frame",
					"params": map[string]any{
						"children": []any{map[string]any{"js": "figma.createFrame()"}},
					},
				}},
			},
			wantError: "script-like",
		},
		{
			name: "map over must be a ref string or array",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "search_nodes", "params": map[string]any{"query": "Button"}},
					map[string]any{
						"type": "map",
						"over": map[string]any{"not": "supported"},
						"do": map[string]any{
							"type":    "set_visible",
							"nodeIds": []any{"$item.id"},
							"params":  map[string]any{"visible": true},
						},
					},
				},
			},
			wantError: "map.over",
		},
		{
			name: "map as must be a string",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "search_nodes", "params": map[string]any{"query": "Button"}},
					map[string]any{
						"type": "map",
						"over": "$0.nodes[*]",
						"as":   false,
						"do": map[string]any{
							"type":    "set_visible",
							"nodeIds": []any{"$item.id"},
							"params":  map[string]any{"visible": true},
						},
					},
				},
			},
			wantError: "map.as",
		},
		{
			name: "map do param validation uses catalog",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_text_nodes", "params": map[string]any{"nodeId": "1:1"}},
					map[string]any{
						"type": "map",
						"over": "$0.textNodes[*]",
						"as":   "item",
						"do": map[string]any{
							"type":    "set_text",
							"nodeIds": []any{"$item.id"},
							"params":  map[string]any{"characters": "bad"},
						},
					},
				},
			},
			wantError: "characters",
		},
		{
			name: "map do unknown named binding rejected",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_text_nodes", "params": map[string]any{"nodeId": "1:1"}},
					map[string]any{
						"type": "map",
						"over": "$0.textNodes[*]",
						"as":   "item",
						"do": map[string]any{
							"type":    "set_text",
							"nodeIds": []any{"$row.id"},
							"params":  map[string]any{"text": "$row.characters"},
						},
					},
				},
			},
			wantError: "unknown map binding",
		},
		{
			name: "map do named projection rejected",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_nodes_by_types", "params": map[string]any{"nodeId": "1:1", "types": []any{"FRAME"}}},
					map[string]any{
						"type": "map",
						"over": "$0.matchingNodes[*]",
						"as":   "item",
						"do": map[string]any{
							"type":    "set_visible",
							"nodeIds": []any{"$item.children[*].id"},
							"params":  map[string]any{"visible": true},
						},
					},
				},
			},
			wantError: "named binding projections are not supported",
		},
		{
			name: "map do must be an object",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_text_nodes", "params": map[string]any{"nodeId": "1:1"}},
					map[string]any{
						"type": "map",
						"over": "$0.textNodes[*]",
						"as":   "item",
						"do":   []any{},
					},
				},
			},
			wantError: "map.do",
		},
		{
			name: "map do is required",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_text_nodes", "params": map[string]any{"nodeId": "1:1"}},
					map[string]any{
						"type": "map",
						"over": "$0.textNodes[*]",
					},
				},
			},
			wantError: "map.do",
		},
		{
			name: "top-level named binding ref rejected",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "set_text", "nodeIds": []any{"1:1"}, "params": map[string]any{"text": "$item.characters"}},
				},
			},
			wantError: "only allowed inside map.do",
		},
		{
			name: "map as must be identifier",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_text_nodes", "params": map[string]any{"nodeId": "1:1"}},
					map[string]any{
						"type": "map",
						"over": "$0.textNodes[*]",
						"as":   "1item",
						"do": map[string]any{
							"type":    "set_visible",
							"nodeIds": []any{"$item.id"},
							"params":  map[string]any{"visible": true},
						},
					},
				},
			},
			wantError: "map.as",
		},
		{
			name: "map as cannot shadow index",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_text_nodes", "params": map[string]any{"nodeId": "1:1"}},
					map[string]any{
						"type": "map",
						"over": "$0.textNodes[*]",
						"as":   "index",
						"do": map[string]any{
							"type":    "set_text",
							"nodeIds": []any{"$index.id"},
							"params":  map[string]any{"text": "$index"},
						},
					},
				},
			},
			wantError: "reserved",
		},
		{
			name: "map do cannot be another map",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_nodes_by_types", "params": map[string]any{"nodeId": "1:1", "types": []any{"FRAME"}}},
					map[string]any{
						"type": "map",
						"over": "$0.matchingNodes[*]",
						"as":   "item",
						"do": map[string]any{
							"type": "map",
							"over": "$item.children",
							"do": map[string]any{
								"type":    "set_visible",
								"nodeIds": []any{"$item.id"},
								"params":  map[string]any{"visible": true},
							},
						},
					},
				},
			},
			wantError: "map cannot be nested",
		},
		{
			name: "malformed projection rejected",
			args: map[string]any{
				"ops": []any{
					map[string]any{"type": "scan_nodes_by_types", "params": map[string]any{"nodeId": "1:1", "types": []any{"FRAME"}}},
					map[string]any{"type": "set_visible", "nodeIds": []any{"$0.matchingNodes[*].children[*].id"}, "params": map[string]any{"visible": true}},
				},
			},
			wantError: "exactly one [*]",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s, captured := newBatchTestServerWithBackend(t, RPCResponse{
				Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
			})
			res := callToolResult(t, s, "batch", tc.args)
			if !res.IsError {
				t.Fatalf("expected validation error, got success: %s", resultText(t, res))
			}
			if txt := resultText(t, res); !strings.Contains(txt, tc.wantError) {
				t.Fatalf("expected error containing %q, got %q", tc.wantError, txt)
			}
			if captured.Tool != "" {
				t.Fatalf("malformed plan should not call bridge, captured tool %q", captured.Tool)
			}
		})
	}
}

func TestRegisterBatchTools_ValidateOnlyValidationFailureDoesNotCallBridge(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{map[string]any{
			"type":   "create_text",
			"params": map[string]any{"characters": "bad"},
		}},
	})
	if !res.IsError {
		t.Fatalf("expected validateOnly to return validation error, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly validation failure should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_MapOpWithNamedRefsValidateOnly(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{
				"type":   "search_nodes",
				"params": map[string]any{"nodeId": "1:1", "query": "Button", "limit": float64(10)},
			},
			map[string]any{
				"type": "map",
				"over": "$0.nodes[*]",
				"as":   "item",
				"do": map[string]any{
					"type":    "set_visible",
					"nodeIds": []any{"$item.id"},
					"params":  map[string]any{"visible": true},
				},
			},
		},
	})
	if res.IsError {
		t.Fatalf("expected valid map op, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_MapOpLiteralOverAndMidStringLiteralsValidateOnly(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{
				"type": "map",
				"over": []any{
					map[string]any{"id": "1:1", "name": "One"},
					map[string]any{"id": "1:2", "name": "Two"},
				},
				"as": "item",
				"do": map[string]any{
					"type":    "set_text",
					"nodeIds": []any{"$item.id"},
					"params":  map[string]any{"text": "Section $index"},
				},
			},
		},
	})
	if res.IsError {
		t.Fatalf("expected map literal over and mid-string literal passthrough to validate, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_ValidateOnlyDoesNotCallBridge(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{map[string]any{
			"type":   "create_frame",
			"params": map[string]any{"name": "Card"},
		}},
	})
	if res.IsError {
		t.Fatalf("expected validateOnly success, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal validateOnly result: %v", err)
	}
	if got["valid"] != true {
		t.Fatalf("expected valid=true, got %v", got)
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("validateOnly must return structuredContent map, got %T", res.StructuredContent)
	}
	if structured["valid"] != true || structured["opCount"] != float64(1) {
		t.Fatalf("unexpected validateOnly structuredContent: %#v", structured)
	}
}

func TestRegisterBatchTools_ValidateOnlyRejectsBadImportKeysBeforeBridge(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	cases := []struct {
		name string
		op   map[string]any
		want string
	}{
		{
			name: "component truncated key",
			op:   map[string]any{"type": "import_component_by_key", "params": map[string]any{"key": "8b931898634bdc63"}},
			want: "truncated",
		},
		{
			name: "style node id",
			op:   map[string]any{"type": "import_style_by_key", "params": map[string]any{"key": "410:49695"}},
			want: "node id",
		},
		{
			name: "style bad hex",
			op:   map[string]any{"type": "import_style_by_key", "params": map[string]any{"key": strings.Repeat("z", 40)}},
			want: "malformed style key",
		},
		{
			name: "variable node id",
			op:   map[string]any{"type": "import_variable_by_key", "params": map[string]any{"key": "410:49695"}},
			want: "node id",
		},
		{
			name: "variable empty key",
			op:   map[string]any{"type": "import_variable_by_key", "params": map[string]any{"key": ""}},
			want: "key is required",
		},
		{
			name: "component bad hex",
			op:   map[string]any{"type": "import_component_by_key", "params": map[string]any{"key": strings.Repeat("z", 40)}},
			want: "malformed component key",
		},
		{
			name: "component bad assetType",
			op:   map[string]any{"type": "import_component_by_key", "params": map[string]any{"key": strings.Repeat("a", 40), "assetType": "STYLE"}},
			want: "assetType",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := callToolResult(t, s, "batch", map[string]any{
				"validateOnly": true,
				"ops":          []any{tc.op},
			})
			if !res.IsError {
				t.Fatalf("expected import key validation error, got %s", resultText(t, res))
			}
			if txt := resultText(t, res); !strings.Contains(txt, tc.want) {
				t.Fatalf("expected error containing %q, got %q", tc.want, txt)
			}
			if captured.Tool != "" {
				t.Fatalf("invalid validateOnly batch should not call bridge, captured tool %q", captured.Tool)
			}
		})
	}
}

func TestRegisterBatchTools_ValidateOnlyAllowsSchemaRefs(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{"type": "get_node", "nodeIds": []any{"1:1"}, "params": map[string]any{"depth": float64(1)}},
			map[string]any{
				"type":    "resize_nodes",
				"nodeIds": []any{"$0.id"},
				"params": map[string]any{
					"width":  "$0.bounds.width",
					"height": "$0.bounds.height",
				},
			},
		},
	})
	if res.IsError {
		t.Fatalf("schema validation should allow runtime refs for typed params, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_ValidateOnlyAllowsRefImportKeys(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{"type": "search_nodes", "params": map[string]any{"nodeId": "1:1", "query": "Button"}},
			map[string]any{"type": "import_component_by_key", "params": map[string]any{"key": "$0.nodes.0.componentKey"}},
		},
	})
	if res.IsError {
		t.Fatalf("ref import key should validate before runtime resolution, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_ValidateOnlyAllowsNamedRefImportKeysInsideMap(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{
				"type": "map",
				"over": []any{
					map[string]any{"key": strings.Repeat("a", 40), "assetType": "COMPONENT"},
					map[string]any{"key": strings.Repeat("b", 40), "assetType": "COMPONENT_SET"},
				},
				"as": "asset",
				"do": map[string]any{
					"type":   "import_component_by_key",
					"params": map[string]any{"key": "$asset.key", "assetType": "$asset.assetType"},
				},
			},
		},
	})
	if res.IsError {
		t.Fatalf("named binding import refs should validate before runtime resolution, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_InjectsCatalogAssetTypeInsideBatch(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})
	key := strings.Repeat("d", 40)
	rememberLibraryCatalogKeys(map[string]any{
		key: map[string]any{"type": "COMPONENT_SET", "name": "Button"},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type":   "import_component_by_key",
			"params": map[string]any{"key": key},
		}},
	})
	if res.IsError {
		t.Fatalf("valid batch import unexpectedly failed: %s", resultText(t, res))
	}
	forwardedOps, ok := captured.Params["ops"].([]any)
	if !ok || len(forwardedOps) != 1 {
		t.Fatalf("forwarded params.ops = %#v", captured.Params["ops"])
	}
	op0, ok := forwardedOps[0].(map[string]any)
	if !ok {
		t.Fatalf("forwarded op[0] = %#v", forwardedOps[0])
	}
	params, ok := op0["params"].(map[string]any)
	if !ok {
		t.Fatalf("forwarded op[0].params = %#v", op0["params"])
	}
	if params["assetType"] != "COMPONENT_SET" {
		t.Fatalf("assetType = %v, want COMPONENT_SET", params["assetType"])
	}
}

func TestRegisterBatchTools_InjectsCatalogAssetTypeInsideMapDo(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})
	key := strings.Repeat("e", 40)
	rememberLibraryCatalogKeys(map[string]any{
		key: map[string]any{"type": "COMPONENT_SET", "name": "Card"},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type": "map",
			"over": []any{
				map[string]any{"id": "1:1"},
				map[string]any{"id": "1:2"},
			},
			"do": map[string]any{
				"type":   "import_component_by_key",
				"params": map[string]any{"key": key},
			},
		}},
	})
	if res.IsError {
		t.Fatalf("valid nested batch import unexpectedly failed: %s", resultText(t, res))
	}
	forwardedOps, ok := captured.Params["ops"].([]any)
	if !ok || len(forwardedOps) != 1 {
		t.Fatalf("forwarded params.ops = %#v", captured.Params["ops"])
	}
	op0, ok := forwardedOps[0].(map[string]any)
	if !ok {
		t.Fatalf("forwarded op[0] = %#v", forwardedOps[0])
	}
	do, ok := op0["do"].(map[string]any)
	if !ok {
		t.Fatalf("forwarded map.do = %#v", op0["do"])
	}
	params, ok := do["params"].(map[string]any)
	if !ok {
		t.Fatalf("forwarded map.do.params = %#v", do["params"])
	}
	if params["assetType"] != "COMPONENT_SET" {
		t.Fatalf("nested assetType = %v, want COMPONENT_SET", params["assetType"])
	}
}

func TestRegisterBatchTools_ValidateOnlyHiddenCatalogOp(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{map[string]any{
			"type":    "set_corner_radius",
			"nodeIds": []any{"1:1"},
			"params":  map[string]any{"cornerRadius": float64(8)},
		}},
	})
	if res.IsError {
		t.Fatalf("expected hidden catalog op validateOnly success, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal validateOnly result: %v", err)
	}
	if got["valid"] != true || got["opCount"] != float64(1) {
		t.Fatalf("expected valid hidden-op report, got %v", got)
	}

	bad := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{map[string]any{
			"type":    "set_corner_radius",
			"nodeIds": []any{"1:1"},
			"params":  map[string]any{"radius": float64(8)},
		}},
	})
	if !bad.IsError {
		t.Fatalf("expected bad hidden catalog op to fail validation, got %s", resultText(t, bad))
	}
	if txt := resultText(t, bad); !strings.Contains(txt, "radius") || !strings.Contains(txt, "cornerRadius") {
		t.Fatalf("expected unknown param hint for radius, got %q", txt)
	}
	if captured.Tool != "" {
		t.Fatalf("failed validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_ValidateOnlyEnforcesCatalogSchema(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	cases := []struct {
		name string
		op   map[string]any
		want string
	}{
		{
			name: "missing required param",
			op:   map[string]any{"type": "boolean_operation", "nodeIds": []any{"1:1", "1:2"}},
			want: "operation",
		},
		{
			name: "invalid enum",
			op:   map[string]any{"type": "boolean_operation", "nodeIds": []any{"1:1", "1:2"}, "params": map[string]any{"operation": "MERGE"}},
			want: "UNION",
		},
		{
			name: "wrong primitive type",
			op:   map[string]any{"type": "rotate_nodes", "nodeIds": []any{"1:1"}, "params": map[string]any{"rotation": "90"}},
			want: "number",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := callToolResult(t, s, "batch", map[string]any{
				"validateOnly": true,
				"ops":          []any{tc.op},
			})
			if !res.IsError {
				t.Fatalf("expected schema validation error, got %s", resultText(t, res))
			}
			if txt := resultText(t, res); !strings.Contains(txt, tc.want) {
				t.Fatalf("expected error containing %q, got %q", tc.want, txt)
			}
			if captured.Tool != "" {
				t.Fatalf("failed validateOnly should not call bridge, captured tool %q", captured.Tool)
			}
		})
	}
}

func TestRegisterBatchTools_ValidateOnlyEnforcesDirectSemanticGuards(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	cases := []struct {
		name string
		op   map[string]any
		want string
	}{
		{
			name: "set_fills requires color or paints",
			op:   map[string]any{"type": "set_fills", "nodeIds": []any{"1:1"}, "params": map[string]any{}},
			want: "color or paints",
		},
		{
			name: "set_strokes requires color or paints",
			op:   map[string]any{"type": "set_strokes", "nodeIds": []any{"1:1"}, "params": map[string]any{}},
			want: "color or paints",
		},
		{
			name: "delete_variable requires variableId or collectionId",
			op:   map[string]any{"type": "delete_variable", "params": map[string]any{}},
			want: "variableId or collectionId",
		},
		{
			name: "delete_page requires pageId or pageName",
			op:   map[string]any{"type": "delete_page", "params": map[string]any{}},
			want: "pageId or pageName",
		},
		{
			name: "rename_page requires page selector",
			op:   map[string]any{"type": "rename_page", "params": map[string]any{"newName": "Sprint"}},
			want: "pageId or pageName",
		},
		{
			name: "set_corner_radius requires at least one radius",
			op:   map[string]any{"type": "set_corner_radius", "nodeIds": []any{"1:1"}, "params": map[string]any{}},
			want: "cornerRadius",
		},
		{
			name: "set_constraints requires at least one axis",
			op:   map[string]any{"type": "set_constraints", "nodeIds": []any{"1:1"}, "params": map[string]any{}},
			want: "horizontal or vertical",
		},
		{
			name: "set_effects validates effect types",
			op: map[string]any{"type": "set_effects", "nodeIds": []any{"1:1"}, "params": map[string]any{
				"effects": []any{map[string]any{"type": "MAGIC_SHADOW"}},
			}},
			want: "DROP_SHADOW",
		},
		{
			name: "update_paint_style requires an actual update",
			op:   map[string]any{"type": "update_paint_style", "params": map[string]any{"styleId": "S:1"}},
			want: "at least one",
		},
		{
			name: "per-op channel is rejected",
			op:   map[string]any{"type": "set_fills", "nodeIds": []any{"1:1"}, "params": map[string]any{"color": "#fff", "channel": "file-a"}},
			want: "channel",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := callToolResult(t, s, "batch", map[string]any{
				"validateOnly": true,
				"ops":          []any{tc.op},
			})
			if !res.IsError {
				t.Fatalf("expected semantic validation error, got %s", resultText(t, res))
			}
			if txt := resultText(t, res); !strings.Contains(txt, tc.want) {
				t.Fatalf("expected error containing %q, got %q", tc.want, txt)
			}
			if captured.Tool != "" {
				t.Fatalf("failed validateOnly should not call bridge, captured tool %q", captured.Tool)
			}
		})
	}
}

func TestRegisterBatchTools_ValidateOnlyAcceptsPaintStylePaintsUpdate(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{
				"type": "update_paint_style",
				"params": map[string]any{
					"styleId": "S:1",
					"paints":  []any{map[string]any{"type": "SOLID"}},
				},
			},
		},
	})
	if res.IsError {
		t.Fatalf("paints[] is a valid update_paint_style payload and must not be rejected: %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_RejectsOversizedBatchBeforePlugin(t *testing.T) {
	t.Setenv("FIGMA_MCP_BATCH_MAX_OPS", "1")
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{"type": "get_metadata"},
			map[string]any{"type": "get_pages"},
		},
	})
	if !res.IsError {
		t.Fatalf("expected oversized batch to fail, got %s", resultText(t, res))
	}
	if txt := resultText(t, res); !strings.Contains(txt, "exceeding the cap") || !strings.Contains(txt, "FIGMA_MCP_BATCH_MAX_OPS") {
		t.Fatalf("expected actionable max-ops error, got %q", txt)
	}
	if captured.Tool != "" {
		t.Fatalf("oversized validateOnly batch should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_RejectsOversizedBatchPayloadBeforePlugin(t *testing.T) {
	t.Setenv("FIGMA_MCP_BATCH_MAX_BYTES", "128")
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{
			map[string]any{
				"type": "create_text",
				"params": map[string]any{
					"text": strings.Repeat("x", 256),
				},
			},
		},
	})
	if !res.IsError {
		t.Fatalf("expected oversized batch payload to fail, got %s", resultText(t, res))
	}
	if txt := resultText(t, res); !strings.Contains(txt, "encoded ops payload") || !strings.Contains(txt, "FIGMA_MCP_BATCH_MAX_BYTES") {
		t.Fatalf("expected actionable max-bytes error, got %q", txt)
	}
	if captured.Tool != "" {
		t.Fatalf("oversized validateOnly batch should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_RejectsUnknownTopLevelBatchParam(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"timeoutMs":    float64(5000),
		"ops":          []any{map[string]any{"type": "get_metadata"}},
	})
	if !res.IsError {
		t.Fatalf("expected unknown top-level param to fail, got %s", resultText(t, res))
	}
	if txt := resultText(t, res); !strings.Contains(txt, "unknown top-level param") || !strings.Contains(txt, "timeoutMs") {
		t.Fatalf("expected unknown-param error, got %q", txt)
	}
	if captured.Tool != "" {
		t.Fatalf("failed validateOnly batch should not call bridge, captured tool %q", captured.Tool)
	}
}

func TestRegisterBatchTools_NormalizesKnownIDParamsBeforeValidationAndForwarding(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{
			"type":    "create_instance",
			"nodeIds": []any{},
			"params": map[string]any{
				"componentId": "10-20",
				"parentId":    "30-40",
			},
		}},
	})
	if res.IsError {
		t.Fatalf("hyphen IDs in known params should normalize before validation, got %s", resultText(t, res))
	}
	forwardedOps, ok := captured.Params["ops"].([]any)
	if !ok || len(forwardedOps) != 1 {
		t.Fatalf("forwarded params.ops = %#v", captured.Params["ops"])
	}
	op0, ok := forwardedOps[0].(map[string]any)
	if !ok {
		t.Fatalf("forwarded op[0] = %#v", forwardedOps[0])
	}
	params, ok := op0["params"].(map[string]any)
	if !ok {
		t.Fatalf("forwarded op[0].params = %#v", op0["params"])
	}
	if params["componentId"] != "10:20" || params["parentId"] != "30:40" {
		t.Fatalf("IDs were not normalized in forwarded params: %#v", params)
	}
}

func TestRegisterBatchTools_AllowsScriptLikeFreeformComponentProperties(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"validateOnly": true,
		"ops": []any{map[string]any{
			"type":    "set_instance_properties",
			"nodeIds": []any{"1:1"},
			"params": map[string]any{
				"properties": map[string]any{
					"Code":     "A1",
					"Function": "Primary",
				},
			},
		}},
	})
	if res.IsError {
		t.Fatalf("free-form component property names should not be rejected as script-like keys, got %s", resultText(t, res))
	}
	if captured.Tool != "" {
		t.Fatalf("validateOnly should not call bridge, captured tool %q", captured.Tool)
	}
}

// ── Bridge call + response passthrough ────────────────────────────────────────

// newBatchTestServerWithBackend stands up an httptest /rpc backend and points the
// node's follower (the Unknown-role send path) at it, so a valid batch request is
// actually forwarded over the RPC seam. The handler captures the forwarded
// RPCRequest and replies with the given RPCResponse.
func newBatchTestServerWithBackend(t *testing.T, reply RPCResponse) (*server.MCPServer, *RPCRequest) {
	t.Helper()

	captured := &RPCRequest{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, captured); err != nil {
			t.Errorf("backend: unmarshal RPCRequest: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(backend.Close)

	s := server.NewMCPServer("test", "0.0.1")
	node := NewNode("127.0.0.1", 19940, "test")
	// Unknown role → node.Send routes to the follower; repoint it at our backend.
	node.follower = NewFollower(backend.URL)
	RegisterTools(s, node)
	return s, captured
}

func TestRegisterBatchTools_ForwardsToBridge(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	ops := []any{
		map[string]any{"type": "create_frame", "params": map[string]any{"name": "Card"}},
	}
	res := callToolResult(t, s, "batch", map[string]any{"ops": ops})

	if res.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(t, res))
	}

	// The request must be forwarded with tool type "batch" and an ops array param.
	if captured.Tool != "batch" {
		t.Errorf("forwarded Tool = %q, want %q", captured.Tool, "batch")
	}
	forwardedOps, ok := captured.Params["ops"].([]any)
	if !ok {
		t.Fatalf("forwarded params.ops is not an array: %#v", captured.Params["ops"])
	}
	if len(forwardedOps) != 1 {
		t.Fatalf("forwarded ops length = %d, want 1", len(forwardedOps))
	}
	op0, _ := forwardedOps[0].(map[string]any)
	if op0["type"] != "create_frame" {
		t.Errorf("forwarded ops[0].type = %v, want create_frame", op0["type"])
	}
}

func TestRegisterBatchTools_ResponsePassthrough(t *testing.T) {
	want := map[string]any{
		"okCount":   float64(2),
		"failCount": float64(0),
		"results": []any{
			map[string]any{"i": float64(0), "type": "create_frame", "data": map[string]any{"id": "10:5"}},
			map[string]any{"i": float64(1), "type": "set_fills", "data": map[string]any{"id": "10:5"}},
		},
	}
	s, _ := newBatchTestServerWithBackend(t, RPCResponse{Data: want})

	ops := []any{
		map[string]any{"type": "create_frame", "params": map[string]any{"name": "Card"}},
		map[string]any{"type": "set_fills", "nodeIds": []any{"$0.id"}, "params": map[string]any{"color": "#fff"}},
	}
	res := callToolResult(t, s, "batch", map[string]any{"ops": ops})
	if res.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(t, res))
	}

	// The bridge Data must come back to the caller unchanged.
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal tool result text: %v", err)
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("response not passed through unchanged:\n got=%s\nwant=%s", gotJSON, wantJSON)
	}
}

func TestRegisterBatchTools_ContinueOnErrorForwarded(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{"okCount": float64(1), "failCount": float64(0)},
	})

	ops := []any{map[string]any{"type": "create_frame"}}
	res := callToolResult(t, s, "batch", map[string]any{
		"ops":             ops,
		"continueOnError": true,
	})
	if res.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(t, res))
	}
	if v, ok := captured.Params["continueOnError"].(bool); !ok || !v {
		t.Errorf("forwarded params.continueOnError = %v (ok=%v), want true", captured.Params["continueOnError"], ok)
	}
}

// ── Lever 4 demotion gate (set_corner_radius) ─────────────────────────────────
//
// These two tests together prove the "demote to batch-only op" invariant:
//   (a) set_corner_radius is GONE from the MCP tool surface (tools/list), while a
//       sibling write-modify tool (set_opacity) is still present — so enumeration
//       still works and only the one tool was removed.
//   (b) the catalog-backed batch validator ACCEPTS an op with
//       type:"set_corner_radius" and forwards it to the bridge intact inside the
//       batch ops array.

// TestLever4_DirectInvocationDemoted asserts set_corner_radius is absent from the
// registered MCP tool list, with set_opacity present as the control.
func TestLever4_DirectInvocationDemoted(t *testing.T) {
	s, _ := newTestServer(t)
	names := listToolNames(t, s)

	has := func(target string) bool {
		for _, n := range names {
			if n == target {
				return true
			}
		}
		return false
	}

	if has("set_corner_radius") {
		t.Errorf("set_corner_radius is DEMOTED — it must NOT appear on the MCP tool surface; got %v", names)
	}
	// Control: a sibling write-modify tool must still be registered, proving the
	// enumeration path works and we did not break tools/list wholesale.
	if !has("set_opacity") {
		t.Errorf("control tool set_opacity must remain registered; got %v", names)
	}
}

// TestLever4_BatchDispatchPreserved asserts the batch relay accepts a cataloged
// op that is hidden from tools/list and forwards it to the bridge with type
// "set_corner_radius" intact inside the ops array.
func TestLever4_BatchDispatchPreserved(t *testing.T) {
	s, captured := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{
			"okCount":   float64(1),
			"failCount": float64(0),
			"results": []any{
				map[string]any{"i": float64(0), "type": "set_corner_radius", "data": map[string]any{"results": []any{}}},
			},
		},
	})

	ops := []any{
		map[string]any{
			"type":    "set_corner_radius",
			"nodeIds": []any{"1:1"},
			"params":  map[string]any{"cornerRadius": float64(8)},
		},
	}
	res := callToolResult(t, s, "batch", map[string]any{"ops": ops})

	// NOT rejected by the catalog-backed op gate — the batch round-trip succeeded.
	if res.IsError {
		t.Fatalf("batch must accept a demoted op type, not reject it; got error: %s", resultText(t, res))
	}

	// The whole batch is one RPC with Tool "batch"; the demoted op rides inside ops.
	if captured.Tool != "batch" {
		t.Errorf("forwarded Tool = %q, want %q", captured.Tool, "batch")
	}
	forwardedOps, ok := captured.Params["ops"].([]any)
	if !ok || len(forwardedOps) != 1 {
		t.Fatalf("forwarded params.ops malformed: %#v", captured.Params["ops"])
	}
	op0, _ := forwardedOps[0].(map[string]any)
	if op0["type"] != "set_corner_radius" {
		t.Errorf("forwarded ops[0].type = %v, want set_corner_radius (relayed intact)", op0["type"])
	}
}

// lever4DemotedOps is the demote set from the legacy top-level trim. Each must be
// ABSENT from tools/list yet ACCEPTED by the batch relay as an op `type`.
// (set_constraints was promoted back to a top-level tool and is no longer demoted.)
var lever4DemotedOps = []string{
	"set_corner_radius",
	"lock_nodes",
	"unlock_nodes",
	"rotate_nodes",
	"reorder_nodes",
	"set_blend_mode",
	"rename_node",
	"boolean_operation",
	"detach_instance",
	"ungroup_nodes",
	"delete_style",
	"delete_variable",
	"delete_page",
	"rename_page",
	"remove_reactions",
}

// lever4KeptControls are the two write-modify tools deliberately KEPT on the
// top-level surface — proving the trim removed only the demote set, not siblings.
var lever4KeptControls = []string{"set_visible", "set_opacity"}

// TestLever4_AllDemotedAbsentFromToolList asserts every demoted tool is gone from
// tools/list, while the kept controls remain present.
func TestLever4_AllDemotedAbsentFromToolList(t *testing.T) {
	s, _ := newTestServer(t)
	names := listToolNames(t, s)
	has := func(target string) bool {
		for _, n := range names {
			if n == target {
				return true
			}
		}
		return false
	}

	for _, op := range lever4DemotedOps {
		op := op
		t.Run("absent/"+op, func(t *testing.T) {
			if has(op) {
				t.Errorf("%s is DEMOTED — it must NOT appear on the MCP tool surface; got %v", op, names)
			}
		})
	}

	for _, ctrl := range lever4KeptControls {
		ctrl := ctrl
		t.Run("present/"+ctrl, func(t *testing.T) {
			if !has(ctrl) {
				t.Errorf("control tool %s must remain registered (not demoted); got %v", ctrl, names)
			}
		})
	}
}

// TestLever4_AllDemotedAcceptedByBatch asserts the batch relay ACCEPTS an op of
// each demoted type through the catalog validator and forwards the type intact
// inside the ops array. This is what guarantees each demoted tool stays fully
// usable via `batch`.
func TestLever4_AllDemotedAcceptedByBatch(t *testing.T) {
	for _, op := range lever4DemotedOps {
		op := op
		t.Run(op, func(t *testing.T) {
			s, captured := newBatchTestServerWithBackend(t, RPCResponse{
				Data: map[string]any{
					"okCount":   float64(1),
					"failCount": float64(0),
					"results": []any{
						map[string]any{"i": float64(0), "type": op, "data": map[string]any{}},
					},
				},
			})

			res := callToolResult(t, s, "batch", map[string]any{
				"ops": []any{minimalDemotedBatchOp(op)},
			})
			if res.IsError {
				t.Fatalf("batch must accept demoted op %q, not reject it; got error: %s", op, resultText(t, res))
			}
			if captured.Tool != "batch" {
				t.Errorf("forwarded Tool = %q, want %q", captured.Tool, "batch")
			}
			forwardedOps, ok := captured.Params["ops"].([]any)
			if !ok || len(forwardedOps) != 1 {
				t.Fatalf("forwarded params.ops malformed: %#v", captured.Params["ops"])
			}
			op0, _ := forwardedOps[0].(map[string]any)
			if op0["type"] != op {
				t.Errorf("forwarded ops[0].type = %v, want %q (relayed intact)", op0["type"], op)
			}
		})
	}
}

func minimalDemotedBatchOp(op string) map[string]any {
	payload := map[string]any{"type": op, "nodeIds": []any{"1:1"}}
	switch op {
	case "boolean_operation":
		payload["nodeIds"] = []any{"1:1", "1:2"}
		payload["params"] = map[string]any{"operation": "UNION"}
	case "delete_style":
		delete(payload, "nodeIds")
		payload["params"] = map[string]any{"styleId": "S:abc"}
	case "delete_variable":
		delete(payload, "nodeIds")
		payload["params"] = map[string]any{"variableId": "VariableID:1:2"}
	case "delete_page":
		delete(payload, "nodeIds")
		payload["params"] = map[string]any{"pageId": "0:2"}
	case "rename_node":
		payload["params"] = map[string]any{"name": "Renamed"}
	case "rename_page":
		delete(payload, "nodeIds")
		payload["params"] = map[string]any{"pageId": "0:2", "newName": "Renamed Page"}
	case "reorder_nodes":
		payload["params"] = map[string]any{"order": "bringToFront"}
	case "rotate_nodes":
		payload["params"] = map[string]any{"rotation": float64(90)}
	case "set_blend_mode":
		payload["params"] = map[string]any{"blendMode": "NORMAL"}
	case "set_constraints":
		payload["params"] = map[string]any{"horizontal": "MIN"}
	case "set_corner_radius":
		payload["params"] = map[string]any{"cornerRadius": float64(8)}
	case "remove_reactions":
		payload["params"] = map[string]any{"indices": []any{float64(0)}}
	}
	return payload
}

func TestRegisterBatchTools_FailureHintFolded(t *testing.T) {
	// A partial-failure batch round-trip succeeds at the transport level but the
	// handler folds a recovery hint into resp.Data.
	s, _ := newBatchTestServerWithBackend(t, RPCResponse{
		Data: map[string]any{
			"okCount":   float64(1),
			"failCount": float64(1),
			"failedAt":  float64(1),
		},
	})

	res := callToolResult(t, s, "batch", map[string]any{
		"ops": []any{map[string]any{"type": "create_frame"}},
	})
	if res.IsError {
		t.Fatalf("a partial-failure batch is a successful round-trip; got error: %s", resultText(t, res))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal tool result text: %v", err)
	}
	if _, ok := got["hint"]; !ok {
		t.Errorf("expected a recovery hint folded into the result; got %v", got)
	}
}

func TestBatchFailureHintNoopCases(t *testing.T) {
	if got := batchFailureHint(BridgeResponse{Data: "not a map"}); got != "" {
		t.Fatalf("non-map data should not get a hint, got %q", got)
	}
	if got := batchFailureHint(BridgeResponse{Data: map[string]any{"failCount": float64(0)}}); got != "" {
		t.Fatalf("zero failures should not get a hint, got %q", got)
	}
}
