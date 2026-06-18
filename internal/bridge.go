package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var bridgeLogger = log.New(os.Stderr, "[bridge] ", 0)

// envInt reads an integer environment variable, returning def when the variable is
// unset, unparseable, or non-positive. Shared across bridge.go, gate.go, and
// readcache.go so every int-env knob has one canonical parse path.
func envInt(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// parseRequestTimeout reads FIGMA_MCP_TIMEOUT from the environment (integer
// seconds) and returns it as a time.Duration. Falls back to 120s if unset or invalid.
// This is the base ceiling for writes and cheap reads (not in the generous-read set).
func parseRequestTimeout() time.Duration {
	return time.Duration(envInt("FIGMA_MCP_TIMEOUT", 120)) * time.Second
}

// parseReadTimeout reads FIGMA_MCP_READ_TIMEOUT from the environment (integer
// seconds) and returns it as a time.Duration. Falls back to 600s if unset or
// invalid. This generous default prevents legitimate heavy reads (full page tree,
// large component catalogs, batch-with-reads) from timing out on slow files while
// the LLM is not burdened with picking a timeout value.
func parseReadTimeout() time.Duration {
	return time.Duration(envInt("FIGMA_MCP_READ_TIMEOUT", 600)) * time.Second
}

// isHeavyReadOrBatch reports whether a request type belongs to the set of
// operations that legitimately run long and should receive the generous
// FIGMA_MCP_READ_TIMEOUT ceiling rather than the base FIGMA_MCP_TIMEOUT.
//
// The set is intentionally narrower than isReadOnly: it includes only reads whose
// payloads can be large / slow, plus "batch" (which often contains reads).
// Cheap reads (get_metadata, get_styles, get_pages, …) are not included — they
// use the base ceiling and should be fast.
func isHeavyReadOrBatch(requestType string) bool {
	switch requestType {
	case "get_node", "get_nodes_info", "get_design_context", "get_document",
		"scan_nodes_by_types", "scan_text_nodes", "search_nodes",
		"get_local_components", "batch":
		return true
	}
	return false
}

// resolveRequestTimeout returns the per-request inactivity ceiling for the given
// op type, determined entirely by the server:
//
//   - Heavy reads and batch → FIGMA_MCP_READ_TIMEOUT (default 600s). These ops can
//     legitimately take a long time on large files; the inactivity timer + progress
//     resets mean a PROGRESSING read never fires it. A fired timer = narrow the
//     read, not raise a timeout.
//   - All other ops → FIGMA_MCP_TIMEOUT (default 120s).
//
// There is no LLM-supplied override. Timeouts are server-managed.
func resolveRequestTimeout(requestType string) time.Duration {
	if isHeavyReadOrBatch(requestType) {
		return parseReadTimeout()
	}
	return parseRequestTimeout()
}

// pendingEntry holds the response channel and inactivity timer for an in-flight request.
type pendingEntry struct {
	ch    chan BridgeResponse
	timer *time.Timer
	// connEntry is the connection this request was dispatched on; used for
	// connection-drop drain so readLoop can resolve only its own pending entries.
	conn *connEntry
	// connDropped is set (atomically) when the drain closes ch on a connection drop,
	// so sendOnce can return a clear "connection closed" error rather than "timed out".
	connDropped atomic.Bool
	// resetTimeout is the inactivity window a progress_update tick resets the timer to.
	// Set to the full server-side ceiling for this op type so a progressing read always
	// keeps its complete window, not a hardcoded 120s.
	resetTimeout time.Duration
	once         sync.Once // guards channel close/send — prevents panic on concurrent timeout + response
	// onResolve releases the per-channel serial slot (and, for imports, clears the
	// connEntry.importInFlight marker). resolveOnce makes it fire exactly once, at
	// WHICHEVER resolution site wins (plugin response, timeout timer, write-error, or
	// Close) — but NOT on a client-context-cancel, which deliberately leaves the slot
	// held (and the import marker set) until the request truly resolves, so the next
	// request never overlaps the still-busy single-threaded plugin.
	onResolve   func()
	resolveOnce sync.Once
}

func (pe *pendingEntry) resolve() {
	if pe.onResolve != nil {
		pe.resolveOnce.Do(pe.onResolve)
	}
}

// connEntry is one connected plugin (one open Figma file), keyed by channel id.
type connEntry struct {
	conn *websocket.Conn
	wmu  sync.Mutex // serialises writes to THIS connection (coder/websocket forbids concurrent writes)
	// sem is the per-plugin serial slot (buffered, cap 1). The single-threaded
	// plugin processes one request at a time; holding sem for a request's whole
	// lifetime stops a burst of concurrent in-flight requests from flooding the
	// plugin event loop and racing each other's timeouts.
	sem chan struct{}
	// waiters counts requests currently queued on sem (waiting to acquire the serial
	// slot), for C1 queue visibility. Incremented before the sem-acquire select and
	// decremented after, so a request reads "how many OTHERS are waiting" by sampling
	// it at acquisition time (minus itself). Atomic — read/written without b.mu.
	waiters     atomic.Int64
	fileName    string
	fileKey     string
	pageName    string
	connectedAt time.Time

	// waitingOrigins tracks WHICH presence origins are currently waiting on sem,
	// keyed by a per-wait unique token (value = the roster origin string). Used to
	// build the presence_queue frame pushed to the plugin. Guarded by waitingMu —
	// a separate lock from b.mu so registering/clearing a waiter never contends
	// with the bridge's main map lock. Only requests that carry a known roster
	// `origin` register here; anonymous reads/writes do not.
	waitingMu      sync.Mutex
	waitingOrigins map[string]string

	// importInFlight marks that an import_*_by_key is currently occupying the
	// single-threaded plugin. It shares the slot's lifetime — both are held from
	// dispatch until the import's pendingEntry actually RESOLVES (plugin response or
	// the request-timeout timer), surviving a client-cancel of a still-hung import.
	// The marker is what the slot alone cannot do: it lets a RETRIED import be rejected
	// EARLY with ErrImportInFlight (before it even queues on sem) instead of being
	// dispatched and re-jamming the thread (the ~118× retry-loop amplifier). Cleared
	// via onResolve at the same instant the slot is released.
	inFlightMu     sync.Mutex
	importInFlight bool

	// lastProgressAt is the unix-nanos of the current slot-holder's last sign of
	// life: stamped when it acquires the slot, and refreshed on every progress tick
	// (readLoop). It drives the stalled-head guard (isStalled) — a generalization of
	// the import-jam guard to ANY hung op. Deliberately NOT a "stalled" boolean:
	// stall is computed live from this timestamp + actual slot occupancy, so there is
	// no flag that could be left set and brick the channel — it self-heals the instant
	// the head frees the slot. Atomic: written at acquire/progress, read at early-reject.
	lastProgressAt atomic.Int64
}

// addWaitingOrigin records that the wait identified by token is waiting on the
// serial slot under the given roster origin.
func (e *connEntry) addWaitingOrigin(token, origin string) {
	e.waitingMu.Lock()
	if e.waitingOrigins == nil {
		e.waitingOrigins = make(map[string]string)
	}
	e.waitingOrigins[token] = origin
	e.waitingMu.Unlock()
}

// removeWaitingOrigin clears the wait identified by token (acquired or cancelled).
func (e *connEntry) removeWaitingOrigin(token string) {
	e.waitingMu.Lock()
	delete(e.waitingOrigins, token)
	e.waitingMu.Unlock()
}

// snapshotWaitingOrigins returns the DISTINCT roster origins currently waiting on
// the slot, sorted for deterministic frames. Always non-nil ([]string{} when
// nobody waits) so the plugin clears its queued list on the empty frame.
func (e *connEntry) snapshotWaitingOrigins() []string {
	e.waitingMu.Lock()
	seen := make(map[string]struct{}, len(e.waitingOrigins))
	for _, o := range e.waitingOrigins {
		seen[o] = struct{}{}
	}
	e.waitingMu.Unlock()
	out := make([]string, 0, len(seen))
	for o := range seen {
		out = append(out, o)
	}
	sort.Strings(out)
	return out
}

func (e *connEntry) setImportInFlight(v bool) {
	e.inFlightMu.Lock()
	e.importInFlight = v
	e.inFlightMu.Unlock()
}

func (e *connEntry) isImportInFlight() bool {
	e.inFlightMu.Lock()
	defer e.inFlightMu.Unlock()
	return e.importInFlight
}

// markProgress stamps the current slot-holder's liveness — called when the holder
// acquires the slot and on every progress tick.
func (e *connEntry) markProgress() {
	e.lastProgressAt.Store(time.Now().UnixNano())
}

// isStalled reports whether the slot is currently held by an op that has shown no
// progress for longer than threshold. Live computation, no persistent state:
//   - slot free (len(sem)==0) → never stalled (self-heals when the head resolves);
//   - holder not yet stamped (lastProgressAt==0) → not stalled (don't reject before
//     the holder has even started);
//   - otherwise stalled iff now - lastProgressAt > threshold.
//
// A slightly-stale len(sem) snapshot only ever causes a harmless soft-retry.
func (e *connEntry) isStalled(threshold time.Duration) bool {
	if len(e.sem) == 0 {
		return false
	}
	lp := e.lastProgressAt.Load()
	if lp == 0 {
		return false
	}
	return time.Since(time.Unix(0, lp)) > threshold
}

// isImportRequest reports whether a request type occupies the plugin thread with a
// library import that can hang it.
func isImportRequest(requestType string) bool {
	return requestType == "import_component_by_key" || requestType == "import_variable_by_key"
}

// ErrImportInFlight is returned (without dispatching) when a second import arrives
// while a prior import is still occupying the plugin thread. It tells the LLM to
// stop retrying and do non-import work until the thread clears — retrying an import
// only re-poisons a recovering thread.
var ErrImportInFlight = errors.New("plugin thread busy with a prior import — do not retry; do non-import work (clone_node, set_text, set_fills, save_screenshots) until it clears")

// ErrChannelStalled is returned (without dispatching) when a NEW request arrives on
// a channel whose current op has held the serial slot past the stall threshold with
// no progress — almost certainly a hung Figma call. It generalizes ErrImportInFlight
// to any op: rather than silently queue behind the wedge (and eat the head's full
// inactivity ceiling), the new request fast-fails with guidance. The stuck op itself
// is unchanged — it still force-resolves at its ceiling, freeing the channel.
var ErrChannelStalled = errors.New("an op has been stuck on this channel with no progress (likely a hung Figma call) — it will be force-resolved at its ceiling; do other work or retry shortly")

// flight is one in-flight read whose result is shared by any identical
// concurrent callers (singleflight). done is closed when resp/err are set.
type flight struct {
	done chan struct{}
	resp BridgeResponse
	err  error
}

// Bridge multiplexes WebSocket connections from one or more Figma plugins,
// keyed by channel id (one channel per open file), and matches responses to
// pending requests via globally-unique request IDs.
//
// Connections on DIFFERENT channels coexist — a new connection NEVER closes a
// different channel's connection. Only a reconnect on the SAME channel replaces
// the prior socket. This is what eliminates the connect/disconnect flap that the
// old single-slot "replace on any connect" design produced with >1 plugin.
type Bridge struct {
	mu      sync.RWMutex
	conns   map[string]*connEntry
	pending map[string]*pendingEntry
	counter atomic.Int64
	chanSeq atomic.Int64 // sequence for auto-assigned channel ids

	flightMu sync.Mutex         // guards flights
	flights  map[string]*flight // in-flight read dedup, keyed by flightKey

	// readCache holds short-TTL, write-invalidated, in-process read responses so N
	// near-simultaneous subagent reads of the same node collapse onto ONE plugin
	// round-trip (extends singleflight's collapse window). See readcache.go.
	readCache *readCache

	// Heartbeat config (issue #32). A dead/partitioned transport that never sends a
	// TCP FIN leaves the readLoop blocked and every in-flight request waiting the
	// full per-request inactivity ceiling. A periodic WebSocket ping detects that
	// case in heartbeatTimeout instead, closing the conn so the existing drain
	// fails the requests fast and resolve() rejects new ones. Mutable so tests can
	// dial these down; production never reassigns them.
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	// stallThreshold drives the stalled-head guard: a NEW request to a channel whose
	// current op has held the slot this long with no progress is fast-failed
	// (ErrChannelStalled) rather than queued behind a likely-hung head. Generalizes
	// the import-jam guard to any op. Mutable so tests dial it down.
	stallThreshold time.Duration
}

// Heartbeat defaults: ping every 15s, fail the transport if no pong within 10s.
// Generous enough that a healthy plugin (its UI continuously reads, so the
// browser auto-pongs) is never falsely dropped; tight enough that a partition is
// caught in seconds, not the ~120s request ceiling.
const (
	defaultHeartbeatInterval = 15 * time.Second
	defaultHeartbeatTimeout  = 10 * time.Second
	// defaultStallThreshold: how long the slot-holder may show no progress before a
	// peer's NEW request is early-rejected. Well under the 120s/600s ceiling (so it
	// actually helps), well above any legitimate no-progress gap — writes finish
	// sub-second, reads/exports tick via makeProgress, imports self-cap at 15s — so a
	// progressing op is never falsely flagged.
	defaultStallThreshold = 45 * time.Second
)

// NewBridge creates a ready-to-use Bridge.
func NewBridge() *Bridge {
	return &Bridge{
		conns:             make(map[string]*connEntry),
		pending:           make(map[string]*pendingEntry),
		flights:           make(map[string]*flight),
		readCache:         newReadCache(),
		heartbeatInterval: defaultHeartbeatInterval,
		heartbeatTimeout:  defaultHeartbeatTimeout,
		stallThreshold:    parseStallThreshold(),
	}
}

// parseStallThreshold reads FIGMA_MCP_STALL_THRESHOLD (integer seconds), falling
// back to defaultStallThreshold when unset/invalid/non-positive.
func parseStallThreshold() time.Duration {
	return time.Duration(envInt("FIGMA_MCP_STALL_THRESHOLD", int(defaultStallThreshold/time.Second))) * time.Second
}

// HandleUpgrade upgrades an HTTP request to a WebSocket connection on the channel
// named by the `channel` query param (auto-assigned when absent). A reconnect on
// the same channel replaces that channel's socket; other channels are untouched.
func (b *Bridge) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // skip Origin check — plugin connects via Figma's sandbox
		// permessage-deflate: figma read payloads (node trees) compress ~6–14× on real
		// captures at ~5–10ms/MB CPU — a large bandwidth win for a trivial CPU cost. The
		// browser plugin client negotiates this automatically; falls back to no-compression
		// if a client doesn't offer the extension. ContextTakeover keeps a small per-conn
		// window (few WS conns — one per open file — so the RAM cost is negligible).
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		bridgeLogger.Printf("upgrade error: %v", err)
		return
	}

	// Raise the read limit to 100 MB — Figma documents can be large.
	conn.SetReadLimit(100 * 1024 * 1024)

	if channel == "" {
		channel = fmt.Sprintf("auto-%d", b.chanSeq.Add(1))
	}

	entry := &connEntry{conn: conn, connectedAt: time.Now(), sem: make(chan struct{}, 1), waitingOrigins: make(map[string]string)}

	b.mu.Lock()
	prev, replaced := b.conns[channel]
	b.conns[channel] = entry
	b.mu.Unlock()

	if replaced {
		// Same-channel reconnect: close ONLY the prior socket for this channel.
		if err := prev.conn.Close(websocket.StatusNormalClosure, "replaced by new connection on same channel"); err != nil {
			bridgeLogger.Printf("close previous connection (channel=%s) error: %v", channel, err)
		}
		bridgeLogger.Printf("plugin connected (replaced same channel %q) from %s", channel, r.RemoteAddr)
	} else {
		bridgeLogger.Printf("plugin connected (channel %q) from %s", channel, r.RemoteAddr)
	}

	// One context per connection: readLoop's exit cancels it (stopping the
	// heartbeat), and a heartbeat failure cancels it (unblocking readLoop's Read).
	connCtx, cancelConn := context.WithCancel(context.Background())
	go b.heartbeat(connCtx, cancelConn, channel, conn)
	go b.readLoop(connCtx, cancelConn, channel, entry, conn)
}

// heartbeat periodically pings the plugin so a dead or partitioned transport
// (one that never sends a TCP FIN) is detected in heartbeatTimeout rather than at
// the full per-request inactivity ceiling (issue #32). On a failed ping it closes
// the conn, which makes readLoop's Read return an error → the existing drain fails
// every in-flight request fast and resolve() rejects new ones.
//
// SCOPE (honest): this catches a dead/partitioned/half-open TRANSPORT. It does
// NOT catch a JS-frozen-but-connected plugin — a browser auto-pongs at the
// protocol layer on a thread independent of the frozen JS, so the ping still
// succeeds. Detecting a frozen-but-connected plugin needs an application-level
// "alive" ping from the plugin (a larger change, tracked separately). And NO
// disconnect is recoverable: in-flight work is lost and writes are not
// replay-safe — this only makes the loss FAST and clean, not recoverable.
func (b *Bridge) heartbeat(ctx context.Context, cancelConn context.CancelFunc, channel string, conn *websocket.Conn) {
	ticker := time.NewTicker(b.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, b.heartbeatTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return // conn already closing for another reason
				}
				bridgeLogger.Printf("heartbeat ping failed (channel %q): %v — closing dead transport", channel, err)
				// Cancel the conn ctx first so readLoop's Read returns IMMEDIATELY
				// (→ drain) rather than waiting for a close handshake the dead peer
				// will never answer. CloseNow then frees the socket without the
				// handshake's multi-second timeout.
				cancelConn()
				_ = conn.CloseNow()
				return
			}
		}
	}
}

// readLoop reads messages from one plugin connection and resolves pending requests.
func (b *Bridge) readLoop(ctx context.Context, cancelConn context.CancelFunc, channel string, entry *connEntry, conn *websocket.Conn) {
	// Stop the heartbeat goroutine whenever this loop exits (normal close, read
	// error, or heartbeat-triggered close).
	defer cancelConn()
	// A panic while handling one response must not tear down the whole loop
	// silently; recover, log, and let the cleanup defer below run normally.
	defer func() {
		if r := recover(); r != nil {
			bridgeLogger.Printf("readLoop panic (channel %q): %v", channel, r)
		}
	}()
	defer func() {
		b.mu.Lock()
		// Only delete if this exact entry is still registered for the channel
		// (a same-channel reconnect may have already replaced it).
		if cur, ok := b.conns[channel]; ok && cur == entry {
			delete(b.conns, channel)
			// Channel is gone for good (not a same-channel reconnect, which keeps a
			// different entry) — drop its read-cache generation so the gens map does
			// not grow without bound across auto-N reconnects. DeleteChannel is
			// nil-safe (enabled() guards a nil receiver), matching the other call sites.
			b.readCache.DeleteChannel(channel)
		}
		// Drain all pending requests that were dispatched on this connection so they
		// resolve IMMEDIATELY with a "connection closed" error, rather than waiting
		// for the (now generous) inactivity ceiling. Critical now that read/batch
		// ceilings can be several hundred seconds — a dropped connection must not
		// silently stall callers for the full ceiling duration.
		// We match by the connEntry pointer (pe.conn == entry) rather than the channel
		// name string, because the channel name may be "" when the caller omitted it.
		var toDrain []*pendingEntry
		var toDeleteIDs []string
		for id, pe := range b.pending {
			if pe.conn == entry {
				toDrain = append(toDrain, pe)
				toDeleteIDs = append(toDeleteIDs, id)
			}
		}
		for _, id := range toDeleteIDs {
			delete(b.pending, id)
		}
		b.mu.Unlock()

		// Resolve outside the lock — once.Do prevents double-send on a concurrent
		// timer fire. pe.resolve() clears import markers when relevant.
		// Mark connDropped BEFORE closing the channel so sendOnce sees the flag when
		// it receives from ch (close is the signal; flag distinguishes reason).
		for _, pe := range toDrain {
			pe.timer.Stop()
			pe.connDropped.Store(true)
			pe.once.Do(func() { close(pe.ch) })
			pe.resolve()
		}

		bridgeLogger.Printf("plugin disconnected (channel %q)", channel)
	}()

	for {
		var resp BridgeResponse
		if err := wsjson.Read(ctx, conn, &resp); err != nil {
			if !errors.Is(err, context.Canceled) {
				bridgeLogger.Printf("read error (channel %q): %v", channel, err)
			}
			return
		}

		// Registration message — attach file metadata to this channel.
		if resp.Type == registerMessageType {
			b.applyRegister(channel, entry, resp.Data)
			continue
		}

		// Progress updates — extend timeout, do not resolve.
		// Guard on the message TYPE first (belt) so that even a progress:0 tick
		// (e.g. the first tick before any percentage is known) never falls through
		// to the resolution block.  The Progress > 0 check is kept as a second
		// guard (suspenders) for any message that lacks an explicit type field but
		// carries a non-zero progress value.
		if (resp.Type == progressUpdateType || resp.Progress > 0) && resp.RequestID != "" {
			// Read resetTimeout under the SAME RLock that fetches pe — it is written
			// in sendOnce before the entry is registered in b.pending under b.mu, so
			// reading it here under b.mu gives a clean happens-before (no torn read).
			b.mu.RLock()
			pe, ok := b.pending[resp.RequestID]
			var resetTo time.Duration
			if ok {
				resetTo = pe.resetTimeout
			}
			b.mu.RUnlock()
			if ok {
				// Reset to the request's server-side inactivity ceiling — the full
				// ceiling for the op type so a progressing read keeps its complete
				// window on every tick. Floor guard defends against a zero/negative
				// value from a hypothetical misconfigured env.
				if resetTo <= 0 {
					resetTo = 120 * time.Second
				}
				pe.timer.Stop()
				pe.timer.Reset(resetTo)
				// Refresh the holder's liveness so the stalled-head guard treats a
				// progressing op as alive (pe.conn is the connEntry it was dispatched on).
				if pe.conn != nil {
					pe.conn.markProgress()
				}
				bridgeLogger.Printf("progress %s: %d%% %s", resp.RequestID, resp.Progress, resp.Message)
			} else {
				bridgeLogger.Printf("progress %s: %d%% %s (no pending entry)", resp.RequestID, resp.Progress, resp.Message)
			}
			continue
		}

		if resp.RequestID == "" {
			bridgeLogger.Printf("received message with empty requestID (channel %q) — ignored", channel)
			continue
		}

		b.mu.Lock()
		pe, ok := b.pending[resp.RequestID]
		if ok {
			delete(b.pending, resp.RequestID)
		}
		b.mu.Unlock()

		if ok {
			if resp.Error != "" {
				bridgeLogger.Printf("← %s error: %s", resp.RequestID, resp.Error)
			} else {
				bridgeLogger.Printf("← %s ok", resp.RequestID)
			}
			pe.timer.Stop()
			pe.once.Do(func() { pe.ch <- resp })
			pe.resolve() // import resolved → clear the import marker
		} else {
			bridgeLogger.Printf("← %s received but no pending entry (timed out?)", resp.RequestID)
		}
	}
}

// applyRegister stores file metadata sent by the plugin for its channel.
func (b *Bridge) applyRegister(channel string, entry *connEntry, data interface{}) {
	m, ok := data.(map[string]interface{})
	if !ok {
		return
	}
	b.mu.Lock()
	cur, ok := b.conns[channel]
	if !ok || cur != entry {
		b.mu.Unlock()
		return // stale registration from a replaced connection
	}
	if v, ok := m["fileName"].(string); ok {
		cur.fileName = v
	}
	if v, ok := m["fileKey"].(string); ok {
		cur.fileKey = v
	}
	if v, ok := m["pageName"].(string); ok {
		cur.pageName = v
	}
	bridgeLogger.Printf("channel %q registered file=%q key=%q", channel, cur.fileName, cur.fileKey)
	b.mu.Unlock()

	// A (re)registration can carry a page/file change (the plugin re-registers on page
	// switch, updating pageName) — invalidate this channel's cached reads so we never
	// serve a read from the prior page. Harmless on the initial connect (empty cache).
	// This is the one plugin-side signal already reaching the bridge readLoop; there is
	// no separate selection/page-change message, so external canvas edits stay bounded
	// by the short TTL (see readcache.go). Done AFTER releasing b.mu — readCache has its
	// own lock and never acquires b.mu, so there is no lock-ordering cycle.
	b.readCache.InvalidateChannel(channel)
}

// resolve picks the target connection for a channel. Caller must hold b.mu (R or W).
//   - explicit channel → that connection, or an error if absent
//   - empty channel + exactly one connection → that connection
//   - empty channel + zero → "plugin not connected"
//   - empty channel + many → an error listing the channels so the caller can pick
func (b *Bridge) resolve(channel string) (*connEntry, error) {
	if channel != "" {
		e, ok := b.conns[channel]
		if !ok {
			return nil, fmt.Errorf("channel %q not connected", channel)
		}
		return e, nil
	}
	switch len(b.conns) {
	case 0:
		return nil, errors.New("plugin not connected")
	case 1:
		for _, e := range b.conns {
			return e, nil
		}
	}
	var names []string
	for ch, e := range b.conns {
		if e.fileName != "" {
			names = append(names, fmt.Sprintf("%s (%s)", ch, e.fileName))
		} else {
			names = append(names, ch)
		}
	}
	sort.Strings(names)
	return nil, fmt.Errorf("multiple files connected — pass the channel param to choose one: %s", strings.Join(names, "; "))
}

// canonicalChannel resolves an empty channel string to the sole live
// connection's id (mirroring the resolve() semantics used inside sendOnce), so
// that cache keys, generation snapshots, and invalidations are all computed
// against the same canonical id regardless of whether the caller passed the
// explicit channel name or omitted it.
//
// Returns (id, true) when the channel can be unambiguously identified:
//   - explicit non-empty channel → returned as-is (no lock needed)
//   - empty + exactly one connection → that connection's id
//
// Returns ("", false) when:
//   - empty + zero connections (plugin not connected — will error in sendOnce)
//   - empty + >1 connections (ambiguous — sendOnce will return a descriptive error)
//
// When ok==false the caller must treat the request as NON-CACHEABLE: do not
// Get/Put/InvalidateChannel under a guessed key. The raw channel string still
// flows into sendOnce unchanged so resolve()'s error messages are unaffected.
//
// Caller must NOT hold b.mu — this acquires RLock internally.
func (b *Bridge) canonicalChannel(channel string) (string, bool) {
	if channel != "" {
		return channel, true
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.conns) == 1 {
		for id := range b.conns {
			return id, true
		}
	}
	return "", false
}

// Send routes a request to a plugin connection and waits for the response.
// The target channel is read from params["channel"] (stripped before sending).
//
// Read-only requests are de-duplicated (singleflight): identical concurrent
// reads share one in-flight plugin round-trip. Every request — read or write —
// is then funnelled through a per-channel serial slot in sendOnce, so the
// single-threaded plugin never receives a burst of concurrent in-flight work.
//
// bridge.Send is hint-free: failure hints are a node.Send-layer concern (see
// node.go), added once at the chokepoint every tool handler funnels through.
// Do not add hint decoration here, or follower-proxied calls double-decorate.
func (b *Bridge) Send(ctx context.Context, requestType string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	channel, _ := params["channel"].(string)
	if params != nil {
		delete(params, "channel") // never forward routing metadata to the plugin
	}

	// Resolve the canonical channel id for all cache operations.  An omitted
	// channel ("") that maps to the sole live connection must share the same
	// cache key, generation, and invalidation path as that connection's explicit
	// id.  When the channel is ambiguous (0 or >1 connections) we treat the
	// request as non-cacheable — sendOnce will resolve / error independently.
	cacheChannel, cacheOK := b.canonicalChannel(channel)

	// WRITE-INVALIDATION (C4): any non-read op (incl. "batch", which may contain
	// writes) invalidates ALL cached reads for this channel — coarse but airtight.
	// Invalidate UNCONDITIONALLY of write success: a partial/failed write may still
	// have mutated state, and a stale read crossing a mutation is a correctness bug.
	//
	// We invalidate TWICE — before AND after the plugin round-trip — because the
	// actual mutation is deferred to sendOnce (after the per-channel sem queue), so
	// the bump and the mutation are NOT atomic:
	//   - PRE bump: drops Puts from reads that started before this write (the read-
	//     started-before-write case).
	//   - POST bump: closes the window where a read snapshots the post-PRE gen, wins
	//     the sem race AHEAD of this write, reads pre-mutation state, and Puts under
	//     that gen. The POST bump advances the gen again so that entry misses on the
	//     next Get (and clears it outright). Without the POST bump, that stale entry
	//     would survive (its gen still matched) and cross the mutation = a bug.
	// set_presence reaches the plugin (to record presence) but performs NO Figma
	// mutation, so it must neither invalidate the read cache (it would needlessly flush
	// the cross-agent cache — the orchestrator calls it per agent) nor be cached (no
	// cacheable result). Send it straight through the serial slot.
	if requestType == "set_presence" {
		return b.sendOnce(ctx, channel, requestType, nodeIDs, params)
	}

	if !isReadOnly(requestType) {
		if cacheOK {
			b.readCache.InvalidateChannel(cacheChannel)
		}
		resp, err := b.sendOnce(ctx, channel, requestType, nodeIDs, params)
		if cacheOK {
			b.readCache.InvalidateChannel(cacheChannel)
		}
		return resp, err
	}

	// READ-CACHE (C4): for a cacheable read, check the cache first. A hit returns
	// instantly WITHOUT touching the single-threaded plugin — the real multi-subagent
	// win on top of singleflight (which only collapses SIMULTANEOUS reads).
	var cacheKey string
	var cacheable bool
	if cacheOK {
		cacheKey, cacheable = readCacheKey(cacheChannel, requestType, nodeIDs, params)
	}
	// gen is captured from Get on a miss: Get returns the channel's current generation
	// at the miss point under its own lock — reusing it here avoids a second lock
	// acquisition for currentGen(). On a hit we return immediately; on a miss the
	// returned gen is exactly what we need to pass to Put.
	var gen uint64
	if cacheable {
		var resp BridgeResponse
		var hit bool
		resp, gen, hit = b.readCache.Get(cacheKey, cacheChannel)
		if hit {
			bridgeLogger.Printf("⚡ readcache hit %s", cacheKey)
			return resp, nil
		}
	}

	// Singleflight collapses simultaneous reads onto one plugin round-trip, then the
	// leader populates the cache so subsequent reads hit the cache.
	if key, ok := flightKey(channel, requestType, nodeIDs, params); ok {
		// gen was captured at the miss point (above) — BEFORE the live read begins —
		// so Put can detect (and drop) a result that a concurrent write invalidated
		// mid-flight. When cacheable is false, gen is 0 and Put is never called.
		resp, err := b.doSingleflight(ctx, key, func() (BridgeResponse, error) {
			return b.sendOnce(ctx, channel, requestType, nodeIDs, params)
		})
		if err == nil && cacheable {
			// Cache the RESULT, not the leader's queue metadata: a future cache HIT
			// never queued, so it must not inherit the leader's queueWaitMs/Depth (see
			// the BridgeResponse comment). Zero them on the copy we store.
			cached := resp
			cached.QueueWaitMs = 0
			cached.QueueDepth = 0
			b.readCache.Put(cacheKey, cacheChannel, gen, cached)
		}
		return resp, err
	}
	return b.sendOnce(ctx, channel, requestType, nodeIDs, params)
}

// sendOnce performs a single plugin round-trip: it acquires the per-channel
// serial slot, then issues the request and waits for the response. The timeout
// timer starts only AFTER the slot is acquired, so time spent queued behind an
// earlier request never counts against this request's own timeout.
func (b *Bridge) sendOnce(ctx context.Context, channel, requestType string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	b.mu.RLock()
	entry, resolveErr := b.resolve(channel)
	b.mu.RUnlock()
	if resolveErr != nil {
		return BridgeResponse{}, resolveErr
	}

	// Import-jam guard: if a prior import is still occupying the plugin thread,
	// reject a NEW import immediately (before sem-acquire / dispatch) so a retry
	// loop can't re-poison a recovering thread. Non-import calls fall through and
	// queue normally. Returns before the sem-acquire defer, so nothing to release.
	isImport := isImportRequest(requestType)
	if isImport && entry.isImportInFlight() {
		bridgeLogger.Printf("⨯ %s rejected: import already in flight (channel %q)", requestType, channel)
		return BridgeResponse{}, ErrImportInFlight
	}

	// Stalled-head guard (generalizes the import-jam guard to ANY hung op): if the
	// slot is held by an op that has shown no progress for stallThreshold, reject this
	// NEW request immediately rather than queue it behind a likely-hung head where it
	// would eat the head's full inactivity ceiling. Live check (slot occupancy +
	// last-progress), no persistent flag — self-heals the instant the head frees.
	if entry.isStalled(b.stallThreshold) {
		bridgeLogger.Printf("⨯ %s rejected: channel stalled — head op no progress for >%s (channel %q)", requestType, b.stallThreshold, channel)
		return BridgeResponse{}, ErrChannelStalled
	}

	// Acquire the per-plugin serial slot. We wait ONLY on the caller's ctx: the
	// current holder is guaranteed to release within its own (progress-extended)
	// timeout, so a healthy long heavy read never spuriously fails queued work.
	// INVARIANT: only the sem-acquire branch reaches the release defer below;
	// the ctx.Done() branch returns first, so we never block forever on release.
	//
	// C1 queue visibility: mark this request as waiting (entry.waiters) for the whole
	// time it sits on sem, so a peer can observe the queue depth. Decrement on EITHER
	// exit branch (acquired OR ctx-cancelled) — a cancelled waiter must not be counted.
	entry.waiters.Add(1)
	enqueued := time.Now()
	// Presence "queued" (E): if this request carries a known roster origin, record
	// it as waiting on the slot under a per-wait unique token, and push the updated
	// queue to the plugin. Cleared on BOTH exit branches (acquired and cancelled),
	// each followed by another broadcast so the plugin's queued list stays accurate.
	waitToken := ""
	if origin, ok := params["origin"].(string); ok && origin != "" {
		waitToken = b.nextID()
		entry.addWaitingOrigin(waitToken, origin)
		b.broadcastQueue(channel, entry)
	}
	select {
	case entry.sem <- struct{}{}:
		if waitToken != "" {
			entry.removeWaitingOrigin(waitToken)
			b.broadcastQueue(channel, entry)
		}
	case <-ctx.Done():
		entry.waiters.Add(-1)
		if waitToken != "" {
			entry.removeWaitingOrigin(waitToken)
			b.broadcastQueue(channel, entry)
		}
		return BridgeResponse{}, ctx.Err()
	}
	// queueDepth = OTHER requests still waiting at the moment we acquired (exclude
	// self, which is still counted in waiters until the Add(-1) below). queueWaitMs =
	// time spent queued before acquisition. Both feed the C1 response metadata.
	queueDepth := entry.waiters.Add(-1) // returns the post-decrement count = others still waiting
	queueWaitMs := time.Since(enqueued).Milliseconds()
	// Stamp this holder's liveness at acquisition so the stalled-head guard measures
	// from when THIS op started occupying the slot (not from a prior holder), and a
	// fresh holder is never instantly flagged. Refreshed on each progress tick below.
	entry.markProgress()
	// The serial slot is intentionally NOT released on return. Release flows through
	// pe.onResolve (true resolution only), so a client-cancelled-but-still-running
	// request holds the slot until the plugin finishes. See pe.onResolve below.
	if queueWaitMs > 50 {
		bridgeLogger.Printf("⧗ %s waited %dms for plugin slot, queueDepth=%d (channel %q)", requestType, queueWaitMs, queueDepth, channel)
	}

	requestID := b.nextID()
	req := BridgeRequest{
		Type:      requestType,
		RequestID: requestID,
		NodeIDs:   nodeIDs,
		Params:    params,
	}

	ch := make(chan BridgeResponse, 1)
	pe := &pendingEntry{ch: ch, conn: entry}
	// Release the per-channel serial slot at TRUE resolution (plugin response, server
	// inactivity timer, write error, or connection-drop drain) — NOT when sendOnce
	// returns. resolveOnce fires this exactly once at whichever resolution site wins,
	// so the slot is freed exactly once (a double <-sem would steal the next holder's
	// token). Crucially it does NOT fire on a client ctx-cancel — that branch returns
	// without resolving — so a cancelled-but-still-running request keeps the slot until
	// the plugin actually finishes, and the next request cannot overlap the busy
	// single-threaded plugin. For imports, clear the import-jam marker at the same instant.
	pe.onResolve = func() {
		if isImport {
			entry.setImportInFlight(false)
		}
		// Reset liveness BEFORE releasing the slot so the NEXT holder's brief
		// pre-stamp window (between its sem-acquire and its markProgress) reads
		// lastProgressAt==0 → isStalled false, identical to the first-ever holder.
		// Without this, a holder that acquires after an idle gap > stallThreshold
		// would inherit the prior holder's stale timestamp and a racing peer could
		// spuriously see it as stalled. The sem send/receive happens-before edge
		// orders this store ahead of the next acquire.
		entry.lastProgressAt.Store(0)
		<-entry.sem
	}

	// Resolve the per-request inactivity ceiling (server-managed by op type).
	// pe.resetTimeout = timeout so a progressing read's timer resets to the full
	// ceiling on each progress tick, not a fixed 120s. Writes/cheap reads reset
	// to their own (shorter) base ceiling.
	timeout := resolveRequestTimeout(requestType)
	pe.resetTimeout = timeout
	pe.timer = time.AfterFunc(timeout, func() {
		bridgeLogger.Printf("→ %s %s timed out after %s", requestID, requestType, timeout)
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		pe.once.Do(func() { close(ch) })
		pe.resolve()
	})

	b.mu.Lock()
	b.pending[requestID] = pe
	b.mu.Unlock()

	bridgeLogger.Printf("→ %s %s channel=%q nodeIDs=%v params=%v", requestID, requestType, channel, nodeIDs, params)
	start := time.Now()

	// Mark the import as occupying the thread BEFORE the write — set before dispatch
	// so the plugin's response (handled concurrently in readLoop) can never clear a
	// marker that hasn't been set yet. Cleared via pe.resolve() at resolution.
	if isImport {
		entry.setImportInFlight(true)
	}

	entry.wmu.Lock()
	writeErr := wsjson.Write(ctx, entry.conn, req)
	entry.wmu.Unlock()
	if writeErr != nil {
		pe.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		pe.resolve() // write never reached the plugin → clear the marker
		bridgeLogger.Printf("→ %s %s write error: %v", requestID, requestType, writeErr)
		return BridgeResponse{}, fmt.Errorf("send: %w", writeErr)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			if pe.connDropped.Load() {
				return BridgeResponse{}, errors.New("connection closed: plugin disconnected")
			}
			return BridgeResponse{}, errors.New("request timed out")
		}
		bridgeLogger.Printf("→ %s %s completed in %dms", requestID, requestType, time.Since(start).Milliseconds())
		// C1: stamp queue-wait metadata onto the response the plugin returned, so it
		// reaches renderResponse (and the LLM). Non-breaking — omitempty fields.
		resp.QueueWaitMs = queueWaitMs
		resp.QueueDepth = int(queueDepth)
		return resp, nil
	case <-ctx.Done():
		// Plugin is single-threaded and STILL executing this request. Do NOT release the
		// slot or tear down pe here: leave it registered with its timer running so the
		// eventual plugin response (readLoop) or the server inactivity timer resolves it —
		// that resolution releases the serial slot (and clears the import marker via
		// onResolve). Releasing now would let the next request overlap the busy plugin
		// thread (the slot-release edge case). The buffered ch (cap 1) means a late
		// readLoop send never blocks; pe is GC'd once its resolution deletes it from pending.
		bridgeLogger.Printf("→ %s %s context cancelled (slot held until plugin resolves): %v", requestID, requestType, ctx.Err())
		return BridgeResponse{}, ctx.Err()
	}
}

// broadcastQueue pushes the current distinct set of waiting roster origins to the
// plugin as a presence_queue frame. It is best-effort and FULLY DROPPABLE: presence
// is non-critical and must NEVER block the Send path nor risk the connection.
//
// Two hazards this guards against, both from coder/websocket semantics:
//  1. A write whose ctx deadline fires CLOSES the whole connection (not a recoverable
//     per-write error) — so too tight a deadline on a briefly-backpressured-but-alive
//     socket could tear down the channel. But NO deadline is worse: sync.Mutex.Lock()
//     is NOT ctx-aware, so a goroutine blocked forever in Write(context.Background())
//     while holding wmu wedges EVERY subsequent op write at entry.wmu.Lock() — those
//     op writes never reach their own ctx'd Write, so the conn-close-on-expiry that
//     normally self-heals a dead socket never fires, and the channel wedges permanently.
//     We therefore use a GENEROUS finite deadline: an ~80-byte frame writes effectively
//     instantly on any live socket (10 s is never reached on real backpressure), while a
//     genuinely dead socket is torn down so the channel can self-heal — matching the
//     op-write path's bounded-write behavior.
//  2. coder/websocket forbids concurrent writes (wmu). We never block on Lock (which
//     would let a tiny presence frame delay a real op write on a backpressured socket);
//     instead we TryLock and, on contention, RETRY for a bounded window rather than
//     dropping. Dropping was wrong: once the queue drains to EMPTY there is no "later
//     change" to ride a re-broadcast on, so a single dropped clearing frame left the
//     plugin's queued list stale forever (the agent showed "queued" permanently and its
//     row re-stamped to "Just now" on every sweep, never decaying to away). Re-snapshot
//     on each attempt so a delivered frame always reflects CURRENT truth — this makes
//     delivery order-independent even when an add- and a remove-broadcast goroutine race.
//
// Runs async so the Send hot path is never blocked even by the tiny write.
func (b *Bridge) broadcastQueue(channel string, entry *connEntry) {
	go func() {
		// Bound the retry so a genuinely dead/saturated socket can't spin a goroutine
		// forever; a single ~80-byte frame writes instantly between op writes, so this
		// budget is reached only under pathological sustained contention (where a later
		// waiter change re-broadcasts anyway). An idle channel has no contender to lose to.
		const (
			maxAttempts = 50
			retryDelay  = 10 * time.Millisecond
		)
		for attempt := 0; attempt < maxAttempts; attempt++ {
			if !entry.wmu.TryLock() {
				time.Sleep(retryDelay) // an op write holds the lock — wait and retry
				continue
			}
			// Re-snapshot under no concurrent op write: reflect the queue's current truth.
			frame := PresenceQueueFrame{
				Type:    presenceQueueType,
				Channel: channel,
				Origins: entry.snapshotWaitingOrigins(),
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := wsjson.Write(ctx, entry.conn, frame)
			cancel()
			entry.wmu.Unlock()
			if err != nil {
				bridgeLogger.Printf("presence_queue write error (channel %q): %v", channel, err)
			}
			return
		}
		bridgeLogger.Printf("presence_queue dropped after %d attempts (channel %q): write mutex stayed contended", maxAttempts, channel)
	}()
}

// doSingleflight collapses identical concurrent read requests onto one in-flight
// call. The first caller (leader) runs fn; later callers (followers) wait for
// the leader's result or their own ctx, whichever comes first. The leader holds
// no lock while fn runs, so there is no deadlock with the per-channel slot.
func (b *Bridge) doSingleflight(ctx context.Context, key string, fn func() (BridgeResponse, error)) (BridgeResponse, error) {
	b.flightMu.Lock()
	if fl, ok := b.flights[key]; ok {
		b.flightMu.Unlock()
		bridgeLogger.Printf("⊕ deduped into in-flight %s", key)
		select {
		case <-fl.done:
			return fl.resp, fl.err
		case <-ctx.Done():
			return BridgeResponse{}, ctx.Err()
		}
	}
	fl := &flight{done: make(chan struct{})}
	b.flights[key] = fl
	b.flightMu.Unlock()

	fl.resp, fl.err = fn()

	b.flightMu.Lock()
	delete(b.flights, key)
	b.flightMu.Unlock()
	close(fl.done)
	return fl.resp, fl.err
}

// isReadOnly reports whether a request type is safe to de-duplicate. Only pure
// reads qualify — writes must never be collapsed (create_frame ×2 = two frames).
func isReadOnly(requestType string) bool {
	return strings.HasPrefix(requestType, "get_") ||
		strings.HasPrefix(requestType, "scan_") ||
		strings.HasPrefix(requestType, "search_") ||
		requestType == "export_tokens"
}

// isCacheable reports whether a read request type may be stored in the
// short-TTL read-cache. All cacheable requests are also read-only (isReadOnly),
// but not vice versa:
//
//   - get_screenshot, get_selection, get_viewport reflect LIVE USER STATE in
//     Figma Desktop (current selection, viewport position, rendered pixels)
//     that the bridge cannot observe and cannot invalidate. Caching them would
//     serve a stale screenshot / stale selection to callers even within the 3s
//     TTL. Keep them in the singleflight (isReadOnly) so simultaneous identical
//     calls share one round-trip, but never Put them into the cache.
func isCacheable(requestType string) bool {
	switch requestType {
	case "get_screenshot", "get_selection", "get_viewport":
		return false
	}
	return isReadOnly(requestType)
}

// hashReadKey builds a cache/flight key from (channel, requestType, nodeIDs, params)
// via a deterministic marshal+FNV-64a hash. Returns (key, true) on success or
// ("", false) on a marshal error — callers must bypass dedup/cache on false to
// avoid merging unrelated requests under an empty key.
//
// json.Marshal sorts map keys recursively, so the key is deterministic for equal
// (nodeIDs, params). This is the shared core used by both flightKey (singleflight)
// and readCacheKey (read cache) so the key format has one definition.
// isPresenceParam reports whether a param key is presence metadata (identity /
// status narration) that rides params for the plugin's Watch-agent panel but never
// affects a read RESULT — so it must be stripped from read keys (see hashReadKey).
func isPresenceParam(k string) bool {
	switch k {
	case "origin", "status", "sessionId", "task":
		return true
	}
	return false
}

// hasPresenceParam reports whether any presence param is present, so hashReadKey
// only allocates a filtered copy when there is something to strip.
func hasPresenceParam(params map[string]interface{}) bool {
	for k := range params {
		if isPresenceParam(k) {
			return true
		}
	}
	return false
}

func hashReadKey(channel, requestType string, nodeIDs []string, params map[string]interface{}) (string, bool) {
	// Presence-only params (origin/status/sessionId/task) do NOT change the read
	// RESULT, so they must be excluded from the key — otherwise two agents (or two
	// sessions) reading the same node would split the read cache / singleflight and each
	// pay a separate serial plugin round-trip, defeating cross-agent coalescing
	// (the cache-first discipline). Trade-off: a cache HIT / singleflight FOLLOWER
	// never reaches the plugin, so the SERVED agent's "scanning" status won't fire
	// for that read — acceptable (reads are the noisiest, least-important status).
	keyParams := params
	if hasPresenceParam(params) {
		keyParams = make(map[string]interface{}, len(params))
		for k, v := range params {
			if isPresenceParam(k) {
				continue
			}
			keyParams[k] = v
		}
	}
	payload, err := json.Marshal(struct {
		N []string               `json:"n"`
		P map[string]interface{} `json:"p"`
	}{nodeIDs, keyParams})
	if err != nil {
		return "", false
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(payload))
	return fmt.Sprintf("%s|%s|%x", channel, requestType, h.Sum64()), true
}

// flightKey builds a singleflight key for a read request, or returns ok=false to
// bypass dedup (non-read types, or a marshal error — never collapse on an empty
// key, which would merge unrelated reads).
func flightKey(channel, requestType string, nodeIDs []string, params map[string]interface{}) (string, bool) {
	if !isReadOnly(requestType) {
		return "", false
	}
	key, ok := hashReadKey(channel, requestType, nodeIDs, params)
	if !ok {
		bridgeLogger.Printf("flightKey marshal error (type=%s) — bypassing singleflight", requestType)
	}
	return key, ok
}

// ListChannels returns a snapshot of all connected plugin channels.
func (b *Bridge) ListChannels() []ChannelInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]ChannelInfo, 0, len(b.conns))
	for ch, e := range b.conns {
		out = append(out, ChannelInfo{
			Channel:     ch,
			FileName:    e.fileName,
			FileKey:     e.fileKey,
			PageName:    e.pageName,
			ConnectedAt: e.connectedAt.Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Channel < out[j].Channel })
	return out
}

// Close shuts down the bridge, rejecting all pending requests and closing all sockets.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, pe := range b.pending {
		pe.timer.Stop()
		pe.once.Do(func() { close(pe.ch) })
		pe.resolve()
		delete(b.pending, id)
	}

	for ch, e := range b.conns {
		if err := e.conn.Close(websocket.StatusNormalClosure, "bridge closed"); err != nil {
			bridgeLogger.Printf("close connection (channel %q) error: %v", ch, err)
		}
		delete(b.conns, ch)
	}
}

// nextID generates a request ID in the format req-HHMMSS-N.
func (b *Bridge) nextID() string {
	n := b.counter.Add(1)
	now := time.Now()
	return fmt.Sprintf("req-%02d%02d%02d-%d",
		now.Hour(), now.Minute(), now.Second(), n)
}

// IsConnected reports whether at least one plugin is currently connected.
func (b *Bridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.conns) > 0
}

// MarshalJSON is used when logging — avoid printing full conn objects.
func (b *Bridge) MarshalJSON() ([]byte, error) {
	b.mu.RLock()
	channels := len(b.conns)
	pending := len(b.pending)
	b.mu.RUnlock()
	return json.Marshal(map[string]interface{}{
		"channels": channels,
		"pending":  pending,
	})
}
