package internal

import (
	"testing"
	"time"
)

// newTestReadCache builds a readCache with explicit knobs, bypassing env so tests
// are deterministic regardless of the developer's environment.
func newTestReadCache(ttl time.Duration, maxItems int) *readCache {
	c := newReadCache()
	c.ttl = ttl
	c.maxItems = maxItems
	return c
}

func TestReadCache_PutThenGetHit(t *testing.T) {
	c := newTestReadCache(time.Second, 8)
	resp := BridgeResponse{Data: map[string]any{"id": "1:1"}}

	_, gen, ok := c.Get("k", "chA")
	if ok {
		t.Fatal("expected miss on empty cache")
	}
	c.Put("k", "chA", gen, resp)

	got, _, ok := c.Get("k", "chA")
	if !ok {
		t.Fatal("expected hit after Put")
	}
	if got.Data == nil {
		t.Error("expected cached data")
	}
}

func TestReadCache_ChannelIsolation(t *testing.T) {
	c := newTestReadCache(time.Second, 8)
	// In production the key string is derived by readCacheKey, which embeds the
	// channel — so two channels reading the same node get DISTINCT keys. Model that.
	keyA, _ := readCacheKey("chA", "get_node", []string{"1:1"}, nil)
	keyB, _ := readCacheKey("chB", "get_node", []string{"1:1"}, nil)

	_, genA, _ := c.Get(keyA, "chA")
	c.Put(keyA, "chA", genA, BridgeResponse{Data: map[string]any{"v": "A"}})

	// The same node on a DIFFERENT channel must miss — different file = different data.
	if _, _, ok := c.Get(keyB, "chB"); ok {
		t.Error("channel B must not see channel A's entry for the same node")
	}
}

func TestReadCache_InvalidateChannel_DropsEntries(t *testing.T) {
	c := newTestReadCache(time.Second, 8)
	_, gen, _ := c.Get("k", "chA")
	c.Put("k", "chA", gen, BridgeResponse{Data: map[string]any{"v": 1}})

	c.InvalidateChannel("chA")

	if _, _, ok := c.Get("k", "chA"); ok {
		t.Error("entry should be gone after InvalidateChannel")
	}
	if c.len() != 0 {
		t.Errorf("cache should be empty after invalidation, len=%d", c.len())
	}
}

func TestReadCache_InvalidateOnlyTargetChannel(t *testing.T) {
	c := newTestReadCache(time.Second, 8)
	keyA, _ := readCacheKey("chA", "get_node", []string{"1:1"}, nil)
	keyB, _ := readCacheKey("chB", "get_node", []string{"1:1"}, nil)
	_, gA, _ := c.Get(keyA, "chA")
	c.Put(keyA, "chA", gA, BridgeResponse{Data: map[string]any{"v": "A"}})
	_, gB, _ := c.Get(keyB, "chB")
	c.Put(keyB, "chB", gB, BridgeResponse{Data: map[string]any{"v": "B"}})

	c.InvalidateChannel("chA")

	if _, _, ok := c.Get(keyA, "chA"); ok {
		t.Error("chA entry should be invalidated")
	}
	if _, _, ok := c.Get(keyB, "chB"); !ok {
		t.Error("chB entry must survive a chA invalidation")
	}
}

// TestReadCache_PopulateAfterInvalidateDropped is the core correctness guard: a read
// that captured gen BEFORE a write must NOT land its (stale) result into the cache
// after that write's invalidation.
func TestReadCache_PopulateAfterInvalidateDropped(t *testing.T) {
	c := newTestReadCache(time.Second, 8)

	// Read begins: snapshot the generation (miss point).
	_, snap, ok := c.Get("k", "chA")
	if ok {
		t.Fatal("expected miss")
	}

	// A write invalidates the channel WHILE the read is still in flight.
	c.InvalidateChannel("chA")

	// The read now resolves and tries to populate with its pre-write snapshot gen.
	c.Put("k", "chA", snap, BridgeResponse{Data: map[string]any{"stale": true}})

	// The stale Put must have been dropped — next read goes live (miss).
	if _, _, ok := c.Get("k", "chA"); ok {
		t.Error("stale result populated after invalidation must be dropped (populate-after-invalidate race)")
	}
}

// TestReadCache_PutInWriteWindowClearedByPostInvalidate pins the second half of the
// write-invalidation guarantee: a read that snapshots the gen AFTER a write's PRE
// invalidation but lands its Put BEFORE the write's POST invalidation must still be
// dropped. (Models the case where the read wins the sem race ahead of the write and
// reads pre-mutation state.) The bridge's double-invalidate (pre + post sendOnce) is
// what makes this safe; here we exercise the cache mechanism directly.
func TestReadCache_PutInWriteWindowClearedByPostInvalidate(t *testing.T) {
	c := newTestReadCache(time.Second, 8)

	c.InvalidateChannel("chA") // write PRE-bump (g0 → g1)
	snap := c.currentGen("chA")
	// Read snapshots g1, reads pre-mutation state, and Puts under g1 — accepted for now.
	c.Put("k", "chA", snap, BridgeResponse{Data: map[string]any{"preMutation": true}})
	if _, _, ok := c.Get("k", "chA"); !ok {
		t.Fatal("entry should be present immediately after the in-window Put")
	}

	c.InvalidateChannel("chA") // write POST-bump (g1 → g2) — must clear the window entry

	if _, _, ok := c.Get("k", "chA"); ok {
		t.Error("in-write-window Put must be cleared by the post-write invalidation (stale read crossing a mutation)")
	}
}

func TestReadCache_TTLExpiry(t *testing.T) {
	c := newTestReadCache(20*time.Millisecond, 8)
	_, gen, _ := c.Get("k", "chA")
	c.Put("k", "chA", gen, BridgeResponse{Data: map[string]any{"v": 1}})

	if _, _, ok := c.Get("k", "chA"); !ok {
		t.Fatal("expected fresh hit")
	}
	time.Sleep(30 * time.Millisecond)
	if _, _, ok := c.Get("k", "chA"); ok {
		t.Error("entry should have expired after TTL")
	}
}

func TestReadCache_SizeCapEvictsOldest(t *testing.T) {
	c := newTestReadCache(time.Second, 2)
	for _, k := range []string{"k1", "k2", "k3"} {
		_, gen, _ := c.Get(k, "chA")
		c.Put(k, "chA", gen, BridgeResponse{Data: map[string]any{"k": k}})
	}
	if c.len() != 2 {
		t.Fatalf("expected cap of 2, got len=%d", c.len())
	}
	// k1 was the oldest → evicted; k2/k3 remain.
	if _, _, ok := c.Get("k1", "chA"); ok {
		t.Error("oldest entry k1 should have been evicted")
	}
	if _, _, ok := c.Get("k3", "chA"); !ok {
		t.Error("newest entry k3 should remain")
	}
}

func TestReadCache_ErrorResponseNotCached(t *testing.T) {
	c := newTestReadCache(time.Second, 8)
	_, gen, _ := c.Get("k", "chA")
	c.Put("k", "chA", gen, BridgeResponse{Error: "node not found"})
	if _, _, ok := c.Get("k", "chA"); ok {
		t.Error("error responses must never be cached")
	}
}

func TestReadCache_Disabled(t *testing.T) {
	c := newTestReadCache(0, 8) // TTL 0 disables
	_, gen, ok := c.Get("k", "chA")
	if ok {
		t.Error("disabled cache must always miss")
	}
	c.Put("k", "chA", gen, BridgeResponse{Data: map[string]any{"v": 1}})
	if _, _, ok := c.Get("k", "chA"); ok {
		t.Error("disabled cache must not store")
	}
}

// ── readCacheKey ─────────────────────────────────────────────────────────────

func TestReadCacheKey_ResultParamsDiscriminate(t *testing.T) {
	k1, _ := readCacheKey("chA", "get_node", []string{"1:1"}, map[string]interface{}{"detail": "full"})
	k2, _ := readCacheKey("chA", "get_node", []string{"1:1"}, map[string]interface{}{"detail": "summary"})
	if k1 == k2 {
		t.Error("result-affecting params must produce distinct keys")
	}
}

func TestReadCacheKey_ChannelDiscriminates(t *testing.T) {
	k1, _ := readCacheKey("chA", "get_node", []string{"1:1"}, nil)
	k2, _ := readCacheKey("chB", "get_node", []string{"1:1"}, nil)
	if k1 == k2 {
		t.Error("different channels must produce distinct keys")
	}
}

func TestReadCacheKey_NonReadBypassed(t *testing.T) {
	if _, ok := readCacheKey("chA", "create_frame", nil, nil); ok {
		t.Error("non-read types must not be cacheable")
	}
	if _, ok := readCacheKey("chA", "batch", nil, nil); ok {
		t.Error("batch must not be cacheable")
	}
}
