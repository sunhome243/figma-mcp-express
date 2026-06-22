package internal

import (
	"strings"
	"testing"
)

// Regression: validateBatchParamsAgainstSchema must NOT enforce origin/status/
// nodeId/nodeIds as per-op required params. nodeId(s) are supplied out-of-band,
// and origin/status are TOP-LEVEL presence params that an op catalog spec can
// inherit as "required" when FIGMA_MCP_PRESENCE_REQUIRED=1 makes `origin`
// required. Without the skip, required `origin` leaked into every inner batch op
// and failed all status heartbeats with `missing required param "origin"`. A
// genuinely-required op param must still be enforced.
func TestValidateBatchParamsSkipsPresenceAndNodeIDRequireds(t *testing.T) {
	const op = "__test_presence_required_op__"
	batchOpCatalog[op] = BatchOpSpec{
		Name: op,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []any{"origin", "status", "nodeId", "nodeIds", "realField"},
			"properties": map[string]any{
				"origin":    map[string]any{"type": "string"},
				"status":    map[string]any{"type": "string"},
				"nodeId":    map[string]any{"type": "string"},
				"nodeIds":   map[string]any{"type": "array"},
				"realField": map[string]any{"type": "string"},
			},
		},
	}
	defer delete(batchOpCatalog, op)

	t.Run("origin/status/nodeId/nodeIds are not required per-op", func(t *testing.T) {
		// Only realField is supplied; the four skipped names are absent → must pass.
		if err := validateBatchParamsAgainstSchema(op, map[string]interface{}{"realField": "x"}, nil); err != nil {
			t.Fatalf("inner op must not require origin/status/nodeId/nodeIds; got: %v", err)
		}
	})

	t.Run("a genuinely-required op param is still enforced", func(t *testing.T) {
		err := validateBatchParamsAgainstSchema(op, map[string]interface{}{}, nil)
		if err == nil {
			t.Fatal("expected an error for the missing realField, got nil")
		}
		if !strings.Contains(err.Error(), "realField") {
			t.Fatalf("error should name the missing realField, got: %v", err)
		}
	})
}

// ── status (presence workflow state) ──────────────────────────────────────────

func TestPickStatus(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]interface{}
		wantVal string
		wantOK  bool
	}{
		{"thinking", map[string]interface{}{"status": "thinking"}, "thinking", true},
		{"waiting_review", map[string]interface{}{"status": "waiting_review"}, "waiting_review", true},
		{"reviewing", map[string]interface{}{"status": "reviewing"}, "reviewing", true},
		{"approved", map[string]interface{}{"status": "approved"}, "approved", true},
		{"escalated", map[string]interface{}{"status": "escalated"}, "escalated", true},
		{"done", map[string]interface{}{"status": "done"}, "done", true},
		{"unknown dropped", map[string]interface{}{"status": "blocked"}, "", false},
		{"empty string dropped", map[string]interface{}{"status": ""}, "", false},
		{"missing key dropped", map[string]interface{}{}, "", false},
		{"non-string dropped", map[string]interface{}{"status": 7}, "", false},
		{"case-sensitive (Thinking != thinking)", map[string]interface{}{"status": "Thinking"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickStatus(tc.args)
			if got != tc.wantVal || ok != tc.wantOK {
				t.Fatalf("pickStatus(%v) = (%q, %v); want (%q, %v)", tc.args, got, ok, tc.wantVal, tc.wantOK)
			}
		})
	}
}

// Guard: every roster status must round-trip through pickStatus so the Go enum
// and the validation list never drift apart. The roster MUST be EXACTLY the 6
// LLM-set workflow states.
func TestPickStatusAcceptsEveryRosterStatus(t *testing.T) {
	want := map[string]bool{
		"thinking": true, "waiting_review": true, "reviewing": true,
		"approved": true, "escalated": true, "done": true,
	}
	if len(rosterStatuses) != len(want) {
		t.Fatalf("rosterStatuses has %d entries, want %d (the 6 LLM-set states)", len(rosterStatuses), len(want))
	}
	for _, s := range rosterStatuses {
		if !want[s] {
			t.Errorf("unexpected roster status %q (not one of the 6 LLM-set states)", s)
		}
		got, ok := pickStatus(map[string]interface{}{"status": s})
		if !ok || got != s {
			t.Errorf("roster status %q rejected by pickStatus (got %q, ok=%v)", s, got, ok)
		}
	}
}

// validateAndPrepareBatchParams (follower /rpc path) must STRIP `status` from batch
// entirely: manual workflow status moved to the dedicated set_presence tool, so
// batch carries identity only. Known or unknown, status is dropped (leader and
// follower paths agree).
func TestValidateAndPrepareBatchParamsStripsStatus(t *testing.T) {
	newParams := func(status interface{}) map[string]interface{} {
		p := map[string]interface{}{
			"ops": []interface{}{
				map[string]interface{}{"type": "get_metadata"},
			},
		}
		if status != nil {
			p["status"] = status
		}
		return p
	}

	for _, status := range []interface{}{"reviewing", "blocked", nil} {
		t.Run("strips status", func(t *testing.T) {
			p := newParams(status)
			if err := validateAndPrepareBatchParams(p); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, present := p["status"]; present {
				t.Errorf("status (%v) must not survive on batch, got %v", status, p["status"])
			}
		})
	}
}

// End-to-end through the batch tool handler: batch does NOT forward `status` (it
// moved to set_presence). Even a valid status is dropped; the handler still
// succeeds. Origin is still forwarded — see TestReadToolForwardsOrigin.
func TestRegisterBatchToolsDoesNotForwardStatus(t *testing.T) {
	okReply := RPCResponse{Data: map[string]any{"okCount": float64(1), "failCount": float64(0)}}
	s, captured := newBatchTestServerWithBackend(t, okReply)
	res := callToolResult(t, s, "batch", map[string]any{
		"ops":    []any{map[string]any{"type": "get_metadata"}},
		"status": "reviewing",
	})
	if res.IsError {
		t.Fatalf("batch with a status arg should still succeed: %s", resultText(t, res))
	}
	if _, present := captured.Params["status"]; present {
		t.Fatalf("batch must not forward status (use set_presence), got %#v", captured.Params["status"])
	}
}

// A read tool (get_metadata) must forward a valid `origin` into the params map
// that reaches the bridge, so reads can be attributed to a named agent. Mirrors
// the batch origin-forwarding test but on the read path.
func TestReadToolForwardsOrigin(t *testing.T) {
	okReply := RPCResponse{Data: map[string]any{"name": "doc"}}

	t.Run("valid origin reaches forwarded params on a read", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "get_metadata", map[string]any{"origin": "grace"})
		if res.IsError {
			t.Fatalf("get_metadata with valid origin errored: %s", resultText(t, res))
		}
		if captured.Tool != "get_metadata" {
			t.Fatalf("forwarded tool = %q, want get_metadata", captured.Tool)
		}
		if captured.Params["origin"] != "grace" {
			t.Fatalf("forwarded params.origin = %#v; want grace", captured.Params["origin"])
		}
	})

	t.Run("unknown origin is not forwarded on a read", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "get_metadata", map[string]any{"origin": "intruder"})
		if res.IsError {
			t.Fatalf("unknown origin should be dropped, not error: %s", resultText(t, res))
		}
		if _, present := captured.Params["origin"]; present {
			t.Fatalf("unknown origin should not be forwarded, got %#v", captured.Params["origin"])
		}
	})

	// An inline-handler read (get_node) must also forward origin alongside its
	// own params, proving applyOrigin is wired on the inline path too.
	t.Run("inline read (get_node) forwards origin", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "get_node", map[string]any{"nodeId": "1:2", "origin": "theo"})
		if res.IsError {
			t.Fatalf("get_node with valid origin errored: %s", resultText(t, res))
		}
		if captured.Params["origin"] != "theo" {
			t.Fatalf("forwarded params.origin = %#v; want theo", captured.Params["origin"])
		}
	})
}

func TestPickOrigin(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]interface{}
		wantVal string
		wantOK  bool
	}{
		{"valid roster member", map[string]interface{}{"origin": "grace"}, "grace", true},
		{"another valid member", map[string]interface{}{"origin": "theo"}, "theo", true},
		{"unknown label dropped", map[string]interface{}{"origin": "bob"}, "", false},
		{"empty string dropped", map[string]interface{}{"origin": ""}, "", false},
		{"missing key dropped", map[string]interface{}{}, "", false},
		{"non-string dropped", map[string]interface{}{"origin": 42}, "", false},
		{"case-sensitive (Grace != grace)", map[string]interface{}{"origin": "Grace"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickOrigin(tc.args)
			if got != tc.wantVal || ok != tc.wantOK {
				t.Fatalf("pickOrigin(%v) = (%q, %v); want (%q, %v)", tc.args, got, ok, tc.wantVal, tc.wantOK)
			}
		})
	}
}

// Guard: every roster origin must round-trip through pickOrigin so the Go enum
// and the validation list never drift apart.
func TestPickOriginAcceptsEveryRosterMember(t *testing.T) {
	if len(rosterOrigins) == 0 || rosterOrigins[0] != "wolfgang" {
		t.Fatalf("rosterOrigins must put orchestrator/default origin first, got %v", rosterOrigins)
	}
	for _, o := range rosterOrigins {
		got, ok := pickOrigin(map[string]interface{}{"origin": o})
		if !ok || got != o {
			t.Errorf("roster member %q rejected by pickOrigin (got %q, ok=%v)", o, got, ok)
		}
	}
}

// validateAndPrepareBatchParams is the follower /rpc path (not MCP-schema-
// validated), so it must sanitize `origin` itself: keep known roster members,
// drop everything else before the params reach the plugin.
func TestValidateAndPrepareBatchParamsSanitizesOrigin(t *testing.T) {
	newParams := func(origin interface{}) map[string]interface{} {
		p := map[string]interface{}{
			"ops": []interface{}{
				map[string]interface{}{"type": "get_metadata"},
			},
		}
		if origin != nil {
			p["origin"] = origin
		}
		return p
	}

	t.Run("keeps a known roster member", func(t *testing.T) {
		p := newParams("grace")
		if err := validateAndPrepareBatchParams(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p["origin"] != "grace" {
			t.Errorf("origin = %v; want grace", p["origin"])
		}
	})

	t.Run("drops an unknown label", func(t *testing.T) {
		p := newParams("intruder")
		if err := validateAndPrepareBatchParams(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, present := p["origin"]; present {
			t.Errorf("unknown origin should have been deleted, got %v", p["origin"])
		}
	})

	t.Run("no origin key stays absent", func(t *testing.T) {
		p := newParams(nil)
		if err := validateAndPrepareBatchParams(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, present := p["origin"]; present {
			t.Errorf("origin should be absent, got %v", p["origin"])
		}
	})
}

// End-to-end through the registered batch tool handler: a valid origin is
// forwarded to the plugin in params; an unknown one is dropped (handler still
// succeeds). Proves the full path — schema → pickOrigin → forwarded params.
func TestRegisterBatchToolsForwardsValidOriginOnly(t *testing.T) {
	okReply := RPCResponse{Data: map[string]any{"okCount": float64(1), "failCount": float64(0)}}

	t.Run("valid origin reaches forwarded params", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "batch", map[string]any{
			"ops":    []any{map[string]any{"type": "get_metadata"}},
			"origin": "grace",
		})
		if res.IsError {
			t.Fatalf("batch with valid origin errored: %s", resultText(t, res))
		}
		if captured.Params["origin"] != "grace" {
			t.Fatalf("forwarded params.origin = %#v; want grace", captured.Params["origin"])
		}
	})

	t.Run("unknown origin is dropped, handler still succeeds", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "batch", map[string]any{
			"ops":    []any{map[string]any{"type": "get_metadata"}},
			"origin": "intruder",
		})
		if res.IsError {
			t.Fatalf("unknown origin should be dropped, not error: %s", resultText(t, res))
		}
		if _, present := captured.Params["origin"]; present {
			t.Fatalf("unknown origin should not be forwarded, got %#v", captured.Params["origin"])
		}
	})
}

// ── task (presence: sticky one-sentence narration) ────────────────────────────

func TestPickTask(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]interface{}
		wantVal string
		wantOK  bool
	}{
		{"simple sentence", map[string]interface{}{"task": "build the dashboard sidebar"}, "build the dashboard sidebar", true},
		{"trims surrounding whitespace", map[string]interface{}{"task": "  build sidebar  "}, "build sidebar", true},
		{"empty dropped", map[string]interface{}{"task": ""}, "", false},
		{"whitespace-only dropped", map[string]interface{}{"task": "   "}, "", false},
		{"missing key dropped", map[string]interface{}{}, "", false},
		{"non-string dropped", map[string]interface{}{"task": 7}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickTask(tc.args)
			if got != tc.wantVal || ok != tc.wantOK {
				t.Fatalf("pickTask(%v) = (%q, %v); want (%q, %v)", tc.args, got, ok, tc.wantVal, tc.wantOK)
			}
		})
	}
}

// Internal whitespace (newlines, tabs, runs of spaces) is collapsed to single spaces
// so a multi-line task can't break the single-line Watch-agent row layout.
func TestPickTask_CollapsesInternalWhitespace(t *testing.T) {
	got, ok := pickTask(map[string]interface{}{"task": "build the\n\tsidebar   now"})
	if !ok || got != "build the sidebar now" {
		t.Fatalf("internal whitespace must collapse to single spaces, got %q (ok=%v)", got, ok)
	}
}

// An over-long task is truncated to maxTaskLen RUNES (not bytes — Unicode-safe for
// Korean) rather than rejected, so a verbose agent still gets a (clipped) label.
func TestPickTask_TruncatesToRuneCap(t *testing.T) {
	long := strings.Repeat("가", maxTaskLen+50) // multi-byte runes
	got, ok := pickTask(map[string]interface{}{"task": long})
	if !ok {
		t.Fatal("over-length task must still be accepted (truncated), got ok=false")
	}
	if n := len([]rune(got)); n != maxTaskLen {
		t.Fatalf("task must truncate to %d runes, got %d", maxTaskLen, n)
	}
}
