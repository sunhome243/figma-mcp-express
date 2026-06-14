# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [2.0.0] — 2026-06-14

### Breaking Changes

- **Default tool profile is now `core`** — legacy clients that call low-level write tools directly must either set `FIGMA_MCP_TOOL_PROFILE=full` or migrate to `search_batch_ops` -> `get_batch_op_spec` -> `batch(validateOnly:true)` -> `batch`.
- **Batch routes `channel` only on the outer call** — `ops[*].params.channel` is rejected. Use `batch(channel:"auto-N", ops:[...])` for multi-file routing.

### Added

- **Progressive batch op discovery** — added `search_batch_ops` and `get_batch_op_spec` so agents can search the validated FigmaPlan/batch catalog first, inspect one op's exact params only when needed, then execute through `batch`.
- **Validated FigmaPlan dry-run** — `batch(validateOnly:true)` now validates op types, params, refs, node IDs, map shape, and script-like fields without sending anything to the Figma plugin.

### Changed

- **Default MCP surface is now compact core** — `FIGMA_MCP_TOOL_PROFILE=core` is the default and exposes a small stable tool set; every plugin-supported operation remains available through validated `batch` op types. Set `FIGMA_MCP_TOOL_PROFILE=full` for the legacy top-level compatibility surface.
- **Default `tools/list` is materially smaller than both the upstream open-source baseline and v1.0.3** — measured from actual JSON-RPC `tools/list` responses with `tiktoken` `o200k_base`: upstream `vkhanhqui/figma-mcp-go@fe6cd768` was 73 tools / 51,125 bytes / 12,214 tokens, while 2.0.0 default core+compact is 21 tools / 14,573 bytes / 3,283 tokens, saving 8,931 tokens (73.1%) and 36,552 bytes (71.5%). Against the existing public v1.0.3 release, v1.0.3 default was 70 tools / 90,038 bytes / 20,822 tokens, saving 17,539 tokens (84.2%) and 75,465 bytes (83.8%).
- **MCP tool schema is compact by default** — `tools/list` now trims long tool and parameter descriptions while preserving key scoping/spill guidance. Set `FIGMA_MCP_TOOL_SCHEMA_MODE=verbose` to restore the full in-schema guidance.
- **Batch op search now indexes params** — `search_batch_ops` matches param keys such as `fontSize`, `componentId`, and `cornerRadius`, so agents can find the right op from the field they need to set instead of already knowing the op name.
- **Batch validation now uses a catalog source of truth** — hidden and demoted ops are validated against `BatchOpCatalog`, so wrong params like `characters` and script-like fields are rejected before mutation instead of being silently ignored by the plugin.
- **Batch payloads now have fail-fast server caps** — oversized generated plans are rejected before plugin execution via `FIGMA_MCP_BATCH_MAX_OPS` (default 200) and `FIGMA_MCP_BATCH_MAX_BYTES` (default 2097152), both overridable for controlled local runs.
- **Batch `map` validation is stricter** — invalid named bindings, string-interpolation attempts such as `"Section $index"`, named-binding projections, reserved `map.as` values, and nested `map` ops are rejected before plugin execution.
- **Batch op specs now match runtime shapes more closely** — `map.over` is exposed as string-or-array, per-op `channel` is omitted from catalog schemas, and search includes enum vocabulary such as `FLATTEN` / `UNION`.
- **Library import asset type is schema-constrained** — `import_component_by_key.assetType` now exposes `COMPONENT|COMPONENT_SET` as an enum so `get_batch_op_spec` and top-level schemas steer agents away from slow wrong-route imports.
- **Library catalog import hints stay bounded** — cached import routing hints now keep only `COMPONENT_SET` keys and cap growth at 10k entries, avoiding unnecessary in-process cache growth from component/style catalog rows.

### Fixed

- **Batch dry-run now preserves direct-tool semantic guards** — `batch(validateOnly:true)` rejects missing `color|paints`, missing page/variable selectors, missing radius/constraint fields, invalid effect types, and no-op paint-style updates before any plugin call.
- **Resolved batch refs are revalidated before plugin dispatch** — after `$N.path` / `$item.path` substitution, concrete ops such as `set_fills`, `set_strokes`, `set_effects`, `set_corner_radius`, and page/variable deletes run fail-fast semantic checks instead of falling through to plugin defaults.
- **Batch ID params normalize like direct tools** — common ID params such as `nodeId`, `parentId`, `pageId`, and `componentId` accept hyphen-form IDs in generated plans and normalize before validation/forwarding.
- **Resolved batch import refs are validated inside the plugin** — after `$N.path` / `$item.path` substitution, `batch` now rejects node IDs, truncated component/style keys, malformed component/style keys, invalid component `assetType`, and bare-node variable keys before any `figma.import*ByKeyAsync` call.
- **Batch validation now also protects transport-level calls** — `Node.Send("batch", ...)` and leader `/rpc` batch requests now run the same `BatchOpCatalog` validation/preparation as the MCP `batch` handler, so direct follower/leader calls cannot bypass schema checks for hidden/core-only ops.
- **Import keys fail fast before reaching the plugin** — `import_component_by_key` / `import_style_by_key` now reject node IDs, truncated keys, and malformed non-40-char lowercase hex keys in the Go server, and `import_variable_by_key` rejects empty keys and bare node IDs without forcing component/style key rules. Cached library catalogs now keep a bounded component-set route-hint index so the plugin skips the slow component-first fallback when a set key is already known.
- **Batch import validation now matches direct tools** — `batch(validateOnly:true)` rejects bad import keys and invalid component `assetType` values before any plugin call; executable batches also inject cached `assetType` hints for `import_component_by_key` ops, including inside `map.do`.
- **Batch schema validation preserves runtime refs** — typed params such as numeric widths and enum values can use valid `$N.path` / `$item.path` refs and are validated structurally before runtime resolution instead of being rejected as the wrong primitive type.
- **Serial slot is no longer released on client-cancel before the plugin responds** — when a non-import request was client-cancelled (e.g. an MCP request-timeout firing mid-flight) after dispatch but before the single-threaded plugin answered, the per-channel serial slot was freed immediately, so the next request was written to the plugin while the cancelled-but-still-running request still occupied the thread (two requests overlapping a one-at-a-time surface). Slot release now flows through `pendingEntry.onResolve` and fires only at true resolution (plugin response, inactivity timer, write error, or connection drop) — never on client-cancel — so a cancelled request keeps the slot until the plugin actually finishes and the next request can no longer overlap it. Covered by new `bridge_slotrelease` tests (held-until-resolve, released-by-late-response, released-by-timer) run under `-race`.
- **Import-jam marker self-heals after a client-cancel** — a client-cancelled `import_*_by_key` previously left `importInFlight` set until the channel reconnected (the cancel path tore down the pending entry, so no resolution site ever cleared it), import-poisoning the channel. It now clears via the same `onResolve` path when the request truly resolves (response or inactivity timer), while still rejecting retried imports during the in-flight window.
- **`search_batch_ops` matches natural multi-word queries** — the matcher was a single contiguous substring check, so `"create frame"`, `"auto layout"`, and `"delete node"` returned zero results (the op names use underscores: `create_frame` / `set_auto_layout` / `delete_nodes`). It now ANDs over whitespace-separated tokens, so multi-word phrasing matches; single-token queries are unchanged.
- **Node-target batch ops accept the target nested in `params`** — `get_batch_op_spec` lists `nodeIds` under `paramKeys`, so an op composed straight from its spec (e.g. `delete_nodes` with `params.nodeIds`) failed with "nodeIds is required" because the batch executor reads the target from the op-level `nodeIds` field. A plural `params.nodeIds` is now hoisted to the op level when the op-level field is empty (validate and execute paths both). The singular `nodeId` is intentionally left in `params` — read/scan ops take it as a subtree root.

## [1.0.3] — 2026-06-13

### Fixed

- **Hung library import no longer wedges the plugin thread** — `figma.importComponentByKeyAsync` / `importComponentSetByKeyAsync` / `importVariableByKeyAsync` / `importStyleByKeyAsync` can hang and never resolve or reject (a COMPONENT_SET key passed to the component importer, or an unpublished/unreachable library), with no built-in timeout or progress tick. Such a hang occupied the single plugin thread until the server-side 120s inactivity ceiling fired, and under concurrent load each fresh attempt re-armed that window — so the import path appeared permanently wedged until a manual plugin restart (observed in a 4-screen concurrent redesign run). All four `import_*_by_key` calls in `plugin/src/write-library.ts` are now wrapped in `withImportTimeout` (a `Promise.race` against a 15s reject-on-timeout, timer always cleared on settle), so a hung import fails fast — the server clears its `importInFlight` marker and the next import proceeds without a restart. The timeout message deliberately omits "not found"/"not a component" so it never triggers the COMPONENT_SET fallback (which would re-hang on the set importer). Covered by 5 new `withImportTimeout` tests.

## [1.0.2] — 2026-06-12

### Changed

- **README** — restructured Known Limitations into named subsections (Desktop required, single-threaded execution, memory on large files, community kit import); trimmed prose
- **figma-go skill** — probe rule calibrated: full probe for organisms only; property-name check for unfamiliar atoms; skip for already-used atoms. Removed private-skill cross-references (`/figma-matching`, hardcoded paths). Component configuration rules now point to `figma-design-patterns/references/component-usage.md`. Clear role separation: figma-go = tool mechanics, figma-design-patterns = composition rules

## [1.0.1] — 2026-06-12

### Fixed

- **Windows cross-compile** — `appendSpillManifest` called `syscall.Flock` (Unix-only) inline, which broke the `GOOS=windows` builds in the release matrix (`undefined: syscall.Flock`). The advisory spill-manifest lock is now platform-split (`spill_lock_unix.go` uses `flock`; `spill_lock_windows.go` is a no-op — a single `O_APPEND` line write is atomic on local filesystems, so the lock is best-effort by design). All six release targets (darwin/linux/windows × amd64/arm64) build again.

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
- **Unknown params are REJECTED, not silently dropped** — every tool's params are validated against its registered MCP schema on BOTH the direct path (`ValidateRPC`) AND each op inside `batch`. A Plugin-API-name typo (`characters`→`text`, `fills`→`fillColor`, `lineHeight`→`lineHeightValue`, `width` on `create_text`→`resize_nodes`) now returns an actionable error instead of producing an empty/default invisible node. The allowlist is **derived from the live tool registration**, so it can never drift from the schema.
- **Tool surface trimmed to 70 live** (16 demoted to batch-only op types). Demoted ops leave `tools/list` but their plugin handlers stay intact — call them as a `batch` op `type`.
- **Timeouts are server-managed** — removed the client `timeoutMs` param. Ceilings are set by op type: `FIGMA_MCP_READ_TIMEOUT` (default 600s) for heavy reads + `batch`, 120s for light ops. A timeout means **re-scope narrower**, never "raise a timeout." The timer is inactivity-based — a read that ticks progress never trips it.
- **Style and variable workflows are broader than upstream** — paint/effect styles accept richer `paints[]` / `effects[]` payloads, and `bind_variable_to_node` now supports a much larger set of bindable fields with stricter request-level validation for unsupported ones.
- **Reparenting is less destructive by default** — `reparent_nodes` now preserves absolute canvas position unless callers explicitly opt out.
### Fixed

- `set_opacity` / `set_visible` are LIVE top-level tools (previously mislabeled as demoted) — they are _also_ valid `batch` ops.
- `rotate_nodes` batch param is `rotation` (not `angle`); positive rotation = counter-clockwise.
- `delete_nodes` no longer aborts the whole batch when one node is un-removable (e.g. an instance child, which Figma natively refuses) — each node is guarded independently and reports a per-node `{nodeId, error}`, matching the not-found path. A `Removing this node is not allowed` error now carries an intent-ordered recovery hint: to actually remove the node, delete it on the master component (propagates) or `detach_instance` first then delete; to replace it, swap the nested instance; `set_visible:false` is called out as a hide, not a delete.
- Spill provenance manifest no longer appends a duplicate line on a cache-hit re-gate of a spilled+cacheable read (unbounded-growth fix).
- Live-state reads (`get_screenshot`, `get_selection`, `get_viewport`) are no longer cached, so callers do not receive stale viewport / selection / image data from the short-TTL read cache.
- Omitted-channel and explicit-channel reads now canonicalize to the same cache entry, and post-write invalidation clears both paths correctly instead of serving stale reads after a mutation.
- Queue metadata injection no longer mutates shared response maps, avoiding `concurrent map writes` failures under singleflight/cache fan-out.
- Cache hits no longer inherit the queued leader's `queueWaitMs` / `queueDepth`; only requests that actually waited report queue metadata.
- Follower HTTP timeout now stays ahead of the leader's longer read ceilings, so follower-proxied heavy reads are less likely to abort before the leader resolves them.
- Leader takeover is more reliable when a dead process is still holding the port; the election path now kills zombie holders before retrying leadership.
- Non-solid paints are no longer silently dropped during serialization; image / gradient paints are surfaced explicitly so downstream consumers can see them.
- `swap_component` now uses Figma's override-preserving swap path instead of direct `mainComponent=` replacement, so instance overrides survive component swaps.
