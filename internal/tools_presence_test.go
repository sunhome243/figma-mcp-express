package internal

import "testing"

// set_presence is the DEDICATED presence tool: it carries the agent's identity
// (origin), manual sticky workflow status, and one-sentence task to the plugin
// WITHOUT performing a Figma operation. It forwards those fields verbatim (plus the
// auto-injected sessionId) so the plugin records them per (sessionId, origin).
func TestSetPresence_ForwardsIdentityStatusTask(t *testing.T) {
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	callTool(t, s, "set_presence", map[string]any{
		"origin": "grace",
		"status": "reviewing",
		"task":   "redesigning the dashboard sidebar",
	})

	req := rec.lastRequest(t)
	if req.Tool != "set_presence" {
		t.Fatalf("tool = %q, want set_presence", req.Tool)
	}
	if got, _ := req.Params["origin"].(string); got != "grace" {
		t.Errorf("origin = %q, want grace", got)
	}
	if got, _ := req.Params["status"].(string); got != "reviewing" {
		t.Errorf("status = %q, want reviewing", got)
	}
	if got, _ := req.Params["task"].(string); got != "redesigning the dashboard sidebar" {
		t.Errorf("task = %q, want the sentence", got)
	}
	if got, _ := req.Params["sessionId"].(string); got == "" {
		t.Error("sessionId must be auto-injected onto every Send")
	}
}

// Only valid presence fields are forwarded: an unknown status is dropped (mirrors
// pickStatus), and origin alone is a valid call (status/task optional).
func TestSetPresence_DropsUnknownStatus_OriginOnly(t *testing.T) {
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	callTool(t, s, "set_presence", map[string]any{
		"origin": "theo",
		"status": "bogus", // not a roster status → dropped
	})

	req := rec.lastRequest(t)
	if _, present := req.Params["status"]; present {
		t.Errorf("unknown status must be dropped, got %v", req.Params["status"])
	}
	if got, _ := req.Params["origin"].(string); got != "theo" {
		t.Errorf("origin = %q, want theo", got)
	}
}

func TestPluginWriteToolsForwardOriginThroughCommonParams(t *testing.T) {
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	callTool(t, s, "import_image", map[string]any{
		"imageData": "abc123",
		"origin":    "theo",
	})

	req := rec.lastRequest(t)
	if req.Tool != "import_image" {
		t.Fatalf("tool = %q, want import_image", req.Tool)
	}
	if got, _ := req.Params["origin"].(string); got != "theo" {
		t.Fatalf("origin = %q, want theo", got)
	}
}

func TestPluginWriteToolsDropUnknownOrigin(t *testing.T) {
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	callTool(t, s, "import_image", map[string]any{
		"imageData": "abc123",
		"origin":    "random-agent",
	})

	req := rec.lastRequest(t)
	if _, present := req.Params["origin"]; present {
		t.Fatalf("unknown origin must be dropped before plugin forwarding, got %v", req.Params["origin"])
	}
}

// Regression for the follower /rpc path: the leader RE-VALIDATES proxied calls, and
// `sessionId` is INJECTED by Node.Send (never declared in any tool schema). It must
// pass param validation or every follower (2nd+ session) call 400s — the exact
// multi-orchestrator case this feature serves.
func TestRejectUnknownToolParams_AllowsInjectedSessionId(t *testing.T) {
	newTestServer(t) // populates registeredParamKeys via RegisterTools
	if msg := rejectUnknownToolParams("get_node", map[string]interface{}{"sessionId": "sessA"}); msg != "" {
		t.Fatalf("injected sessionId must pass tool-param validation, got: %q", msg)
	}
	if msg := rejectUnknownToolParams("set_presence", map[string]interface{}{"sessionId": "sessA", "task": "x"}); msg != "" {
		t.Fatalf("sessionId/task must pass set_presence validation, got: %q", msg)
	}
}

// Same on the batch top-level allowlist: an injected sessionId must not be rejected.
func TestBatchOpsFromParams_AllowsInjectedSessionId(t *testing.T) {
	_, err := batchOpsFromParams(map[string]interface{}{
		"ops":       []interface{}{map[string]interface{}{"type": "get_metadata"}},
		"sessionId": "sessA",
		"origin":    "grace",
	})
	if err != nil {
		t.Fatalf("injected sessionId/origin must be allowed on batch, got: %v", err)
	}
}
