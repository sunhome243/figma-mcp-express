# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/).

## [1.0.0] — 2026-06-11

Initial release as figma-mcp-express, forked from [vkhanhqui/figma-mcp-go](https://github.com/vkhanhqui/figma-mcp-go).

### Added

- **Multi-channel routing** — connect N Figma files simultaneously; each file gets its own channel id; agents target files explicitly via `channel:` param
- **Multi-page support** — navigate and operate across all pages in a file without re-running the plugin
- **Batch tool** — chain N typed ops (`create_frame`, `set_fills`, `get_node`, …) in one plugin round-trip with `$N.field` ref resolution between steps
- **Concurrent agent queue** — request queue serializes calls per channel safely under parallel multi-agent load; no dropped calls or race conditions
- **Stable connections** — reconnect logic and heartbeat keep sessions alive through long tasks and AI tool restarts
- **Cooperative yield** — heavy reads (`search_nodes`, `scan_nodes_by_types`) yield periodically so the single-threaded plugin stays responsive on 100K+ node files
- **Response spill-to-disk** — oversized responses spill to `.figma-mcp-cache/` instead of returning in-band; preserves token budget and keeps large reads usable on big files
- **Library automation** — `import_component_by_key`, `import_variable_by_key`, `import_style_by_key`, `fetch_library_catalog` (REST), `list_library_variable_collections`, `get_library_variables`
- **Codegen context** — `get_design_context detail:"codegen"` returns token names, auto layout spec, and component references per node
- **Depth-limited traversal** — `get_node` accepts a `depth` param; truncated nodes surface `childCount` so callers know to re-request
- **`get_nodes_info`** — bulk metadata fetch for N known node ids in one round-trip
- **`scan_nodes_by_types` / `scan_text_nodes`** — native C++ traversal, faster than recursive `get_node` walks
- **`enablePrivatePluginApi`** — exposes `figma.fileKey` so the server can identify the connected file without user input
- **Demoted batch ops** — 16 lower-signal ops remain available via `batch` only (`boolean_operation`, `lock_nodes`, `unlock_nodes`, `rotate_nodes`, `set_corner_radius`, and others), while `set_opacity` / `set_visible` stay top-level
- **Claude Code + Codex plugin** — `.claude-plugin/` and `.codex-plugin/` manifests; two portable skills (`/figma-mcp-express`, `/figma-design-patterns`); PreToolUse hook
- **Minimizable plugin window** — the Figma plugin UI can collapse to a small pill and expand again, so it stays out of the way during longer edit or review runs
- **npm cross-platform distribution** — prebuilt binaries for darwin/linux/windows × amd64/arm64 via `npm/run.js`
- **MCP Registry publishing** — `release.yml` publishes to npm and the MCP Registry on tag push
- **`map` batch op** — per-item-varying params in ONE round-trip: `{ type:"map", over:"$0.nodes[*]", as:"item", do:{…} }` with `$item`/`$item.path`/`$index` bindings (cap 500, throttled progress). The way to apply _different_ values per node (vs. `[*]` projection which applies the same value to all).
- **Bulk-apply** — 8 setters (`set_fills`, `set_strokes`, `set_opacity`, `set_visible`, `swap_component`, `set_instance_properties`, `bind_variable_to_node`, `set_corner_radius`) accept `nodeIds[]` in `batch` and return `{results:[{nodeId,…}|{nodeId,error}]}` (one bad id never aborts the rest). Fan a scan into a bulk setter in one op via the `[*]` projection ref: `swap_component nodeIds:["$0.matchingNodes[*].id"]`.
- **Server read-cache** — per-channel in-process cache (default 3s TTL); identical concurrent reads collapse (singleflight) and cache hits skip the plugin → fully parallel. Any write on a channel invalidates that channel's cached reads (generation bump + map clear). Env: `FIGMA_MCP_READCACHE_TTL_MS`, `FIGMA_MCP_READCACHE_MAX`.
- **Queue visibility** — a queued call surfaces `queueWaitMs`/`queueDepth` so an agent (and the LLM) knows it is waiting in line, not hung.
- **NDJSON spill sidecar + provenance manifest** — large spilled responses get a line-per-record `.ndjson` sidecar (grep/jq/duckdb without full-parsing the blob) and an `index.ndjson` provenance manifest for grep-by-intent recovery.
- **Spill-cache eviction** — oversized spill files now age out by TTL / size-cap rules without orphaning sidecars or deleting the provenance manifest
- **Per-op `skipInvisibleInstanceChildren`** — traversal reads can opt into skipping hidden instance subtrees for faster scans on heavy files
- **`multi-agent.md` skill reference** — partition by disjoint regions + coordinator-creates-shared-once + verify-before-create; cache is coherence, not a lock.
- **`import_image` `imagePath`** — pass a local file path and the server reads + base64-encodes it (no inline-base64 transcription, which an LLM can't do reliably for large assets). `imageData` (raw base64) still accepted; provide exactly one.
- **`--version` flag + build-time version stamp** — `figma-mcp-express --version` prints the `git describe` version stamped via `make build-go` ldflags (falls back to `dev`). Lets a reloading MCP client confirm it launched a freshly built binary instead of silently running a stale one. New `make run` target = single-source build-then-launch.
- **Actionable recovery hints** — timeout / stale-id / REST-path errors now return narrower “what to do next” guidance instead of dead-end failures

### Changed

- **Dependent edit sequences no longer pay one LLM round-trip per step** — the `batch` tool collapses create → style → verify chains into one ordered plugin request with `$N.path` ref resolution, which cuts latency and reduces queue contention compared to the original one-tool-call-per-step flow.
- **Server restarts are much less disruptive** — leader/follower takeover keeps the plugin WebSocket owned by the active leader, so restarting the MCP client no longer forces the Figma plugin to reconnect or drop the session the way the original single-process shape did.
- **Read throughput is materially higher than the original `figma-mcp-go`** — identical concurrent reads now collapse onto one plugin round-trip (`singleflight`), a short-TTL per-channel read-cache reuses hot results, and queued calls surface `queueWaitMs` / `queueDepth` so agent orchestration can react to contention instead of blindly piling onto the same plugin slot.
- **Heavy reads are optimized for large files instead of stalling the plugin** — long traversals now yield progress periodically, keep the inactivity timer alive while real work is happening, and pair with spill-to-disk plus depth-limited reads so large audits complete with less UI blocking and less transport/token overhead than the original behavior.
- **Traversal reads are more scopeable and cheaper by default** — `get_node` / `get_nodes_info` are depth-aware for wide-then-deep inspection, `get_node` now defaults to a bounded depth instead of unbounded traversal, and traversal tools can skip hidden instance children when speed matters more than full hidden-state anatomy on instance-heavy files.
- **Type-only scans moved onto native Figma traversal** — `scan_nodes_by_types` / `scan_text_nodes` now use `findAllWithCriteria(...)` instead of manual recursive JS walks, which reduces thread-block time and improves scan responsiveness on large trees.
- **Connection handling is tuned for faster recovery under parallel work** — same-channel reconnects replace only that channel, and a dropped socket drains in-flight work immediately with a connection-closed error instead of leaving requests hung behind a long timeout ceiling.
- **Large spill output is more queryable and less wasteful** — repeated oversized responses now write canonical spill files with NDJSON sidecars and a provenance manifest, so agents can grep/jq targeted slices and repeated cache-hit re-gates do not keep appending duplicate provenance entries.
- **Unknown params are REJECTED, not silently dropped** — every tool's params are validated against its registered MCP schema on BOTH the direct path (`ValidateRPC`) AND each op inside `batch`. A Plugin-API-name typo (`characters`→`text`, `fills`→`fillColor`, `lineHeight`→`lineHeightValue`, `width` on `create_text`→`resize_nodes`) now returns an actionable error instead of producing an empty/default invisible node. The allowlist is **derived from the live tool registration**, so it can never drift from the schema; demoted batch-only ops (unregistered) are left unguarded by design.
- **Tool surface trimmed to 70 live** (16 demoted to batch-only op types). Demoted ops leave `tools/list` but their plugin handlers stay intact — call them as a `batch` op `type` (params pass through verbatim).
- **Timeouts are server-managed** — removed the client `timeoutMs` param. Ceilings are set by op type: `FIGMA_MCP_READ_TIMEOUT` (default 600s) for heavy reads + `batch`, 120s for light ops. A timeout means **re-scope narrower**, never "raise a timeout." The timer is inactivity-based — a read that ticks progress never trips it.
- **Style and variable workflows are broader than upstream** — paint/effect styles accept richer `paints[]` / `effects[]` payloads, and `bind_variable_to_node` now supports a much larger set of bindable fields with stricter request-level validation for unsupported ones.
- **Reparenting is less destructive by default** — `reparent_nodes` now preserves absolute canvas position unless callers explicitly opt out.

### Fixed

- `set_opacity` / `set_visible` are LIVE top-level tools (previously mislabeled as demoted) — they are _also_ valid `batch` ops.
- `rotate_nodes` batch param is `rotation` (not `angle`); positive rotation = counter-clockwise.
- Spill provenance manifest no longer appends a duplicate line on a cache-hit re-gate of a spilled+cacheable read (unbounded-growth fix).
- Live-state reads (`get_screenshot`, `get_selection`, `get_viewport`) are no longer cached, so callers do not receive stale viewport / selection / image data from the short-TTL read cache.
- Omitted-channel and explicit-channel reads now canonicalize to the same cache entry, and post-write invalidation clears both paths correctly instead of serving stale reads after a mutation.
- Queue metadata injection no longer mutates shared response maps, avoiding `concurrent map writes` failures under singleflight/cache fan-out.
- Cache hits no longer inherit the queued leader's `queueWaitMs` / `queueDepth`; only requests that actually waited report queue metadata.
- Follower HTTP timeout now stays ahead of the leader's longer read ceilings, so follower-proxied heavy reads are less likely to abort before the leader resolves them.
- Leader takeover is more reliable when a dead process is still holding the port; the election path now kills zombie holders before retrying leadership.
- Non-solid paints are no longer silently dropped during serialization; image / gradient paints are surfaced explicitly so downstream consumers can see them.
- `swap_component` now uses Figma's override-preserving swap path instead of direct `mainComponent=` replacement, so instance overrides survive component swaps.
