package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── handlePing ────────────────────────────────────────────────────────────────

func TestLeaderHandlePing_OK(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "v1.2.3")

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	l.handlePing(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
	if body["version"] != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", body["version"])
	}
}

func TestLeaderHandlePing_MethodNotAllowed(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/ping", nil)
		w := httptest.NewRecorder()
		l.handlePing(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /ping: status = %d, want 405", method, w.Code)
		}
	}
}

// ── handleRPC ─────────────────────────────────────────────────────────────────

func TestLeaderHandleRPC_MethodNotAllowed(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")

	req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestLeaderHandleRPC_InvalidJSON(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString("{bad json}"))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected error in response body")
	}
}

func TestLeaderHandleRPC_RejectsOversizedRequest(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.ContentLength = maxRPCPayloadBytes + 1
	w := httptest.NewRecorder()

	l.handleRPC(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != "request body too large" {
		t.Fatalf("error = %q, want request body too large", resp.Error)
	}
}

func TestLeaderHandleRPC_RejectsOversizedStreamedRequest(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")
	req := httptest.NewRequest(http.MethodPost, "/rpc", io.LimitReader(repeatedByteReader('x'), maxRPCPayloadBytes+1))
	req.ContentLength = -1
	w := httptest.NewRecorder()

	l.handleRPC(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestLeaderHandleRPC_ValidationError(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")

	// set_text with nodeId but missing text → validation error
	body, _ := json.Marshal(RPCRequest{
		Tool:    "set_text",
		NodeIDs: []string{"1:1"},
		Params:  map[string]any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected validation error in response")
	}
}

func TestLeaderHandleRPC_BatchValidationError(t *testing.T) {
	newTestServer(t) // syncs registered top-level schemas into BatchOpCatalog.
	l := NewLeader("127.0.0.1", 0, "")

	body, _ := json.Marshal(RPCRequest{
		Tool: "batch",
		Params: map[string]any{
			"ops": []any{map[string]any{
				"type":   "import_component_by_key",
				"params": map[string]any{"key": "410:49695"},
			}},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !containsCI(resp.Error, "node id") {
		t.Fatalf("expected batch validation error mentioning node id, got %q", resp.Error)
	}
}

func TestLeaderHandleRPC_BridgeNotConnected(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")

	// get_document has no required params — passes validation, hits bridge
	body, _ := json.Marshal(RPCRequest{Tool: "get_document"})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	// Bridge returns "plugin not connected" error → 200 with error field
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected 'plugin not connected' error in response")
	}
}

// ── Start / Stop ──────────────────────────────────────────────────────────────

func TestLeaderStart_BindsPort(t *testing.T) {
	port := freePort(t)
	l := NewLeader("127.0.0.1", port, "")

	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(l.Stop)

	// Second leader on the same port must fail.
	l2 := NewLeader("127.0.0.1", port, "")
	if err := l2.Start(); err == nil {
		l2.Stop()
		t.Error("expected error when binding already-used port")
	}
}

func TestLeaderStart_usesBoundedHTTPServer(t *testing.T) {
	l := NewLeader("127.0.0.1", freePort(t), "")
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(l.Stop)

	if l.server.ReadHeaderTimeout != remoteHTTPReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", l.server.ReadHeaderTimeout, remoteHTTPReadHeaderTimeout)
	}
	if l.server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v, want 0 for long-lived WebSocket connections", l.server.WriteTimeout)
	}
	if l.server.IdleTimeout != remoteHTTPIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", l.server.IdleTimeout, remoteHTTPIdleTimeout)
	}
	if l.server.MaxHeaderBytes != remoteHTTPMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", l.server.MaxHeaderBytes, remoteHTTPMaxHeaderBytes)
	}
}

func TestLeaderStop_FreesPort(t *testing.T) {
	port := freePort(t)
	l := NewLeader("127.0.0.1", port, "")

	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	l.Stop()

	// Allow OS to release the port.
	time.Sleep(20 * time.Millisecond)

	l2 := NewLeader("127.0.0.1", port, "")
	if err := l2.Start(); err != nil {
		t.Fatalf("port should be free after Stop: %v", err)
	}
	l2.Stop()
}

func TestLeaderStop_Idempotent(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "")
	// Stop on a never-started leader should not panic.
	l.Stop()
	l.Stop()
}

// ── /ping endpoint (integration via httptest.Server) ─────────────────────────

func TestLeaderPingEndpoint(t *testing.T) {
	port := freePort(t)
	l := NewLeader("127.0.0.1", port, "test-ver")
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(l.Stop)

	f := NewFollower("http://127.0.0.1:" + itoa(port))
	if !f.Ping(t.Context()) {
		t.Error("expected ping to succeed for running leader")
	}
}
