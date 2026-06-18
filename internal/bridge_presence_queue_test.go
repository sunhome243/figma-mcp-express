package internal

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"
)

// readFrame is a permissive view over everything the bridge writes to the plugin
// socket — both real BridgeRequests (have a requestId) and presence_queue frames
// (have origins). The test inspects Type to tell them apart.
type readFrame struct {
	Type      string   `json:"type"`
	RequestID string   `json:"requestId"`
	Channel   string   `json:"channel"`
	Origins   []string `json:"origins"`
}

// soleConnEntry returns the single connEntry registered on the bridge (the test
// fixture connects exactly one client).
func soleConnEntry(t *testing.T, b *Bridge) *connEntry {
	t.Helper()
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.conns) != 1 {
		t.Fatalf("expected exactly one connection, got %d", len(b.conns))
	}
	for _, e := range b.conns {
		return e
	}
	return nil
}

// Two waiters on the per-channel serial slot, each carrying a roster origin, must
// be reflected in entry.waitingOrigins AND pushed to the plugin as presence_queue
// frames. Both the acquired path and the ctx-cancel path must clear the entry.
func TestBridgePresenceQueue_TwoWaitersBroadcastAndClear(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	entry := soleConnEntry(t, b)

	// Mock plugin: collect every frame the bridge writes (requests + presence frames).
	// Real requests are echoed onto reqs so the test can resolve them; presence_queue
	// frames are collected onto queues for assertions.
	reqs := make(chan readFrame, 16)
	queues := make(chan []string, 64)
	go func() {
		for {
			var f readFrame
			if err := wsjson.Read(context.Background(), clientConn, &f); err != nil {
				return
			}
			if f.Type == presenceQueueType {
				origins := append([]string(nil), f.Origins...)
				sort.Strings(origins)
				queues <- origins
			} else if f.RequestID != "" {
				reqs <- f
			}
		}
	}()

	// Request A: a write op with origin "grace". Client-cancel it AFTER it has been
	// dispatched (it reached the plugin) — the slot stays HELD (single-threaded
	// plugin still "executing" it), so subsequent waiters genuinely queue on sem.
	ctxA, cancelA := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancelA()
	_, _ = b.Send(ctxA, "set_fills", []string{"1:1"}, map[string]interface{}{
		"color": "#ffffff", "origin": "grace",
	})
	// A reached the plugin (slot acquired). A's waiting registration was added then
	// removed on acquisition → the plugin saw [grace] then [] for A.
	select {
	case <-reqs:
	case <-time.After(time.Second):
		t.Fatal("A never reached the plugin")
	}

	// Request B: a write op with origin "theo". A holds the slot, so B queues —
	// registering "theo" as waiting and broadcasting [theo].
	go func() {
		_, _ = b.Send(context.Background(), "set_strokes", []string{"1:1"}, map[string]interface{}{
			"color": "#000000", "origin": "theo",
		})
	}()

	// Poll the live waitingOrigins until "theo" appears (B has registered as waiting).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hasWaitingOrigin(entry, "theo") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !hasWaitingOrigin(entry, "theo") {
		t.Fatal("B's origin 'theo' never registered in waitingOrigins")
	}
	// B has NOT acquired the slot yet, so it must still be in flight as a waiter.
	if got := snapshotForTest(entry); len(got) != 1 || got[0] != "theo" {
		t.Fatalf("waitingOrigins while B queued = %v; want [theo]", got)
	}

	// The plugin must have received at least one presence_queue frame listing [theo].
	if !sawQueueFrame(t, queues, []string{"theo"}, time.Second) {
		t.Fatal("plugin never received a presence_queue frame listing [theo]")
	}

	// Resolve A late so its held slot releases → B acquires → B's waiter clears →
	// a final broadcast of [] (nobody waiting).
	wsjson.Write(context.Background(), clientConn, BridgeResponse{ //nolint:errcheck
		Type: "set_fills", RequestID: lastPendingRequestID(b), Data: map[string]any{"ok": true},
	})

	// After A resolves, B dispatches and acquires; its waiter is cleared.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(snapshotForTest(entry)) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := snapshotForTest(entry); len(got) != 0 {
		t.Fatalf("waitingOrigins after B acquired = %v; want empty (B's waiter cleared)", got)
	}
}

// The ctx-cancel branch must clear a waiter: a queued request whose ctx is
// cancelled while still waiting on sem removes its origin from waitingOrigins.
func TestBridgePresenceQueue_CancelledWaiterClears(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	entry := soleConnEntry(t, b)

	reqs := make(chan readFrame, 16)
	go func() {
		for {
			var f readFrame
			if err := wsjson.Read(context.Background(), clientConn, &f); err != nil {
				return
			}
			if f.Type != presenceQueueType && f.RequestID != "" {
				reqs <- f
			}
		}
	}()

	// A holds the slot (client-cancelled after dispatch, slot stays held).
	ctxA, cancelA := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelA()
	_, _ = b.Send(ctxA, "set_fills", []string{"1:1"}, map[string]interface{}{"color": "#fff", "origin": "grace"})
	select {
	case <-reqs:
	case <-time.After(time.Second):
		t.Fatal("A never reached the plugin")
	}

	// B queues with origin "zoe", then is cancelled while still waiting on sem.
	ctxB, cancelB := context.WithCancel(context.Background())
	bDone := make(chan error, 1)
	go func() {
		_, err := b.Send(ctxB, "set_strokes", []string{"1:1"}, map[string]interface{}{"color": "#000", "origin": "zoe"})
		bDone <- err
	}()

	// Wait until "zoe" is registered as waiting.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hasWaitingOrigin(entry, "zoe") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !hasWaitingOrigin(entry, "zoe") {
		t.Fatal("B's origin 'zoe' never registered as waiting")
	}

	// Cancel B while it is still queued (A still holds the slot).
	cancelB()
	if err := <-bDone; err == nil {
		t.Fatal("expected cancelled B to return an error")
	}

	// B's waiter must be cleared by the ctx.Done() branch.
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !hasWaitingOrigin(entry, "zoe") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if hasWaitingOrigin(entry, "zoe") {
		t.Fatalf("cancelled waiter 'zoe' was not cleared: %v", snapshotForTest(entry))
	}
}

// When the serial slot is acquired and the waiter cleared, the clearing
// presence_queue frame ([] empty) MUST still reach the plugin even if an op write
// is holding entry.wmu at the instant the broadcast fires. Once the queue has
// drained to empty there is NO "later change" to ride a re-broadcast on, so a
// single dropped attempt leaves the plugin's queued list stale forever — the agent
// shows "queued" permanently and its row keeps re-stamping to "Just now" on every
// sweep (never decaying to away). Regression for the stuck-queued bug.
func TestBridgePresenceQueue_ClearFrameSurvivesWriteContention(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	entry := soleConnEntry(t, b)

	queues := make(chan []string, 64)
	go func() {
		for {
			var f readFrame
			if err := wsjson.Read(context.Background(), clientConn, &f); err != nil {
				return
			}
			if f.Type == presenceQueueType {
				origins := append([]string(nil), f.Origins...)
				sort.Strings(origins)
				queues <- origins
			}
		}
	}()

	// Register a waiter, then simulate the acquiring op write holding wmu at the exact
	// instant the clearing broadcast fires — broadcastQueue's TryLock loses the race.
	tok := "wait-1"
	entry.addWaitingOrigin(tok, "grace")

	entry.wmu.Lock() // stand in for the op write that holds the write mutex
	entry.removeWaitingOrigin(tok)
	b.broadcastQueue("test-channel", entry) // clear → [] ; its goroutine must not give up
	// Hold the lock long enough that a single TryLock attempt is guaranteed to lose,
	// then release so a retry can deliver the cleared state.
	time.Sleep(40 * time.Millisecond)
	entry.wmu.Unlock()

	// The plugin MUST eventually receive the empty clearing frame. With drop-on-first-
	// contention it never arrives (no later change re-broadcasts an empty queue).
	if !sawQueueFrame(t, queues, []string{}, 2*time.Second) {
		t.Fatal("plugin never received the empty clearing presence_queue frame after wmu contention")
	}
}

// ── test helpers ──────────────────────────────────────────────────────────────

func snapshotForTest(e *connEntry) []string { return e.snapshotWaitingOrigins() }

func hasWaitingOrigin(e *connEntry, origin string) bool {
	for _, o := range e.snapshotWaitingOrigins() {
		if o == origin {
			return true
		}
	}
	return false
}

// sawQueueFrame drains presence_queue frames until one equals want (sorted) or the
// timeout elapses.
func sawQueueFrame(t *testing.T, queues <-chan []string, want []string, timeout time.Duration) bool {
	t.Helper()
	sort.Strings(want)
	deadline := time.After(timeout)
	for {
		select {
		case got := <-queues:
			if equalStrings(got, want) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lastPendingRequestID returns the requestID of the (single) currently-pending
// request — the held-slot request A in these tests, used to craft its late reply.
func lastPendingRequestID(b *Bridge) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for id := range b.pending {
		return id
	}
	return ""
}
