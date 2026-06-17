package internal

import "testing"

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
// PoC workflow states.
func TestPickStatusAcceptsEveryRosterStatus(t *testing.T) {
	want := map[string]bool{
		"thinking": true, "waiting_review": true, "reviewing": true,
		"approved": true, "escalated": true, "done": true,
	}
	if len(rosterStatuses) != len(want) {
		t.Fatalf("rosterStatuses has %d entries, want %d (the 6 PoC states)", len(rosterStatuses), len(want))
	}
	for _, s := range rosterStatuses {
		if !want[s] {
			t.Errorf("unexpected roster status %q (not one of the 6 PoC states)", s)
		}
		got, ok := pickStatus(map[string]interface{}{"status": s})
		if !ok || got != s {
			t.Errorf("roster status %q rejected by pickStatus (got %q, ok=%v)", s, got, ok)
		}
	}
}

// validateAndPrepareBatchParams (follower /rpc path) must sanitize `status` the
// same way it sanitizes `origin`: keep a known status, drop everything else.
func TestValidateAndPrepareBatchParamsSanitizesStatus(t *testing.T) {
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

	t.Run("keeps a known status", func(t *testing.T) {
		p := newParams("reviewing")
		if err := validateAndPrepareBatchParams(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p["status"] != "reviewing" {
			t.Errorf("status = %v; want reviewing", p["status"])
		}
	})

	t.Run("drops an unknown status", func(t *testing.T) {
		p := newParams("blocked")
		if err := validateAndPrepareBatchParams(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, present := p["status"]; present {
			t.Errorf("unknown status should have been deleted, got %v", p["status"])
		}
	})

	t.Run("no status key stays absent", func(t *testing.T) {
		p := newParams(nil)
		if err := validateAndPrepareBatchParams(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, present := p["status"]; present {
			t.Errorf("status should be absent, got %v", p["status"])
		}
	})
}

// End-to-end through the batch tool handler: a valid status is forwarded to the
// plugin in params; an unknown one is dropped (handler still succeeds).
func TestRegisterBatchToolsForwardsValidStatusOnly(t *testing.T) {
	okReply := RPCResponse{Data: map[string]any{"okCount": float64(1), "failCount": float64(0)}}

	t.Run("valid status reaches forwarded params", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "batch", map[string]any{
			"ops":    []any{map[string]any{"type": "get_metadata"}},
			"status": "reviewing",
		})
		if res.IsError {
			t.Fatalf("batch with valid status errored: %s", resultText(t, res))
		}
		if captured.Params["status"] != "reviewing" {
			t.Fatalf("forwarded params.status = %#v; want reviewing", captured.Params["status"])
		}
	})

	t.Run("unknown status is dropped, handler still succeeds", func(t *testing.T) {
		s, captured := newBatchTestServerWithBackend(t, okReply)
		res := callToolResult(t, s, "batch", map[string]any{
			"ops":    []any{map[string]any{"type": "get_metadata"}},
			"status": "blocked",
		})
		if res.IsError {
			t.Fatalf("unknown status should be dropped, not error: %s", resultText(t, res))
		}
		if _, present := captured.Params["status"]; present {
			t.Fatalf("unknown status should not be forwarded, got %#v", captured.Params["status"])
		}
	})
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
