package internal

import (
	"fmt"
	"testing"
)

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
