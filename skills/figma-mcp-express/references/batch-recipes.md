# Batch Recipes

Load before composing any `batch` call. This file covers refs, projection, `map`,
validation, and failure behavior. Exact op params live in `BatchOpCatalog`.

**Quick navigation:** [Discovery](#discovery) · [Shape](#batch-shape) · [Refs](#refs-and-projection) · [Map](#map) · [Validation](#validation) · [Failure](#failure-semantics) · [Examples](#minimal-examples)

---

## Discovery

Progressive discovery flow:

1. `search_batch_ops(query/category/readOnly/mutates)` to find the capability. Search
   by intent words or param key such as `fontSize` or `componentId`.
2. `get_batch_op_spec(op, includeExamples:true)` for exact params/enums.
3. `batch(validateOnly:true)` / `batch(validateOnly:true, ops:[...])` for generated or unfamiliar plans.
4. `batch(ops:[...])` only after validation passes.

Do not copy op-specific schemas into skills, prompts, or hooks. `BatchOpCatalog` is
the SSOT.

## Batch shape

Every write primitive in the default `core` profile is a batch op type, not a
top-level tool. Route files at the outer call: `batch(channel:"auto-2", ops:[...])`.
Never put `channel` inside `ops[*].params`; those params are validated against the op
schema.

Canonical op shapes:

```json
{ "type": "set_fills", "nodeIds": ["<id-or-$ref>"], "params": { "...": "..." } }
{ "type": "map", "over": "$0.nodes[*]", "as": "node", "do": { "type": "set_visible", "nodeIds": ["$node.id"], "params": { "visible": true } } }
```

Node targets go in op-level `nodeIds`, not `params.nodeIds`. Singular `params.nodeId`
is for subtree-root read/scan ops.

## Refs and projection

Refs point backward only: `$0.id`, `$1.nodes.0.id`, `$0.matchingNodes[*].id`.
self/forward refs are rejected. Only ONE `[*]` wildcard is allowed per ref.

Projection fans a scan/search result into one bulk op:

| Source op | Return field | Projection ref |
|---|---|---|
| `search_nodes` | `nodes` | `$0.nodes[*].id` |
| `scan_nodes_by_types` | `matchingNodes` | `$0.matchingNodes[*].id` |
| `scan_text_nodes` | `textNodes` | `$0.textNodes[*].id` |

Use projection for same-param bulk setters such as `set_fills`, `set_strokes`,
`set_visible`, `swap_component`, `set_instance_properties`, `bind_variable_to_node`,
and `set_corner_radius`. Prefer `continueOnError:true` for independent bulk work.

## Map

Use `map` when each item needs different params.

```json
{
  "ops": [
    { "type": "scan_text_nodes", "params": { "nodeId": "<frame-id>" } },
    { "type": "map", "over": "$0.textNodes[*]", "as": "item",
      "do": { "type": "set_text", "nodeIds": ["$item.id"], "params": { "text": "$item.name" } } }
  ]
}
```

Rules:
- `map.over` must resolve to an array.
- `map.as` must be an identifier and cannot be `index`.
- named binding refs are only allowed inside `map.do`.
- `$item` / `$index` substitute only as whole JSON values; string interpolation like
  `"Section $index"` is literal and invalid for generated names.
- Named binding projections such as `$item.children[*].id` are rejected.
- `map.do` cannot be another `map`.
- cap is 500 items.

## Validation

Hard rejects before plugin execution:

- unknown op type; nested `batch`
- unknown op fields
- `nodeIds` not an array of strings; `params` not an object
- unknown params; stale aliases such as `characters` for `create_text`/`set_text`
- invalid component import `assetType`
- script-like keys anywhere: `script`, `code`, `js`, `eval`, `function`
- self/forward refs, malformed refs, more than one `[*]`
- invalid `map.over`, `map.as`, or `map.do`; named binding refs outside `map.do`

No raw Plugin API JS in batch. A script-like UX must compile to declarative
FigmaPlan JSON and pass catalog validation.

## Failure semantics

Batch is NOT transactional. Ops before a failure remain applied. On partial failure,
the result includes `failedAt` and per-op errors.

Fix the failing op and resend from `failedAt`; resending the whole batch can
double-create prior nodes. A forward-ref error means the producer op must move before
the consumer.

Keep one batch to one logical section, not one giant batch. The single plugin serial
slot is held for the whole batch, so huge batches block reads, writes, and verification.
Server caps (`FIGMA_MCP_BATCH_MAX_OPS`, `FIGMA_MCP_BATCH_MAX_BYTES`) reject before
plugin execution.

## Minimal examples

Create -> bind -> layout -> verify:

```json
{
  "ops": [
    { "type": "create_frame", "params": { "name": "Card", "width": 320, "height": 200, "parentId": "<wrapper-id>" } },
    { "type": "set_fills", "nodeIds": ["$0.id"], "params": { "variableId": "<surface-var-id>" } },
    { "type": "set_auto_layout", "nodeIds": ["$0.id"], "params": { "layoutMode": "VERTICAL", "itemSpacingVariableId": "<gap-var-id>" } },
    { "type": "get_node", "nodeIds": ["$0.id"], "params": { "depth": 2 } }
  ]
}
```

Import -> place -> configure:

```json
{
  "ops": [
    { "type": "import_component_by_key", "params": { "key": "<component-set-key>", "assetType": "COMPONENT_SET" } },
    { "type": "create_instance", "params": { "componentId": "$0.id", "parentId": "<wrapper-id>" } },
    { "type": "set_instance_properties", "nodeIds": ["$1.id"], "params": { "properties": { "Variant": "Primary" } } }
  ]
}
```

More non-prototype task workflows live in `workflow-recipes.md`; prototype wiring
and reaction maps live in the `figma-prototype` skill; native effect mechanics live
in `effects.md`.
