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
//   (b) the batch op-validation/relay path ACCEPTS an op with
//       type:"set_corner_radius" (no registered-tool allowlist) and forwards it to
//       the bridge intact inside the batch ops array.

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

// TestLever4_BatchDispatchPreserved asserts the batch relay accepts an op of the
// demoted type and forwards it to the bridge with type "set_corner_radius" intact
// inside the ops array (the per-op gate in tools_batch.go has no allowlist).
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

	// NOT rejected by the Go-side op gate — the batch round-trip succeeded.
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

// lever4DemotedOps is the full 16-tool demote set (Lever 4). Each must be ABSENT
// from tools/list yet ACCEPTED by the batch relay as an op `type`.
var lever4DemotedOps = []string{
	"set_corner_radius",
	"lock_nodes",
	"unlock_nodes",
	"rotate_nodes",
	"reorder_nodes",
	"set_blend_mode",
	"set_constraints",
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

// TestLever4_AllDemotedAbsentFromToolList asserts every one of the 16 demoted
// tools is gone from tools/list, while the kept controls remain present.
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
// each demoted type (the per-op gate in tools_batch.go has no allowlist) and
// forwards the type intact inside the ops array. This is what guarantees each
// demoted tool stays fully usable via `batch`.
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
				"ops": []any{map[string]any{"type": op, "nodeIds": []any{"1:1"}}},
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
