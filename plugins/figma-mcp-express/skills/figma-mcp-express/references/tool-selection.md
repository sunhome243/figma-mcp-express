# Tool Selection and Validation

Use after loading `figma-mcp-express`. Pick narrow reads, discover writes through the
catalog, then verify every mutation.

**Quick navigation:** [Reads](#reads) · [Batch discovery](#batch-discovery) · [Validation](#validation) · [Detail levels](#detail-levels) · [Style audit](#style-audit) · [Libraries](#libraries) · [Structure](#structure) · [Common errors](#common-errors)

## Reads

| Need | Tool |
|---|---|
| File/page orientation | `get_metadata`, then `get_pages` |
| Node by name/type | `search_nodes` with `nodeId`, `types`, `limit` |
| All nodes of a type | `scan_nodes_by_types` |
| Text only | `scan_text_nodes` |
| One known id | `get_node` with small `depth` |
| Many known ids | `get_nodes_info` |
| Token-efficient tree | `get_design_context` |
| Codegen/fidelity audit | `get_design_context detail:"codegen"` |
| Published library catalog | `fetch_library_catalog` with `FIGMA_TOKEN` |

Avoid page-level deep reads. Start shallow: `get_design_context detail:"minimal"
depth:1`, then inspect one frame at a time. If a response spills to
`.figma-mcp-cache/`, query the sidecar with shell tools.

## Batch discovery

All writes in `core` go through `batch`; write primitives are batch op types. Use
`search_batch_ops` first, including by param key. This is how to discover ops from
terms like `fontSize`, `componentId`, `cornerRadius`, `delete_node`, or `reorder`.
Then inspect with `get_batch_op_spec` and validate with `batch(validateOnly:true)`.

Do not guess top-level write tools. Do not mirror schemas here. `BatchOpCatalog` is
the source of truth.

Library import keys are not node IDs. Component/style keys are 40-char lowercase hex;
for component sets pass `assetType:"COMPONENT_SET"` or fetch the catalog first.

## Validation

| Write kind | Validate with | Assert |
|---|---|---|
| Create/place | trailing or follow-up `get_node` | id, parent, name, size |
| Layout/resize | `get_node depth:1` | layout mode, sizing, gap, padding |
| Token bind | `get_node depth:0` | variable/style binding, no raw fallback |
| Instance config | `get_nodes_info` | properties/content changed |
| Section complete | `save_screenshots` | alignment, overflow, theme, copy |

Screenshots are final visual review, not mutation proof.

## Detail levels

| `detail` | Use |
|---|---|
| `minimal` | orientation: id/name/type/bounds |
| `compact` | fills/strokes/opacity scan |
| `full` | complete node fidelity |
| `codegen` | autoLayout, token names, component refs |

Use `dedupe_components:true` for repeated instances such as card lists, rows, and nav
items. Inspect stubs and read only unique overrides deeply.

## Style audit

1. `get_styles()` + `get_variable_defs()` for named styles and local variables.
2. `get_design_context detail:"compact"` to scan fills/strokes/text styles.
3. Flag raw colors/fonts without style refs; skip intentional instance overrides.
4. Match raw values to existing styles. If matched, compose batch ops; if not, flag a
   design-system gap.
5. Validate first and process chunks of <=20 nodes.

Rule: never change appearance just to link a style. Use batch op set_fills for backgrounds; do not use `fillColor` as a general background mutator.

## Libraries

Empty `get_variable_defs` means no local variables, not no design system.

1. `list_library_variable_collections`
2. `get_library_variables` per collection key
3. `import_variable_by_key` / `import_component_by_key`

Use subscribed variables/components instead of recreating raw local approximations.

## Structure

Orient first, then create parent frames before children in reading order. Use
semantic names: screens PascalCase, sections `Section/Name`, containers
`ComponentName/Container`, icons `Icon/Name`, text `Label`/`Title`/`Body`/`Caption`.

Use Figma-native production features: auto layout/grid/constraints, variables/modes,
text/paint/effect styles, component variants/properties, prototype ops, annotations,
and dev resources where useful. A raw frame that only looks like a component is not a
production design artifact.

## Common errors

| Symptom | Fix |
|---|---|
| Plugin not connected | Open target file and run plugin; no retry loop |
| Timeout/no response | Narrow scope: frame id, depth, result volume |
| Wrong file mutated | `list_channels`, pass intended `channel` |
| Batch rejects `channel` | Put `channel` on outer `batch`, never `ops[*].params` |
| Node id not found | Use colon ids; source ids from live reads |
| Text op rejects params | Use `text`, not stale aliases such as `characters` |
| FILL ignored | Place under parent before setting FILL |
| Known server bugs | Read `mcp-known-bugs.md` |
