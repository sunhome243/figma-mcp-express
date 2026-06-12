# gotchas.md — figma-mcp-express failure modes

Deep reference. Load when SKILL.md error table doesn't cover the symptom.
Each entry: **symptom → root cause → exact fix → prevention**.

---

## Plugin thread jams

**Symptom:** tool call hangs indefinitely — no timeout error, but `save_screenshots` and `list_channels` still work.

**Root cause:** `import_component_by_key` with an invalid key blocks the single-threaded JS event loop in the plugin. The bridge detects no progress (no yield, no result) and eventually fires the inactivity timer. But if the call is still "in flight", subsequent calls queue behind it.

**Exact fix:**
1. Wait for the server-side inactivity timeout to expire (~600s for heavy reads/batch; ~120s for lightweight ops)
2. Retry ONCE with the correct key (validated via `get_local_components` or `fetch_library_catalog` first)
3. If still hanging after one retry: ask the user to close and reopen the plugin in Figma Desktop

**Prevention:** Before any import, confirm the key via `get_local_components` → find the entry → verify `assetType` field. Never import a key from a Figma URL without validation.

---

## COMPONENT_SET vs COMPONENT key mismatch

**Symptom:** `import_component_by_key` returns "not found" for a key you believe is correct.

**Root cause:** Figma URLs show the COMPONENT_SET key. `import_component_by_key` requires a COMPONENT (variant) key — the key of one specific variant within the set.

**Exact fix:**
```
get_local_components(pageId=<lib-page>)
→ find your component set by name
→ look at the `children` or `variants` array
→ take the `key` of the first child (the default variant)
→ use THAT key with import_component_by_key
```

**Prevention:** Always store `assetType: "COMPONENT"` alongside the key in any catalog. A SET key is `assetType: "COMPONENT_SET"` — never pass it to import directly.

---

## FILL sizing silently ignored

**Symptom:** `layoutSizingHorizontal: "FILL"` set on a node, but it still renders at content size. No error returned.

**Root cause:** FILL sizing is only meaningful when the node is already a child of an auto-layout parent. Setting it on a parentless node is silently accepted and ignored by Figma.

**Exact fix:**
```json
{
  "ops": [
    { "type": "create_frame", "params": { "name": "Parent", "layoutMode": "HORIZONTAL" } },
    { "type": "create_frame", "params": { "name": "Child", "parentId": "$0.id" } },
    { "type": "set_auto_layout", "nodeIds": ["$1.id"], "params": { "primaryAxisSizingMode": "FILL" } }
  ]
}
```
For existing nodes already in a parent: `resize_nodes(nodeIds=[...], layoutSizingHorizontal="FILL")`.

**Prevention:** Always scaffold the parent frame and append children before setting FILL. Use `batch` with `$N.id` refs so ordering is guaranteed.

---

## Node ID format errors

**Symptom:** "node not found" for a node that clearly exists.

**Root cause:** Node IDs from Figma UI URLs use hyphen as separator (`94-78539`). The plugin API requires colon notation (`94:78539`).

**Exact fix:** Replace `-` with `:` in any ID sourced from a Figma URL.

**Prevention:** Always obtain node IDs from `search_nodes`, `get_node`, or `get_metadata` responses — never copy from the browser URL bar.

---

## Spilled response — wrong query approach

**Symptom:** `jq` on a `.json` sidecar returns empty results even though the data is there.

**Root cause:** The `.json` file is the canonical nested payload. The `.ndjson` sidecar is flat — one record per line. If you try to use nested `jq` paths on the NDJSON, or flat grep patterns on the nested JSON, you get nothing.

**Exact fix:**
- Field-level search → `.ndjson`: `grep '"type":"INSTANCE"' .figma-mcp-cache/get_node-abc.ndjson | jq -c '{id,name}'`
- Subtree extraction → `.json`: `jq '.children[] | select(.id=="2:34")' .figma-mcp-cache/get_node-abc.json`

**Prevention:** Always grep `.ndjson` first for simple field lookups. Only open the full `.json` when you need nested subtree access.

---

## Multi-file channel confusion

**Symptom:** A write op modifies the wrong Figma file.

**Root cause:** No `channel:` param on the tool call. The server defaults to the first connected channel, which may not be the intended file.

**Exact fix:** `list_channels` → note which channel ID maps to which file → add `channel: "auto-N"` explicitly to every subsequent call.

**Prevention:** At session start, when more than one file is open, always call `list_channels` and annotate which channel is which before any reads or writes.

---

## Dark mode not cascading

**Symptom:** `set_variable_mode` applied, but child components still show light fills.

**Root cause:** `set_variable_mode` was called on a child node inside the tree, not on the outermost wrapper. Variable mode cascades down from the node it's set on — children above that node in the tree are unaffected.

**Exact fix:** Find the ID of the top-level wrapper frame (the one that contains the entire screen or section), call `set_variable_mode` on that node.

**Prevention:** Mode pinning belongs on the outermost wrapper — set it once and let tokens cascade.

---

## `set_text` fails on locked / unavailable font

**Symptom:** `set_text` call returns an error, or the text silently doesn't change.

**Root cause:** The text style or text node references a font that isn't available in the plugin's JavaScript sandbox — typically a licensed or embedded font (e.g. a brand font not installed in Figma Desktop).

**Exact fix:**
1. Create the text node with an available fallback family (match the script — e.g. a CJK-capable family for CJK text, not a Latin-only font)
2. Call `set_text` to set the characters using the fallback
3. Apply the correct text style via `apply_style_to_node` — the style's visual rendering uses the library's intended font
4. Flag the font issue in your build output

**Prevention:** Call `get_fonts` at session start to see which fonts are available. If a required font is missing, plan the fallback before building text-heavy frames.

---

## Unknown params are REJECTED, not silently dropped — use the tool's own param names

**Symptom:** A tool call returns an error like `create_text: unknown param "characters" — use text` or `set_fills: unknown param "fill" (silently ignored by the plugin) — check the tool schema for the correct name`.

**Root cause:** Earlier the plugin silently ignored any param it didn't recognize, so a Plugin-API field name (`characters`, `fills`, `lineHeight`) produced an empty/default node with no signal. The server now validates every tool's params against its **registered schema** — on the direct path AND inside each `batch` op — and rejects an undeclared key with an actionable hint. The discrete tools use their OWN param names, which are **not** the raw Plugin API names:

| You might reach for (Plugin API) | The discrete tool's param |
| --- | --- |
| `characters` | `text` (create_text) / `text` (set_text) |
| `fills` / `fill` | `fillColor` (hex string, e.g. `#FFFFFF`) |
| `lineHeight` | `lineHeightValue` + `lineHeightUnit` |
| `width` on create_text | none — create with `textAutoResize:"HEIGHT"`, then `resize_nodes` to set the wrap width |

**Exact fix:** Read the error's suggested param name, or check the tool's MCP schema, and resend with the correct key. The allowlist is derived from the live registration, so it always matches the real schema.

**Prevention:** When a build tool is unfamiliar, glance at its MCP schema first; don't assume Plugin-API field names carry over.

---

## Editing text INSIDE an instance — address the inner node with a compound id

**Symptom:** You want to change a label inside a component instance, but `set_text` on the instance id does nothing, or you can't find the text node.

**Root cause:** A component instance's children are not independent nodes — their ids are **compound**: `I{instanceId};{innerNodeId}` (e.g. `I2167:9091;186:1579`). `get_node(instanceId, depth=…)` reveals the inner text node's id, which you then address directly.

**Exact fix:**
1. Prefer `set_instance_properties` when the text is an exposed **component property** (a `Label#…` TEXT property) — that's the component-aware path and survives swaps.
2. When there is no exposed property, `get_node(instanceId, depth=6)` to find the inner text node, then `set_text` on the **compound id** `I{instanceId};{innerNodeId}` (load its font first if it's a locked/brand font — see the font gotcha above).

**Prevention:** Probe an unfamiliar component once (`create_instance` → `get_node depth:6` → note the exposed properties and inner text-node ids) before building N of it.

---

## Connection drop — call resolves as error, not a hang

**Symptom:** A tool call returns an error like "connection closed" or "channel disconnected" instead of hanging.

**Root cause:** When the WebSocket between the Go server and the Figma plugin drops while a request is in-flight, the bridge drains all pending requests on that connection and resolves them with a "connection closed" error immediately — they do not hang waiting for a timeout. The plugin auto-reconnects with exponential backoff. Once the same channel reconnects, only that socket's pending requests are resolved; other channels are unaffected.

**Exact fix:**
1. Check whether the plugin is still visible in Figma Desktop (it may have been closed or reloaded)
2. Wait a few seconds for auto-reconnect — do not retry in a loop
3. If the plugin is gone, reopen it; then retry the call once

**Prevention:** Call `list_channels` every 15–20 write calls during long sessions to confirm the channel is still live before a critical write.

---

## `set_text` restyles, not just retypes — and `text` is optional

**Symptom:** You need to change a text node's alignment, wrap behaviour, or font, and reach for a raw-JS hack (e.g. prepending spaces to fake-center a label) because you think `set_text` only swaps the characters.

**Root cause:** `set_text` carries the full text-style param set, not just `text`. `text` itself is **optional** — pass any styling param alone to restyle in place. (At least one of `text` or a styling param is required.)

**Exact fix:** `set_text` (and `create_text`) accept: `textAlignHorizontal` / `textAlignVertical`, `textAutoResize`, `letterSpacingValue` + `letterSpacingUnit`, `lineHeightValue` + `lineHeightUnit`, `textCase`, `textDecoration`, plus the font trio `fontSize` / `fontFamily` / `fontStyle` (the font auto-loads on change).

- **Word-wrap / multi-line growth:** `set_text(textAutoResize:"HEIGHT")` makes the node grow vertically to fit wrapped lines. Pair with a `resize_nodes(width=…)` to fix the wrap width (fixed width + HEIGHT mode = wrap).
- **Alignment:** set `textAlignHorizontal:"CENTER"` directly — never fake-center with leading spaces.

**Prevention:** Reach for `set_text`'s own styling params before any structural workaround; the spacing/alignment/wrap you want is almost always a single param.

---

## Restoring a component instance to defaults — `resetOverrides:true`, not blanking fills

**Symptom:** You want to clear an instance's overrides back to the component's design-token defaults, and try `set_fills` with an empty/transparent fill — which *adds* an override instead of removing one.

**Root cause:** Blanking a property is itself an override layered on top, not a reset. The discrete-tool path to actually reset is a flag on `set_instance_properties`.

**Exact fix:** `set_instance_properties(nodeId, { resetOverrides: true, properties: { … } })` — `resetOverrides:true` restores the instance to component defaults *before* applying any `properties` you pass in the same call. To reset only, pass `resetOverrides:true` with no `properties`.

- **Status/variant color** lives in the variant, not in a manual fill — set `properties:{ "Variant":"Success" }` and let the token system drive text/bg color. Never hand-write a fill on an instance's internal text/fill layer to change its status color.

**Prevention:** Treat instance fills as token-driven. Configure via variant + `set_instance_properties`; reach for `resetOverrides` to undo, never an empty fill.

---

## Reading a remote (library) variable collection — `get_remote_variable_collection`

**Symptom:** You need a remote/library collection's mode IDs (e.g. to pin dark mode via `set_variable_mode`) but the local-collection read doesn't surface them.

**Root cause:** A local-collections read misses remote library collections. For a remote collection (the common case for cross-library mode switching) you need the dedicated remote read.

**Exact fix:** `get_remote_variable_collection(collectionId)` — the `collectionId` comes from a bound variable's `variableCollectionId` (read it off a node's `boundVariables`). It returns the collection's modes so you can pass the right `modeId` to `set_variable_mode`.

**Prevention:** When mode-pinning against a subscribed library, resolve the collection's modes via the remote read first; don't assume a local-collection list contains it.

---

## Community UI kit components are unpublished — probe, don't trust REST

**Symptom:** `import_component_by_key` on a community kit returns `Cannot import component … since it is unpublished`; `fetch_library_catalog` returns `components: 0` / REST `404`.

**Why:** "Published to **Community**" (file is duplicatable) ≠ "published as a **library**" (components importable by key). Community kits are only the former → their components are **unpublished local components**, and the Plugin API has no cross-file copy (the multi-channel server moves *data*, not component *links*). Two states only: published/enabled library → linked instances; everything else → detached copies.

**REST is NOT the arbiter — the live probe is.** REST `404`/`components:0` reflects token access + REST-publish state, not the `import_component_by_key` path. Proven: after the user published a `shadcn (Copy)` as a library, `fetch_library_catalog` *still* returned 404, yet its `get_local_components` keys imported fine into another file as real `remote:true` instances (publishing doesn't change keys). Material/iOS kits failed only because never published.

**So:** catalog keys with `get_local_components` (any publish state) → ONE live `import_component_by_key` probe into the *target* channel decides it. `remote:true` → `create_instance`. `unpublished` → fall back: **Publish as library** (Assets panel, paid) for real links, or one-time Cmd+C/V into the target (detached local copies). Never loop retries on an unpublished key (jams the plugin).

---

## Per-instance icons/content where instances share one master → edit master + nested-swap

**Constraint:** you can't add/remove/reparent a child *inside* an INSTANCE (or a frame nested in one) — only override existing children (text, fills, visibility, instance-swap). To give N instances of one master *different* nested icons:

1. **Edit the MASTER once:** in its target sub-frame, remove the placeholder and `create_instance` a default icon → propagates to all instances as `I<instance>;<masterIconId>`.
2. **Differ per instance:** `swap_component` the nested path (e.g. `I3:244;13:49`) — works through the MCP (instance-swap override). Leave one on default, swap the rest.
3. **Recolor LAST** (after swaps): `scan_nodes_by_types(<container>, ["VECTOR"])` → bulk `set_strokes` on `["$0.matchingNodes[*].id"]`. Survives swaps, no hand-built paths, covers multi-path icons. **Lucide/shadcn icons are STROKED** → `set_strokes`, not `set_fills`.

**`"Removing this node is not allowed"` (native Figma, not a handler guard):** `node.remove()` is blocked when the layer **backs a component property** (text prop, exposed instance-swap, …) — adding a nested instance to a master can auto-expose one, so even a just-added node may resist deletion; plain layers delete fine. When blocked, **hide** (`set_visible:false`) instead — a hidden auto-layout child drops from flow, same visual result.

**`batch` quirks:** in `batch`, `swap_component` takes `nodeIds[]` (not `nodeId`); reads like `scan_nodes_by_types` take `nodeId` **inside `params`**.

**Fallbacks:** nested swap fails → detach the few instances, place icons directly (don't thrash). Multi-file channels reassign their id on every reconnect (node ids stay stable) → re-run `list_channels` before each op group.
