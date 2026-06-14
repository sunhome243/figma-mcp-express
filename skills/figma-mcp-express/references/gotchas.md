# gotchas.md — figma-mcp-express failure modes

Deep reference. Load when SKILL.md's error table doesn't cover the symptom.
Each entry: **symptom → cause → fix** (prevention folded into the fix).

---

## Slow import delays the queue (bounded, self-clears — not a jam)

**Symptom:** a call sits "in progress" a long time while `save_screenshots` / `list_channels` still respond.
**Cause:** `import_component_by_key` with an invalid/unpublished key has no progress ticks, so it holds the channel's serial queue slot until its inactivity timeout fires (~120s light / ~600s heavy); calls queued behind it wait that long, then the queue drains on its own. NOT a permanent jam — the queue + inactivity-timeout clear it (concurrent calls are otherwise safe and pipeline normally).
**Fix:** validate the key via `get_local_components`/`fetch_library_catalog` BEFORE importing; don't loop-retry (each try queues another timeout window). Calls behind it complete once it clears — no reopen needed (reopen only if the WebSocket actually dropped — a different "connection closed" error).

---

## COMPONENT_SET vs COMPONENT key

**Symptom:** `import_component_by_key` returns "not found" for a seemingly-correct key.
**Cause:** Figma URLs expose the COMPONENT_SET key; import needs a COMPONENT (variant) key.
**Fix:** `get_local_components(pageId)` → find the set → use a child variant's `key`. Store `assetType` with every cataloged key; never pass a SET key to import directly (or pass `assetType:"COMPONENT_SET"`).

---

## FILL sizing silently ignored

**Symptom:** `layoutSizingHorizontal:"FILL"` set, node still renders at content size, no error.
**Cause:** FILL only applies once the node is a child of an auto-layout parent; on a parentless node it's accepted and ignored.
**Fix:** append to the auto-layout parent FIRST, then set sizing — use `batch` with `$N.id` refs so order is guaranteed. Existing already-parented node: `resize_nodes(nodeIds, layoutSizingHorizontal:"FILL")`.

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
**Fix:** use its params — `textAlignHorizontal/Vertical`, `textAutoResize`, `letterSpacing*`, `lineHeight*`, `textCase`, `textDecoration`, `fontSize/Family/Style` (auto-loads). Wrap/grow: `textAutoResize:"HEIGHT"` + `resize_nodes(width)`. Center: `textAlignHorizontal:"CENTER"` — never fake with spaces.

---

## Reset an instance to defaults — `resetOverrides:true`, not blank fills

**Symptom:** clearing overrides by setting an empty/transparent fill — which ADDS an override instead.
**Cause:** blanking a property is itself an override layered on top, not a reset.
**Fix:** `set_instance_properties(nodeId, {resetOverrides:true, properties:{…}})` resets to defaults before applying `properties` (omit `properties` to reset only). Status/variant color lives in the variant — set `properties:{"Variant":"Success"}`, never hand-write a fill.

---

## Remote (library) variable collection — `get_remote_variable_collection`

**Symptom:** you need a remote collection's mode ids (e.g. to pin dark mode) but local-collection reads miss them.
**Cause:** local reads don't surface remote library collections.
**Fix:** `get_remote_variable_collection(collectionId)` — `collectionId` from a node's `boundVariables[].variableCollectionId` — returns the modes to pass to `set_variable_mode`.

---

## Community UI kit components are unpublished — probe, don't trust REST

**Symptom:** `import_component_by_key` on a community kit returns `Cannot import component … since it is unpublished`; `fetch_library_catalog` returns `components: 0` / REST `404`.
**Cause:** "Published to **Community**" (file is duplicatable) ≠ "published as a **library**" (components importable by key). Community kits are only the former → **unpublished local components**, and the Plugin API has no cross-file copy (the multi-channel server moves *data*, not component *links*). Two states only: published/enabled library → linked instances; else → detached copies.
**REST is NOT the arbiter — the live probe is.** REST `404`/`components:0` reflects token access + REST-publish state, not the import path. Proven: after the user published a `shadcn (Copy)` as a library, `fetch_library_catalog` *still* returned 404, yet its `get_local_components` keys imported into another file as real `remote:true` instances (publishing doesn't change keys). Material/iOS kits failed only because never published.
**Fix:** catalog keys via `get_local_components` (any publish state) → ONE live `import_component_by_key` probe into the *target* channel decides it. `remote:true` → `create_instance`. `unpublished` → fall back: **Publish as library** (Assets panel, paid) for real links, or one-time Cmd+C/V into the target (detached local copies). Never loop retries on an unpublished key (jams the plugin).

---

## Per-instance icons/content where instances share one master → edit master + nested-swap

**Constraint:** you can't add/remove/reparent a child *inside* an INSTANCE (or a frame nested in one) — only override existing children (text, fills, visibility, instance-swap). To give N instances of one master *different* nested icons:

1. **Edit the MASTER once:** in its target sub-frame, remove the placeholder and `create_instance` a default icon → propagates to all instances as `I<instance>;<masterIconId>`.
2. **Differ per instance:** `swap_component` the nested path (e.g. `I3:244;13:49`) — works through the MCP (instance-swap override). Leave one on default, swap the rest.
3. **Recolor LAST** (after swaps): `scan_nodes_by_types(<container>, ["VECTOR"])` → bulk `set_strokes` on `["$0.matchingNodes[*].id"]`. Survives swaps, no hand-built paths, covers multi-path icons. **Lucide/shadcn icons are STROKED** → `set_strokes`, not `set_fills`.

**`"Removing this node is not allowed"` (native Figma, not a handler guard):** `node.remove()` is blocked when the layer **backs a component property** (text prop, exposed instance-swap, …) — adding a nested instance to a master can auto-expose one, so even a just-added node may resist deletion; plain (non-property) layers delete fine. **Pick by intent, don't reflexively hide:** to truly delete it, remove the component-property binding first (then it deletes) or `detach_instance` and delete on the plain frame; to suppress it in specific instances, `set_visible:false` is the correct *override* (hidden auto-layout child drops from flow) — but it only HIDES (node persists), never a stand-in for deletion. (`delete_nodes` now reports this per-node with the same hint instead of aborting the batch.)

**`batch` quirks:** `swap_component` takes `nodeIds[]` (not `nodeId`); reads like `scan_nodes_by_types` take `nodeId` **inside `params`**.

**Fallbacks:** nested swap fails → detach the few instances, place icons directly (don't thrash). Multi-file channels reassign their id on every reconnect (node ids stay stable) → re-run `list_channels` before each op group.
