# batch-recipes.md — canonical batch chains + demoted op specs

Load this before composing any `batch` call. Part 1 = ready-to-use recipes (including map and projection bulk). Part 2 = full param spec for all 16 demoted (batch-only) op types.

---

## Part 1 — Canonical recipes

---

### Recipe 1: Create frame → bind token fills → set auto layout → verify

```json
{
  "ops": [
    { "type": "create_frame",    "params": { "name": "Card", "width": 320, "height": 200, "parentId": "<wrapper-id>" } },
    { "type": "set_fills",       "nodeIds": ["$0.id"], "params": { "variableId": "<surface-var-id>" } },
    { "type": "set_auto_layout", "nodeIds": ["$0.id"], "params": { "layoutMode": "VERTICAL", "itemSpacingVariableId": "<gap-var-id>" } },
    { "type": "get_node",        "nodeIds": ["$0.id"], "params": { "depth": 2 } }
  ]
}
```

Stop policy: `$N` refs present → stops at first error. Trailing `get_node` is the inline structural verify — no second round-trip needed.

---

### Recipe 2: Library component — import → place → configure → pin mode

```json
{
  "ops": [
    { "type": "import_component_by_key",  "params": { "key": "<variant-key>" } },
    { "type": "create_instance",          "params": { "componentId": "$0.id", "parentId": "<wrapper-id>" } },
    { "type": "set_instance_properties",  "nodeIds": ["$1.id"], "params": { "properties": { "Variant": "Primary", "State": "Default" } } },
    { "type": "set_variable_mode",        "nodeIds": ["$1.id"], "params": { "collectionId": "<col-id>", "modeId": "<dark-mode-id>" } }
  ]
}
```

`create_instance` *requires* `componentId` — thread the imported component's `$0.id` into it. (`componentKey` is an optional fallback the handler auto-imports only when `componentId` doesn't resolve; with the import op present, use the returned id directly.) Stop policy: every op refs `$N` → stops at first error, so if import fails there's no create attempt.

---

### Recipe 3: Chain read — search → targeted deep read

```json
{
  "ops": [
    { "type": "search_nodes", "params": { "nodeId": "<page-id>", "query": "Dashboard", "types": ["FRAME"], "limit": 5 } },
    { "type": "get_node", "nodeIds": ["$0.nodes.0.id"], "params": { "depth": 3, "skipInvisibleInstanceChildren": true } }
  ]
}
```

Stop policy: `$0.nodes.0.id` ref stops if search returns empty. Confirm result count > 0 before batching, or handle the case where `$0.nodes` is empty.

---

### Recipe 4: `[*]` projection — scan → bulk-apply same param to N nodes

Use this when you want to apply the **same operation** to every node in a scan result — one round-trip, no intermediate list-building.

```json
{
  "ops": [
    { "type": "scan_nodes_by_types", "params": { "nodeId": "<frame-id>", "types": ["INSTANCE"] } },
    { "type": "swap_component", "nodeIds": ["$0.matchingNodes[*].id"], "params": { "componentId": "<plus-component-id>" } }
  ],
  "continueOnError": true
}
```

**Field-name rule** — the return-field that `[*]` scans depends on the source op:

| Source op | Return field | Projection ref |
|---|---|---|
| `search_nodes` | `nodes` | `$0.nodes[*].id` |
| `scan_nodes_by_types` | `matchingNodes` | `$0.matchingNodes[*].id` |
| `scan_text_nodes` | `textNodes` | `$0.textNodes[*].id` |

Only ONE `[*]` wildcard per ref. Sub-field after the wildcard extracts a scalar: `$0.matchingNodes[*].id` fans-in to an array of ID strings.

Works for **all 8 bulk setters**: `set_fills`, `set_strokes`, `set_opacity`, `set_visible`, `swap_component`, `set_instance_properties`, `bind_variable_to_node`, `set_corner_radius`. Use `continueOnError: true` — one bad id must not abort the rest.

**Examples:**

```json
// Bind a token to all text nodes in a frame
{
  "ops": [
    { "type": "scan_text_nodes", "params": { "nodeId": "<frame-id>" } },
    { "type": "bind_variable_to_node", "nodeIds": ["$0.textNodes[*].id"],
      "params": { "variableId": "<color-var-id>", "field": "fillColor" } }
  ],
  "continueOnError": true
}

// Hide all instances of a specific component
{
  "ops": [
    { "type": "search_nodes", "params": { "nodeId": "<page-id>", "query": "Banner", "types": ["INSTANCE"] } },
    { "type": "set_visible", "nodeIds": ["$0.nodes[*].id"], "params": { "visible": false } }
  ],
  "continueOnError": true
}
```

---

### Recipe 5: `map` — per-item-VARYING params in one round-trip

Use `map` when you need **different param values for each node** (e.g. set each node's text to its own name, or apply per-row data). This is the only way to vary params per node without N separate round-trips.

```json
{
  "ops": [
    { "type": "scan_text_nodes", "params": { "nodeId": "<frame-id>" } },
    {
      "type": "map",
      "over": "$0.textNodes[*]",
      "as": "item",
      "do": {
        "type": "set_text",
        "nodeIds": ["$item.id"],
        "params": { "text": "$item.name" }
      }
    }
  ]
}
```

**`map` fields:**

| Field | Required | Description |
|---|---|---|
| `type` | yes | must be `"map"` |
| `over` | yes | ref resolving to an array; use `$N.field[*]` or any ref that yields an array |
| `as` | no | binding name for each element (default: `"item"`) |
| `do` | yes | op template; use `$item`, `$item.path`, `$index` for per-element values |

**Binding variables inside `do`:**
- `$item` — the current element object
- `$item.someField` — a field on the element
- `$index` — zero-based iteration index (name is always `index`, not affected by `as`)

**Limits:** cap 500 items per `map`. Emits progress updates every 10 items. Returns `{ results:[{i,type,data}|{i,type,error}], okCount, failCount }`.

```json
// Renumber headings: "Section 1", "Section 2", …
{
  "ops": [
    { "type": "search_nodes", "params": { "nodeId": "<frame-id>", "query": "heading", "types": ["TEXT"] } },
    {
      "type": "map",
      "over": "$0.nodes[*]",
      "as": "node",
      "do": {
        "type": "set_text",
        "nodeIds": ["$node.id"],
        "params": { "text": "Section $index" }
      }
    }
  ]
}
```

---

### Recipe 6: Probe one instance — test → anatomy → screenshot → delete

```json
{
  "ops": [
    { "type": "import_component_by_key", "params": { "key": "<key>" } },
    { "type": "create_instance",  "params": { "componentId": "$0.id", "parentId": "<page-root-id>" } },
    { "type": "get_node",         "nodeIds": ["$1.id"], "params": { "depth": 6 } },
    { "type": "save_screenshots", "nodeIds": ["$1.id"], "params": { "outputPath": ".figma-mcp-cache/probe-<name>.png" } },
    { "type": "delete_nodes",     "nodeIds": ["$1.id"] }
  ]
}
```

Use before building N instances of any unfamiliar component. `get_node(depth:6)` reveals text node IDs, actual rendered heights, and property names for `set_instance_properties`.

---

### Batch failure semantics — NOT transactional, resend from `failedAt`

Figma has **no rollback.** Ops before a failure stay applied — each op is its own undo step. On a partial failure the result carries `failedAt` and per-op `{i, error}` (plus a `💡` hint).

**Fix only the failing op and resend FROM `failedAt`** — resending the whole batch double-creates everything before the failure. A forward-ref error (`$N` with `N` ≥ the current op index) means your ops are out of order: move the producer op before the consumer.

Stop policy is automatic: any op uses a `$N` ref → dependent chain → stops at first failure (downstream refs would break). No refs → independent bulk → runs all ops, reports each. Override with `continueOnError: true|false`.

Keep one batch to one logical section (a few dozen ops). A batch holds the plugin's single serial slot for its whole run, so an enormous batch blocks every other call and leaves nothing to verify between steps. Build incrementally with batches, not one giant batch.

---

## Part 2 — Demoted op types (batch-only)

These 16 ops are NOT on `tools/list`. Invoke them ONLY as a `batch` op `type`. Their params pass through verbatim to the plugin handler — Go's param-parsing layer is bypassed. Use `continueOnError: true` for bulk demoted ops.

---

### `boolean_operation`

Build custom shapes when the library has no matching component.

```json
{
  "type": "boolean_operation",
  "nodeIds": ["<id-1>", "<id-2>"],
  "params": {
    "operation": "UNION",
    "name": "Diamond",
    "parentId": "<target-parent-id>"
  }
}
```

| `operation` | Effect |
|---|---|
| `UNION` | Merge all nodes into one combined shape |
| `SUBTRACT` | Subtract second node from first |
| `INTERSECT` | Keep only the overlapping area |
| `EXCLUDE` | Keep non-overlapping area |
| `FLATTEN` | Rasterize 1+ nodes to a single vector path |

Result lands in `firstNode.parent` unless `parentId` is passed. Use only when the shape genuinely doesn't exist in the library.

---

### `ungroup_nodes`

```json
{ "type": "ungroup_nodes", "nodeIds": ["<group-id>"] }
```

Ungroups GROUP nodes, moving their children to the parent and removing the group. No params beyond `nodeIds`.

---

### `detach_instance`

```json
{ "type": "detach_instance", "nodeIds": ["<instance-id>"] }
```

Converts INSTANCE nodes to plain frames. Library link is broken; visual properties are preserved. No params beyond `nodeIds`.

---

### `rename_node`

```json
{
  "type": "rename_node",
  "nodeIds": ["<id>"],
  "params": { "name": "Icons/Arrow/Left" }
}
```

`name` (required): new node name. Supports slash-separated path notation for component panel organisation. For multiple nodes use `batch_rename_nodes` (a live top-level tool).

---

### `reorder_nodes`

```json
{
  "type": "reorder_nodes",
  "nodeIds": ["<id>"],
  "params": { "order": "bringToFront" }
}
```

`order` (required): `bringToFront` | `sendToBack` | `bringForward` | `sendBackward`

Returns `{ results:[{nodeId, index}|{nodeId,error}] }` (manual-loop shape, not bulkApply).

---

### `rotate_nodes`

```json
{ "type": "rotate_nodes", "nodeIds": ["<id>"], "params": { "rotation": 45 } }
```

`rotation` (required): degrees. **Positive = counter-clockwise** (Figma sets `node.rotation` directly). Returns `{ results:[{nodeId,rotation}|{nodeId,error}] }` (manual-loop shape, not bulkApply).

---

### `set_blend_mode`

```json
{
  "type": "set_blend_mode",
  "nodeIds": ["<id>"],
  "params": { "blendMode": "MULTIPLY" }
}
```

`blendMode` (required): `NORMAL` | `MULTIPLY` | `SCREEN` | `OVERLAY` | `DARKEN` | `LIGHTEN` | `COLOR_DODGE` | `COLOR_BURN` | `HARD_LIGHT` | `SOFT_LIGHT` | `DIFFERENCE` | `EXCLUSION` | `HUE` | `SATURATION` | `COLOR` | `LUMINOSITY` | `PASS_THROUGH`

---

### `set_constraints`

```json
{
  "type": "set_constraints",
  "nodeIds": ["<id>"],
  "params": {
    "horizontal": "STRETCH",
    "vertical": "MIN"
  }
}
```

`horizontal`: `MIN` (left) | `MAX` (right) | `CENTER` | `STRETCH` | `SCALE`
`vertical`: `MIN` (top) | `MAX` (bottom) | `CENTER` | `STRETCH` | `SCALE`

Both params are optional; pass only the axis you want to change.

---

### `lock_nodes` / `unlock_nodes`

```json
{ "type": "lock_nodes",   "nodeIds": ["<id>"] }
{ "type": "unlock_nodes", "nodeIds": ["<id>"] }
```

No params beyond `nodeIds`.

---

### `set_corner_radius`

`set_corner_radius` is a **bulk setter** — it uses `bulkApply` and returns `{ results:[{nodeId,…}|{nodeId,error}] }`.

```json
{ "type": "set_corner_radius", "nodeIds": ["<id>"], "params": { "cornerRadius": 8 } }
```

Per-corner control:
```json
{
  "type": "set_corner_radius",
  "nodeIds": ["<id>"],
  "params": {
    "topLeftRadius": 8,
    "topRightRadius": 8,
    "bottomRightRadius": 0,
    "bottomLeftRadius": 0
  }
}
```

---

### `delete_page`

```json
{
  "type": "delete_page",
  "params": { "pageId": "<page-id>" }
}
```

Pass `pageId` OR `pageName` (not both). Cannot delete the only remaining page.

---

### `rename_page`

```json
{
  "type": "rename_page",
  "params": { "pageId": "<page-id>", "newName": "Screens v2" }
}
```

`newName` (required). Pass `pageId` or `pageName` to identify the target page.

---

### `delete_variable`

```json
{ "type": "delete_variable", "params": { "variableId": "<var-id>" } }
```

Pass `variableId` OR `collectionId` (not both). Passing `collectionId` removes the entire collection and all its variables.

---

### `delete_style`

```json
{ "type": "delete_style", "params": { "styleId": "<style-id>" } }
```

`styleId` (required): the ID of a paint, text, effect, or grid style.

---

### `remove_reactions`

```json
{
  "type": "remove_reactions",
  "nodeIds": ["<id>"],
  "params": { "indices": [0, 2] }
}
```

`indices`: zero-based array of reactions to remove. Omit or pass `[]` to remove all. Use `get_reactions` first to see current indices.

---

### `resize_nodes` (FILL retrofit)

Also a live top-level tool, but commonly used in batch to retrofit FILL/HUG on existing nodes.

```json
{
  "type": "resize_nodes",
  "nodeIds": ["<id>"],
  "params": {
    "layoutSizingHorizontal": "FILL",
    "layoutSizingVertical": "HUG"
  }
}
```

| Param | Effect |
|---|---|
| `layoutSizingHorizontal` / `layoutSizingVertical` | `"FILL"` fills available space · `"HUG"` shrinks to content · `"FIXED"` stays at explicit w/h |
| `layoutGrow` | Grow factor along the parent's main axis (`0` = don't grow, `1` = fill remaining) |
| `layoutAlign` | Cross-axis self-alignment: `MIN` / `CENTER` / `MAX` / `STRETCH` / `INHERIT` |
| `layoutPositioning` | `AUTO` (in-flow) or `ABSOLUTE` (free position inside an auto-layout parent) |

`width`/`height` become optional once you pass a sizing param. Node must already be inside an auto-layout parent — otherwise Figma throws (the tool returns a clear per-node error). Use the probe recipe to verify parent structure before applying.
