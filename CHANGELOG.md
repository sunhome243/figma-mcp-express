# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [2.6.0] — 2026-06-20

### Added

- **Figma Plugin API gap coverage for media/link, dev resources, style organization, and variable helpers.**
  Added top-level tools plus validated `batch` op support for:
  `import_image(imageUrl)` via `createImageAsync`, `create_video` via `createVideoAsync`,
  `create_gif`, `create_link_preview`, `create_vector`, `create_slice`, `create_page_divider`,
  `create_text_path`, `get_image_by_hash`, `get_file_thumbnail` / `set_file_thumbnail`,
  node-level Dev Resource CRUD (`get_dev_resources`, `add_dev_resource`, `edit_dev_resource`,
  `delete_dev_resource`), `get_selection_colors`, local style and style-folder reordering
  (`reorder_local_style`, `reorder_local_style_folder`), `create_variable_alias`,
  `resolve_variable_for_consumer`, `update_variable(removeCodeSyntax)`,
  `bind_variable_to_effect`, and `bind_variable_to_layout_grid`.
- **Multi-agent origin discipline in the bundled skill docs.** The multi-agent reference now states
  that `origin` is a fixed roster enum, agents must use the origin assigned to them, random enum
  selection is forbidden, the orchestrator origin is `wolfgang`, `sessionId+origin` is the identity
  key, `set_presence` should be called at dispatch/workflow transitions, and `batch` carries
  `origin` as a top-level argument.
- **Native `GLASS` / `NOISE` / `TEXTURE` effects** in `set_effects` and `create_effect_style`
  (top-level tools + `batch` ops). Previously only `DROP_SHADOW`/`INNER_SHADOW`/`LAYER_BLUR`/
  `BACKGROUND_BLUR` were accepted, forcing callers to fake frosted glass with a background-blur +
  translucent-fill recipe. Now a caller can request Figma's real 2025 **Glass** effect
  (`{type:"GLASS", lightIntensity, lightAngle, refraction, depth, dispersion, radius}` — all
  defaulted), plus `TEXTURE` (`noiseSize, radius, clipToShape`) and `NOISE`
  (`noiseType MONOTONE|DUOTONE|MULTITONE, color, secondaryColor, opacity, noiseSize, density`).
  `TEXTURE` and `NOISE` also preserve optional anisotropic `noiseSizeVector:{x,y}` when provided.
  The plugin builds the effect via `buildAdvancedEffect` and assigns it to `node.effects` /
  `style.effects`; the Go schema validator accepts the three new type literals.
- **PROGRESSIVE (gradual) blur** for `LAYER_BLUR` / `BACKGROUND_BLUR`. Pass `blurType:"PROGRESSIVE"`
  (with optional `startRadius`, `radius` end, and normalized `startOffset`/`endOffset` vectors,
  defaulting to a top→bottom ramp `{0.5,0}`→`{0.5,1}`) to get an iOS-style gradual blur instead of a
  uniform one. Omitting `blurType` still builds a uniform `NORMAL` blur (unchanged default).
- **`update_variable` / `update_variable_collection` — variable & collection metadata management.**
  `update_variable`: rename, set publishing `scopes` (validated against the 22-value VariableScope
  enum), `hiddenFromPublishing`, and per-platform `codeSyntax` (`WEB`/`ANDROID`/`iOS`).
  `update_variable_collection`: rename, `hiddenFromPublishing`, `renameMode`, `removeMode` (with a
  clear error when removing the last remaining mode). Both are also `batch` ops.
- **`set_constraints` promoted to a top-level tool.** Previously batch-only (LEVER 4 demotion);
  now a first-class tool for pinning behaviour on non-auto-layout children. Still a `batch` op.
- **`set_text_range` — per-span (character-range) text styling.** Apply mixed fonts/sizes,
  per-span color, hyperlinks (`{url}` or `{nodeId}`), list options (`ORDERED`/`UNORDERED`/`NONE`),
  indentation, decoration, and per-range spacing to a `[startOffset, endOffset)` slice of a TEXT node.
  All fonts covering the range are loaded before mutation; offsets are validated. Also a `batch` op.
- **Whole-node text properties on `set_text` / `create_text`:** `textStyleId` (link a named text
  style), `textTruncation` + `maxLines` (modern truncation), `paragraphIndent`, `paragraphSpacing`,
  `listSpacing`, `leadingTrim`, `hangingPunctuation`, `hangingList`.
- **Five new node-creation tools** (also available as `batch` ops):
  - `create_line` — straight LineNode for dividers/rules (defaults to a visible 1px stroke; `strokeWeight`, `strokeColor`, `strokeCap`, `rotation`, `length`).
  - `create_polygon` — regular PolygonNode (`pointCount` ≥3, `fillColor`).
  - `create_star` — StarNode (`pointCount`, `innerRadius` 0–1, `fillColor`).
  - `import_svg` — vector nodes from raw SVG markup via `figma.createNodeFromSvg` (the simplest way to add custom icons without a library component).
  - `create_table` — TableNode (`numRows`, `numColumns`, optional `cells` 2D text array).
- **GRID auto-layout mode.** `set_auto_layout` (and `create_frame`) now accept `layoutMode: "GRID"`
  with `gridRowCount`, `gridColumnCount`, `gridRowGap`, `gridColumnGap`, plus `gridRowGapVariableId` /
  `gridColumnGapVariableId` for token-bound grid gaps. Previously the Go schema rejected `"GRID"`
  outright even though the plugin API supports it.
- **Responsive min/max constraints.** `minWidth`, `maxWidth`, `minHeight`, `maxHeight` are now settable
  on `set_auto_layout` / `create_frame` (frame-level) and `resize_nodes` (auto-layout child level).
  Pass `null` to clear a constraint.
- **`import_image` now exposes the full ImagePaint surface:** `rotation` (FILL/FIT/TILE),
  `scalingFactor` (TILE density), `imageTransform` (CROP crop/zoom matrix), and all 7 `ImageFilters`
  (`exposure`, `contrast`, `saturation`, `temperature`, `tint`, `highlights`, `shadows`, each -1..1).
  The filter object is built only from explicitly-provided fields so unintended zeros are never sent.
- **More auto-layout properties on `set_auto_layout`:** `counterAxisAlignContent` (`AUTO` /
  `SPACE_BETWEEN`, wrapped-track distribution), `overflowDirection` (`NONE` / `HORIZONTAL` /
  `VERTICAL` / `BOTH`), `strokesIncludedInLayout`, `itemReverseZIndex`, and `counterAxisSpacingVariableId`
  (token binding for the wrapped-track gap).

### Notes

- The newer surfaces here (GRID auto-layout, `create_table`, and the recent TextNode props
  `leadingTrim` / `textTruncation` / `maxLines` / `hangingList` / `hangingPunctuation`) require a
  reasonably current Figma (built against `@figma/plugin-typings` 1.124.0). On an older Figma desktop
  these may no-op or throw at runtime; the schema layer can't detect the host version.
- Node ID tool schemas remain colon-format-first (`4029:12345`), matching Figma plugin IDs; the
  runtime normalizes common URL hyphen IDs before validation as a compatibility recovery path.
- `create_polygon` / `create_star` validate the Figma shape bounds at the schema layer
  (`pointCount >= 3`, `innerRadius` 0-1) and keep plugin-side clamping as a defensive guard for raw
  plugin or older batch paths.

### Fixed

- **`create_variable_alias` → `set_variable_value` workflow now validates end-to-end.**
  `set_variable_value.value` is no longer advertised as string-only, so a `VARIABLE_ALIAS` object
  returned by `create_variable_alias` passes both MCP tool schema inspection and validated `batch`
  plans.
- **Variable metadata updates validate before mutating.** `update_variable` and
  `update_variable_collection` now reject invalid code-syntax, scope, rename-mode, and last-mode
  removal inputs before changing names, scopes, hidden flags, or modes.
- **Media paint inputs reject non-finite values and malformed transforms.** `import_image` and
  `create_video` now validate filter values, `rotation`, `scalingFactor`, and 2x3 media transforms
  at the Go schema and plugin runtime boundaries before writing fills.
- **`reorder_local_style` now verifies resolved style types.** A mismatched `styleType`, target
  `styleId`, or `afterStyleId` is rejected before calling the corresponding Figma move API.
- **`get_local_components` #29 follow-up.** Docs and hints now distinguish the whole-file
  local-master recovery scan from `pageId` bounded one-page enumeration, and tests cover duplicate
  suppression in the recovery scan.
- **`set_blend_mode` rejecting `LINEAR_BURN` / `LINEAR_DODGE`.** Both are valid Figma blend modes (the
  plugin handler already accepted them) but the Go schema allowlist omitted them, failing the call
  before it reached the plugin. Added to `validBlendModes`.
- **`create_table` cell text now loads each distinct target cell font before mutation.** New Figma
  tables normally share one cell font, but loading per distinct cell font avoids the single-font
  assumption if a table cell font differs before text insertion.
- **Skill docs now route SVG ingestion through `import_svg`.** The old gotcha still described SVG
  import as a missing MCP capability.

## [2.5.3] — 2026-06-19

### Fixed

- **`batch` `import_image` ignoring `imagePath`** — `prepareBatchImportOpParams` now handles `import_image` ops the same way the standalone tool handler does: reads the file at `imagePath` and converts it to base64 `imageData` before sending to the plugin. Previously, `imagePath` was forwarded raw and the plugin (which only speaks `imageData`) returned `"imageData (base64) is required"`.

## [2.5.2] — 2026-06-18

### Changed

- **Transport compression on both hops.** Enabled permessage-deflate on the plugin↔leader
  WebSocket (`websocket.AcceptOptions.CompressionMode`) and gzip on the leader's `/rpc` + `/channels`
  HTTP endpoints (follower hop). Measured on the real captured wire corpus (78 payloads, 6.0 MB):
  node-tree reads compress ~6–14× (whole-corpus 6.3×, ~84% fewer bytes) at ~10 ms/MB CPU. The plugin
  (browser WS client) and follower (Go `http.Client`) both auto-negotiate and transparently
  decompress — no client-side change. base64 image payloads compress only ~1.3× (already-compressed),
  so this is primarily a JSON-read win.
- **Plugin bundle minified for consumers, readable for developers.** `vite.config.main.ts` now
  gates minification on build mode: the default `vite build` (and the release CI artifact) produces a
  minified `code.js` (190KB → ~102KB, ~46% smaller), while the `dev` watch builds with
  `--mode development` (unminified + sourcemap) so plugin-logic stack traces stay readable. No
  behavior change — only bundle size.

### Fixed

- **readCache generation-map leak.** `gens[channel]` entries were never removed, so auto-assigned
  channel ids (`auto-N`) accumulated across reconnects. Added `readCache.DeleteChannel`, invoked on
  final channel disconnect, which drops the channel's generation counter and any residual cached
  entries.

## [2.5.1] — 2026-06-18

### Added

- **Marquee scroll for long task text.** `agent-task` lines in the Watch-agent panel now scroll horizontally in a seamless loop instead of truncating. Falls back to ellipsis when `prefers-reduced-motion` is set.
- **`set_presence` task `maxLength: 80` in schema.** LLMs now see the 80-char cap directly in the tool schema, not just via server-side truncation.

### Changed

- **Agent row UI polish.** Avatar 28 → 32 px, border-radius 9 → 10 px, subtle `box-shadow`, task text `#555` → `#3d3d3d`, name/task/action gap 1 → 2 px, status chip 7 → 8 px, row padding slightly more generous.

## [2.5.0] — 2026-06-18

### Added

- **Per-session presence identity (`sessionId`).** Two orchestrators sharing the same roster name no longer clobber each other in the Watch-agent panel. Each process mints a `sessionId`; the panel keys rows by `(sessionId, origin)` instead of `origin` alone.
- **`set_presence` tool.** Dedicated path for manual `status` + sticky one-sentence `task`. No canvas mutation; records directly to the Watch-agent panel.
- **`get_design_context` `nodeId` param.** Scopes the read to a specific node subtree regardless of current Figma selection (fixes #34).
- **`mainComponentId` on INSTANCE reads.** Surfaces the master's node id alongside `mainComponent: {key,name,remote}`. Useful for file-local masters that have no publishable key (fixes #29 part 1).
- **WebSocket heartbeat.** Pings the plugin every 15 s; cancels the connection if no pong arrives within 10 s so a dead transport fails fast instead of waiting at the 120 s ceiling (fixes #32, partial).
- **Stalled-head guard.** When a slot-holder shows no progress for 45 s (`FIGMA_MCP_STALL_THRESHOLD`), new requests on that channel are early-rejected with `ErrChannelStalled` instead of queuing silently behind the wedge. Self-heals the instant the head frees the slot.

### Changed

- **`search_batch_ops` / `get_batch_op_spec` — synonym-aware, ranked fallback.** Queries tokenize camelCase/separators, fold in search-only synonyms (`remove`→`delete_nodes`, `duplicate`→`clone_node`, etc.), and return ranked suggestions when nothing matches. `get_batch_op_spec` on an unknown op returns "Did you mean: …?"
- **`makeProgress` time-based liveness floor.** Ticks every 10 s of wall-clock in addition to every 800 nodes, so slow low-count reads keep the inactivity timer and stalled-head guard alive.
- **Codex plugin marketplace.** `codex plugin marketplace add sunhome243/figma-mcp-express` now installs the plugin.
- **`status` moved off `batch` → `set_presence`.** Operational tools carry only `origin`; manual workflow `status` and `task` go through `set_presence`.

### Fixed

- **`queued` status clears after queue drains.** Agents no longer stay pinned to "queued" when the `presence_queue` clear frame was dropped under write contention. Fixed with server-side retry + plugin-side local eviction.
- **`find_replace_text` skips component masters.** Page-wide replaces no longer corrupt masters by propagating text edits to every instance (fixes #33).
- **Fill color-variable bindings surface in reads.** `SOLID` fills bound to a variable serialize as `{color, variableId}` instead of bare hex, making token-binding verifiable from a read (fixes #27).
- **`save_screenshots` validates input.** Returns an actionable error instead of silent `{succeeded:0}` when `items` is missing or empty (fixes #28).
- **Version-aware leader election.** A strictly-newer binary evicts a stale older leader at startup and on monitor ticks, instead of proxying every call through the stale process (fixes #26).
- **`batch` op-type timeout.** Heavy reads keep the 600 s ceiling; writes cap at 120 s so a hung mutation frees the channel instead of wedging for the full 600 s (fixes #31).

### Tests

- Regression guards for issues #35 (`create_variable` batch validation) and #36 (`get_batch_op_spec` schema for `set_text`/`set_opacity`).

## [2.4.0] — 2026-06-18

### Added

- **Prototype scroll & fixed-children ops.** `set_overflow` (scroll direction + `clipsContent`), `set_fixed_children` (raw leading-fixed count), and `pin_child` (full pin recipe: ABSOLUTE + move into fixed band + bump parent count). All batch-only; validated server-side.
- **`set_prototype_background` op.** Sets a page's prototype presentation background to a solid color, or clears it with `mode:"clear"`. Batch-only.
- **`figma-prototype` skill — §10 scroll & fixed children.** Documents the `overflowDirection`+`clipsContent` gotcha, the leading-fixed-band model, and a scrollable-body-with-pinned-tab-bar recipe.

## [2.3.0] — 2026-06-17

### Added

- **Multi-agent presence — `origin` label + Watch-agent panel.** Agents stamp a required `origin` enum on every call; the plugin panel shows each active agent as a row (avatar + name + last action + status + `[→]` jump). Canvas highlights the union of active agents' recent nodes without auto-scrolling.
- **Per-agent status (auto-derived + LLM-set).** Auto statuses (`building`, `importing`, `screenshotting`, `scanning`, `theming`) cost zero tokens and are derived from the op. LLM-set statuses (`thinking`, `waiting_review`, `reviewing`, `approved`, `escalated`, `done`) are sticky and set via `set_presence`.
- **Server-auto `queued` via `presence_queue` WS frame.** When calls back up on a channel's serial slot, the server pushes the waiting list to the plugin as an unsolicited frame so the panel can show `Queued · #N`.
- **`origin` extended to read tools + new `status` param.** Read tools (`get_`, `scan_`, `search_`, etc.) now carry `origin` so reads are attributed and auto-status works on non-write ops.
- **Watch-agent UI polish.** Typewriter status transitions, per-agent join animation, followed agent pinned to top, auto-fit window height.

### Changed

- **Removed 8 unused prompt strategies; added tool-guidance hints.** Trimmed the prompt surface; added `batch-recipes.md` and `tool-selection.md` skill references.
- **Design-craft skill refinements.** Tightened `component-reuse`, `composition-patterns`, and `handoff-checklist`.
- **Discovery-discipline guidance.** Added `gotchas.md` entry covering "this op doesn't exist" phantom misses — naming `clone_node` and `reparent_nodes` as the most commonly assumed-absent ops.

## [2.2.0] — 2026-06-16

### Added

- **`get_prototype` — page-level prototype flow graph.** Returns every reaction-bearing node as `source → destination` edges plus flow starting points and overlay config. Complements `get_reactions` for flow auditing.
- **`set_prototype_start` — set a page's flow starting point(s).** Four modes: `replace` (default), `append`, `remove`, `clear`.
- **`figma-prototype` skill.** Auto-wires prototype interactions onto existing designs; bundles `prototype-patterns.md` with navigation/trigger/transition/overlay conventions and an analysis & audit workflow.

### Changed

- **`set_reactions` — full Plugin API coverage.** Validates `SET_VARIABLE`, `SET_VARIABLE_MODE`, `CONDITIONAL`, `UPDATE_MEDIA_RUNTIME`; validates plural `actions[]`; documents `ON_KEY_DOWN`/`ON_MEDIA_HIT`, `SCROLL_ANIMATE`, advanced easings, and `URL.openInNewTab`.
- **`figma-mcp-express` skill — library check in First Checks.** Empty `get_variable_defs` means "no local tokens," not "no tokens" — directs to `list_library_variable_collections` for subscribed libraries.

### Fixed

- **Version-mismatch hint on stale-leader op rejection.** When a newer follower forwards an op the older leader doesn't know, the error now names the real cause (stale leader on :1994) instead of "unknown op type."
- **PreToolUse skill-reminder fires only once per session.** Was keying on `os.getppid()` (fresh per hook process) so the nudge repeated on every MCP call.
- **`get_design_context dedupe_components:true` subtree serialization bounded.** Per-node serialize was unbounded even though only direct-child fields were used; capped to `maxDepth:1`.

## [2.1.0] — 2026-06-15

### Fixed

- **Deep reads no longer re-serialize the subtree per level.** `serializeNode` is now depth-aware; `get_node`, `get_nodes_info`, and `get_design_context` walk the subtree once instead of O(N·D). ~10× fewer serialization passes on a depth-10 tree; measured ~14× on depth-14. Output byte-identical.
- **Read lookups prefetched in parallel.** Style ids, main-components, and bound-variable ids are resolved with one `Promise.all` before the walk, instead of awaiting each serially mid-walk.
- **Bulk writes prefetch nodes in parallel.** `bulkApply` now `Promise.all`s `getNodeByIdAsync` before mutating sequentially, eliminating per-node lazy-page-load latency under `documentAccess:"dynamic-page"`.

### Changed

- **`get_nodes_info` depth-bounded.** Previously recursed unbounded; now defaults to the same depth cap as `get_node` and accepts an optional `depth` param.
- **`figma-mcp-express` skill leaned.** Read recipe and import-key format moved to `references/tool-selection.md`; two-phase bounded read documented as default for large nodes.

## [2.0.1] — 2026-06-15

### Added

- **Plugin-API hygiene linting.** ESLint with `eslint-plugin-figma-plugins` bans deprecated synchronous Figma APIs; wired into CI (`verify-plugin` → `make lint-ts`, `--max-warnings 0`).

### Fixed

- **Long exports keep the bridge alive.** `get_screenshot` now exports serially and both `get_screenshot` / `export_frames_to_pdf` emit a `progress_update` per frame.
- **Bulk writes keep the bridge alive.** `bulkApply` and multi-node write loops emit a `progress_update` every 50 mutations.
- **Whole-page reads keep the bridge alive.** `get_fonts` is now async with a per-node heartbeat; `get_document` threads a heartbeat through `serializeNode`.
- **UI→plugin messages are origin-scoped.** Every `parent.postMessage` carries `pluginId`, so another plugin can't intercept the WS config.

### Internal

- Extracted shared `makeProgress` heartbeat helper into `plugin/src/progress.ts`.

## [2.0.0] — 2026-06-14

### Breaking Changes

- **Default tool profile is now `core`.** Clients using low-level write tools directly must set `FIGMA_MCP_TOOL_PROFILE=full` or migrate to `search_batch_ops` → `get_batch_op_spec` → `batch`.
- **`channel` routes only on the outer `batch` call.** `ops[*].params.channel` is rejected.

### Added

- **Progressive batch op discovery.** `search_batch_ops` + `get_batch_op_spec` let agents search the validated catalog and inspect one op's exact params before executing.
- **`batch(validateOnly:true)`.** Validates op types, params, refs, and node IDs without sending to the plugin.

### Changed

- **Default MCP surface is compact core.** Measured with `tiktoken o200k_base`: upstream `vkhanhqui/figma-mcp-go@fe6cd768` was 73 tools / 51,125 bytes / 12,214 tokens; v1.0.3 default was 70 tools / 90,038 bytes / 20,822 tokens; 2.0.0 default core is 21 tools / 3,283 tokens — saving 8,931 tokens (73.1%) / 36,552 bytes (71.5%) vs. upstream and 17,539 tokens (84.2%) vs. v1.0.3. Set `FIGMA_MCP_TOOL_PROFILE=full` for the full surface.
- **`search_batch_ops` matches param keys.** `fontSize`, `componentId`, `cornerRadius`, etc. are indexed so agents can find ops from the field they need to set.
- **Batch validation uses a catalog source of truth.** Wrong params and script-like fields are rejected before mutation.
- **Batch payloads have fail-fast server caps.** `FIGMA_MCP_BATCH_MAX_OPS` (200) and `FIGMA_MCP_BATCH_MAX_BYTES` (2 MB) reject oversized plans before plugin execution.
- **Unknown params are rejected, not silently dropped.** Every tool validates params against its registered schema; `characters`→`text` typos return an actionable error.

### Fixed

- **Serial slot held until true resolution, not client-cancel.** A cancelled in-flight request no longer frees the slot early, preventing the next request from overlapping a still-running plugin op.
- **Import-jam marker clears on client-cancel.** A cancelled `import_*_by_key` no longer poisons the channel's import gate permanently.
- **`search_batch_ops` matches multi-word queries.** `"create frame"`, `"auto layout"` now match via AND-over-tokens instead of single contiguous substring.
- **Node-target batch ops accept target in `params`.** A plural `params.nodeIds` is hoisted to the op level when the op-level field is empty.
- Various: `batch` dry-run semantic guards, resolved-ref revalidation, ID normalization, import key fail-fast, `map` validation tightening.

## [1.0.3] — 2026-06-13

### Fixed

- **Hung library import no longer wedges the plugin thread.** All four `import*ByKeyAsync` calls are now wrapped in `withImportTimeout` (15 s `Promise.race`), so a never-resolving import fails fast instead of occupying the plugin thread until the 120 s server ceiling.

## [1.0.2] — 2026-06-12

### Changed

- **README** — restructured Known Limitations; trimmed prose.
- **figma-go skill** — probe rule calibrated; removed private-skill cross-references; clear role separation between tool mechanics and composition rules.

## [1.0.1] — 2026-06-12

### Fixed

- **Windows cross-compile.** `appendSpillManifest` used `syscall.Flock` (Unix-only); split into `spill_lock_unix.go` / `spill_lock_windows.go` so all six release targets build.

## [1.0.0] — 2026-06-11

Initial release as figma-mcp-express, forked from [vkhanhqui/figma-mcp-go](https://github.com/vkhanhqui/figma-mcp-go).

### Added

- **Multi-channel routing** — connect N Figma files simultaneously; each gets its own channel id.
- **Multi-page support** — navigate across all pages without re-running the plugin.
- **`batch` tool** — chain N typed ops in one plugin round-trip with `$N.field` ref resolution.
- **`map` batch op** — apply different values per node in one round-trip via `$item`/`$index` bindings.
- **Bulk-apply** — 8 setters accept `nodeIds[]` in `batch`; one bad id never aborts the rest.
- **Concurrent agent queue** — serializes calls per channel safely under parallel multi-agent load.
- **Server read-cache** — per-channel 3 s TTL + singleflight; any write invalidates the channel cache.
- **Response spill-to-disk** — oversized responses spill to `.figma-mcp-cache/` with NDJSON sidecars and a provenance manifest.
- **Library automation** — `import_component_by_key`, `import_variable_by_key`, `import_style_by_key`, `fetch_library_catalog`, `list_library_variable_collections`, `get_library_variables`.
- **Codegen context** — `get_design_context detail:"codegen"` returns token names, auto-layout spec, and component refs.
- **`get_nodes_info`** — bulk metadata fetch for N node ids in one round-trip.
- **`scan_nodes_by_types` / `scan_text_nodes`** — native C++ traversal, faster than recursive `get_node` walks.
- **Depth-limited traversal** — `get_node` accepts `depth`; truncated nodes surface `childCount`.
- **`per-op skipInvisibleInstanceChildren`** — skip hidden instance subtrees for faster scans.
- **`import_image imagePath`** — pass a local file path; server reads + base64-encodes it.
- **`--version` flag** — prints the `git describe` stamp so clients can confirm they launched the right binary.
- **Queue visibility** — `queueWaitMs` / `queueDepth` surface in responses so agents know they waited.
- **Claude Code + Codex plugin** — `.claude-plugin/` and `.codex-plugin/` manifests; two portable skills.
- **Minimizable plugin window** — collapses to a pill; expands on click.
- **npm cross-platform distribution** — prebuilt binaries for darwin/linux/windows × amd64/arm64.
- **MCP Registry publishing** — `release.yml` publishes to npm and the MCP Registry on tag push.

### Changed

- **Batch collapses dependent edit sequences** — create → style → verify in one round-trip instead of one tool call per step.
- **Leader/follower takeover** — restarting the MCP client no longer forces the Figma plugin to reconnect.
- **Timeouts are server-managed** — `FIGMA_MCP_READ_TIMEOUT` (600 s) for heavy reads, 120 s for light ops; inactivity-based, never trips on a progressing read.
- **`reparent_nodes` preserves canvas position by default.**
- **Non-solid paints surfaced in serialization** — image/gradient paints no longer silently dropped.
- **`swap_component` uses Figma's override-preserving swap path.**
