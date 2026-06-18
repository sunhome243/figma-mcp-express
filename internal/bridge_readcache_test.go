package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// countingPlugin runs an echo-responder loop over a client connection and counts how
// many requests it actually received from the bridge — i.e. how many reached the
// (simulated) single-threaded plugin. A cache hit must NOT increment this count.
type countingPlugin struct {
	conn  *websocket.Conn
	hits  atomic.Int64
	gate  chan struct{} // when non-nil, each request blocks until a token is sent
	close chan struct{}
}

func startCountingPlugin(t *testing.T, conn *websocket.Conn) *countingPlugin {
	t.Helper()
	p := &countingPlugin{conn: conn, close: make(chan struct{})}
	go func() {
		ctx := context.Background()
		for {
			var req BridgeRequest
			if err := wsjson.Read(ctx, conn, &req); err != nil {
				return
			}
			p.hits.Add(1)
			if p.gate != nil {
				select {
				case <-p.gate:
				case <-p.close:
					return
				}
			}
			resp := BridgeResponse{
				RequestID: req.RequestID,
				Type:      req.Type,
				Data:      map[string]any{"id": "1:1", "echoType": req.Type},
			}
			_ = wsjson.Write(ctx, conn, resp)
		}
	}()
	t.Cleanup(func() { close(p.close) })
	return p
}

// setupBridgeChannel wires a bridge + a single named channel and a counting plugin.
func setupBridgeChannel(t *testing.T, channel string) (*Bridge, *countingPlugin) {
	t.Helper()
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)
	conn := dialChannel(t, srv, channel)
	waitChannels(t, b, 1)
	return b, startCountingPlugin(t, conn)
}

// ── C4: read-cache end-to-end ─────────────────────────────────────────────────

func TestBridge_ReadCacheHitSkipsPlugin(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()

	if _, err := b.Send(ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileA"}); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := b.Send(ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileA"}); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if got := plugin.hits.Load(); got != 1 {
		t.Errorf("expected exactly 1 plugin hit (second read served from cache), got %d", got)
	}
}

// A write between two reads must invalidate the cache so the second read goes live.
func TestBridge_WriteInvalidatesCache(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()
	ch := map[string]interface{}{"channel": "fileA"}

	mustSend(t, b, ctx, "get_node", []string{"1:1"}, cloneParams(ch))  // hit #1 (live)
	mustSend(t, b, ctx, "set_fills", []string{"1:1"}, cloneParams(ch)) // write → invalidate (hit #2)
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, cloneParams(ch))  // must go live (hit #3)

	if got := plugin.hits.Load(); got != 3 {
		t.Errorf("expected 3 plugin hits (read, write, re-read live), got %d", got)
	}
}

// A read issued via the batch tool must NOT be served from or populate the cache.
func TestBridge_BatchReadBypassesCache(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()
	ch := map[string]interface{}{"channel": "fileA"}

	// Prime the cache with a direct read.
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, cloneParams(ch)) // hit #1
	// A batch send (top-level type "batch") must always go live — it is not read-only.
	mustSend(t, b, ctx, "batch", nil, cloneParams(ch)) // hit #2 (live, not cached)
	// The batch also invalidated the channel (it may contain writes), so the next
	// direct read goes live too.
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, cloneParams(ch)) // hit #3 (live)

	if got := plugin.hits.Load(); got != 3 {
		t.Errorf("expected 3 plugin hits (batch never uses cache), got %d", got)
	}
}

// Identical params must still hit the same cache entry (regression: no timeoutMs
// param exists any longer, so any two identical reads must share one cache entry).
func TestBridge_IdenticalReadsHitSameCacheEntry(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()

	mustSend(t, b, ctx, "get_node", []string{"1:1"},
		map[string]interface{}{"channel": "fileA"}) // live
	mustSend(t, b, ctx, "get_node", []string{"1:1"},
		map[string]interface{}{"channel": "fileA"}) // cache hit

	if got := plugin.hits.Load(); got != 1 {
		t.Errorf("identical reads should hit the same cache entry: expected 1 plugin hit, got %d", got)
	}
}

// Different channels are isolated — same node id on two files = two live reads.
func TestBridge_DifferentChannelsIsolated(t *testing.T) {
	b := NewBridge()
	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)
	connA := dialChannel(t, srv, "fileA")
	connB := dialChannel(t, srv, "fileB")
	waitChannels(t, b, 2)
	pA := startCountingPlugin(t, connA)
	pB := startCountingPlugin(t, connB)
	ctx := context.Background()

	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileB"})

	if pA.hits.Load() != 1 || pB.hits.Load() != 1 {
		t.Errorf("each channel must serve its own read live: A=%d B=%d", pA.hits.Load(), pB.hits.Load())
	}
}

// THE RACE TEST: a read that goes live and crosses a write must NOT cache a stale
// result. We block the in-flight read at the plugin, fire a write, release the read,
// then assert the next read goes live (the populate-after-invalidate guard).
func TestBridge_StaleReadAfterWriteGoesLive(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	plugin.gate = make(chan struct{}, 4) // block each request until a token is sent
	ctx := context.Background()
	ch := func() map[string]interface{} { return map[string]interface{}{"channel": "fileA"} }

	// Start read R; it will block inside the plugin (gated).
	rDone := make(chan struct{})
	go func() {
		_, _ = b.Send(ctx, "get_node", []string{"1:1"}, ch())
		close(rDone)
	}()
	// Wait until the plugin has actually received R (hit count == 1).
	waitHits(t, plugin, 1)

	// While R is in flight (blocked, not yet populated), perform a write. The write
	// also queues on the serial slot behind R, so release R's plugin response first,
	// but the write's InvalidateChannel runs in Send() BEFORE it queues — so the
	// generation is already bumped by the time R tries to Put.
	wDone := make(chan struct{})
	go func() {
		_, _ = b.Send(ctx, "set_fills", []string{"1:1"}, ch())
		close(wDone)
	}()

	// Give the write goroutine a moment to enter Send() and bump the generation.
	time.Sleep(30 * time.Millisecond)

	// Release R's plugin response. R resolves and tries to Put with its pre-write gen
	// snapshot — which must be dropped.
	plugin.gate <- struct{}{}
	<-rDone

	// Release the write's plugin response.
	plugin.gate <- struct{}{}
	<-wDone

	// Now a fresh read must go LIVE (R's stale result must not have populated the cache).
	plugin.gate <- struct{}{}
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, ch())

	// hits: R(1) + write(2) + final read live(3). If the stale Put had landed, the
	// final read would have been a cache hit and hits would be 2.
	if got := plugin.hits.Load(); got != 3 {
		t.Errorf("stale-after-write read must go live: expected 3 plugin hits, got %d", got)
	}
}

// A cache HIT never queued, so it must report zero queue metadata even if the
// leader that populated the entry had waited. The leader queues behind another READ
// (not a write — a write would invalidate the channel and drop the leader's Put,
// since the leader overlaps the write window).
func TestBridge_CacheHitReportsZeroQueueWait(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	plugin.gate = make(chan struct{}, 4)
	ctx := context.Background()

	// Hold the slot with a gated READ of a DIFFERENT node so a concurrent read of
	// node 1:1 queues behind it. Both are reads → no invalidation, so the leader's
	// Put survives.
	holderDone := make(chan struct{})
	go func() {
		_, _ = b.Send(ctx, "get_node", []string{"9:9"}, map[string]interface{}{"channel": "fileA"})
		close(holderDone)
	}()
	waitHits(t, plugin, 1)

	// The leader read of 1:1 queues behind the holder → queueWaitMs > 0 when it runs.
	leaderResp := make(chan BridgeResponse, 1)
	go func() {
		r, _ := b.Send(ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})
		leaderResp <- r
	}()
	waitWaiters(t, b, "fileA", 1)
	time.Sleep(30 * time.Millisecond)

	plugin.gate <- struct{}{} // release holder
	<-holderDone
	plugin.gate <- struct{}{} // release the queued leader read (populates cache)
	leader := <-leaderResp
	if leader.QueueWaitMs <= 0 {
		t.Fatalf("precondition: the leader read should have queued, got queueWaitMs=%d", leader.QueueWaitMs)
	}

	// A subsequent identical read is a cache HIT (no plugin call) — must report zero
	// queue metadata, not inherit the leader's queueWaitMs.
	hit := mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})
	if hit.QueueWaitMs != 0 || hit.QueueDepth != 0 {
		t.Errorf("cache hit must report zero queue metadata, got queueWaitMs=%d queueDepth=%d", hit.QueueWaitMs, hit.QueueDepth)
	}
}

// ── C1: queue visibility ──────────────────────────────────────────────────────

// An uncontended request has queueWaitMs ~0; a request that waited behind another
// has queueWaitMs > 0 and queueDepth reflecting the waiter.
func TestBridge_QueueWaitVisibility(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	plugin.gate = make(chan struct{}, 4)
	ctx := context.Background()

	// First request: occupies the serial slot, blocked at the plugin. Use a write so
	// neither request is served from cache.
	var firstResp BridgeResponse
	firstDone := make(chan struct{})
	go func() {
		firstResp, _ = b.Send(ctx, "set_fills", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})
		close(firstDone)
	}()
	waitHits(t, plugin, 1) // first reached the plugin → holds the slot

	// Second request: must queue behind the first. Launch it and let it accumulate wait.
	var secondResp BridgeResponse
	secondDone := make(chan struct{})
	go func() {
		secondResp, _ = b.Send(ctx, "set_strokes", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})
		close(secondDone)
	}()

	// Wait until the second request is actually queued (waiters == 1).
	waitWaiters(t, b, "fileA", 1)
	time.Sleep(40 * time.Millisecond) // let measurable wait accrue

	// Release both.
	plugin.gate <- struct{}{}
	<-firstDone
	plugin.gate <- struct{}{}
	<-secondDone

	if firstResp.QueueWaitMs > 20 {
		t.Errorf("first (uncontended) request should have ~0 queueWaitMs, got %d", firstResp.QueueWaitMs)
	}
	if secondResp.QueueWaitMs <= 0 {
		t.Errorf("second (queued) request should have queueWaitMs > 0, got %d", secondResp.QueueWaitMs)
	}
	// When the second acquired, the first had already left the queue (it was holding
	// the slot, not waiting), so queueDepth (OTHER waiters) is 0 — the meaningful check
	// is that wait time is surfaced. Depth is exercised in the 3-waiter test below.
}

// queueDepth reflects the number of OTHER waiters at acquisition.
func TestBridge_QueueDepthReflectsWaiters(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	plugin.gate = make(chan struct{}, 8)
	ctx := context.Background()
	mk := func() map[string]interface{} { return map[string]interface{}{"channel": "fileA"} }

	// Holder occupies the slot.
	holderDone := make(chan struct{})
	go func() { _, _ = b.Send(ctx, "set_fills", []string{"1:1"}, mk()); close(holderDone) }()
	waitHits(t, plugin, 1)

	// Three more queue up behind the holder.
	var mu sync.Mutex
	depths := []int{}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := b.Send(ctx, "set_strokes", []string{"1:1"}, mk())
			mu.Lock()
			depths = append(depths, resp.QueueDepth)
			mu.Unlock()
		}()
	}
	waitWaiters(t, b, "fileA", 3) // all three queued

	// Release the holder, then the three in turn.
	for i := 0; i < 4; i++ {
		plugin.gate <- struct{}{}
	}
	<-holderDone
	wg.Wait()

	// The first of the three to acquire saw 2 others still waiting (depth 2); the
	// max observed depth must be >= 1 (proves depth is surfaced and non-trivial).
	maxDepth := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}
	if maxDepth < 1 {
		t.Errorf("expected queueDepth >= 1 among contended waiters, got depths=%v", depths)
	}
}

// ── CRITICAL #2: omitted-channel reads must canonicalize to the same entry ───

// TestBridge_OmittedChannelSharesCacheWithExplicit proves that when there is
// exactly ONE live connection:
//  1. A read with the explicit channel id populates the cache.
//  2. A subsequent read with omitted channel "" hits the SAME cache entry
//     (canonicalization): plugin hits stay at 1, not 2.
//  3. After a write with the EXPLICIT channel id, the next omitted-channel read
//     goes LIVE (invalidation reached the canonical entry).
//
// RED: on pre-fix code the omitted-channel read (step 2) is keyed under "" and
// misses the "fileA"-keyed entry → a second live call (hits==2). After the
// write, "fileA" is invalidated but "" is not, so the next omitted read is
// still served from the "" stale entry (hits stays 3 instead of 4).
func TestBridge_OmittedChannelSharesCacheWithExplicit(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()

	// Step 1: read with explicit channel — one live call.
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})
	if got := plugin.hits.Load(); got != 1 {
		t.Fatalf("after explicit read: expected 1 plugin hit, got %d", got)
	}

	// Step 2: read with omitted channel "" — must hit the same cache entry.
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{})
	if got := plugin.hits.Load(); got != 1 {
		t.Errorf("omitted-channel read must hit the same cache entry as explicit-id read: expected 1 plugin hit, got %d (CRITICAL #2 canonicalization failure)", got)
	}

	// Step 3: write with explicit id invalidates the channel.
	mustSend(t, b, ctx, "set_fills", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})

	// Step 4: omitted-channel read after write must go LIVE (not serve stale).
	// Total live plugin hits: step1(read live)=1 + step2(cache hit, no plugin)=1
	// + step3(write live)=2 + step4(read live after invalidation)=3.
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{})
	if got := plugin.hits.Load(); got != 3 {
		t.Errorf("post-write omitted-channel read must go live: expected 3 plugin hits, got %d (CRITICAL #2 stale-serve after invalidation)", got)
	}
}

// TestBridge_ExplicitChannelWriteInvalidatesOmittedRead is a complementary
// scenario that starts with an omitted-channel read (populates "" key on buggy
// code, canonical key on fixed code) and then confirms a write using the
// explicit id invalidates it.
//
// RED on buggy code: the "" entry survives the "fileA" write; the post-write
// omitted read is a stale hit → hits stays at 2 instead of 3.
func TestBridge_ExplicitChannelWriteInvalidatesOmittedRead(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()

	// Omitted-channel read → live (hit #1).
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{})

	// Explicit write → invalidates (hit #2).
	mustSend(t, b, ctx, "set_fills", []string{"1:1"}, map[string]interface{}{"channel": "fileA"})

	// Omitted-channel read after write → must be live (hit #3).
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, map[string]interface{}{})
	if got := plugin.hits.Load(); got != 3 {
		t.Errorf("omitted-channel read after explicit write must go live: expected 3 plugin hits, got %d (CRITICAL #2 stale-serve)", got)
	}
}

// ── MEDIUM: get_screenshot / get_selection / get_viewport must never be cached ─

// TestBridge_GetScreenshotNeverCached asserts that consecutive identical
// get_screenshot calls always reach the plugin (live state), never a cache hit.
//
// RED on pre-fix code: the second call returns a cache hit (hits stays 1).
func TestBridge_GetScreenshotNeverCached(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()
	ch := func() map[string]interface{} { return map[string]interface{}{"channel": "fileA"} }

	mustSend(t, b, ctx, "get_screenshot", nil, ch())
	mustSend(t, b, ctx, "get_screenshot", nil, ch())
	if got := plugin.hits.Load(); got != 2 {
		t.Errorf("get_screenshot must always go live: expected 2 plugin hits, got %d", got)
	}
}

// TestBridge_GetSelectionNeverCached — analogous to screenshot.
func TestBridge_GetSelectionNeverCached(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()
	ch := func() map[string]interface{} { return map[string]interface{}{"channel": "fileA"} }

	mustSend(t, b, ctx, "get_selection", nil, ch())
	mustSend(t, b, ctx, "get_selection", nil, ch())
	if got := plugin.hits.Load(); got != 2 {
		t.Errorf("get_selection must always go live: expected 2 plugin hits, got %d", got)
	}
}

// TestBridge_GetViewportNeverCached — analogous to screenshot.
func TestBridge_GetViewportNeverCached(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()
	ch := func() map[string]interface{} { return map[string]interface{}{"channel": "fileA"} }

	mustSend(t, b, ctx, "get_viewport", nil, ch())
	mustSend(t, b, ctx, "get_viewport", nil, ch())
	if got := plugin.hits.Load(); got != 2 {
		t.Errorf("get_viewport must always go live: expected 2 plugin hits, got %d", got)
	}
}

// TestBridge_GetScreenshotNeverPut verifies at the unit level (readCacheKey)
// that get_screenshot is not cacheable — readCacheKey returns ok=false.
func TestBridge_GetScreenshotNeverPut(t *testing.T) {
	if _, ok := readCacheKey("chA", "get_screenshot", nil, nil); ok {
		t.Error("get_screenshot must not be cacheable (readCacheKey ok=true is wrong)")
	}
	if _, ok := readCacheKey("chA", "get_selection", nil, nil); ok {
		t.Error("get_selection must not be cacheable")
	}
	if _, ok := readCacheKey("chA", "get_viewport", nil, nil); ok {
		t.Error("get_viewport must not be cacheable")
	}
	// Regular gets must still be cacheable.
	if _, ok := readCacheKey("chA", "get_node", nil, nil); !ok {
		t.Error("get_node must still be cacheable")
	}
}

// ── helpers ────────────────────────────────────────────────────────────────────

func mustSend(t *testing.T, b *Bridge, ctx context.Context, reqType string, nodeIDs []string, params map[string]interface{}) BridgeResponse {
	t.Helper()
	resp, err := b.Send(ctx, reqType, nodeIDs, params)
	if err != nil {
		t.Fatalf("Send(%s): %v", reqType, err)
	}
	return resp
}

func cloneParams(p map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

func waitHits(t *testing.T, p *countingPlugin, want int64) {
	t.Helper()
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if p.hits.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected >= %d plugin hits, got %d", want, p.hits.Load())
}

func waitWaiters(t *testing.T, b *Bridge, channel string, want int64) {
	t.Helper()
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.RLock()
		e, ok := b.conns[channel]
		b.mu.RUnlock()
		if ok && e.waiters.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected >= %d waiters on channel %s", want, channel)
}

// TestReadCacheKey_ExcludesPresenceParams locks the cache-first invariant against
// the presence params: two agents in two sessions reading the SAME node must
// produce the SAME read key so singleflight/readcache still coalesce the duplicate
// live plugin call. origin/status/sessionId/task are presence-only — they never
// change the read RESULT — so they must be stripped from the key. A genuine param
// (depth) must still split the key.
func TestReadCacheKey_ExcludesPresenceParams(t *testing.T) {
	keyA, okA := readCacheKey("chA", "get_node", []string{"1:1"}, map[string]interface{}{
		"origin": "grace", "sessionId": "sessA", "status": "thinking", "task": "build sidebar",
	})
	keyB, okB := readCacheKey("chA", "get_node", []string{"1:1"}, map[string]interface{}{
		"origin": "theo", "sessionId": "sessB", "status": "reviewing", "task": "fix table",
	})
	if !okA || !okB {
		t.Fatalf("get_node must be cacheable (okA=%v okB=%v)", okA, okB)
	}
	if keyA != keyB {
		t.Errorf("presence params must not split the read key:\n  A=%q\n  B=%q", keyA, keyB)
	}

	keyDepth, _ := readCacheKey("chA", "get_node", []string{"1:1"}, map[string]interface{}{
		"origin": "grace", "sessionId": "sessA", "depth": float64(2),
	})
	if keyDepth == keyA {
		t.Error("a real functional param (depth) must still change the read key")
	}
}

// set_presence records presence on the plugin but performs NO Figma mutation, so it
// must NOT invalidate the channel read-cache. Otherwise the orchestrator calling it
// per-agent at dispatch would repeatedly flush the cross-agent read cache that the
// cache-first discipline depends on.
func TestBridge_SetPresenceDoesNotInvalidateCache(t *testing.T) {
	b, plugin := setupBridgeChannel(t, "fileA")
	ctx := context.Background()
	ch := map[string]interface{}{"channel": "fileA"}

	mustSend(t, b, ctx, "get_node", []string{"1:1"}, cloneParams(ch)) // hit #1 (live), caches
	// No `origin` here on purpose: it would trigger async presence_queue broadcasts the
	// counting plugin also tallies, confounding the cache-invalidation hit count.
	mustSend(t, b, ctx, "set_presence", nil, map[string]interface{}{
		"channel": "fileA", "task": "x",
	}) // hit #2 (live) — must NOT invalidate
	mustSend(t, b, ctx, "get_node", []string{"1:1"}, cloneParams(ch)) // must be a CACHE HIT

	if got := plugin.hits.Load(); got != 2 {
		t.Errorf("set_presence must not invalidate cache: expected 2 plugin hits (read + set_presence; re-read cached), got %d", got)
	}
}
