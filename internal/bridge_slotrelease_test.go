package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"
)

// REGRESSION GUARD (was the reproduction): when a NON-import request is client-
// cancelled AFTER it was dispatched to the plugin but BEFORE the plugin responded,
// the per-channel serial slot must STAY HELD until the request truly resolves — the
// plugin is single-threaded and is still executing the cancelled request (it was
// never told to cancel), so the next request must not overlap it.
//
// The bug this guards: the slot used to be released by an unconditional
// `defer func(){ <-entry.sem }()` the moment sendOnce returned, including the
// ctx.Done() client-cancel branch — so request B acquired the freed slot and was
// written to the plugin while cancelled-A was still occupying the thread. The fix
// routes slot release through pe.onResolve (true resolution only: plugin response,
// inactivity timer, write error, or connection drop), never on client-cancel.
//
// This test asserts the hardened behavior: request B must NOT reach the plugin while
// cancelled-A is still in flight. (Companion tests below prove the slot is eventually
// released once A truly resolves, so this is a held slot, not a leaked one.)
func TestBridgeSend_NonImportSlotHeldUntilResolveOnCancel(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)

	// Mock plugin: record every request that reaches the wire, but never respond —
	// this models the single thread still busy executing the cancelled request.
	arrived := make(chan string, 4)
	go func() {
		for {
			var req BridgeRequest
			if err := wsjson.Read(context.Background(), clientConn, &req); err != nil {
				return
			}
			arrived <- req.Type
		}
	}()

	// Request A: a non-import read. The client cancels it (mirrors the MCP-client
	// request-timeout firing mid-flight) before the plugin ever responds.
	ctxA, cancelA := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancelA()
	if _, err := b.Send(ctxA, "get_node", []string{"1:1"}, nil); err == nil {
		t.Fatal("expected request A to fail on client-cancel")
	}

	// Confirm A actually reached the plugin (so the thread is genuinely occupied).
	select {
	case got := <-arrived:
		if got != "get_node" {
			t.Fatalf("expected A=get_node on the wire, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("A never reached the plugin — test precondition failed")
	}

	// Request B: the next op the agent dispatches. A has NOT resolved (no response,
	// timer is the long op ceiling). B is a write, so no readcache/singleflight
	// interaction — it can only proceed by acquiring the serial slot.
	ctxB, cancelB := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancelB()
	go func() { _, _ = b.Send(ctxB, "set_fills", []string{"1:1"}, map[string]interface{}{"fills": []any{}}) }()

	// HARDENED EXPECTATION: B must be held back while cancelled-A is still in flight.
	select {
	case got := <-arrived:
		t.Fatalf("REGRESSION: B (%q) was dispatched to the plugin while cancelled-A is still in flight — "+
			"the non-import serial slot was released on client-cancel before the plugin responded; "+
			"two requests now overlap on the single-threaded plugin", got)
	case <-time.After(200 * time.Millisecond):
		// Hardened: B correctly withheld until A resolves.
	}
}

// The held slot is RELEASED — not leaked — when the cancelled request truly resolves
// via a (late) plugin response: the next request then proceeds and completes cleanly.
// Proves onResolve fires exactly once on real resolution after a client-cancel.
func TestBridgeSend_SlotReleasedWhenCancelledRequestLateResponds(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)

	reqs := make(chan BridgeRequest, 4)
	go func() {
		for {
			var req BridgeRequest
			if err := wsjson.Read(context.Background(), clientConn, &req); err != nil {
				return
			}
			reqs <- req
		}
	}()

	// A: non-import read, client-cancelled before any plugin response.
	ctxA, cancelA := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancelA()
	if _, err := b.Send(ctxA, "get_node", []string{"1:1"}, nil); err == nil {
		t.Fatal("expected A to fail on client-cancel")
	}
	var reqA BridgeRequest
	select {
	case reqA = <-reqs:
	case <-time.After(time.Second):
		t.Fatal("A never reached the plugin")
	}

	// B queues on the still-held slot.
	bDone := make(chan error, 1)
	go func() {
		_, err := b.Send(context.Background(), "set_fills", []string{"1:1"}, map[string]interface{}{"color": "#ffffff"})
		bDone <- err
	}()

	// B must not have reached the plugin while cancelled-A is unresolved.
	select {
	case <-reqs:
		t.Fatal("B dispatched while cancelled-A still in flight (slot not held)")
	case <-time.After(150 * time.Millisecond):
	}

	// A's late response arrives → resolves A → releases the slot exactly once.
	wsjson.Write(context.Background(), clientConn, BridgeResponse{ //nolint:errcheck
		Type: reqA.Type, RequestID: reqA.RequestID, Data: map[string]any{"ok": true},
	})

	// B now reaches the plugin; respond so it completes cleanly (no leak, no error).
	select {
	case reqB := <-reqs:
		wsjson.Write(context.Background(), clientConn, BridgeResponse{ //nolint:errcheck
			Type: reqB.Type, RequestID: reqB.RequestID, Data: map[string]any{"ok": true},
		})
	case <-time.After(2 * time.Second):
		t.Fatal("B never dispatched after A's late response should have released the slot")
	}
	if err := <-bDone; err != nil {
		t.Fatalf("B should complete cleanly after acquiring the released slot, got: %v", err)
	}
}

// The held slot is also released by the SERVER inactivity timer when the cancelled
// request never gets a response (genuinely-hung plugin) — bounding the worst-case
// hold to the op ceiling rather than leaking the slot forever.
func TestBridgeSend_SlotReleasedByTimerAfterCancel(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "1") // 1s inactivity ceiling for non-heavy ops
	b, clientConn := setupBridgeWithClient(t)

	reqs := make(chan BridgeRequest, 4)
	go func() {
		for {
			var req BridgeRequest
			if err := wsjson.Read(context.Background(), clientConn, &req); err != nil {
				return
			}
			reqs <- req
		}
	}()

	// A: a write op (ceiling = FIGMA_MCP_TIMEOUT, not the 600s heavy-read ceiling),
	// client-cancelled and never answered by the mock.
	ctxA, cancelA := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancelA()
	_, _ = b.Send(ctxA, "set_fills", []string{"1:1"}, map[string]interface{}{"color": "#ffffff"})
	select {
	case <-reqs:
	case <-time.After(time.Second):
		t.Fatal("A never reached the plugin")
	}

	go func() { _, _ = b.Send(context.Background(), "set_strokes", []string{"1:1"}, map[string]interface{}{"color": "#000000"}) }()

	// Well under the 1s ceiling, B is still held back.
	select {
	case <-reqs:
		t.Fatal("B dispatched before A's inactivity timer fired (slot not held)")
	case <-time.After(300 * time.Millisecond):
	}

	// After A's 1s timer fires, onResolve releases the slot and B dispatches.
	select {
	case <-reqs:
	case <-time.After(2 * time.Second):
		t.Fatal("B never dispatched — slot not released by A's inactivity timer")
	}
}

// A client-cancelled import leaves importInFlight set (retries rejected) AND holds the
// slot — but unlike before the fix, it SELF-HEALS: once the inactivity timer fires,
// onResolve clears the marker and releases the slot, so a later import is accepted
// instead of the channel being import-poisoned until reconnect.
func TestBridgeSend_ImportMarkerSelfHealsAfterCancelTimer(t *testing.T) {
	t.Setenv("FIGMA_MCP_TIMEOUT", "1") // import ceiling = FIGMA_MCP_TIMEOUT (imports aren't heavy reads)
	b, clientConn := setupBridgeWithClient(t)

	go func() {
		for {
			var req BridgeRequest
			if err := wsjson.Read(context.Background(), clientConn, &req); err != nil {
				return
			}
			_ = req // never respond — the import "hangs"
		}
	}()

	// First import: client-cancelled before responding.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel1()
	_, _ = b.Send(ctx1, "import_component_by_key", nil, map[string]interface{}{"key": "abc"})

	// During the window the marker is set → a retried import is rejected EARLY.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel2()
	if _, err := b.Send(ctx2, "import_component_by_key", nil, map[string]interface{}{"key": "abc2"}); !errors.Is(err, ErrImportInFlight) {
		t.Fatalf("retried import during the window should be rejected with ErrImportInFlight, got: %v", err)
	}

	// Wait past the 1s ceiling so the first import's timer fires → marker cleared, slot freed.
	time.Sleep(1300 * time.Millisecond)

	// A fresh import is now ACCEPTED: it dispatches and times out on its own ctx, rather
	// than being rejected with ErrImportInFlight.
	ctx3, cancel3 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel3()
	if _, err := b.Send(ctx3, "import_component_by_key", nil, map[string]interface{}{"key": "def"}); errors.Is(err, ErrImportInFlight) {
		t.Fatal("import marker did not self-heal after cancel + inactivity timer — channel still import-poisoned")
	}
}
