package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// setupBridgeWithClient creates a Bridge with an active WebSocket client connected to it.
// Returns the bridge and the client-side connection (already cleaned up on t.Cleanup).
func setupBridgeWithClient(t *testing.T) (*Bridge, *websocket.Conn) {
	t.Helper()
	bridge := NewBridge()

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	// Poll until bridge registers the server-side connection.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bridge.IsConnected() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !bridge.IsConnected() {
		t.Fatal("bridge not connected after 500ms")
	}

	return bridge, clientConn
}

// ── NewBridge ─────────────────────────────────────────────────────────────────

func TestNewBridge(t *testing.T) {
	b := NewBridge()
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
	if b.IsConnected() {
		t.Error("new bridge should not be connected")
	}
}

// ── nextID ────────────────────────────────────────────────────────────────────

func TestBridgeNextID(t *testing.T) {
	b := NewBridge()
	id1 := b.nextID()
	id2 := b.nextID()

	if id1 == id2 {
		t.Error("consecutive IDs must be unique")
	}
	if !strings.HasPrefix(id1, "req-") {
		t.Errorf("ID %q does not have req- prefix", id1)
	}
	// Format: req-HHMMSS-N  (14 chars min: "req-000000-1")
	parts := strings.Split(id1, "-")
	if len(parts) != 3 {
		t.Errorf("ID %q has wrong format (want 3 dash-separated parts)", id1)
	}
}

// ── MarshalJSON ───────────────────────────────────────────────────────────────

func TestBridgeMarshalJSON_Disconnected(t *testing.T) {
	b := NewBridge()
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["channels"] != float64(0) {
		t.Errorf("channels = %v, want 0", m["channels"])
	}
	if m["pending"] != float64(0) {
		t.Errorf("pending = %v, want 0", m["pending"])
	}
}

func TestBridgeMarshalJSON_Connected(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["channels"] != float64(1) {
		t.Errorf("channels = %v, want 1", m["channels"])
	}
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestBridgeClose_NoPanic(t *testing.T) {
	b := NewBridge()
	// Close on an unconnected bridge should not panic.
	b.Close()
}

func TestBridgeClose_DrainsPending(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// Manually insert a pending entry so we can verify Close drains it.
	ch := make(chan BridgeResponse, 1)
	entry := &pendingEntry{ch: ch}
	entry.timer = time.AfterFunc(10*time.Second, func() {})

	b.mu.Lock()
	b.pending["test-id"] = entry
	b.mu.Unlock()

	b.Close()

	// Channel must be closed (receive returns zero value, ok=false).
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timed out waiting for channel to be closed")
	}
}

// ── Send ─────────────────────────────────────────────────────────────────────

func TestBridgeSend_NotConnected(t *testing.T) {
	b := NewBridge()
	_, err := b.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestBridgeSend_ContextCancelled(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestBridgeSend_Success(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Goroutine: echo request back as a successful response.
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		resp := BridgeResponse{
			RequestID: req.RequestID,
			Type:      req.Type,
			Data:      map[string]any{"id": "1:1", "name": "Frame 1"},
		}
		wsjson.Write(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.Data == nil {
		t.Error("expected non-nil data in response")
	}
}

func TestBridgeSend_PluginError(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		resp := BridgeResponse{
			RequestID: req.RequestID,
			Error:     "node not found",
		}
		wsjson.Write(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"9:9"}, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if got.Error == "" {
		t.Error("expected error field from plugin")
	}
}

func TestBridgeSend_Timeout(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	// Don't send any response from the client — bridge should time out.
	// We manipulate the timeout via a very short context rather than waiting 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

// ── IsConnected ───────────────────────────────────────────────────────────────

func TestBridgeIsConnected(t *testing.T) {
	b := NewBridge()
	if b.IsConnected() {
		t.Error("should not be connected before any upgrade")
	}

	b2, _ := setupBridgeWithClient(t)
	if !b2.IsConnected() {
		t.Error("should be connected after upgrade")
	}
}

// ── Multi-channel routing (flap fix) ──────────────────────────────────────────

// dialChannel connects a websocket client to the bridge on a specific channel.
func dialChannel(t *testing.T, srv *httptest.Server, channel string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?channel=" + channel
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial channel %s: %v", channel, err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

func waitChannels(t *testing.T, b *Bridge, want int) {
	t.Helper()
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if len(b.ListChannels()) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d channels, got %d", want, len(b.ListChannels()))
}

// TWO different channels must coexist — connecting B must NOT close A.
// This is the core flap fix: the old single-slot bridge closed A on B's connect,
// A auto-reconnected (closing B), forever.
func TestBridge_TwoChannelsCoexist(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	dialChannel(t, srv, "fileA")
	waitChannels(t, b, 1)
	dialChannel(t, srv, "fileB")
	waitChannels(t, b, 2) // both present — B did not evict A

	chans := b.ListChannels()
	if chans[0].Channel != "fileA" || chans[1].Channel != "fileB" {
		t.Errorf("channels = %+v, want fileA + fileB", chans)
	}
}

// Reconnecting on the SAME channel replaces that channel's socket (count stays 1).
func TestBridge_SameChannelReconnectReplaces(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	dialChannel(t, srv, "fileA")
	waitChannels(t, b, 1)
	dialChannel(t, srv, "fileA")
	// Still exactly one channel after a same-channel reconnect.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && len(b.ListChannels()) != 1 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(b.ListChannels()); got != 1 {
		t.Errorf("channels after same-channel reconnect = %d, want 1", got)
	}
}

// Send with an explicit channel routes to that connection.
func TestBridge_SendRoutesByChannel(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	dialChannel(t, srv, "fileA")
	connB := dialChannel(t, srv, "fileB")
	waitChannels(t, b, 2)

	// B's client reads the request and replies.
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(context.Background(), connB, &req); err != nil {
			return
		}
		_ = wsjson.Write(context.Background(), connB, BridgeResponse{
			Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"ok": "B"},
		})
	}()

	resp, err := b.Send(context.Background(), "get_node", []string{"1:1"},
		map[string]interface{}{"channel": "fileB"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	data, _ := resp.Data.(map[string]interface{})
	if data["ok"] != "B" {
		t.Errorf("response routed wrong: %+v", resp.Data)
	}
}

// Send with no channel + multiple connections returns a helpful error (not a guess).
func TestBridge_SendAmbiguousChannelErrors(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	dialChannel(t, srv, "fileA")
	dialChannel(t, srv, "fileB")
	waitChannels(t, b, 2)

	_, err := b.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err == nil || !strings.Contains(err.Error(), "multiple files connected") {
		t.Errorf("expected multiple-files error, got %v", err)
	}
}

// Send to an unknown channel errors clearly.
func TestBridge_SendUnknownChannelErrors(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	dialChannel(t, srv, "fileA")
	waitChannels(t, b, 1)

	_, err := b.Send(context.Background(), "get_node", []string{"1:1"},
		map[string]interface{}{"channel": "ghost"})
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected not-connected error, got %v", err)
	}
}

// A register message attaches file metadata to the channel.
func TestBridge_RegisterUpdatesMetadata(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	conn := dialChannel(t, srv, "fileA")
	waitChannels(t, b, 1)

	_ = wsjson.Write(context.Background(), conn, BridgeResponse{
		Type: registerMessageType,
		Data: map[string]any{"fileName": "Dashboard", "fileKey": "abc123", "pageName": "Page 1"},
	})

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if c := b.ListChannels(); len(c) == 1 && c[0].FileName == "Dashboard" {
			if c[0].FileKey != "abc123" {
				t.Errorf("fileKey = %q, want abc123", c[0].FileKey)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("register metadata not applied: %+v", b.ListChannels())
}

// ── Progress-update safety ────────────────────────────────────────────────────

// TestBridgeSend_ProgressUpdateDoesNotResolve verifies that a progress_update
// message — with type:"progress_update", a valid in-flight requestId, Progress:0,
// and nil Data — does NOT resolve or delete the pending request.  The caller
// must keep waiting; the pending entry must survive with its timer extended.
// A subsequent real response with the same requestId DOES resolve it with the
// real data.  This guards the CRITICAL bug where progress:undefined → Progress:0
// fell through to the resolution block and resolved the caller with empty data.
func TestBridgeSend_ProgressUpdateDoesNotResolve(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	realData := map[string]any{"id": "1:1", "name": "BigFrame"}

	go func() {
		// 1. Read the request the bridge sent.
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}

		// 2. Send a progress_update with Progress:0 (the bug scenario).
		//    This must NOT resolve the pending request.
		progressMsg := BridgeResponse{
			Type:      progressUpdateType,
			RequestID: req.RequestID,
			Progress:  0,
			Message:   "tick 1",
			// Data intentionally nil
		}
		if err := wsjson.Write(ctx, clientConn, progressMsg); err != nil {
			return
		}

		// 3. Give the bridge a moment to process the progress message.
		time.Sleep(30 * time.Millisecond)

		// 4. Now send the real response — same requestId, non-nil data.
		realResp := BridgeResponse{
			Type:      req.Type,
			RequestID: req.RequestID,
			Data:      realData,
		}
		wsjson.Write(ctx, clientConn, realResp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if got.Data == nil {
		t.Fatal("Send resolved with nil Data — progress_update incorrectly resolved the request")
	}
	dataMap, ok := got.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected Data type %T", got.Data)
	}
	if dataMap["id"] != "1:1" {
		t.Errorf("Data[id] = %q, want %q", dataMap["id"], "1:1")
	}
	if dataMap["name"] != "BigFrame" {
		t.Errorf("Data[name] = %q, want %q", dataMap["name"], "BigFrame")
	}
}

// TestBridgeSend_ProgressUpdateExtendsTimeout verifies that a progress_update
// message resets the per-request timer.  We set a very short context deadline
// that would expire before the real response arrives; the progress message must
// extend the bridge timer so the real response still lands successfully.
// (This test only checks that the progress branch resets the timer — it does NOT
// manipulate the internal timer directly; instead it relies on the type guard.)
func TestBridgeSend_ProgressUpdateTypeGuard(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}

		// Send several progress_update messages — mix of Progress:0 and Progress:50.
		for _, prog := range []int{0, 0, 50, 0} {
			msg := BridgeResponse{
				Type:      progressUpdateType,
				RequestID: req.RequestID,
				Progress:  prog,
				Message:   "tick",
			}
			if err := wsjson.Write(ctx, clientConn, msg); err != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		// Real response.
		wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
			Type:      req.Type,
			RequestID: req.RequestID,
			Data:      map[string]any{"ok": true},
		})
	}()

	got, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send error after mixed-progress ticks: %v", err)
	}
	if got.Data == nil {
		t.Fatal("resolved with nil data — one of the Progress:0 ticks incorrectly resolved the request")
	}
}

// ── parseRequestTimeout (Part 2) ─────────────────────────────────────────────

// TestParseRequestTimeout_Default asserts the default timeout is 120s when
// FIGMA_MCP_TIMEOUT is unset or invalid.
func TestParseRequestTimeout_Default(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "")
	got := parseRequestTimeout()
	if got != 120*time.Second {
		t.Errorf("default timeout = %v, want 120s", got)
	}
}

func TestParseRequestTimeout_CustomValue(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "60")
	got := parseRequestTimeout()
	if got != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", got)
	}
}

func TestParseRequestTimeout_InvalidFallsToDefault(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "not-a-number")
	got := parseRequestTimeout()
	if got != 120*time.Second {
		t.Errorf("invalid env should fall to default 120s, got %v", got)
	}
}

func TestParseRequestTimeout_ZeroFallsToDefault(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "0")
	got := parseRequestTimeout()
	if got != 120*time.Second {
		t.Errorf("zero value should fall to default 120s, got %v", got)
	}
}

func TestParseRequestTimeout_NegativeFallsToDefault(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "-5")
	got := parseRequestTimeout()
	if got != 120*time.Second {
		t.Errorf("negative value should fall to default 120s, got %v", got)
	}
}

// ── resolveRequestTimeout (server-side ceilings) ───────────────────────────────

// TestResolveRequestTimeout_TableDriven exercises the pure timeout-resolution
// function: reads/batch get the generous FIGMA_MCP_READ_TIMEOUT ceiling;
// writes/other ops get the base FIGMA_MCP_TIMEOUT ceiling; both are env-configurable.
func TestResolveRequestTimeout_TableDriven(t *testing.T) {
	const (
		defaultBase = 120 * time.Second
		defaultRead = 600 * time.Second
	)
	tests := []struct {
		name        string
		requestType string
		baseEnv     string // FIGMA_MCP_TIMEOUT ("" = unset → default 120s)
		readEnv     string // FIGMA_MCP_READ_TIMEOUT ("" = unset → default 600s)
		want        time.Duration
	}{
		// ── read ops get generous read ceiling ──────────────────────────────────
		{
			name:        "get_node → read ceiling (default 600s)",
			requestType: "get_node",
			want:        defaultRead,
		},
		{
			name:        "get_nodes_info → read ceiling",
			requestType: "get_nodes_info",
			want:        defaultRead,
		},
		{
			name:        "get_design_context → read ceiling",
			requestType: "get_design_context",
			want:        defaultRead,
		},
		{
			name:        "get_document → read ceiling (no longer special-cased to 120s)",
			requestType: "get_document",
			want:        defaultRead,
		},
		{
			name:        "scan_nodes_by_types → read ceiling",
			requestType: "scan_nodes_by_types",
			want:        defaultRead,
		},
		{
			name:        "scan_text_nodes → read ceiling",
			requestType: "scan_text_nodes",
			want:        defaultRead,
		},
		{
			name:        "search_nodes → read ceiling",
			requestType: "search_nodes",
			want:        defaultRead,
		},
		{
			name:        "get_local_components → read ceiling",
			requestType: "get_local_components",
			want:        defaultRead,
		},
		{
			name:        "batch → read ceiling (heavy reads occur inside batch)",
			requestType: "batch",
			want:        defaultRead,
		},
		// ── writes and other ops get base ceiling ───────────────────────────────
		{
			name:        "create_frame → base ceiling",
			requestType: "create_frame",
			want:        defaultBase,
		},
		{
			name:        "set_fills → base ceiling",
			requestType: "set_fills",
			want:        defaultBase,
		},
		{
			name:        "get_metadata → base ceiling (cheap read, not in generous set)",
			requestType: "get_metadata",
			want:        defaultBase,
		},
		// ── env configurability ─────────────────────────────────────────────────
		{
			name:        "FIGMA_MCP_READ_TIMEOUT overrides read ceiling",
			requestType: "get_node",
			readEnv:     "300",
			want:        300 * time.Second,
		},
		{
			name:        "FIGMA_MCP_TIMEOUT overrides base ceiling",
			requestType: "create_frame",
			baseEnv:     "180",
			want:        180 * time.Second,
		},
		{
			name:        "invalid FIGMA_MCP_READ_TIMEOUT falls back to default 600s",
			requestType: "get_node",
			readEnv:     "not-a-number",
			want:        defaultRead,
		},
		{
			name:        "zero FIGMA_MCP_READ_TIMEOUT falls back to default 600s",
			requestType: "get_node",
			readEnv:     "0",
			want:        defaultRead,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FIGMA_MCP_TIMEOUT", tc.baseEnv)
			t.Setenv("FIGMA_MCP_READ_TIMEOUT", tc.readEnv)
			got := resolveRequestTimeout(tc.requestType)
			if got != tc.want {
				t.Errorf("resolveRequestTimeout(%q) = %v, want %v", tc.requestType, got, tc.want)
			}
		})
	}
}

// solePending returns the single in-flight pendingEntry (white-box), polling until
// one is registered. Used to inspect resetTimeout mid-flight.
func solePending(t *testing.T, b *Bridge) *pendingEntry {
	t.Helper()
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.RLock()
		for _, pe := range b.pending {
			b.mu.RUnlock()
			return pe
		}
		b.mu.RUnlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no pending entry registered")
	return nil
}

// TestBridgeSend_ResetWindowMatchesOpCeiling verifies the progress-reset window:
// after A+B, pe.resetTimeout must equal the server-side ceiling for that op type
// (read ops get the generous read ceiling; writes get the base ceiling). No LLM
// override exists — the ceiling is determined entirely by op type + env.
func TestBridgeSend_ResetWindowMatchesOpCeiling(t *testing.T) {
	tests := []struct {
		name        string
		requestType string
		readEnv     string
		baseEnv     string
		wantResetTo time.Duration
	}{
		{
			name:        "read op → reset window equals read ceiling (default 600s)",
			requestType: "get_node",
			wantResetTo: 600 * time.Second,
		},
		{
			name:        "read op with custom FIGMA_MCP_READ_TIMEOUT → custom ceiling",
			requestType: "get_node",
			readEnv:     "300",
			wantResetTo: 300 * time.Second,
		},
		{
			name:        "write op → reset window equals base ceiling (default 120s)",
			requestType: "create_frame",
			wantResetTo: 120 * time.Second,
		},
		{
			name:        "batch → reset window equals read ceiling",
			requestType: "batch",
			wantResetTo: 600 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FIGMA_MCP_TIMEOUT", tc.baseEnv)
			t.Setenv("FIGMA_MCP_READ_TIMEOUT", tc.readEnv)
			b, clientConn := setupBridgeWithClient(t)
			ctx := context.Background()

			release := make(chan struct{})
			go func() {
				var req BridgeRequest
				if err := wsjson.Read(ctx, clientConn, &req); err != nil {
					return
				}
				<-release
				wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
					Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"ok": true},
				})
			}()

			done := make(chan struct{})
			go func() {
				b.Send(ctx, tc.requestType, []string{"1:1"}, nil) //nolint:errcheck
				close(done)
			}()

			pe := solePending(t, b)
			if pe.resetTimeout != tc.wantResetTo {
				t.Errorf("resetTimeout = %v, want %v", pe.resetTimeout, tc.wantResetTo)
			}
			close(release)
			<-done
		})
	}
}

// TestBridgeSend_ProgressTickConsumesResetTimeout proves readLoop ACTUALLY resets
// the in-flight timer to pe.resetTimeout (not a hardcoded value) on each progress
// tick. Mechanism: white-box-shrink the in-flight pe.resetTimeout to a tiny window,
// then drive TWO real progress_update ticks through readLoop; the request must then
// time out within that tiny window. If readLoop were reverted to Reset(120s), the
// tiny field would be ignored and the request would NOT time out inside the test's
// deadline — so this test fails on that revert. Server-managed ceilings only.
func TestBridgeSend_ProgressTickConsumesResetTimeout(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	const tinyReset = 40 * time.Millisecond

	// Client reads the request, emits two real progress ticks, then never replies —
	// so resolution can only come from the (reset) timeout timer firing.
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		for i := 0; i < 2; i++ {
			if err := wsjson.Write(ctx, clientConn, BridgeResponse{
				Type: progressUpdateType, RequestID: req.RequestID, Progress: 10, Message: "tick",
			}); err != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	result := make(chan error, 1)
	go func() {
		_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
		result <- err
	}()

	// Shrink the in-flight reset window so a tick-driven reset is observable in-test.
	// (The server default 600s is far too long to wait on; we only need to prove
	// readLoop reads the field, so we mutate it, not the server policy.)
	pe := solePending(t, b)
	b.mu.Lock()
	pe.resetTimeout = tinyReset
	b.mu.Unlock()

	// After the ticks reset the timer to tinyReset, the request must time out fast.
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected a timeout error after the reset window elapsed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not time out within 3s — readLoop ignored pe.resetTimeout (hardcoded value?)")
	}
}

// TestBridgeSend_SameOpSameChannelDedupsToOneFlight verifies that two concurrent
// identical reads collapse onto one plugin round-trip (the singleflight guarantee),
// which must still work after timeoutMs is removed from the key.
func TestBridgeSend_SameOpSameChannelDedupsToOneFlight(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	var reqCount int32
	leaderInFlight := make(chan struct{})
	releaseLeader := make(chan struct{})
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		atomic.AddInt32(&reqCount, 1)
		close(leaderInFlight)
		<-releaseLeader
		wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
			Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"ok": true},
		})
	}()

	// Leader in background.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.Send(ctx, "get_node", []string{"1:1"}, nil) //nolint:errcheck
	}()
	<-leaderInFlight

	// Follower after leader is confirmed in-flight.
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.Send(ctx, "get_node", []string{"1:1"}, nil) //nolint:errcheck
	}()
	close(releaseLeader)
	wg.Wait()

	if got := atomic.LoadInt32(&reqCount); got != 1 {
		t.Fatalf("plugin saw %d reads, want 1 (identical reads must dedup)", got)
	}
}

// Keeps the "params never reach plugin" invariant for ordinary (non-timeoutMs) params.
// Previously TestBridgeSend_TimeoutMsStrippedFromPluginParams; now tests that
// routing params (channel) don't leak and that other params do pass through cleanly.
func TestBridgeSend_ChannelParamStrippedFromPluginParams(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	gotParams := make(chan map[string]interface{}, 1)
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		gotParams <- req.Params
		wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
			Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"ok": true},
		})
	}()

	_, err := b.Send(ctx, "get_node", []string{"1:1"},
		map[string]interface{}{"depth": float64(2)})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case p := <-gotParams:
		if _, ok := p["channel"]; ok {
			t.Errorf("plugin received channel routing param — must be stripped: %v", p)
		}
		if p["depth"] != float64(2) {
			t.Errorf("non-routing params must survive: depth = %v, want 2", p["depth"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("plugin never received the request")
	}
}

// ── Serial slot + singleflight (concurrency optimisation) ──────────────────────

// soleConn returns the single connEntry registered on the bridge (white-box).
func soleConn(t *testing.T, b *Bridge) *connEntry {
	t.Helper()
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, e := range b.conns {
		return e
	}
	t.Fatal("no connection registered")
	return nil
}

// Identical concurrent reads collapse onto ONE plugin round-trip. Determinism:
// followers are only launched AFTER the leader is confirmed in-flight, and the
// leader holds its response until released — so the flight entry is guaranteed
// present when every follower runs doSingleflight (no scheduler-timing race for
// the leader/follower ordering). The single settle only covers µs-scale
// goroutine scheduling between "follower launched" and "follower parked on the
// flight"; the flight cannot disappear underneath it because the leader is held.
func TestBridgeSend_SingleflightCollapsesReads(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	var reqCount int32
	leaderInFlight := make(chan struct{})
	releaseLeader := make(chan struct{})
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		atomic.AddInt32(&reqCount, 1)
		close(leaderInFlight)                         // request received ⇒ flight is registered & in-flight
		<-releaseLeader                               // hold until all followers have joined
		wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
			Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"id": "1:1"},
		})
		// Drain any stray request (there must be none) so a 2nd round-trip can't
		// deadlock the test; count it so the assertion catches the failure.
		for {
			var extra BridgeRequest
			if err := wsjson.Read(ctx, clientConn, &extra); err != nil {
				return
			}
			atomic.AddInt32(&reqCount, 1)
			wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
				Type: extra.Type, RequestID: extra.RequestID, Data: map[string]any{},
			})
		}
	}()

	const followers = 4
	results := make(chan error, followers+1)
	go func() { _, err := b.Send(ctx, "get_node", []string{"1:1"}, nil); results <- err }()
	<-leaderInFlight // leader now holds the flight

	for i := 0; i < followers; i++ {
		go func() { _, err := b.Send(ctx, "get_node", []string{"1:1"}, nil); results <- err }()
	}
	time.Sleep(50 * time.Millisecond) // let followers reach the flight (it can't vanish — leader held)
	close(releaseLeader)

	for i := 0; i < followers+1; i++ {
		if err := <-results; err != nil {
			t.Errorf("caller %d error: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&reqCount); got != 1 {
		t.Fatalf("plugin saw %d requests, want 1 (singleflight must collapse dupes)", got)
	}
}

// Identical concurrent WRITES must NOT collapse — every one reaches the plugin.
func TestBridgeSend_SingleflightSkipsWrites(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	const n = 3
	var reqCount int32
	go func() {
		for {
			var req BridgeRequest
			if err := wsjson.Read(ctx, clientConn, &req); err != nil {
				return
			}
			atomic.AddInt32(&reqCount, 1)
			wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
				Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"ok": true},
			})
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Send(ctx, "create_frame", nil, map[string]interface{}{"x": 0}) //nolint:errcheck
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&reqCount); got != n {
		t.Fatalf("plugin saw %d create_frame, want %d (writes must never be singleflighted)", got, n)
	}
}

// The per-channel slot serialises requests: while the slot is held, Send must not
// write to the plugin; once released, it proceeds. Deterministic (no sleeps race).
func TestBridgeSend_SerializesPerChannel(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	e := soleConn(t, b)

	// Occupy the slot as if a request were already in flight.
	e.sem <- struct{}{}

	wrote := make(chan struct{})
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(context.Background(), clientConn, &req); err != nil {
			return
		}
		close(wrote)
		wsjson.Write(context.Background(), clientConn, BridgeResponse{ //nolint:errcheck
			Type: req.Type, RequestID: req.RequestID, Data: map[string]any{"ok": true},
		})
	}()

	done := make(chan struct{})
	go func() {
		b.Send(context.Background(), "get_node", []string{"1:1"}, nil) //nolint:errcheck
		close(done)
	}()

	// While the slot is held, Send must NOT have written to the plugin.
	select {
	case <-wrote:
		t.Fatal("Send wrote while slot held — serial gate not working")
	case <-time.After(100 * time.Millisecond):
	}

	// Release the slot; Send must now proceed and write.
	<-e.sem
	select {
	case <-wrote:
	case <-time.After(2 * time.Second):
		t.Fatal("Send did not proceed after slot released")
	}
	<-done
}

// Slot acquisition honours the caller's ctx: a held slot + a short ctx returns a
// ctx error rather than blocking forever, and never writes to the plugin.
func TestBridgeSend_QueueRespectsContext(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	e := soleConn(t, b)
	e.sem <- struct{}{} // occupy

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := b.Send(ctx, "get_node", []string{"1:1"}, nil); err == nil {
		t.Fatal("expected ctx error while slot held")
	}
}

// ── import-jam guard (1C) ───────────────────────────────────────────────────────

// A hung import (client-cancelled before it resolves) must leave the import marker
// set, so a RETRIED import is rejected immediately with ErrImportInFlight instead of
// being dispatched and re-jamming the single-threaded plugin (the ~118× amplifier).
func TestBridgeSend_ImportInFlightRejectsRetry(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// First import: client never responds. A short client-cancel (mirrors the ~10s
	// MCP-client timeout) returns the call WITHOUT the import resolving — the marker
	// must survive client-cancel (the request-timeout timer is 120s, won't fire here).
	ctx1, cancel1 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel1()
	if _, err := b.Send(ctx1, "import_component_by_key", nil, map[string]interface{}{"key": "abc"}); err == nil {
		t.Fatal("expected first import to fail on client-cancel")
	}

	// Retry arrives while the first is still in flight → rejected, not dispatched.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_, err := b.Send(ctx2, "import_variable_by_key", nil, map[string]interface{}{"key": "def"})
	if !errors.Is(err, ErrImportInFlight) {
		t.Fatalf("expected ErrImportInFlight on retry, got: %v", err)
	}
}

// A non-import call must NOT be rejected by the import guard while an import is in
// flight (it isn't the retry amplifier). It is allowed past the guard and then queues
// on the serial slot the cancelled import still holds, timing out on its own short ctx
// — the point is the error is a ctx deadline, NOT ErrImportInFlight.
func TestBridgeSend_ImportInFlightAllowsNonImport(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel1()
	_, _ = b.Send(ctx1, "import_component_by_key", nil, map[string]interface{}{"key": "abc"})

	ctx2, cancel2 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel2()
	_, err := b.Send(ctx2, "get_node", []string{"1:1"}, nil)
	if errors.Is(err, ErrImportInFlight) {
		t.Fatal("non-import call must not be rejected by the import guard")
	}
}

// A cleanly-resolved import must clear the marker, so a later import is allowed.
func TestBridgeSend_ImportMarkerClearsOnResponse(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	go func() {
		for i := 0; i < 2; i++ {
			var req BridgeRequest
			if err := wsjson.Read(ctx, clientConn, &req); err != nil {
				return
			}
			wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
				RequestID: req.RequestID, Type: req.Type, Data: map[string]any{"ok": true},
			})
		}
	}()

	if _, err := b.Send(ctx, "import_component_by_key", nil, map[string]interface{}{"key": "a"}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	// If the marker didn't clear on the first response, this would be ErrImportInFlight.
	if _, err := b.Send(ctx, "import_component_by_key", nil, map[string]interface{}{"key": "b"}); err != nil {
		t.Fatalf("second import after clean resolve should be allowed: %v", err)
	}
}

// ── Task C — error fast-fail ────────────────────────────────────────────────────

// TestBridgeSend_PluginErrorResolvesImmediately verifies that a plugin error
// response resolves the pending request IMMEDIATELY — well under the inactivity
// ceiling. After A+B the read ceiling will be several hundred seconds; a plugin
// that replies with an error must not make the caller wait for that ceiling.
func TestBridgeSend_PluginErrorResolvesImmediately(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Plugin receives the request and replies with an error immediately.
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
			Type:      req.Type,
			RequestID: req.RequestID,
			Error:     "boom: simulated plugin error",
		})
	}()

	start := time.Now()
	resp, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	elapsed := time.Since(start)

	// Must return fast (well under 1s, not after the inactivity ceiling).
	if elapsed > 2*time.Second {
		t.Errorf("plugin error response took %v — must resolve immediately, not wait for ceiling", elapsed)
	}
	// No Go-level error — the error is carried in resp.Error (bridge delivers it as
	// a successful transport, application-level error surface).
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected resp.Error to carry the plugin error message")
	}
}

// TestBridgeSend_ConnectionDropResolvesPendingImmediately verifies that when the
// plugin WebSocket connection is dropped, ALL pending requests for that channel
// are resolved with a "connection closed" error IMMEDIATELY — not after the
// (now generous) inactivity ceiling. This is critical when the read/batch ceiling
// is set to several hundred seconds: a dropped connection must not leave callers
// hanging for the full ceiling duration.
func TestBridgeSend_ConnectionDropResolvesPendingImmediately(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Plugin reads the request but never replies — holds it open.
	gotReq := make(chan BridgeRequest, 1)
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		gotReq <- req
	}()

	result := make(chan error, 1)
	go func() {
		_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
		result <- err
	}()

	// Wait until the plugin has the request in-flight so we know the pending entry
	// is registered in b.pending before we drop the connection.
	select {
	case <-gotReq:
	case <-time.After(2 * time.Second):
		t.Fatal("plugin never received the request")
	}

	// Drop the connection abruptly — mimics a plugin crash / network cut.
	clientConn.Close(websocket.StatusAbnormalClosure, "simulate drop")

	// The pending request must resolve with an error PROMPTLY (well under any ceiling).
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected an error after connection drop, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending request did not resolve after connection drop — must not wait for inactivity ceiling")
	}
}

// ── Heartbeat: dead transport with no FIN (issue #32) ─────────────────────────

// setupBridgeWithDeadClient connects a client that NEVER reads, so it never
// auto-pongs — simulating a half-open / partitioned transport that sends no TCP
// FIN. The bridge's heartbeat is dialed to a few ms so the test is fast.
func setupBridgeWithDeadClient(t *testing.T) (*Bridge, *websocket.Conn) {
	t.Helper()
	b := NewBridge()
	b.heartbeatInterval = 20 * time.Millisecond
	b.heartbeatTimeout = 50 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if b.IsConnected() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !b.IsConnected() {
		t.Fatal("bridge not connected after 500ms")
	}
	return b, clientConn
}

// A transport that stops responding (no pong, no FIN) must be detected by the
// heartbeat and torn down — not left registered until a request's inactivity
// ceiling fires. Mechanism test only: it proves the heartbeat path, NOT that a
// JS-frozen-but-connected plugin is covered (a browser auto-pongs at the protocol
// layer, so that case is intentionally out of scope).
func TestBridge_HeartbeatDropsDeadTransport(t *testing.T) {
	b, _ := setupBridgeWithDeadClient(t)

	// The client never reads → never pongs → heartbeat ping times out → conn closed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !b.IsConnected() {
			return // detected and dropped — pass
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("heartbeat did not drop the unresponsive transport within 2s")
}

// The real payoff: an in-flight request on a dead transport must fail FAST via the
// heartbeat-triggered drain (~heartbeat window), not wait the ~120s ceiling.
func TestBridge_HeartbeatDrainsInflightRequestFast(t *testing.T) {
	b, _ := setupBridgeWithDeadClient(t)

	start := time.Now()
	_, err := b.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from the drained in-flight request, got nil")
	}
	if !strings.Contains(err.Error(), "connection closed") {
		t.Errorf("err = %v, want a 'connection closed' drain error", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Send took %v — heartbeat drain too slow (should be ~heartbeat window, not the request ceiling)", elapsed)
	}
}

// ── Stalled-head early-reject (generalizes the import-jam guard) ───────────────

// connEntry.isStalled is pure: it reports stall from live slot occupancy + the
// last-progress timestamp, with NO persistent flag — so nothing can stick and
// brick the channel. Deterministic unit coverage of every branch.
func TestConnEntry_IsStalled(t *testing.T) {
	e := &connEntry{sem: make(chan struct{}, 1)}

	// Slot free → never stalled, regardless of timestamp.
	if e.isStalled(time.Millisecond) {
		t.Error("a free slot must never be stalled")
	}

	// Occupy the slot.
	e.sem <- struct{}{}

	// Busy but no progress recorded yet (lastProgressAt==0) → not stalled (defensive:
	// never reject before the holder has even been stamped).
	if e.isStalled(time.Millisecond) {
		t.Error("busy slot with no recorded progress must not be stalled")
	}

	// Fresh progress → not stalled.
	e.markProgress()
	if e.isStalled(time.Second) {
		t.Error("fresh progress must not be stalled")
	}

	// Stale progress (older than threshold) → stalled.
	e.lastProgressAt.Store(time.Now().Add(-time.Second).UnixNano())
	if !e.isStalled(10 * time.Millisecond) {
		t.Error("progress older than the threshold must be stalled")
	}

	// Drain the slot → self-heals to not-stalled even with the stale timestamp.
	<-e.sem
	if e.isStalled(10 * time.Millisecond) {
		t.Error("draining the slot must self-heal stall state (no flag to stick)")
	}

	// Inherited-stale-timestamp invariant (what sendOnce's onResolve guarantees):
	// release zeroes lastProgressAt, so the NEXT holder — in its brief window before
	// its own markProgress — reads lp==0 and is NOT flagged from the prior holder's
	// stale timestamp.
	e.sem <- struct{}{}
	e.lastProgressAt.Store(time.Now().Add(-time.Hour).UnixNano()) // very stale holder
	e.lastProgressAt.Store(0)                                     // onResolve zeroes before release
	<-e.sem                                                       // release
	e.sem <- struct{}{}                                           // next holder acquires (pre-stamp window)
	if e.isStalled(time.Millisecond) {
		t.Error("a fresh holder must not inherit the prior holder's stale timestamp")
	}
	<-e.sem
}

// setupBridgeWithClientStall connects a client and sets a tiny stall threshold so
// a hung head is detected fast in tests. The client never responds (the head hangs).
func setupBridgeWithClientStall(t *testing.T, stall time.Duration) *Bridge {
	t.Helper()
	b := NewBridge()
	b.stallThreshold = stall

	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if b.IsConnected() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !b.IsConnected() {
		t.Fatal("bridge not connected after 500ms")
	}
	return b
}

// slotBusy reports whether the (sole) channel's serial slot is currently held.
func slotBusy(b *Bridge) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, e := range b.conns {
		return len(e.sem) == 1
	}
	return false
}

// A new request arriving at a channel whose head op has held the slot past the
// stall threshold with no progress must fast-fail with ErrChannelStalled, instead
// of queueing behind the (likely hung) head and eating its full ceiling.
func TestBridge_StalledHead_RejectsNewArrival(t *testing.T) {
	b := setupBridgeWithClientStall(t, 40*time.Millisecond)

	// op1 hangs: the client never responds, so it holds the slot. Its own ctx is
	// cancelled at test end; the slot stays held by design until true resolution.
	ctx1, cancel1 := context.WithCancel(context.Background())
	t.Cleanup(cancel1)
	go func() { _, _ = b.Send(ctx1, "set_text", []string{"1:1"}, nil) }()

	// Wait until op1 holds the slot, then past the stall threshold.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !slotBusy(b) {
		time.Sleep(2 * time.Millisecond)
	}
	if !slotBusy(b) {
		t.Fatal("op1 never acquired the slot")
	}
	time.Sleep(70 * time.Millisecond) // exceed the 40ms stall threshold

	// op2 must be early-rejected fast (well under any request ceiling).
	start := time.Now()
	_, err := b.Send(context.Background(), "set_text", []string{"2:2"}, nil)
	if !errors.Is(err, ErrChannelStalled) {
		t.Fatalf("op2 err = %v, want ErrChannelStalled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("op2 took %v — early-reject should be immediate", elapsed)
	}
}

// A progressing head keeps lastProgressAt fresh, so a peer is NOT falsely rejected.
// Uses a roomy threshold so the assert-right-after-tick can't flake under a loaded
// scheduler (we are testing the logic, not a tight deadline).
func TestBridge_ProgressingHead_NotStalled(t *testing.T) {
	b := setupBridgeWithClientStall(t, 500*time.Millisecond)

	// Manually occupy the slot and keep progress fresh past the threshold window.
	b.mu.RLock()
	var entry *connEntry
	for _, e := range b.conns {
		entry = e
	}
	b.mu.RUnlock()
	entry.sem <- struct{}{}
	t.Cleanup(func() { <-entry.sem })
	entry.markProgress()
	time.Sleep(50 * time.Millisecond)
	entry.markProgress() // fresh tick — resets the stall clock

	if entry.isStalled(b.stallThreshold) {
		t.Error("a head with a fresh progress tick must not be stalled")
	}
}
