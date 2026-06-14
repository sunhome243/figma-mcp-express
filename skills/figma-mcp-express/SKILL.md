---
name: figma-mcp-express
description: Use when calling any figma-mcp-express MCP tool (mcp__figma-mcp-express__*) — reading, writing, building screens, library imports, audits, or token binding. Load before the first tool call.
---

## SETUP

Before the first tool call:
1. Figma Desktop must be open with the plugin running: **Plugins → Development → Figma MCP Express**
2. `get_metadata` → confirm which file and page you're on
3. Multiple files open? `list_channels` → pass `channel: "auto-N"` on every tool call that targets a specific file

If any call returns "plugin not connected": open the file in Figma Desktop, run the plugin. Do not retry in a loop.

**Surface:** 70 live tools + 16 demoted (batch-only). Demoted ops are NOT on `tools/list` — invoke them only as a `batch` op `type`. See `references/batch-recipes.md § Part 2` for their full param specs.

**Timeouts are server-managed.** There is NO client timeout param. A timeout means **re-scope narrower** (smaller frame / lower depth), never "raise a timeout." Tiers: heavy reads + `batch` → `FIGMA_MCP_READ_TIMEOUT` (default 600s); all other ops → 120s.

---

## READ TOOL DECISION TREE

```
What do I have / want?
│
├─ Nothing yet — orient on the file .............. get_metadata   (file/pages/current + id map)
│     └─ just the page list ...................... get_pages
│
├─ A NAME to find ................................ search_nodes   (nodeId=<frame> + types + limit)
│
├─ ALL nodes of a TYPE under a frame ............. scan_nodes_by_types
│     └─ only TEXT .............................. scan_text_nodes
│
├─ A known node ID, want full detail ............. get_node       (one id, depth-aware)
│     └─ 2+ known IDs ........................... get_nodes_info  (one round-trip — NOT a loop)
│
├─ Token-efficient tree of current selection ..... get_design_context
│     └─ for code generation .................... get_design_context detail:"codegen"
│
└─ A subscribed library catalog (no file open) ... fetch_library_catalog (REST + FIGMA_TOKEN)
                                                    list_library_variable_collections → get_library_variables
```

**Common wrong picks:**

| If you reach for… | but you actually have/want… | use instead |
|---|---|---|
| `get_node` | several ids | `get_nodes_info` |
| `get_node` | no id yet (name/type) | `search_nodes` / `scan_nodes_by_types` |
| `get_document` | token efficiency | `get_design_context` |
| `get_design_context` | a specific known node | `get_node` |

**`get_node` depth cap = 50.** A node with `childCount` but no `children` was truncated — request a larger `depth`. `childCount: 0` = real leaf.

**Read cache:** reads may return a cached result (~3s TTL). Any write on the channel auto-invalidates that channel's cached reads. After an external Figma edit, expect up-to-3s staleness. For a must-be-live read, that window applies — pair with `verify-before-mutate` discipline.

---

## VALIDATE-AFTER-WRITE MATRIX

Don't screenshot to find out whether a write worked. Read it back to prove it did. Screenshot is the final visual pass only.

| After this… | Validate with | Assert | NOT this |
|---|---|---|---|
| `create_*` | `get_node depth:2` (or trailing op in same batch) | id exists, parent, name, size | screenshot to "see if it appeared" |
| `set_auto_layout` | `get_node depth:1` → `layout.*` fields | layoutMode, itemSpacing, padding | re-reading the whole frame |
| `set_fills` / token bind | `get_node depth:0` → `boundVariables.fills` | variableId present, not raw color | screenshot for fill color check |
| `set_instance_properties` | `get_nodes_info` | property values updated | assuming it worked |
| Whole section complete | `save_screenshots([wrapperId])` at `maxDimension: 2048` | visual alignment, overflow, theme | default 1024 (hides defects on 1440+ canvas) |

A trailing `get_node $0.id` in the same `batch` is free — use it.

---

## WORKFLOW 1 — Read a file or frame

**Pattern: wide-shallow → targeted-deep**

```
get_pages                                        → find the page ID
search_nodes(nodeId=<pageId>, types=["FRAME"])   → find frame by name
get_node(nodeId=<frameId>, depth=2)              → get structure
```

Rules:
- Never call `get_node` on a page ID — result is the full page tree (too large, will spill)
- `depth: 2` for structural overview, `depth: 0` for metadata on a single node
- If the response spills to `.figma-mcp-cache/`: use `grep`/`jq` on the `.ndjson` sidecar
- `get_nodes_info` is faster than `get_node(depth:0)` for metadata-only reads on many nodes

**Optimization rules:**
- `skipInvisibleInstanceChildren: true` — 30–60% traversal reduction on the traversal reads (`get_node`, `get_nodes_info`, `get_document`, `get_design_context`, `search_nodes`, `scan_*`, `get_styles`); turn OFF when hidden state / full component anatomy matters. It's **per-call** (reset every op), so mix freely — including per-op inside a `batch`.
- `scan_nodes_by_types` over manual tree walks — uses native C++ traversal
- `search_nodes` with `limit: 20` — never traverse the whole page to find one frame
- Cache page IDs at session start — one `get_pages` call, reuse for all subsequent reads

---

## WORKFLOW 2 — Build a new frame

**Pattern: probe → scaffold → populate → bind tokens → verify per region**

```
Step 0: Probe     — create ONE test instance → get_node(depth:6) → save_screenshots → delete_nodes
Step 1: Scaffold  — create_frame (wrapper + section shells)
Step 2: Populate  — import_component_by_key → create_instance → set_instance_properties
Step 3: Bind      — import_variable_by_key → set_fills(variableId) / bind_variable_to_node
Step 4: Verify    — after each region: save_screenshots of the FULL wrapper
```

**Probe-first (mandatory for any unfamiliar component):**
```
import_component_by_key(key)                     → confirm key resolves; returns the component id
create_instance(componentId=<that id>, parentId=pageRoot)  → test instance, NOT inside real frame
get_node(testInstanceId, depth=6)                → map text node ids, heights, slot names
save_screenshots([testInstanceId])               → visual confirmation
delete_nodes([testInstanceId])                   → clean up
```
Skip this for organisms → broken instances that must be deleted and rebuilt.

**Critical ordering rule:** `appendChild` MUST come before `layoutSizingHorizontal/Vertical = FILL`. Setting FILL on a parentless node is silently ignored.

**Retrofit FILL on existing nodes:** `resize_nodes(nodeIds=[...], layoutSizingHorizontal="FILL")` — no need to recreate.

**Batch pattern (create → style → verify in 1 round-trip):**
```json
{
  "ops": [
    { "type": "create_frame",  "params": { "name": "Card", "width": 320, "height": 200 } },
    { "type": "set_fills",     "nodeIds": ["$0.id"], "params": { "variableId": "VariableID:colors/surface" } },
    { "type": "get_node",      "nodeIds": ["$0.id"], "params": { "depth": 2 } }
  ]
}
```

**Build rules:**
- **Design composition rules live in the `figma-design-patterns` skill — load it BEFORE building.** This workflow is *tool mechanics*; auto-layout discipline, component-first, padding ownership, FILL/HUG/FIXED, semantic names, and the handoff checklist are there.
- **Repeating pattern → ONE local component, reuse instances.** If a unit repeats (card, stat cell, list row), build it once, `create_component`, then `create_instance` for each occurrence + `set_instance_properties` for per-instance content. A flat pile of absolute-positioned raw rectangles + text is the #1 naive build — it screenshots fine but a designer can't edit it. Group each repeating unit in ONE frame/component so it can be pulled out and fixed in isolation.
- After each logical region: `save_screenshots([wrapperId])` of the FULL wrapper, not just the new section
- `itemSpacing > 48` on a frame with no FILL children → switch children to FILL + retrofit with `resize_nodes`
- One `batch` per logical section — one giant batch blocks the plugin
- Import spacing variables once per session, reuse resolved IDs
- Never raw `paddingLeft = 24` — always bind to a spacing variable
- Call `list_channels` every 15–20 write calls to detect silent channel drops

**`create_text` param contract** (discrete-tool names, NOT Plugin-API names — the server now rejects the wrong ones loudly instead of silently making empty nodes):
- `text` (not `characters`), `fillColor` hex string (not `fills:[{color}]`), `lineHeightValue` + `lineHeightUnit` (not `lineHeight:{value,unit}`).
- **No `width` param** — for a wrap width, `create_text(textAutoResize:"HEIGHT")` then a paired `resize_nodes(width=…)` (HEIGHT grows vertically; fixed width wraps).
- **`set_text` restyles, not just retypes.** `text` is OPTIONAL on `set_text` — it (and `create_text`) also take `textAlignHorizontal`/`textAlignVertical`, `textAutoResize`, `letterSpacingValue`+`Unit`, `lineHeightValue`+`Unit`, `textCase`, `textDecoration`, and the font trio `fontSize`/`fontFamily`/`fontStyle` (auto-loads the font). Set alignment with `textAlignHorizontal:"CENTER"` — never fake-center with leading spaces. Full param list + wrap recipe: `references/gotchas.md § set_text restyles`.

**`import_image` — pass `imagePath`, don't inline base64.** `import_image(imagePath="/abs/path/logo.png", parentId=frame, scaleMode="FIT", width=…, height=…)` — the server reads + encodes the local file. Reserve `imageData` (raw base64) for an in-memory image only.

---

## WORKFLOW 3 — Library component

```
1. Catalog  — get_local_components(pageId) or fetch_library_catalog → save to disk
2. Import   — import_component_by_key(variantKey) → returns the component id
3. Place    — create_instance(componentId, parentId)   # componentKey is an optional import-fallback
4. Configure — set_instance_properties(nodeId, { "Variant": "Primary", "State": "Default" })
5. Content  — set_text(nodeId, "Real content") — never leave defaults ("Heading", "Item 1")
6. Mode     — set_variable_mode on the TOP-LEVEL wrapper (not per-child)
7. Verify   — save_screenshots → confirm before continuing
```

**Import rules:**
- `import_component_by_key` rejects a COMPONENT_SET key → use the default variant's own key
- A bad import key just head-of-line-delays the channel queue until its inactivity timeout (bounded — the queue drains on its own, no reopen). Validate the key first; don't loop-retry (each try queues another timeout window).
- After `create_instance()`: always configure with real content. Default stubs = FAIL.

**Configure rules:**
- `set_instance_properties(nodeId, { resetOverrides:true, properties:{…} })` — `resetOverrides:true` restores component defaults *before* applying `properties` (pass it alone to reset only). Never blank an instance fill to "reset" — that *adds* an override.
- Status/variant color lives in the variant: `properties:{ "Variant":"Success" }`, let tokens drive text/bg color. Never hand-write a fill on an instance's internal layer.

**Cataloging a subscribed library** (step 1, REST-first — works with NO file open and is immune to the backgrounded-plugin throttle):
1. `fetch_library_catalog(fileKey, scope:"all", outPath)` — REST via `FIGMA_TOKEN`, writes components/sets/styles to disk. Each entry carries `key` + `node_id` + `containing_frame` + a pre-rendered `thumbnail_url`.
2. On a `404` (library not published to REST — e.g. a documentation library) → fall back to `get_local_components(pageId)` per page (enumerates COMPONENT/COMPONENT_SET `key`+`id`+`name`; survives heavy pages; spills large output to disk).
3. On a `403` for variables (Variables REST is gated) → from any open file that **subscribes** the library: `list_library_variable_collections` → `get_library_variables(key)` returns every variable's `key`+`name`+`resolvedType` for a subscribed-but-not-open library (hex `valuesByMode` is stripped — enough to BIND; resolve exact hex by opening the lib + `get_variable_defs`).

**Subscribed library — USING a component vs DISCOVERING its keys (two different problems):**

- **USE (you already have the key):** `import_component_by_key(key)` returns the component's `id` → `create_instance(componentId=<that id>, parentId=…)`. (`create_instance` *requires* `componentId`; `componentKey` is an optional fallback that the handler auto-imports only when `componentId` doesn't resolve — in a `batch`, thread the import's `$N.id` into `componentId`. For a COMPONENT_SET key pass `assetType:"COMPONENT_SET"` to import.) The library file does **NOT** need to be open — the Plugin API pulls the published component into the current file by key alone, as long as the user's Figma account has that library enabled. Token-free.
- **DISCOVER (you don't have the keys yet)** — the hard part. You can't import a key you don't have. Two routes:
  - **Route A — REST catalog, nothing opened:** the user gives the **library file's Figma URL**; extract the fileKey (path segment after `/design/`) → `fetch_library_catalog(fileKey, outPath)` dumps the full component/style catalog (names + keys) to disk, no file open, no plugin. **Route A only works for a PUBLISHED team library.** A **community file** (duplicated into the user's drafts, not published) returns an **empty** catalog — counts all `0`, not a 403/404. Empty counts = "not a published library" → this is the documented fallback case → use Route B. (A 403/404 — `FIGMA_TOKEN` can't read the file at all — is a separate failure that also falls back to Route B.)
  - **Route B — open once, enumerate, close:** the user opens the library file in Figma Desktop **and runs the plugin in it** (creating its own channel) → enumerate its component keys, then cache them (the kit can be closed and reused by key forever). **Scope to the icon page — do NOT scan the whole file** (icon kits are huge; an all-pages scan times out / jams the plugin):
    1. `get_pages(channel=<kit channel>)` → find the page that holds the icons (by name, e.g. a page named for the icon family/weights).
    2. `get_local_components(channel=<kit channel>, pageId=<that page id>)` → enumerates COMPONENT/COMPONENT_SET `key`+`id`+`name` for that page only (large output spills to disk → query with jq).
    3. If the icon family spans several pages, repeat step 2 per page — one scoped call each — never omit `pageId` to "get everything at once."
- **Do NOT use `list_library_variable_collections` to enumerate components** — it returns only VARIABLE collections (empty for an icon kit that ships no variables). An empty result does **not** mean "library not connected," and it is not a component enumerator.

**Component priority:** INSTANCE (library) > local COMPONENT > raw structural FRAME.

---

## WORKFLOW 4 — Bulk operations

### Same param to N nodes: `[*]` projection

When the **same operation + same params** applies to many nodes, fan-in scan results directly into a bulk setter. One round-trip.

```json
{
  "ops": [
    { "type": "scan_nodes_by_types", "params": { "nodeId": "<frame-id>", "types": ["INSTANCE"] } },
    { "type": "swap_component", "nodeIds": ["$0.matchingNodes[*].id"], "params": { "componentId": "<target-id>" } }
  ],
  "continueOnError": true
}
```

Return-field names: `search_nodes`→`nodes`, `scan_nodes_by_types`→`matchingNodes`, `scan_text_nodes`→`textNodes`.

All 8 bulk setters accept `nodeIds[]` and return `{ results:[{nodeId,…}|{nodeId,error}] }` (partial success — one bad id never aborts the rest):
`set_fills` · `set_strokes` · `set_opacity` · `set_visible` · `swap_component` · `set_instance_properties` · `bind_variable_to_node` · `set_corner_radius`

`set_visible` and `set_opacity` are ALSO live top-level tools (not demoted). Use them directly for simple single calls; use projection/batch for N nodes.

`set_fills` defaults to replacing a node's fills. Pass `mode:"append"` to stack a new fill on top of the existing ones instead.

Always set `continueOnError: true` when applying to a scanned list — not every node may support the operation. Check `results[i].error` per node.

### Different param per node: `map`

When params **vary per node**, use `map`. See `references/batch-recipes.md § Recipe 5` for the full recipe and `$item`/`$index` binding spec. Cap: 500 items.

Cross-link: projection (same params) → Recipe 4; map (varying params) → Recipe 5.

---

## WORKFLOW 5 — Multi-file / parallel agents

```
list_channels → [
  { channel: "auto-1", fileName: "UI Kit",      fileKey: "ABC123" },
  { channel: "auto-2", fileName: "Product App", fileKey: "XYZ789" }
]

get_local_components(channel: "auto-1")      → catalog the library
create_instance(channel: "auto-2", ...)      → build in the product file
```

**Rules:**
- Agents CAN share a channel — parallel agents working on different frames of the same file is fine
- A channel = one Figma file; use a separate channel only when targeting a different file
- Writes on the same channel serialize (single-threaded plugin per channel) — calls queue, all complete
- Don't expect parallel speedup from concurrent writes on the same channel
- Identical concurrent reads on the same channel are deduplicated by read singleflight
- For multi-file work: pass `channel: "auto-N"` explicitly on every tool call

**Safe multi-agent pattern (3 rules):**
1. **Partition by region** — one agent per screen/section; non-overlapping write sets = zero semantic conflict.
2. **Coordinator creates shared resources once upfront** (page scaffold, nav, header, imported variables) before fan-out; agents create only their own content — no existence ambiguity, no verify-before-create in the hot path.
3. **No lock** — partition + coordinator-shared-once removes the need; the per-channel queue makes concurrent calls safe anyway. Fan out live calls freely — they queue safely (no corruption) and pipeline *faster* than sequential calls (no LLM round-trip between each). Agents also read the on-disk cache in parallel (no plugin). For *overlapping* execution, use separate channels.

**verify-before-create:** a cache miss ≠ "doesn't exist." Before any create-if-absent decision, do one bounded live `search_nodes` to confirm. Rare when the coordinator owns shared resources.

Deep spec (concurrency model, conflict modes, failure handling, agent prompt template): `references/multi-agent.md`

---

## WORKFLOW 6 — Audit / scan

```
get_pages
scan_nodes_by_types(types=["TEXT"])       → typography audit
scan_nodes_by_types(types=["INSTANCE"])   → component usage
scan_nodes_by_types(types=["RECTANGLE"]) → potential hardcoded fills
export_tokens                             → variable/token definitions
```

Audit + bulk-fix loop:
```json
{
  "ops": [
    { "type": "scan_nodes_by_types", "params": { "nodeId": "<frame-id>", "types": ["RECTANGLE"] } },
    { "type": "bind_variable_to_node", "nodeIds": ["$0.matchingNodes[*].id"],
      "params": { "variableId": "<surface-var-id>", "field": "fillColor" } }
  ],
  "continueOnError": true
}
```

- Results spill automatically for large files — pipe `.ndjson` through `grep`/`jq`
- `export_tokens` → `grep -v '"variableId"'` surfaces hardcoded values
- `get_design_context(detail:"codegen")` — gets `tokens`, `componentRef`, `autoLayout` per node

---

## OPTIMIZATION RULES

| Rule | Why |
|---|---|
| `batch` for dependent sequences | N round-trips → 1 |
| Cache page IDs at session start | Eliminates repeated `get_pages` |
| `depth: 2` not `depth: -1` | Avoids megabyte responses |
| `skipInvisibleInstanceChildren: true` | 30–60% traversal reduction |
| `scan_nodes_by_types` not manual walk | Native C++ vs JS loop |
| `save_screenshots` not `get_screenshot` | Disk write vs base64 context flood |
| `grep .ndjson` not read full `.json` | Constant memory vs parse megabytes |
| Import variables once, reuse IDs | One import per session per variable |
| One `batch` per section | Fewer plugin calls total |
| Do NOT retry imports in a loop | Jams the single-threaded plugin thread |

---

## ERROR HANDLING

| Symptom | Cause | Fix |
|---|---|---|
| "plugin not connected" | Plugin not running | Open file, run plugin — do NOT loop |
| Timeout / no response | Read unbounded (page-level, no scope) | Re-scope: frame nodeId + `depth: 2`; never raise a timeout |
| FILL sizing has no effect | Node set FILL before `appendChild` | `appendChild` first, then set FILL |
| `import_component_by_key` "not found" | COMPONENT_SET key, not variant key | Use the default variant's own key |
| Node ID not found | Hyphen format instead of colon | `94-78539` → `94:78539` |
| Response at `.figma-mcp-cache/` path | Response > 25KB gating threshold | Query `.ndjson` sidecar with grep/jq |
| Wrong file targeted (multi-file) | Missing `channel:` param | `list_channels` → add `channel: "auto-N"` |
| Import slow; calls queue behind it | Bad key has no progress ticks → head-of-line delay until its inactivity timeout (bounded; self-clears) | Validate the key first; don't loop-retry; no reopen needed |
| Dark mode doesn't cascade to children | Mode set on child, not wrapper | `set_variable_mode` on TOP-LEVEL wrapper |
| `set_text` fails on locked font | Font unavailable in sandbox | Use fallback font; flag in build output |
| "connection closed" / "channel disconnected" | WebSocket drop during in-flight call | Plugin auto-reconnects; wait a few seconds, retry once — see `references/gotchas.md § Connection drop` |

**References:** `references/gotchas.md` — deep failure mode analysis. `references/batch-recipes.md` — canonical batch chains, `map`, `[*]` projection, and demoted op specs.

---

## WHAT THIS SERVER CANNOT DO

- Create new Figma files (requires REST — use official Figma MCP `create_new_file`)
- Full plugin read/write to files not open in Figma Desktop (REST path `fetch_library_catalog` + `get_library_variables` can enumerate published assets from any file with viewer access + `FIGMA_TOKEN`)
- FigJam, Figma Slides
- Arbitrary script eval (`eval` / `new Function` — plugin sandbox forbids it; no `use_figma` equivalent)
- Publish components or styles to a library
- `figma.teamLibrary` API on free plan
