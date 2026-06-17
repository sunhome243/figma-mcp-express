# gotchas.md — figma-mcp-express failure modes

Remaining failure modes not covered by the structured references.
For permanent Figma Plugin API constraints (instance children, clone IDs, auto-layout children, etc.)
→ read `platform-constraints.md`.
For server bugs with workarounds (find_replace_text scope, get_design_context nodeId, etc.)
→ read `mcp-known-bugs.md`.

---

## Write tool "not found" / "unknown tool" as a top-level call

**Symptom:** Calling `create_frame`, `set_fills`, `import_component_by_key`, etc. as a top-level MCP tool returns "method/tool not found".
**Cause:** The default `core` profile does NOT expose low-level write primitives as top-level tools — they are `batch` op TYPES.
**Fix:** Invoke them inside `batch(ops:[{ "type": "create_frame", "params": {…} }])`. Discover via `search_batch_ops` → `get_batch_op_spec`, validate with `batch(validateOnly:true)`. Only if a legacy client genuinely needs the old surface, set `FIGMA_MCP_TOOL_PROFILE=full`.

---

## "There's no op for X" — discover BEFORE concluding

**Symptom:** An agent declares an op missing or a param broken without querying the catalog, then invents a workaround.
**Cause:** Guessing the op surface from memory instead of asking the server. This is the single most common wasted-loop.
**Fix:** `search_batch_ops("<intent words>")` → `get_batch_op_spec(op:"<name>")` FIRST, every time, before concluding anything is absent. Confirmed-present ops people wrongly assume are missing: **`clone_node`**, **`reparent_nodes`**, and nearly every write primitive.

Two recurring shape mistakes: (1) the **node target goes in the op-level `nodeIds` array**, not `params.nodeId` (singular `params.nodeId` is only a read/scan subtree root); (2) `get_batch_op_spec` takes `op:"<name>"`, not a `nodeId`.

Note: `get_batch_op_spec` may return 0 results for some valid ops (known bug #36) — fall back to `search_batch_ops` + `batch(validateOnly:true)` with a best-guess payload.

---

## Node IDs and placement coordinates — live-resolve, never trust cached values

**Symptom:** "node not found", silent off-canvas placement, or a write hitting the wrong node — even when the ID "looks right" from a brief, summary, or memory.

**Three compounding causes — all fixed by the same discipline:**

1. **Stale IDs and coords from briefs/summaries.** A session summary may record a frame at x≈22000 when it actually lives at x≈1814. **Never trust a node id or x/y from a brief/summary/memory — `search_nodes` on the live channel and read real `absoluteBoundingBox` from `get_node` before touching anything.**

2. **`move_nodes`/placement is parent-relative, NOT absolute canvas coords.** Child x/y are relative to the parent frame's top-left. Feeding a large "absolute" value flings a child off-canvas. To place a new sibling screen: live-read an existing sibling's `absoluteBoundingBox.x` via `get_node`, then offset.

3. **Duplicate same-named frames.** After a fresh rebuild, the old frame stays — creating two frames at the same coords. **After any rebuild: `delete_nodes` the superseded frame, then `search_nodes` to confirm exactly ONE frame with that name remains.**

**Fix:** Before any edit or placement: `search_nodes` by name → `get_node` on the live id → read real `absoluteBoundingBox` → use those values.

---

## Node ID format

**Symptom:** "node not found" for a node that exists.
**Cause:** URL ids use hyphens (`94-78539`); the API needs colons (`94:78539`).
**Fix:** replace `-`→`:`. Better: source ids from `search_nodes`/`get_node`/`get_metadata`, never the URL bar.

---

## Spilled response — query the right sidecar

**Symptom:** `jq` on a spilled `.json` returns empty though the data's there.
**Cause:** `.json` = nested payload; `.ndjson` = flat one-record-per-line.
**Fix:** field lookup → grep `.ndjson` (`grep '"type":"INSTANCE"' …ndjson | jq -c '{id,name}'`); subtree → `jq` the `.json`.

---

## `set_text` fails on a locked/brand font

**Symptom:** `set_text` errors or silently no-ops.
**Cause:** the node/style references a font absent from the sandbox (licensed/embedded brand font).
**Fix:** create with a script-appropriate fallback family → `set_text` the characters → `apply_style_to_node` (the style renders the intended font). Call `get_fonts` at session start to plan fallbacks.

---

## Connection drop resolves as an error (not a hang)

**Symptom:** a call returns "connection closed" / "channel disconnected".
**Cause:** the WS dropped in-flight; the bridge drains pending requests with an error immediately and the plugin auto-reconnects with backoff.
**Fix:** confirm the plugin is still open in Figma → wait a few seconds for reconnect → reopen if gone, then retry once. Call `list_channels` every ~15–20 writes on long sessions.

---

## `set_text` restyles too — and `text` is optional

**Symptom:** you hack alignment (e.g. leading spaces to fake-center) thinking `set_text` only swaps characters.
**Cause:** `set_text` carries the full text-style set; `text` is optional (pass a styling param alone to restyle in place).
**Fix:** use its params — `textAlignHorizontal/Vertical`, `textAutoResize`, `letterSpacing*`, `lineHeight*`, `textCase`, `fontSize/Family/Style`. Center: `textAlignHorizontal:"CENTER"` — never fake with spaces. Wrap/grow: `textAutoResize:"HEIGHT"` + `resize_nodes` with width.

---

## Reset an instance to defaults — `resetOverrides:true`, not blank fills

**Symptom:** clearing overrides by setting an empty/transparent fill — which ADDS an override instead.
**Cause:** blanking a property is itself an override layered on top, not a reset.
**Fix:** `set_instance_properties` with `resetOverrides:true` resets to defaults before applying `properties`.

---

## Discrete tools REJECT unknown params — use their OWN names

**Symptom:** errors like `create_text: unknown param "characters" — use text`.
**Cause:** params are validated against each tool's registered schema; discrete tools don't use raw Plugin-API names.

| Plugin-API name | Discrete tool's param |
|---|---|
| `characters` | `text` |
| `fills` / `fill` | `fillColor` (hex, create_text only) |
| `lineHeight` | `lineHeightValue` + `lineHeightUnit` |
| `width` on create_text | none — `textAutoResize:"HEIGHT"` then `resize_nodes` |

---

## Remote (library) variable collection — `get_remote_variable_collection`

**Symptom:** you need a remote collection's mode ids (e.g. to pin dark mode) but local-collection reads miss them.
**Fix:** batch op `get_remote_variable_collection` with `collectionId` from a node's `boundVariables[].variableCollectionId`; it returns the modes to pass to `set_variable_mode`.

---

## Fill/paint variable bindings are NOT READABLE
> ⏳ TRACKED: figma-mcp-express#27

See `mcp-known-bugs.md` #27 for the full explanation. Short version: `get_node` flattens
every fill to resolved hex — a bound fill and a raw hex of the same value look identical.
The only readable D3 violation is an off-palette hex. Trust the write path; don't fail on
palette-matching values.

---

## Bringing external assets into Figma (verified ingestion recipes)

The orchestrator should pre-fetch assets and hand builders **local file paths**.

| Asset type | Ingestion method | MCP path |
|---|---|---|
| PNG / JPEG | `batch import_image` with `imagePath` | Direct batch op |
| SVG | `use_figma` + `figma.createNodeFromSvg` | Plugin runtime required ⏳ |
| Lottie .json | Import poster PNG + note the .json path | Animation not ingestible ⏳ |

SVG: figma-mcp-express has NO `import_svg` or `create_vector_from_svg` batch op. Requires
official Figma plugin runtime via `use_figma`. This is a genuine MCP capability gap.

Lottie: `.json` files cannot be imported as live animations through any MCP op or plugin script.
Export the static poster frame as PNG. Keep the `.json` path for developer handoff.
