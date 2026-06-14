# Tool Selection and Validation

Use this reference after loading the `figma-mcp-express` skill. It describes which core tools to call and how to verify writes without duplicating the batch operation catalog.

## Read Tool Decision

| Situation | Tool |
|---|---|
| Need file/page orientation | `get_metadata`, then `get_pages` |
| Need a node by name/type | `search_nodes` with `nodeId`, `types`, and `limit` |
| Need all nodes of a type under a frame | `scan_nodes_by_types` |
| Need text nodes only | `scan_text_nodes` |
| Known one node id | `get_node` with a small `depth` |
| Known many node ids | `get_nodes_info` |
| Need token-efficient selected tree | `get_design_context` |
| Need codegen-grade context | `get_design_context` with `detail:"codegen"` |
| Need published library catalog | `fetch_library_catalog` with `FIGMA_TOKEN` |

Avoid page-level deep reads. If a result spills to `.figma-mcp-cache/`, query the sidecar files with shell tools instead of pasting the full payload into context.

## Batch Discovery

1. Use `search_batch_ops` with a capability name, op name, or param key to find candidate ops. Examples: `fontSize`, `componentId`, `cornerRadius`.
2. Use `get_batch_op_spec` for exact params, read/write flags, examples, and output shape.
3. Use `batch(validateOnly:true)` before generated or unfamiliar mutations.
4. Execute the same plan with `batch` only after validation passes.

Do not mirror operation schemas in this file. `BatchOpCatalog` is the source of truth.

## Validate After Write

| Write kind | Validate with | Assert |
|---|---|---|
| Create/place | trailing `get_node` in the same batch or follow-up `get_node` | id exists, parent is correct, size/name are correct |
| Auto layout / resize | `get_node depth:1` | layout mode, sizing, gap, padding |
| Fill/stroke/token bind | `get_node depth:0` | variable binding exists; no raw color fallback |
| Instance configuration | `get_nodes_info` | properties and visible content changed |
| Section complete | `save_screenshots` at high dimension | alignment, overflow, theme, placeholder text |

Screenshots are not mutation proof. They are the final visual review.

## Common Errors

| Symptom | Fix |
|---|---|
| Plugin not connected | Open the target file in Figma Desktop and run the plugin; do not retry-loop |
| Timeout/no response | Reduce scope: frame id, smaller depth, lower result volume |
| Wrong file mutated | Call `list_channels` and pass the intended `channel` |
| Node id not found | Convert hyphen ids to colon ids when needed |
| Text op rejects params | Use server param names from `get_batch_op_spec`; `text`, not stale Plugin API aliases |
| FILL sizing did not apply | Place the node under its parent before setting FILL sizing in the batch sequence |
