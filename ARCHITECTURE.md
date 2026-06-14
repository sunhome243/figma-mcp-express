# ARCHITECTURE.md — figma-mcp-express

This document covers:

1. [How this MCP works](#how-this-mcp-works) — the plugin-bridge model and request lifecycle
2. [How this fork improves on the original](#improvements-over-the-original) — what was added and why
3. [System deep-dives](#system-deep-dives) — Response Gating, Read Singleflight, Batch, Multi-Channel

---

## How This MCP Works

### The plugin-bridge model

Most Figma MCP servers talk to Figma via the **REST API**. That has hard rate limits (as low as 6 calls/month on free plans) and requires a paid Dev seat for meaningful use.

This server takes a different approach: it communicates with Figma through a **local plugin** running inside Figma Desktop. The plugin has full read/write access to the open file — no REST, no rate limits, no API token required for most operations.

```
Your AI tool (Claude, Cursor, etc.)
      │  stdio (MCP protocol)
      ▼
Go MCP Server (port 1994)
      │  JSON over WebSocket
      ▼
Figma Plugin (running inside Figma Desktop)
      │  Plugin API (JavaScript sandbox)
      ▼
Figma file (live, in-memory)
```

### Request lifecycle

1. **AI tool calls an MCP tool** (e.g. `get_node`) via stdio JSON-RPC
2. **Go server validates** the input against the tool's JSON Schema
3. **Bridge dispatches** the request to the plugin over WebSocket, targeting a specific channel (file)
4. **Plugin executes** the operation using the Figma Plugin API in its JS sandbox
5. **Plugin sends back** the result (or streams progress updates for long operations)
6. **Gate checks** the response size — if it exceeds `FIGMA_MCP_SPILL_BYTES`, the payload is written to `.figma-mcp-cache/` and the LLM receives a file path instead of raw JSON
7. **AI tool receives** the result (inline JSON or a spill handle)

### What the plugin can and cannot do

The plugin runs in a **sandboxed JavaScript environment** inside Figma Desktop. This gives it full access to the open file's node tree, styles, variables, and components — but imposes a few constraints:

| Can do                                                    | Cannot do                                                                 |
| --------------------------------------------------------- | ------------------------------------------------------------------------- |
| Read/write any node in the open file                      | Access files that are not currently open                                  |
| Import components and variables from subscribed libraries | Search published components across Figma (requires REST)                  |
| Read/write design variables and styles                    | `eval()` or `new Function()` — the sandbox forbids dynamic code execution |
| Multi-file work via channel routing                       | Create new files (requires REST)                                          |

This is why the `use_figma` pattern (arbitrary script injection) from official Figma MCP cannot be ported here. All operations are exposed as **typed MCP tools** instead.

### Leader/follower election

Multiple instances of the Go server can run simultaneously (e.g. if you restart your AI tool mid-session). The first instance to bind port 1994 becomes the **leader** and owns the plugin WebSocket connections. Subsequent instances become **followers** and proxy their tool calls to the leader via HTTP `/rpc`. If the leader dies, a follower detects the health check failure, kills any zombie processes, and takes over.

This means the plugin never needs to reconnect when you restart your AI tool — only the MCP client process restarts, not the plugin.

---

## Improvements Over the Original

The original [vkhanhqui/figma-mcp-go](https://github.com/vkhanhqui/figma-mcp-go) is a solid foundation: no REST API, no rate limits, ~73 tools covering read/write access to a single open Figma file. This fork adds the systems needed for **enterprise-scale automation** — working with multiple files simultaneously, automating library migration workflows, reducing LLM round-trips, and preventing the plugin from jamming on large files.

### 1. Multi-file channel routing

**Original behavior:** Connecting a second plugin instance disconnected the first. Only one file could be automated at a time.

**This fork:** The bridge maintains a `map[string]*connEntry` keyed by channel ID. Each connected file gets its own entry and serial slot. New connections on a different channel never affect existing ones.

**Why it matters:** Library migration requires reading from a source library file and writing to a product file at the same time. Parallel agents can now target independent channels without contention.

**New tool:** `list_channels` — enumerate all connected files with `fileName`, `fileKey`, `pageName`.

### 2. Batch tool with $N.field ref resolution

**Original behavior:** Every operation was a separate MCP tool call. A create → style → verify sequence required 3 round-trips through the LLM.

**This fork:** The `batch` tool sends an ordered array of ops to the plugin in a single WebSocket message. Later ops can reference earlier ops' results via `$N.path` refs — the plugin resolves them at runtime without returning to the LLM.

**Why it matters:**

- A 10-op build sequence goes from 10 LLM round-trips to 1 plugin round-trip
- The LLM never needs to handle intermediate `nodeId` values — refs wire ops together internally
- Write-then-verify patterns (create frame → read it back to check structure) become a single atomic call

### 3. Response gating (spill-to-disk)

**Original behavior:** Large responses (e.g. a full-page `get_document` on an enterprise file) were returned inline, frequently overflowing the LLM's context window.

**This fork:** `internal/gate.go` intercepts responses exceeding `FIGMA_MCP_SPILL_BYTES` (default 25KB) and writes them to `.figma-mcp-cache/`. The LLM receives `{ spilled: true, path: "...", preview: "..." }` and uses `jq`/`grep` to extract only what it needs.

**Why it matters:** Enterprise Figma files commonly have node trees in the hundreds of KB to several MB. Without gating, reading any large frame crashes the context. With gating, the LLM can navigate arbitrarily large files by querying the cached payload with shell tools.

### 4. Read singleflight (in-flight dedup)

**Original behavior:** Concurrent identical reads each became separate serial plugin calls.

**This fork:** `internal/bridge.go` deduplicates identical concurrent reads. If 3 agents call `get_styles` simultaneously, one plugin round-trip is made and all 3 agents share the result.

**Why it matters:** Multi-agent workflows (e.g. a parallel scan across 5 sections of a file) frequently issue identical reads at startup (page info, style catalog, variable collections). Dedup eliminates the redundant serial wait.

### 5. Cooperative yield + progress on heavy reads

**Original behavior:** Large traversals (e.g. `search_nodes` on a file with 10,000+ nodes) blocked the single-threaded plugin event loop for tens of seconds, causing the bridge to time out and the plugin to become unresponsive.

**This fork:** All heavy read operations (`get_node`, `get_nodes_info`, `get_design_context`, `search_nodes`, `scan_nodes_by_types`, `scan_text_nodes`, `get_document`, `get_local_components`) yield the JavaScript event loop every ~800 nodes and emit a `progress_update` message. The bridge resets its 120-second timeout on each progress tick, allowing reads to survive arbitrarily large files.

**Why it matters:** Without cooperative yield, the plugin hangs on enterprise files and must be manually reopened. With yield, reads on files with hundreds of thousands of nodes complete reliably.

### 6. Depth parameter on node reads

**Original behavior:** `get_node` always returned the full subtree, which on a large frame means the full response is always large.

**This fork:** `get_node` and `get_nodes_info` accept an optional `depth` parameter. `depth: 1` returns the node and its direct children only. `depth: 0` returns the node alone (no children). Combined with the spill gate, this enables a **wide-then-deep** exploration pattern: first scan shallow to find the right node, then fetch deep only for that node.

### 7. Track A — Library automation tools (8 new tools)

**Original behavior:** No tools for importing library assets or creating instances. All component placement had to be done manually via Figma's UI.

**This fork:** 8 new tools covering the full library automation workflow:

- `import_component_by_key` / `import_variable_by_key` / `import_style_by_key` — import from subscribed libraries
- `create_instance` — import a component set variant and place an instance
- `set_instance_properties` — set variant/slot properties on a placed instance
- `set_variable_mode` — pin a node to a specific mode (dark/light) via `setExplicitVariableModeForCollection`
- `get_remote_variable_collection` — query a library's variable collection modes
- `list_library_variable_collections` — enumerate all subscribed library variable collections

`set_fills` and `set_strokes` are also extended with a `variableId` param for token binding.

**Why it matters:** These tools enable fully automated design token migration and library swap workflows — the core use case for enterprise design system work.

### 8. Track C — Codegen serialization

**Original behavior:** `get_design_context` returned visual structure but not the semantic information needed for code generation.

**This fork:** `get_design_context` accepts `detail: "codegen"` to serialize:

- `autoLayout` — flex semantics (axis, alignment, gap, sizing mode) for CSS/React mapping
- `tokens` — names of bound design variables per node, for token-to-CSS-variable mapping
- `componentRef` — `mainComponent` key + name for code-component mapping
- `codeConnect` — optional `codeConnectMap` param for local Code Connect resolution

**Why it matters:** Design-to-code tools need to know not just "this node has padding 16px" but "this node is bound to the `spacing/4` token" and "this is an instance of the `Button/Primary` component". The codegen detail level exposes all of this.

### 9. Track F — REST catalog tools (3 new tools)

**Original behavior:** No tools for enumerating library assets without opening the library file.

**This fork:** 3 new REST-path tools for library discovery:

- `fetch_library_catalog` — fetch published components + thumbnails from Figma's REST API (requires `FIGMA_TOKEN`)
- `get_local_components` — enumerate unpublished components page-by-page via the plugin (no token required)
- `get_library_variables` — enumerate subscribed library variable collections, bypassing Enterprise 403 gates

### 10. fileKey auto-exposure

**Original behavior:** No way to know which file was open without the user providing a URL.

**This fork:** The plugin calls `figma.enablePrivatePluginApi()` at startup to expose `figma.fileKey`. This value is included in every channel entry and returned by `list_channels`. The server can identify the connected file without any user input.

---

## System Deep-Dives

### Response Gating (Spill-to-Disk)

**Why this exists:** Figma files can produce response payloads far larger than an LLM should read inline. Large node trees, scans, and batch outputs can waste context, slow the model down, and make follow-up reasoning worse. The gate keeps the LLM's response small while preserving the full payload on disk, so agents can inspect only the slice they need with `rg`, `grep`, or `jq`.

Every tool response passes through `internal/gate.go`. If the serialized JSON exceeds `FIGMA_MCP_SPILL_BYTES`:

1. `.figma-mcp-cache/` is created in the working directory
2. The full payload is written to `<label>-<sha256-8hex>.json` (canonical, owner-only 0o600)
3. For collection responses (node arrays, scan results, batch output), an NDJSON sidecar is written alongside — one record per line, greppable without full-parsing the blob
4. The LLM receives:

```json
{
  "spilled": true,
  "path": ".figma-mcp-cache/get_node-a1b2c3d4.json",
  "indexPath": ".figma-mcp-cache/get_node-a1b2c3d4.ndjson",
  "bytes": 184320,
  "preview": "{ \"id\": \"1:2\", \"name\": \"Dashboard\", ...",
  "hint": "query indexPath with grep/rg for fast lookup; use path only for full-fidelity reads"
}
```

Query patterns:

```bash
# Fast: grep the NDJSON index (no full parse, constant RAM)
grep '"type":"INSTANCE"' .figma-mcp-cache/get_node-a1b2c3d4.ndjson | jq -c '{id,name}'

# Full fidelity: parse the canonical JSON for a specific subtree
jq '.children[] | select(.id=="2:34")' .figma-mcp-cache/get_node-a1b2c3d4.json
```

Config: `FIGMA_MCP_SPILL_BYTES` (default 25000). Content-addressed filenames make the gate idempotent — same payload always maps to the same file pair.

---

### Read Singleflight (In-Flight Dedup)

**Why this exists:** The Figma plugin runs on a single UI thread. Even read-only calls compete for that thread, and multi-agent sessions often start by asking for the same context — pages, styles, variables, or a shared node subtree. Singleflight collapses identical concurrent reads into one plugin call, reducing serial queue pressure without changing the result returned to each agent.

```
Agent A ─┐
Agent B ─┼─→ flightKey = fnv(channel + type + nodeIds + params)
Agent C ─┘         │
                   ▼
            existing flight? → join (wait on same channel)
            no flight?       → dispatch to plugin, register flight
                   │
                   ▼
            plugin responds → all waiters receive result simultaneously
```

Only read operations are deduped: `get_*`, `scan_*`, `search_*`, `export_tokens`. Writes are always dispatched independently.

**Per-channel serial slot:** Each channel holds a `sem chan struct{}` (cap 1) that serializes non-deduped requests, matching the single-threaded plugin constraint.

---

### Batch with $N.field Ref Resolution

**Why this exists:** Many Figma edits are naturally back-to-back: create a node, style it, configure layout, bind variables, then read it back. If each step returns to the LLM before the next call, the single-threaded plugin pays many serialized round-trips and multi-agent sessions spend more time waiting behind each other. Batch ref resolution keeps dependent work inside one plugin request, avoiding back-to-back LLM calls and reducing contention under parallel agent load.

```json
{
  "ops": [
    { "type": "create_frame", "params": { "name": "Card", "width": 320, "height": 200 } },
    { "type": "set_fills", "nodeIds": ["$0.id"], "params": { "color": "#FFFFFF" } },
    { "type": "get_node", "nodeIds": ["$0.id"], "params": { "depth": 1 } }
  ]
}
```

`$N.path` refs are resolved at runtime against op N's `data` field. Dot notation and array indexing are supported (`$0.nodes.0.id`). Forward refs (pointing to an op that has not run yet) are rejected.

**Stop policy:**

- Any `$N` ref present → stop at first error (dependent chain — downstream refs would be broken)
- No refs → continue past errors, report all results (independent bulk)
- `continueOnError: true/false` overrides either default

**Not transactional.** Write ops that succeed before a failure remain applied. The response includes `{results, okCount, failCount, failedAt}` for recovery.

---

### Multi-Channel Routing

**Why this exists:** Real design automation often needs more than one file or page: source library plus product file, multiple product surfaces, or parallel audit/build agents. Channel routing keeps each open Figma file isolated while still allowing one MCP server to coordinate them, so work can scale without reconnecting the plugin or manually switching files.

```
Figma Desktop (File A)     Figma Desktop (File B)
      │ channel=auto-1           │ channel=auto-2
      ▼                          ▼
  ┌─────────────────────────────────┐
  │         Go Bridge               │
  │  conns["auto-1"] → connEntry    │
  │  conns["auto-2"] → connEntry    │
  │  (each has own sem + flight map)│
  └─────────────────────────────────┘
```

Same-channel reconnect replaces only that channel's socket. Cross-channel connections never interfere with each other.

The plugin self-reports `fileKey` via `enablePrivatePluginApi`, which is stored in the channel entry and returned by `list_channels`.

---

## Active Optimizations

Recently landed and actively tuned improvements to the architecture. The shared theme is protecting Figma's single plugin thread: keep the UI responsive, avoid silent queue stalls, collapse avoidable work, and guide agents toward narrower retries instead of larger timeouts. These notes are documented here because they affect how the system should be used, debugged, and extended.

### Timeout is an inactivity timer, not a deadline

`FIGMA_MCP_TIMEOUT` (default 120s) and `FIGMA_MCP_READ_TIMEOUT` (default 600s) are **inactivity ceilings**, not hard per-request deadlines. Every `progress_update` the plugin emits resets the timer. A heavy read that keeps ticking progress — `get_node` on a large frame, `get_design_context` on a complex component — runs indefinitely as long as it stays active.

The two ceilings apply by op type:

- **`FIGMA_MCP_TIMEOUT` (120s):** lightweight ops — writes, `get_metadata`, `get_styles`, `get_pages`, and similar cheap reads.
- **`FIGMA_MCP_READ_TIMEOUT` (600s):** heavy reads (`get_node`, `get_nodes_info`, `get_design_context`, `get_document`, `scan_nodes_by_types`, `scan_text_nodes`, `search_nodes`, `get_local_components`) and `batch`.

The timeout only fires during a **silent stretch**: a single blocking plugin call with no yields. When it fires, the correct response is _retry with a narrower scope_, not _raise the ceiling_. Raising the ceiling converts a recoverable inactivity signal into a permanent plugin jam.

Three failure modes are handled, all fast-fail:

- Plugin returns an error response → resolved immediately (no waiting for the ceiling).
- Plugin WebSocket drops → ALL pending requests for that connection resolve immediately with "connection closed: plugin disconnected" — not after the 600s ceiling.
- Silent inactivity (connected but no response or progress tick) → inactivity timer fires at the ceiling.

Three resource axes are handled independently:

- Output too large → spill gate (`FIGMA_MCP_SPILL_BYTES`)
- Compute time → cooperative yield + inactivity timer
- Connection liveness → drain-on-disconnect (immediate resolution)

### Idle canvas stutter

The plugin registers `selectionchange` and `currentpagechange` listeners that call `sendStatus()` on every canvas interaction — pan, click, drag. Each call does 3 synchronous Figma API reads + WebSocket send on the main thread, starving Figma's renderer even when no MCP request is in flight.

Fix in progress: **debounce + diff-before-send** — collapse a pan storm to one send (~300ms trailing debounce); skip the send entirely if file/page/selection hasn't changed. Eliminates steady-state cost from interactions that don't affect channel identity. Also: exponential backoff on reconnect (currently a fixed 1500ms metronome when the server is down).

### Native scan performance

`scan_nodes_by_types` and `scan_text_nodes` currently do manual recursive JS walks, crossing the JS↔C++ boundary at every `.children` access. The Figma Plugin API's `node.findAllWithCriteria({types})` runs the same traversal in C++ — same result, 10–100× faster, and shorter total thread-block so Figma stays responsive during the scan.

Switching type-only scans to native also removes the need for mid-scan yield on those paths (yield was the workaround for the long thread-block; the native scan is short enough it doesn't need it).

### Two-tier tool surface

The Go layer decouples the **MCP tool registry** (`s.AddTool`) from the **plugin command handlers**. Removing a tool from `s.AddTool` drops it from the top-level MCP surface, but the plugin still handles it by name — so `batch` can still call it via `{type: "<tool-name>", params: {...}}`.

This enables a two-tier surface: complex multi-param tools stay top-level (typed schema visible to the LLM); simple single-param setters (`set_opacity`, `set_visible`, `lock_nodes`, `rotate_nodes`, …) can be demoted to batch-only. The LLM calls them through `batch` with the params documented in `batch-recipes.md` rather than as standalone tools, reducing surface noise without losing typed functionality.

**Gate before trimming:** confirm `batch({ops:[{type:"<demoted>",…}]})` dispatches correctly in a live session before removing any tool from the MCP registry.
