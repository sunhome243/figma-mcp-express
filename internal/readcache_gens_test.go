package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A same-channel reconnect must NOT delete the channel's read-cache generation:
// the displaced connection's readLoop cleanup is guarded by `cur == entry`, so the
// new owner's cache state must survive. (Regression guard for moving DeleteChannel
// out of that guard block — which would wipe the live channel's cache on reconnect.)
func TestBridge_ReconnectDoesNotDeleteChannelCache(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	dialChannel(t, srv, "fileA")
	waitChannels(t, b, 1)

	// Simulate a write having populated the channel's generation counter.
	b.readCache.InvalidateChannel("fileA")
	if got := b.readCache.gensLen(); got != 1 {
		t.Fatalf("setup: gensLen=%d, want 1", got)
	}

	// Reconnect on the SAME channel: the displaced conn's readLoop cleanup runs with
	// cur(=new entry) != entry(=old), so it must skip both the conns delete and
	// DeleteChannel. The channel's gen must remain.
	dialChannel(t, srv, "fileA")
	waitChannels(t, b, 1)

	// Poll past the displaced readLoop's async cleanup; gen must never drop to 0.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := b.readCache.gensLen(); got != 1 {
			t.Fatalf("reconnect wiped the channel gen (gensLen=%d) — DeleteChannel fired on a displaced reconnect", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The gens map must not grow without bound: each channel that disconnects for good
// (DeleteChannel) must leave no residual generation entry. This guards the unbounded-
// map leak where auto-N channel ids accumulate across reconnects.
func TestReadCache_DeleteChannel_NoGensLeak(t *testing.T) {
	c := newReadCache()
	if !c.enabled() {
		t.Skip("read cache disabled in this environment")
	}

	const cycles = 500
	for i := 0; i < cycles; i++ {
		ch := fmt.Sprintf("auto-%d", i)
		c.InvalidateChannel(ch) // simulates (re)registration / a write on the channel
	}
	if got := c.gensLen(); got != cycles {
		t.Fatalf("after %d distinct channels, gensLen=%d, want %d (setup sanity)", cycles, got, cycles)
	}

	for i := 0; i < cycles; i++ {
		c.DeleteChannel(fmt.Sprintf("auto-%d", i))
	}
	if got := c.gensLen(); got != 0 {
		t.Fatalf("after deleting all channels, gensLen=%d, want 0 (gens leaked)", got)
	}
}

// DeleteChannel must also evict that channel's cached entries, while leaving other
// channels' entries intact.
func TestReadCache_DeleteChannel_EvictsOnlyThatChannel(t *testing.T) {
	c := newReadCache()
	if !c.enabled() {
		t.Skip("read cache disabled in this environment")
	}
	resp := BridgeResponse{Type: "get_node", Data: map[string]any{"id": "1:2"}}

	genA := c.currentGen("chan-A")
	c.Put("kA", "chan-A", genA, resp)
	genB := c.currentGen("chan-B")
	c.Put("kB", "chan-B", genB, resp)
	if c.len() != 2 {
		t.Fatalf("setup: cache len=%d, want 2", c.len())
	}

	c.DeleteChannel("chan-A")

	if _, _, ok := c.Get("kA", "chan-A"); ok {
		t.Fatalf("chan-A entry should be evicted after DeleteChannel")
	}
	if _, _, ok := c.Get("kB", "chan-B"); !ok {
		t.Fatalf("chan-B entry must survive DeleteChannel(chan-A)")
	}
}
