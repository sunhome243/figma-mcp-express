package internal

import (
	"container/list"
	"sync"
	"time"
)

// readCache is a bounded, TTL'd, in-process cache of read-only plugin responses.
//
// WHY: singleflight (doSingleflight) only collapses reads that are SIMULTANEOUSLY
// in flight. When N subagents read the same node a few hundred ms apart (the common
// multi-subagent pattern), each one still hits the single-threaded plugin. This cache
// extends the collapse window across near-simultaneous reads so only the first one
// touches the plugin; the rest get an instant in-process hit.
//
// CORRECTNESS (a stale read crossing a mutation is a bug):
//   - Only isReadOnly ops are ever cached; writes are never cached.
//   - ANY write on a channel invalidates ALL cached reads for that channel (coarse,
//     safe) by bumping a per-channel generation counter AND clearing the map.
//   - A SHORT TTL (default 3s) bounds staleness from EXTERNAL edits in Figma Desktop
//     (a user dragging the canvas) which our write-invalidation cannot see. The goal
//     is to extend singleflight's window, not to cache long-term.
//   - The generation counter closes the populate-after-invalidate race: a read that
//     went live BEFORE a write must not land its (now-stale) result into the cache
//     AFTER that write's invalidation. Get() snapshots the channel gen at the miss
//     point; Put() stores that snapshot; a later Get() returns a hit only if the
//     entry's stored gen still equals the channel's current gen.
type readCache struct {
	mu       sync.Mutex
	entries  map[string]*list.Element // key → *list element holding a *cacheItem
	order    *list.List               // LRU ordering, front = most recently used
	gens     map[string]uint64        // channel → current generation
	maxItems int
	ttl      time.Duration
}

// cacheItem is one cached read response plus the metadata that governs its validity.
type cacheItem struct {
	key       string
	channel   string
	gen       uint64 // channel generation snapshot captured at the miss point
	resp      BridgeResponse
	expiresAt time.Time
}

// Cache tuning env vars (all optional; sane defaults below).
const (
	readCacheDefaultTTLMs    = 3000 // 3s — conservative; bounds external-edit staleness
	readCacheDefaultMaxItems = 256
)

// newReadCache builds a readCache from environment overrides, falling back to the
// defaults. A non-positive TTL or size disables nothing here — the bridge decides
// whether to use the cache at all; this constructor only resolves the knobs.
func newReadCache() *readCache {
	return &readCache{
		entries:  make(map[string]*list.Element),
		order:    list.New(),
		gens:     make(map[string]uint64),
		maxItems: envInt("FIGMA_MCP_READCACHE_MAX", readCacheDefaultMaxItems),
		ttl:      time.Duration(envInt("FIGMA_MCP_READCACHE_TTL_MS", readCacheDefaultTTLMs)) * time.Millisecond,
	}
}

// enabled reports whether the cache is operational. A TTL or size of zero (only
// reachable if a future caller constructs the struct directly) disables it.
func (c *readCache) enabled() bool {
	return c != nil && c.ttl > 0 && c.maxItems > 0
}

// currentGen returns the channel's current generation, snapshotting it under the
// lock. Callers capture this at the MISS point — before issuing the live read — and
// hand it back to Put so a result that predates a later invalidation cannot land.
func (c *readCache) currentGen(channel string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gens[channel]
}

// Get returns a cached response and ok=true on a fresh, non-invalidated hit.
// On a miss it returns the channel's current generation so the caller can pass it
// to Put after the live read; ok=false signals "go live".
//
// A hit requires: the entry exists, has not expired (TTL), and its stored gen still
// equals the channel's current gen (no write invalidated the channel since the read
// that populated it began).
func (c *readCache) Get(key, channel string) (resp BridgeResponse, gen uint64, ok bool) {
	if !c.enabled() {
		return BridgeResponse{}, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	curGen := c.gens[channel]
	el, present := c.entries[key]
	if !present {
		return BridgeResponse{}, curGen, false
	}
	item := el.Value.(*cacheItem)
	if item.gen != curGen || time.Now().After(item.expiresAt) {
		// Stale (invalidated) or expired — drop it and treat as a miss.
		c.removeElement(el)
		return BridgeResponse{}, curGen, false
	}
	c.order.MoveToFront(el) // LRU touch
	return item.resp, curGen, true
}

// Put stores a read response under key for channel, stamped with the generation that
// was current when the live read BEGAN (the snapshot from Get). If the channel has
// since been invalidated (gen advanced) the Put is silently dropped — the result is
// already stale and must not become a future hit. This is the populate-after-
// invalidate guard.
func (c *readCache) Put(key, channel string, gen uint64, resp BridgeResponse) {
	if !c.enabled() {
		return
	}
	// Never cache an error response — errors are transient (plugin not connected,
	// node not found mid-edit) and caching them would mask recovery.
	if resp.Error != "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gens[channel] != gen {
		return // a write invalidated this channel after the read began — drop.
	}

	if el, present := c.entries[key]; present {
		// Refresh in place.
		item := el.Value.(*cacheItem)
		item.resp = resp
		item.gen = gen
		item.expiresAt = time.Now().Add(c.ttl)
		c.order.MoveToFront(el)
		return
	}

	item := &cacheItem{
		key:       key,
		channel:   channel,
		gen:       gen,
		resp:      resp,
		expiresAt: time.Now().Add(c.ttl),
	}
	el := c.order.PushFront(item)
	c.entries[key] = el

	// Evict the oldest while over the cap.
	for c.order.Len() > c.maxItems {
		c.removeElement(c.order.Back())
	}
}

// InvalidateChannel drops every cached read for one channel and advances that
// channel's generation so any in-flight read that predates this call cannot land a
// stale result via Put. Called on ANY write (incl. batch) and on (re)registration.
func (c *readCache) InvalidateChannel(channel string) {
	if !c.enabled() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.gens[channel]++
	// Drop all entries for this channel. The gen bump alone makes stale entries miss,
	// but clearing reclaims memory immediately rather than waiting for TTL/LRU.
	for el := c.order.Front(); el != nil; {
		next := el.Next()
		if el.Value.(*cacheItem).channel == channel {
			c.removeElement(el)
		}
		el = next
	}
}

// DeleteChannel drops every cached entry for a channel AND its generation counter.
// Called when a channel disconnects for good (NOT on a same-channel reconnect, which
// uses InvalidateChannel). Without this, gens[channel] accumulates forever as
// auto-assigned channel ids (auto-N) churn across reconnects — a textbook unbounded
// map. A later reconnect re-registers the channel and InvalidateChannel re-seeds its
// gen from zero, which is safe: an in-flight read holding an old gen snapshot simply
// fails its Put gen-equality check and is dropped (the channel's data is gone anyway).
func (c *readCache) DeleteChannel(channel string) {
	if !c.enabled() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for el := c.order.Front(); el != nil; {
		next := el.Next()
		if el.Value.(*cacheItem).channel == channel {
			c.removeElement(el)
		}
		el = next
	}
	delete(c.gens, channel)
}

// gensLen returns the number of tracked channel generations (test/observability helper).
func (c *readCache) gensLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.gens)
}

// removeElement unlinks an element from both the map and the LRU list. Caller holds c.mu.
func (c *readCache) removeElement(el *list.Element) {
	if el == nil {
		return
	}
	item := el.Value.(*cacheItem)
	delete(c.entries, item.key)
	c.order.Remove(el)
}

// len returns the number of cached entries (test/observability helper).
func (c *readCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// readCacheKey derives the cache key for a read request. Uses (channel, requestType,
// nodeIDs, params) as discriminators. Channel IS part of the key — a different open
// file is different data.
//
// Returns ok=false to bypass the cache (non-cacheable types or a marshal error —
// never key on an empty payload, which would merge unrelated reads).
func readCacheKey(channel, requestType string, nodeIDs []string, params map[string]interface{}) (string, bool) {
	if !isCacheable(requestType) {
		return "", false
	}
	key, ok := hashReadKey(channel, requestType, nodeIDs, params)
	if !ok {
		bridgeLogger.Printf("readCacheKey marshal error (type=%s) — bypassing cache", requestType)
	}
	return key, ok
}
