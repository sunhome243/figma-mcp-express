# batch-recipes.md — canonical batch chains + catalog validation rules

Load this before composing any `batch` call. Part 1 = ready-to-use recipes (including map and projection bulk). For exact op params, use the live catalog: `search_batch_ops` to find an op, then `get_batch_op_spec` to inspect it. Use `batch(validateOnly:true)` before sending generated or unfamiliar mutations.

---

## Part 1 — Canonical recipes

> Every `type` used in the recipes below (`create_frame`, `set_fills`, `set_auto_layout`, `import_component_by_key`, …) is a **`batch` op type**, NOT a top-level tool in the default `core` profile. They are only callable inside a `batch(ops:[…])` call. Ref syntax is `$N.field` (dot-path, e.g. `$0.nodes.0.id`); the wildcard projection is `$N.field[*].id`. There is no top-level `create_frame`/`set_fills` to call directly in `core`.

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
    { "type": "import_component_by_key",  "params": { "key": "<component-set-key>", "assetType": "COMPONENT_SET" } },
    { "type": "create_instance",          "params": { "componentId": "$0.id", "parentId": "<wrapper-id>" } },
    { "type": "set_instance_properties",  "nodeIds": ["$1.id"], "params": { "properties": { "Variant": "Primary", "State": "Default" } } },
    { "type": "set_variable_mode",        "nodeIds": ["$1.id"], "params": { "collectionId": "<col-id>", "modeId": "<dark-mode-id>" } }
  ]
}
```

Use `assetType:"COMPONENT_SET"` only for component-set keys; omit it for concrete component/variant keys. `create_instance` *requires* `componentId` — thread the imported component's `$0.id` into it. (`componentKey` is an optional fallback the handler auto-imports only when `componentId` doesn't resolve; with the import op present, use the returned id directly.) Stop policy: every op refs `$N` → stops at first error, so if import fails there's no create attempt.

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

**Binding rules:** named binding refs are only allowed inside `map.do`. `$item`/`$index` refs substitute only when the whole JSON string value is the ref; string interpolation like `"Section $index"` is treated as literal text and will not substitute. `map.as` must be an identifier and cannot be `index`. Named binding projections such as `$item.children[*].id` are rejected; use `$N.path[*]` before `map` or map over the child list directly. `map.do` cannot be another `map`.

**Limits:** cap 500 items per `map`. Emits progress updates every 10 items. Returns `{ results:[{i,type,data}|{i,type,error}], okCount, failCount }`.

```json
// Copy each text node name into its visible text
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
        "params": { "text": "$node.name" }
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
    { "type": "import_component_by_key", "params": { "key": "<component-set-key>", "assetType": "COMPONENT_SET" } },
    { "type": "create_instance",  "params": { "componentId": "$0.id", "parentId": "<page-root-id>" } },
    { "type": "get_node",         "nodeIds": ["$1.id"], "params": { "depth": 6 } },
    { "type": "save_screenshots", "nodeIds": ["$1.id"], "params": { "outputPath": ".figma-mcp-cache/probe-<name>.png" } },
    { "type": "delete_nodes",     "nodeIds": ["$1.id"] }
  ]
}
```

Use before building N instances of any unfamiliar component. `get_node(depth:6)` reveals text node IDs, actual rendered heights, and property names for `set_instance_properties`.
For concrete component/variant keys, omit `assetType`; for component-set keys, pass `assetType:"COMPONENT_SET"` or fetch the library catalog first so the server can inject the route hint.

---

### Batch failure semantics — NOT transactional, resend from `failedAt`

Figma has **no rollback.** Ops before a failure stay applied — each op is its own undo step. On a partial failure the result carries `failedAt` and per-op `{i, error}` (plus a `💡` hint).

**Fix only the failing op and resend FROM `failedAt`** — resending the whole batch double-creates everything before the failure. A forward-ref error (`$N` with `N` ≥ the current op index) means your ops are out of order: move the producer op before the consumer.

Stop policy is automatic: any op uses a `$N` ref → dependent chain → stops at first failure (downstream refs would break). No refs → independent bulk → runs all ops, reports each. Override with `continueOnError: true|false`.

Keep one batch to one logical section (a few dozen ops). A batch holds the plugin's single serial slot for its whole run, so an enormous batch blocks every other call and leaves nothing to verify between steps. Build incrementally with batches, not one giant batch.

Server caps fail fast before plugin execution: `FIGMA_MCP_BATCH_MAX_OPS` defaults to `200`, and `FIGMA_MCP_BATCH_MAX_BYTES` defaults to `2097152` encoded bytes. Split the plan when either cap trips; only raise caps for controlled local runs.

---

## Part 2 — Catalog + validation rules

Do not copy op-specific schemas into skills, prompts, or hooks. `BatchOpCatalog` is the SSOT.

Route files at the outer call: `batch(channel:"auto-2", ops:[...])`. Never put `channel` inside `ops[*].params`; those params are validated against the op schema.

**Progressive discovery flow:**
1. `search_batch_ops(query/category/readOnly/mutates)` — find the capability.
2. `get_batch_op_spec(op, includeExamples:true)` — inspect exact params/enums.
3. `batch(validateOnly:true, ops:[...])` — validate generated or unfamiliar plans.
4. `batch(ops:[...])` — execute only after the plan validates.

**Allowed op shapes:**
```json
{ "type": "set_fills", "nodeIds": ["<id-or-$ref>"], "params": { "...": "..." } }
{ "type": "map", "over": "$0.nodes[*]", "as": "node", "do": { "type": "set_visible", "nodeIds": ["$node.id"], "params": { "visible": true } } }
```

**Hard rejects before plugin execution:**
- unknown op type; nested `batch`
- unknown op fields (`type`, `nodeIds`, `params`; for `map`: `type`, `over`, `as`, `do`)
- `nodeIds` not an array of strings; `params` not an object
- unknown params; stale aliases such as `characters` for `create_text`/`set_text` (use `text`)
- invalid component import `assetType` (allowed: `COMPONENT`, `COMPONENT_SET`)
- script-like keys anywhere: `script`, `code`, `js`, `eval`, `function`
- self/forward refs (`$N` must point to an earlier op), malformed refs, more than one `[*]`
- invalid `map.over`, `map.as`, or `map.do`; `map.do` is validated like any other catalog op
- named binding refs outside `map.do`, unknown map bindings, named binding projections, nested `map`

**No raw Plugin API JS in batch.** If a future script-like UX is added, it must compile to declarative FigmaPlan JSON and pass the same catalog validation before execution.
