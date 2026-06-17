# gotchas.md — figma-mcp-express failure modes

Deep reference. Load when SKILL.md's error table doesn't cover the symptom.
Each entry: **symptom → cause → fix** (prevention folded into the fix).

---

## Write tool "not found" / "unknown tool" as a top-level call

**Symptom:** Calling `create_frame`, `set_fills`, `set_auto_layout`, `import_component_by_key`, `create_instance`, etc. as a top-level MCP tool returns "method/tool not found".
**Cause:** The default `core` profile does NOT expose low-level write primitives as top-level tools — they are `batch` op TYPES. (Pre-2.0.0 habit / training-data assumes they're top-level.)
**Fix:** Invoke them inside `batch(ops:[{ "type": "create_frame", "params": {…} }])`. Discover the exact op + params via `search_batch_ops` → `get_batch_op_spec`, validate with `batch(validateOnly:true)`. Only if a legacy client genuinely needs the old top-level surface, set `FIGMA_MCP_TOOL_PROFILE=full`.

## Slow import delays the queue (bounded, self-clears — not a jam)

**Symptom:** a call sits "in progress" a long time while `save_screenshots` / `list_channels` still respond.
**Cause:** malformed/truncated/node-id keys now fail fast in the Go server, but valid-looking unpublished, wrong-library, or permission-blocked keys can still reach the Plugin API and wait for its import timeout; calls queued behind it wait, then the queue drains on its own. NOT a permanent jam — the queue + timeout clear it.
**Fix:** validate the key via `get_local_components`/`fetch_library_catalog` BEFORE importing; pass `assetType:"COMPONENT_SET"` for set keys or fetch the catalog first so the server injects the component-set route hint. Don't loop-retry — each try queues another timeout window. Calls behind it complete once it clears — no reopen needed unless the WebSocket actually dropped.

## save_screenshots can silently return `{succeeded:0,total:0}` — retry once, then fall back to get_screenshot
> ⏳ TRACKED: figma-mcp-express#28 — when the server surfaces a per-item error reason, simplify this.
`save_screenshots` occasionally returns `{succeeded:0,total:0}` with no error and no indication of which node failed (a transient write/contention issue, not a bad request). Do NOT treat `succeeded:0` as a hard failure and do NOT abandon the self-eval. Recover deterministically: **retry the same `save_screenshots` once** after the prior call settles; if it's still `0`, **fall back to `get_screenshot`** (the in-memory base64 export — it's the documented alternative and doesn't touch the filesystem). The visual you need for self-eval/review is still obtainable. Only escalate if BOTH the retry and the `get_screenshot` fallback return nothing — that indicates a real node/selection problem, not the transient zero.

---

## COMPONENT_SET vs COMPONENT key

**Symptom:** `import_component_by_key` returns "not found" for a seemingly-correct key.
**Cause:** The key may be a COMPONENT_SET without a type hint, or the library may be unpublished/not available to the target file.
**Fix:** `get_local_components(pageId)` or `fetch_library_catalog` → confirm the entry type. For sets, pass `assetType:"COMPONENT_SET"` or use a child variant's `key`; cached REST catalog results let the server inject the component-set route hint automatically.

---

## FILL sizing silently ignored

**Symptom:** `layoutSizingHorizontal:"FILL"` set, node still renders at content size, no error.
**Cause:** FILL only applies once the node is a child of an auto-layout parent; on a parentless node it's accepted and ignored.
**Fix:** append to the auto-layout parent FIRST, then set sizing — use `batch` with `$N.id` refs so order is guaranteed. Existing already-parented node: batch op `resize_nodes` with `layoutSizingHorizontal:"FILL"`.

---

## Node ID format

**Symptom:** "node not found" for a node that exists.
**Cause:** URL ids use hyphens (`94-78539`); the API needs colons (`94:78539`).
**Fix:** replace `-`→`:`. Better: source ids from `search_nodes`/`get_node`/`get_metadata`, never the URL bar.

---

## Spilled response — query the right sidecar

**Symptom:** `jq` on a spilled `.json` returns empty though the data's there.
**Cause:** `.json` = nested payload; `.ndjson` = flat one-record-per-line. Nested paths on NDJSON (or flat grep on JSON) match nothing.
**Fix:** field lookup → grep `.ndjson` (`grep '"type":"INSTANCE"' …ndjson | jq -c '{id,name}'`); subtree → `jq` the `.json` (`jq '.children[]|select(.id=="2:34")'`).

---

## Multi-file: wrong file modified

**Symptom:** a write hits the wrong Figma file.
**Cause:** no `channel:` param → the server defaults to the first connected channel.
**Fix:** `list_channels` → map id↔file → pass `channel:"…"` on every call. With >1 file open, do this at session start before any read/write.

---

## Dark mode not cascading

**Symptom:** `set_variable_mode` applied but children still show light fills.
**Cause:** mode cascades DOWN from the node it's set on; it was set on a child, not the wrapper.
**Fix:** set `set_variable_mode` once on the outermost wrapper frame; tokens cascade to all children.

---

## `set_text` fails on a locked/brand font

**Symptom:** `set_text` errors or silently no-ops.
**Cause:** the node/style references a font absent from the sandbox (licensed/embedded brand font).
**Fix:** create with a script-appropriate fallback family (CJK family for CJK, not a Latin-only font) → `set_text` the characters → `apply_style_to_node` (the style renders the intended font) → flag it. Call `get_fonts` at session start to plan fallbacks.

---

## Discrete tools REJECT unknown params — use their OWN names

**Symptom:** errors like `create_text: unknown param "characters" — use text`.
**Cause:** params are validated against each tool's registered schema (direct path AND inside `batch`); discrete tools don't use raw Plugin-API names. (Previously such params were silently dropped → empty/default invisible nodes.)

| Plugin-API name you'd reach for | Discrete tool's param |
| --- | --- |
| `characters` | `text` (create_text / set_text) |
| `fills` / `fill` | `fillColor` (hex, e.g. `#FFFFFF`) |
| `lineHeight` | `lineHeightValue` + `lineHeightUnit` |
| `width` on create_text | none — `textAutoResize:"HEIGHT"` then `resize_nodes` for wrap width |

**Fix:** read the error's suggested name (or the MCP schema) and resend. Glance at an unfamiliar tool's schema before assuming Plugin-API names carry over.

---

## Editing text inside an instance — compound id

**Symptom:** `set_text` on the instance id does nothing / you can't find the text node.
**Cause:** instance children have compound ids `I{instanceId};{innerId}` (e.g. `I2167:9091;186:1579`).
**Fix:** prefer `set_instance_properties` when the text is an exposed `Label#…` property (survives swaps). Else `get_node(instanceId, depth:6)` → `set_text` on the compound id (load its font first if brand/locked). Probe an unfamiliar component once before building N.

---

## Connection drop resolves as an error (not a hang)

**Symptom:** a call returns "connection closed" / "channel disconnected".
**Cause:** the WS dropped in-flight; the bridge drains pending requests with an error immediately (no hang) and the plugin auto-reconnects with backoff. Only that socket's requests are affected.
**Fix:** confirm the plugin is still open in Figma → wait a few seconds for reconnect (don't retry in a loop) → reopen if gone, then retry once. Call `list_channels` every ~15–20 writes on long sessions.

---

## `set_text` restyles too — and `text` is optional

**Symptom:** you hack alignment/wrap (e.g. leading spaces to fake-center) thinking `set_text` only swaps characters.
**Cause:** `set_text` carries the full text-style set; `text` is optional (pass a styling param alone to restyle in place).
**Fix:** use its params — `textAlignHorizontal/Vertical`, `textAutoResize`, `letterSpacing*`, `lineHeight*`, `textCase`, `textDecoration`, `fontSize/Family/Style` (auto-loads). Wrap/grow: `textAutoResize:"HEIGHT"` + batch op `resize_nodes` with width. Center: `textAlignHorizontal:"CENTER"` — never fake with spaces.

---

## Reset an instance to defaults — `resetOverrides:true`, not blank fills

**Symptom:** clearing overrides by setting an empty/transparent fill — which ADDS an override instead.
**Cause:** blanking a property is itself an override layered on top, not a reset.
**Fix:** batch op `set_instance_properties` with `resetOverrides:true` resets to defaults before applying `properties` (omit `properties` to reset only). Status/variant color lives in the variant — set `properties:{"Variant":"Success"}`, never hand-write a fill.

---

## Remote (library) variable collection — `get_remote_variable_collection`

**Symptom:** you need a remote collection's mode ids (e.g. to pin dark mode) but local-collection reads miss them.
**Cause:** local reads don't surface remote library collections.
**Fix:** use the batch op `get_remote_variable_collection` with `collectionId` from a node's `boundVariables[].variableCollectionId`; it returns the modes to pass to `set_variable_mode`.

---

## Community UI kit components are unpublished — probe, don't trust REST

**Symptom:** `import_component_by_key` on a community kit returns `Cannot import component … since it is unpublished`; `fetch_library_catalog` returns `components: 0` / REST `404`.
**Cause:** "Published to **Community**" (file is duplicatable) ≠ "published as a **library**" (components importable by key). Community kits are only the former → **unpublished local components**, and the Plugin API has no cross-file copy (the multi-channel server moves *data*, not component *links*). Two states only: published/enabled library → linked instances; else → detached copies.
**REST is NOT the arbiter — the live probe is.** REST `404`/`components:0` reflects token access + REST-publish state, not the import path. Proven: after the user published a `shadcn (Copy)` as a library, `fetch_library_catalog` *still* returned 404, yet its `get_local_components` keys imported into another file as real `remote:true` instances (publishing doesn't change keys). Material/iOS kits failed only because never published.
**Fix:** catalog keys via `get_local_components` (any publish state) → ONE live `import_component_by_key` probe into the *target* channel decides it. `remote:true` → `create_instance`. `unpublished` → fall back: **Publish as library** (Assets panel, paid) for real links, or one-time Cmd+C/V into the target (detached local copies). Never loop retries on an unpublished key; each attempt can wait for the bounded import timeout.

---

## Per-instance icons/content where instances share one master → edit master + nested-swap

**Constraint:** you can't add/remove/reparent a child *inside* an INSTANCE (or a frame nested in one) — only override existing children (text, fills, visibility, instance-swap). To give N instances of one master *different* nested icons:

1. **Edit the MASTER once:** in its target sub-frame, remove the placeholder and `create_instance` a default icon → propagates to all instances as `I<instance>;<masterIconId>`.
2. **Differ per instance:** `swap_component` the nested path (e.g. `I3:244;13:49`) — works through the MCP (instance-swap override). Leave one on default, swap the rest.
3. **Recolor LAST** (after swaps): `scan_nodes_by_types(<container>, ["VECTOR"])` → bulk `set_strokes` on `["$0.matchingNodes[*].id"]`. Survives swaps, no hand-built paths, covers multi-path icons. **Lucide/shadcn icons are STROKED** → `set_strokes`, not `set_fills`.

**`"Removing this node is not allowed"` (native Figma, not a handler guard):** deletion is blocked when the layer **backs a component property** (text prop, exposed instance-swap, ...) — adding a nested instance to a master can auto-expose one, so even a just-added node may resist deletion; plain non-property layers delete fine. **Pick by intent, don't reflexively hide:** to truly delete it, remove the component-property binding first, or `detach_instance` and delete on the plain frame; to suppress it in specific instances, `set_visible:false` is the correct override. Hidden auto-layout children drop from flow, but hiding only hides; it is not a stand-in for deletion. `delete_nodes` reports this per-node with the same hint instead of aborting the batch.

**`batch` quirks:** `swap_component` takes `nodeIds[]` (not `nodeId`); reads like `scan_nodes_by_types` take `nodeId` **inside `params`**.

**Fallbacks:** nested swap fails → detach the few instances, place icons directly (don't thrash). Multi-file channels reassign their id on every reconnect (node ids stay stable) → re-run `list_channels` before each op group.


## COMPONENT nodes DO support auto-layout
A frequent false belief: "`set_auto_layout` doesn't work on COMPONENT nodes, so I positioned the children absolutely." Wrong — a COMPONENT (and COMPONENT_SET) supports `layoutMode`/auto-layout exactly like a FRAME. Set auto-layout on the component master so its children flow and its instances reflow; never fall back to absolute x/y inside a component because you assumed the op is unsupported. If a layout op seems to fail on a component, check the params (it uses the op's OWN names), don't conclude the node type is the blocker.

## Fill/paint variable bindings are NOT READABLE — do not gate on them
> ⏳ TRACKED: figma-mcp-express#27 — when reads surface fill bindings, REVISE this section to restore binding verification.
No read tool surfaces a fill's variable binding. `get_node`/`get_nodes_info` flatten every fill to a resolved hex (`["#ffffff"]`). `get_design_context` at EVERY detail (compact/full/codegen) serializes fills only as resolved-hex dedup aliases in `globalVars.styles` (`s1=#ffffff`, `s2=#c2410c`, …) — a fill bound to a variable and a hand-typed raw hex of the same value are byte-identical in the output (verified with a positive control: a `set_fills variableId`-bound frame reads identically to an untouched one). Only EFFECT bindings (shadows) emit `boundVariables`; fills never do. Therefore: you CANNOT confirm or deny that a fill is variable-bound by reading, and `globalVars` showing `sN`/hex is NOT evidence of "raw/unbound" — treating it so produces FALSE-NEGATIVE D3 failures. Binding evidence is the WRITE op: `bind_variable_to_node`/`set_fills variableId` return success (bind_variable_to_node echoes the bound `variableId`). The only readable, real D3 fill violation is an OFF-PALETTE hex — a color value not in the project's token spine — which signals a hand-picked non-token color. Palette-matching values are presumed bound (trust the write path); do not fail them.

To bind a fill: `set_fills` with `variableId` (= `setBoundVariableForPaint`) or `bind_variable_to_node` field `fillColor`. Both apply the binding in the file; neither is verifiable by a later read.

---

## Bringing external assets into Figma (verified ingestion recipes)

The orchestrator should pre-fetch assets (brand logos, icons, photos, avatars, Lottie — sourced
keyless from Iconify / SimpleIcons / Picsum / DiceBear / LottieFiles, with a web-search `--url`
fallback) and hand builders **local file paths**. The Figma ingestion path then depends on the
asset type:

### PNG / JPEG → `import_image` batch op (no plugin required)
Use `figma-mcp-express`'s `import_image` batch op with `imagePath` (absolute local path). This is
the direct, no-plugin path for raster images. Hand the builder the file path; it batches the import.

### SVG → `figma.createNodeFromSvg` via `use_figma`
> ⏳ `figma-mcp-express` has NO svg batch op — there is no `import_svg` or `create_vector_from_svg`
> in the `batch` op catalog. SVG ingestion requires the official Figma plugin runtime. Use `use_figma`
> (the plugin code tool) with `figma.createNodeFromSvg(svgString)` inside a plugin script. This is a
> genuine MCP capability gap, not a workaround.

### Lottie .json → NOT MCP-ingestible as animation
> ⏳ Lottie `.json` files cannot be imported as live animations through any MCP op or plugin script.
> Workaround: export the Lottie's static poster frame as a PNG and ingest that with `import_image`.
> Keep the `.json` path in the ledger for the developer handoff note. Do not block a build on this.

**Summary:**

| Asset type | Ingestion method | MCP path |
|---|---|---|
| PNG / JPEG | `batch import_image` with `imagePath` | Direct batch op — no plugin |
| SVG | `use_figma` + `figma.createNodeFromSvg` | Plugin runtime required ⏳ |
| Lottie .json | Import poster PNG + note the .json path | Animation not ingestible ⏳ |
