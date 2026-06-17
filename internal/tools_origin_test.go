package internal

import "testing"

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
