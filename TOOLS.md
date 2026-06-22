# TOOLS.md — figma-mcp-express tool catalog

The default `FIGMA_MCP_TOOL_PROFILE=core` exposes a compact 22-tool MCP surface.
Every plugin-supported operation remains available through validated
`batch`/FigmaPlan op types. Use `search_batch_ops` to find an op,
`get_batch_op_spec` for its authoritative schema, and
`batch(validateOnly:true)` before generated or unfamiliar mutations.

This file documents the current broader compatibility/catalog vocabulary. Set
`FIGMA_MCP_TOOL_PROFILE=full` to expose the full top-level compatibility surface
for debugging or clients that do not use the core batch-first profile.

**The core 22 top-level tools** (everything else below is a `batch` op type or
full-profile-only — NOT directly callable in `core`): `set_presence`, `batch`, `search_batch_ops`,
`get_batch_op_spec`, `get_metadata`, `get_node`, `get_nodes_info`, `get_pages`,
`get_selection`, `get_design_context`, `get_styles`, `get_variable_defs`,
`scan_nodes_by_types`, `scan_text_nodes`, `search_nodes`, `get_local_components`,
`list_channels`, `list_library_variable_collections`, `fetch_library_catalog`,
`export_tokens`, `export_frames_to_pdf`, `save_screenshots`. Every `## Write —*`
section and the `import_*` library ops are batch op types in `core`; the
reads under `## Read — Document` not named above (`get_document`, `get_reactions`,
`get_viewport`, `get_fonts`, `get_annotations`, `get_screenshot`) are
full-profile-only — use `get_metadata` / `get_node` / `save_screenshots` in `core`.

Plugin-facing top-level tools expose optional `channel` routing and required `origin` presence params (omitted from most param tables for brevity — see the Channel and multi-agent skill docs). Local/meta tools (`list_channels`, `search_batch_ops`, `get_batch_op_spec`, and REST-backed `fetch_library_catalog`) do not expose `origin`. For `batch`, pass `channel` and `origin` on the outer `batch` call only; per-op `params.channel` and `params.origin` are rejected because op contracts come from `BatchOpCatalog`. Node IDs are documented and returned in colon format, e.g. `4029:12345`; the runtime normalizes common Figma URL hyphen IDs before validation as a recovery path.

---

## Channel

### list_channels

List connected Figma plugin channels — one entry per open file. Returns channel id, fileName, fileKey, and pageName for each. When more than one file is connected, pass a channel id as the `channel` param on any other tool to target that specific file (or match by fileName first).

| Name          | Type | Required | Description                                               |
| ------------- | ---- | -------- | --------------------------------------------------------- |
| _(no params)_ | —    | —        | Returns array of `{channel, fileName, fileKey, pageName}` |

---

## Read — Document

### get_document

Get the full node tree of the current page (not the whole file — only the active page). Returns all nodes recursively and can be very large. Prefer `get_design_context` for exploration or when token efficiency matters. Large responses spill to disk as `{spilled:true, path, bytes, preview}`. Timeouts are server-managed — re-scope a timed-out read to a smaller subtree, never request a longer timeout.

| Name                          | Type    | Required | Description                                                                                       |
| ----------------------------- | ------- | -------- | ------------------------------------------------------------------------------------------------- |
| skipInvisibleInstanceChildren | boolean | No       | Skip hidden instances' children during traversal (faster on instance-heavy files). Default false. |
| channel                       | string  | No       | Target a specific connected file by channel id.                                                   |

### get_pages

List all pages in the document with their IDs and names. Lightweight alternative to `get_document`.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_metadata

Get metadata about the current Figma document: file name, pages, current page. Large responses spill to disk. This is the structure/id map and the canonical FIRST read — then use `get_node`/`get_nodes_info` for detail, `get_design_context` for a token-efficient region tree, or `search_nodes`/`scan_nodes_by_types` for name/type hunts.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_selection

Get the nodes currently selected in Figma. Returns an empty array if nothing is selected. Use `get_design_context` or `get_node` to retrieve deeper detail about a specific node by ID. Live-state read — never served from the read cache.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_image_by_hash

Get image metadata and bytes for an existing Figma image hash using `figma.getImageByHash`. Returns `{hash, image:null}` when the hash is not in the file.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| hash    | string | Yes      | Image hash from an IMAGE paint                  |
| channel | string | No       | Target a specific connected file by channel id. |

### get_file_thumbnail

Get the node currently assigned as the file thumbnail, if any.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_dev_resources

Read Dev Mode resource links attached to a node via the node DevResourcesMixin. This covers node-level resource CRUD; the private partner-only `figma.devResources` host API is not exposed.

| Name            | Type    | Required | Description                                      |
| --------------- | ------- | -------- | ------------------------------------------------ |
| nodeId          | string  | Yes      | Node ID in colon format                          |
| includeChildren | boolean | No       | Include resources attached to descendants        |
| channel         | string  | No       | Target a specific connected file by channel id.  |

### resolve_variable_for_consumer

Resolve a variable's effective value for a specific scene node using `Variable.resolveForConsumer`.

| Name       | Type   | Required | Description                                     |
| ---------- | ------ | -------- | ----------------------------------------------- |
| variableId | string | Yes      | Variable ID to resolve                          |
| nodeId     | string | Yes      | Consumer scene node ID in colon format          |
| channel    | string | No       | Target a specific connected file by channel id. |

### get_node

Get a single node by ID with full detail. Optional `depth` limits traversal depth to avoid MB-scale payloads. Use `get_nodes_info` to fetch multiple nodes in one round-trip. INSTANCE nodes include `mainComponent:{key,name,remote}` and `componentProperties` (the resolved variant map).

| Name                          | Type    | Required | Description                                                                                                         |
| ----------------------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------------------- |
| nodeId                        | string  | Yes      | Node ID in colon format e.g. `4029:12345`                                                                           |
| depth                         | number  | No       | How many levels deep to traverse. `0` = node only (no children), `1` = node + direct children. Default 50 (bounded, not unbounded) — a node returned with `childCount` but no `children` was truncated; request a larger depth. |
| skipInvisibleInstanceChildren | boolean | No       | Skip hidden instances' children. Default false.                                                                     |
| channel                       | string  | No       | Target a specific connected file by channel id.                                                                     |

### get_nodes_info

Get full details for multiple nodes by ID in one round-trip. Prefer this over calling `get_node` repeatedly. Large responses spill to disk. INSTANCE nodes include `mainComponent:{key,name,remote}` and `componentProperties` (the resolved variant map).

| Name                          | Type     | Required | Description                                     |
| ----------------------------- | -------- | -------- | ----------------------------------------------- |
| nodeIds                       | string[] | Yes      | List of node IDs in colon format                |
| depth                         | number   | No       | Levels deep to traverse per node. Default 50 (bounded). `0` = node only, `1` = node + direct children. A node with `childCount` but no `children` was truncated. |
| skipInvisibleInstanceChildren | boolean  | No       | Skip hidden instances' children. Default false. |
| channel                       | string   | No       | Target a specific connected file by channel id. |

### get_design_context

Get a depth-limited, token-efficient tree of the current selection or page. Supports detail levels and deduplication of repeated component instances. Large responses spill to disk.

| Name                          | Type    | Required | Description                                                                                                                                                                                                                           |
| ----------------------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| depth                         | number  | No       | Levels deep to traverse (default 2)                                                                                                                                                                                                   |
| detail                        | string  | No       | `minimal` (id/name/type/bounds), `compact` (+fills/strokes/opacity), `full` (everything, default), `codegen` (full + autoLayout + resolved design-token names + INSTANCE componentRef/Code-Connect mapping)                           |
| dedupe_components             | boolean | No       | When true, INSTANCE nodes are serialized compactly with mainComponentId + overrides; unique component definitions collected once in a top-level `componentDefs` map. Highly token-efficient for screens with many repeated instances. |
| codeConnectMap                | object  | No       | Optional map of published component key to arbitrary mapping value (e.g. `{"abc123": {"component": "Button", "import": "@/ui/button"}}`). Only used with `detail=codegen`.                                                            |
| skipInvisibleInstanceChildren | boolean | No       | Skip hidden instances' children. Default false.                                                                                                                                                                                       |
| channel                       | string  | No       | Target a specific connected file by channel id.                                                                                                                                                                                       |

### search_nodes

Search for nodes by name substring and/or type within a subtree. Use `scan_nodes_by_types` when you want all nodes of a type regardless of name. Returns at most `limit` results (default 50).

| Name                          | Type     | Required | Description                                                     |
| ----------------------------- | -------- | -------- | --------------------------------------------------------------- |
| query                         | string   | Yes      | Name substring to match (case-insensitive)                      |
| nodeId                        | string   | No       | Scope search to this subtree (default: current page)            |
| types                         | string[] | No       | Filter by Figma node type e.g. `["TEXT", "FRAME", "COMPONENT"]` |
| limit                         | number   | No       | Maximum results to return (default 50)                          |
| skipInvisibleInstanceChildren | boolean  | No       | Skip hidden instances' children. Default false.                 |
| channel                       | string   | No       | Target a specific connected file by channel id.                 |

### scan_text_nodes

Scan all TEXT nodes in a subtree and return their content. Shorthand for `scan_nodes_by_types` with `["TEXT"]`. Scope `nodeId` tightly — scanning a huge subtree can time out. Large results spill to disk.

| Name                          | Type    | Required | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| nodeId                        | string  | Yes      | Root node ID to scan from                       |
| skipInvisibleInstanceChildren | boolean | No       | Skip hidden instances' children. Default false. |
| channel                       | string  | No       | Target a specific connected file by channel id. |

### scan_nodes_by_types

Find all nodes of specific types in a subtree, regardless of name. Use `search_nodes` instead when you need to filter by name. Scope `nodeId` tightly. Large results spill to disk. Each result includes both `bounds` and `bbox` (same shape; `bbox` is kept for back-compat).

| Name                          | Type     | Required | Description                                                  |
| ----------------------------- | -------- | -------- | ------------------------------------------------------------ |
| nodeId                        | string   | Yes      | Root node ID to scan from                                    |
| types                         | string[] | Yes      | Node types to find e.g. `["FRAME", "COMPONENT", "INSTANCE"]` |
| skipInvisibleInstanceChildren | boolean  | No       | Skip hidden instances' children. Default false.              |
| channel                       | string   | No       | Target a specific connected file by channel id.              |

### get_reactions

Get the prototype reactions defined on a node. Returns an array of reaction objects — each has a trigger and an actions array.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| nodeId  | string | Yes      | Node ID in colon format                         |
| channel | string | No       | Target a specific connected file by channel id. |

### get_viewport

Get the current Figma viewport: scroll center, zoom level, and visible bounds. Live-state read — never served from the read cache.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_fonts

List all fonts used in the current page, sorted by usage frequency. Useful for understanding typography without scanning all text nodes.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

---

## Read — Styles & Variables

### get_styles

Get all local styles in the document (paint, text, effect, and grid). Returns `{paints, text, effects, grids}` arrays (not a flat type+properties list); each effect entry carries a full `effects[]`. Use the style ID with `apply_style_to_node` or `update_paint_style`. For design tokens (variables), use `get_variable_defs` instead.

| Name                          | Type    | Required | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| skipInvisibleInstanceChildren | boolean | No       | Skip hidden instances' children. Default false. |
| channel                       | string  | No       | Target a specific connected file by channel id. |

### get_variable_defs

Get all local variable definitions: collections, modes, and values. Variables are Figma's design token system.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_selection_colors

Get Figma's computed colors from the current selection using `getSelectionColors`.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### get_local_components

Get all components defined in the current Figma file. Omit `pageId` for the whole-file recovery scan, which loads all pages and uses the document-level typed search path to recover file-local component masters that bounded page traversal can miss. For very large libraries, pass `pageId` to scan exactly one page (bounded enumeration, not recovery). Large results spill to disk. componentSets entries include a `defaultVariantKey` (import THAT, not the SET key); component entries include `variantProperties`.

| Name    | Type   | Required | Description                                                                                   |
| ------- | ------ | -------- | --------------------------------------------------------------------------------------------- |
| pageId  | string | No       | Scope scan to exactly one page by its node ID (colon format e.g. `0:1`). Omit for whole-file recovery scan. |
| channel | string | No       | Target a specific connected file by channel id.                                               |

### get_annotations

Get dev-mode annotations in the current document or scoped to a specific node. Returns annotation objects with label text, measurement type, and the ID of the annotated node.

| Name    | Type   | Required | Description                                                   |
| ------- | ------ | -------- | ------------------------------------------------------------- |
| nodeId  | string | No       | Scope results to annotations on this node and its descendants |
| channel | string | No       | Target a specific connected file by channel id.               |

### export_tokens

Export all design tokens (variables and paint styles) as JSON or CSS custom properties. Ideal for bridging Figma variables into your codebase.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| format  | string | No       | Output format: `json` (default) or `css`        |
| channel | string | No       | Target a specific connected file by channel id. |

---

## Read — Export

### export_frames_to_pdf

Export multiple frames as a single multi-page PDF file. Each frame becomes one page in order. Ideal for pitch decks, proposals, and slide exports.

| Name       | Type     | Required | Description                                           |
| ---------- | -------- | -------- | ----------------------------------------------------- |
| nodeIds    | string[] | Yes      | Ordered list of frame node IDs to export as PDF pages |
| outputPath | string   | Yes      | File path to write the PDF to, must end in `.pdf`     |
| channel    | string   | No       | Target a specific connected file by channel id.       |

### get_screenshot

Export a screenshot of one or more nodes as base64-encoded image data (held in memory). Use `save_screenshots` instead when you want to write images directly to disk without base64 in the response. Live-state read — never served from the read cache.

| Name    | Type     | Required | Description                                              |
| ------- | -------- | -------- | -------------------------------------------------------- |
| nodeIds | string[] | No       | Node IDs to export. If empty, exports current selection. |
| format  | string   | No       | Export format: `PNG` (default), `SVG`, `JPG`, or `PDF`   |
| scale   | number   | No       | Export scale for raster formats (default 2)              |
| channel | string   | No       | Target a specific connected file by channel id.          |

### save_screenshots

Export screenshots for multiple nodes and write them to the local filesystem. Returns file metadata (path, size, dimensions) — no base64 in the response.

| Name    | Type     | Required | Description                                                    |
| ------- | -------- | -------- | -------------------------------------------------------------- |
| items   | object[] | Yes      | List of `{nodeId, outputPath, format?, scale?}` objects        |
| format  | string   | No       | Default export format: `PNG` (default), `SVG`, `JPG`, or `PDF` |
| scale   | number   | No       | Default export scale for raster formats (default 2)            |
| channel | string   | No       | Target a specific connected file by channel id.                |

---

## Write — Create

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### create_frame

Create a new frame on the current page or inside a parent node. Optional layout-sizing params (FILL/HUG) size the frame within an auto-layout parent.

| Name                   | Type   | Required | Description                                                                   |
| ---------------------- | ------ | -------- | ----------------------------------------------------------------------------- |
| x                      | number | No       | X position (default 0)                                                        |
| y                      | number | No       | Y position (default 0)                                                        |
| width                  | number | No       | Width in pixels (default 100)                                                 |
| height                 | number | No       | Height in pixels (default 100)                                                |
| name                   | string | No       | Frame name                                                                    |
| fillColor              | string | No       | Fill color as hex e.g. `#FFFFFF`                                              |
| layoutMode             | string | No       | Auto-layout direction: `HORIZONTAL`, `VERTICAL`, `GRID`, or `NONE`            |
| gridRowCount           | number | No       | Number of rows when `layoutMode` is `GRID`                                    |
| gridColumnCount        | number | No       | Number of columns when `layoutMode` is `GRID`                                 |
| gridRowGap             | number | No       | Row gap when `layoutMode` is `GRID`                                           |
| gridColumnGap          | number | No       | Column gap when `layoutMode` is `GRID`                                        |
| paddingTop             | number | No       | Auto-layout top padding                                                       |
| paddingRight           | number | No       | Auto-layout right padding                                                     |
| paddingBottom          | number | No       | Auto-layout bottom padding                                                    |
| paddingLeft            | number | No       | Auto-layout left padding                                                      |
| itemSpacing            | number | No       | Auto-layout gap between children                                              |
| primaryAxisAlignItems  | string | No       | Main-axis alignment: `MIN`, `CENTER`, `MAX`, or `SPACE_BETWEEN`               |
| counterAxisAlignItems  | string | No       | Cross-axis alignment: `MIN`, `CENTER`, `MAX`, or `BASELINE`                   |
| primaryAxisSizingMode  | string | No       | Main-axis sizing: `FIXED` or `AUTO` (hug)                                     |
| counterAxisSizingMode  | string | No       | Cross-axis sizing: `FIXED` or `AUTO` (hug)                                    |
| layoutWrap             | string | No       | Wrap behaviour: `NO_WRAP` or `WRAP`                                           |
| counterAxisSpacing     | number | No       | Gap between wrapped rows/columns (only when `layoutWrap` is `WRAP`)           |
| parentId               | string | No       | Parent node ID. Defaults to current page.                                     |
| layoutSizingHorizontal | string | No       | Horizontal sizing inside an auto-layout parent: `FIXED`, `HUG`, or `FILL`     |
| layoutSizingVertical   | string | No       | Vertical sizing inside an auto-layout parent: `FIXED`, `HUG`, or `FILL`       |
| layoutGrow             | number | No       | Grow factor along the parent's main axis (0 = don't grow, 1 = fill remaining) |
| layoutAlign            | string | No       | Cross-axis self-alignment: `MIN`, `CENTER`, `MAX`, `STRETCH`, or `INHERIT`    |
| layoutPositioning      | string | No       | `AUTO` (in-flow) or `ABSOLUTE` (free position inside auto-layout parent)      |
| cornerRadius           | number | No       | Uniform corner radius in pixels                                               |
| clipsContent           | boolean | No      | Clip children to the frame boundary (default true)                            |
| opacity                | number | No       | Frame opacity 0–1 (default 1)                                                 |
| paddingTopVariableId    | string | No       | Variable ID to bind to `paddingTop` instead of a raw value                    |
| paddingRightVariableId  | string | No       | Variable ID to bind to `paddingRight` instead of a raw value                  |
| paddingBottomVariableId | string | No       | Variable ID to bind to `paddingBottom` instead of a raw value                 |
| paddingLeftVariableId   | string | No       | Variable ID to bind to `paddingLeft` instead of a raw value                   |
| itemSpacingVariableId   | string | No       | Variable ID to bind to `itemSpacing` instead of a raw value                   |
| gridRowGapVariableId    | string | No       | Variable ID to bind to `gridRowGap` when `layoutMode` is `GRID`               |
| gridColumnGapVariableId | string | No       | Variable ID to bind to `gridColumnGap` when `layoutMode` is `GRID`            |
| channel                | string | No       | Target a specific connected file by channel id.                               |

### create_rectangle

Create a new rectangle on the current page or inside a parent node. Supports uniform corner radius via `cornerRadius` or independent per-corner rounding via `topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`.

| Name              | Type   | Required | Description                                                                  |
| ----------------- | ------ | -------- | ---------------------------------------------------------------------------- |
| x                 | number | No       | X position (default 0)                                                       |
| y                 | number | No       | Y position (default 0)                                                       |
| width             | number | No       | Width in pixels (default 100)                                                |
| height            | number | No       | Height in pixels (default 100)                                               |
| name              | string | No       | Rectangle name                                                               |
| fillColor         | string | No       | Fill color as hex e.g. `#FF5733`                                             |
| cornerRadius      | number | No       | Uniform corner radius in pixels (all four corners)                           |
| topLeftRadius     | number | No       | Top-left corner radius (overrides `cornerRadius` for this corner)            |
| topRightRadius    | number | No       | Top-right corner radius (overrides `cornerRadius` for this corner)           |
| bottomLeftRadius  | number | No       | Bottom-left corner radius (overrides `cornerRadius` for this corner)         |
| bottomRightRadius | number | No       | Bottom-right corner radius (overrides `cornerRadius` for this corner)        |
| parentId          | string | No       | Parent node ID. Defaults to current page.                                    |
| channel           | string | No       | Target a specific connected file by channel id.                              |

### create_ellipse

Create a new ellipse (circle/oval) on the current page or inside a parent node.

| Name      | Type   | Required | Description                                     |
| --------- | ------ | -------- | ----------------------------------------------- |
| x         | number | No       | X position (default 0)                          |
| y         | number | No       | Y position (default 0)                          |
| width     | number | No       | Width in pixels (default 100)                   |
| height    | number | No       | Height in pixels (default 100)                  |
| name      | string | No       | Ellipse name                                    |
| fillColor | string | No       | Fill color as hex e.g. `#3B82F6`                |
| parentId  | string | No       | Parent node ID. Defaults to current page.       |
| channel   | string | No       | Target a specific connected file by channel id. |

### create_line

Create a straight LineNode for dividers and rules. A visible 1px black stroke is applied by default.

| Name         | Type   | Required | Description                                                                                         |
| ------------ | ------ | -------- | --------------------------------------------------------------------------------------------------- |
| x            | number | No       | X position (default 0)                                                                              |
| y            | number | No       | Y position (default 0)                                                                              |
| length       | number | No       | Line length in pixels (default 100)                                                                 |
| name         | string | No       | Line name                                                                                           |
| strokeWeight | number | No       | Stroke thickness in pixels (default 1)                                                              |
| strokeColor  | string | No       | Stroke color as hex e.g. `#000000` (default black)                                                  |
| strokeCap    | string | No       | `NONE`, `ROUND`, `SQUARE`, `ARROW_LINES`, `ARROW_EQUILATERAL`, `DIAMOND_FILLED`, `TRIANGLE_FILLED`, or `CIRCLE_FILLED` |
| rotation     | number | No       | Rotation in degrees (default 0 = horizontal; 90 = vertical)                                         |
| parentId     | string | No       | Parent node ID. Defaults to current page.                                                           |
| channel      | string | No       | Target a specific connected file by channel id.                                                     |

### create_polygon

Create a regular PolygonNode with a configurable number of sides.

| Name       | Type   | Required | Description                                      |
| ---------- | ------ | -------- | ------------------------------------------------ |
| x          | number | No       | X position (default 0)                           |
| y          | number | No       | Y position (default 0)                           |
| width      | number | No       | Width in pixels (default 100)                    |
| height     | number | No       | Height in pixels (default 100)                   |
| pointCount | number | No       | Number of sides, minimum 3 (default 3)           |
| name       | string | No       | Polygon name                                     |
| fillColor  | string | No       | Fill color as hex e.g. `#3B82F6`                 |
| parentId   | string | No       | Parent node ID. Defaults to current page.        |
| channel    | string | No       | Target a specific connected file by channel id.  |

### create_star

Create a StarNode with configurable point count and inner-radius ratio.

| Name        | Type   | Required | Description                                             |
| ----------- | ------ | -------- | ------------------------------------------------------- |
| x           | number | No       | X position (default 0)                                  |
| y           | number | No       | Y position (default 0)                                  |
| width       | number | No       | Width in pixels (default 100)                           |
| height      | number | No       | Height in pixels (default 100)                          |
| pointCount  | number | No       | Number of star points, minimum 3 (default 5)            |
| innerRadius | number | No       | Inner-radius ratio 0-1 (default 0.5; smaller is spikier) |
| name        | string | No       | Star name                                               |
| fillColor   | string | No       | Fill color as hex e.g. `#FBBF24`                        |
| parentId    | string | No       | Parent node ID. Defaults to current page.               |
| channel     | string | No       | Target a specific connected file by channel id.         |

### import_svg

Create Figma vector nodes from raw SVG markup via `figma.createNodeFromSvg`. Returns a wrapping frame containing the imported vectors.

| Name     | Type   | Required | Description                                      |
| -------- | ------ | -------- | ------------------------------------------------ |
| svg      | string | Yes      | Raw SVG markup string e.g. `<svg>...</svg>`      |
| x        | number | No       | X position (default 0)                           |
| y        | number | No       | Y position (default 0)                           |
| name     | string | No       | Name for the wrapping frame                      |
| parentId | string | No       | Parent node ID. Defaults to current page.        |
| channel  | string | No       | Target a specific connected file by channel id.  |

### create_table

Create a TableNode with the given rows and columns. Optionally fill cells with text.

| Name       | Type     | Required | Description                                                                           |
| ---------- | -------- | -------- | ------------------------------------------------------------------------------------- |
| numRows    | number   | Yes      | Number of rows, minimum 1                                                             |
| numColumns | number   | Yes      | Number of columns, minimum 1                                                          |
| x          | number   | No       | X position (default 0)                                                                |
| y          | number   | No       | Y position (default 0)                                                                |
| name       | string   | No       | Table name                                                                            |
| cells      | string[][] | No     | Optional 2D cell text array indexed `[row][column]`; out-of-range entries are ignored |
| parentId   | string   | No       | Parent node ID. Defaults to current page.                                             |
| channel    | string   | No       | Target a specific connected file by channel id.                                       |

### create_text

Create a new text node on the current page or inside a parent node. The font is loaded automatically before insertion. Returns the created node ID and bounds.

| Name                | Type   | Required | Description                                                                            |
| ------------------- | ------ | -------- | -------------------------------------------------------------------------------------- |
| text                | string | Yes      | Text content to display                                                                |
| x                   | number | No       | X position in pixels (default 0)                                                       |
| y                   | number | No       | Y position in pixels (default 0)                                                       |
| fontSize            | number | No       | Font size in pixels (default 14)                                                       |
| fontFamily          | string | No       | Font family name e.g. `Inter`, `Roboto` (default Inter)                                |
| fontStyle           | string | No       | Font style variant e.g. `Regular`, `Bold`, `Medium` (default Regular)                  |
| fillColor           | string | No       | Text color as hex e.g. `#000000` (default black)                                       |
| name                | string | No       | Node name shown in the layers panel                                                    |
| parentId            | string | No       | Parent node ID. Defaults to current page.                                              |
| textAlignHorizontal | string | No       | Horizontal alignment: `LEFT`, `CENTER`, `RIGHT`, or `JUSTIFIED`                        |
| textAlignVertical   | string | No       | Vertical alignment: `TOP`, `CENTER`, or `BOTTOM`                                       |
| textAutoResize      | string | No       | Auto-resize: `NONE`, `HEIGHT`, `WIDTH_AND_HEIGHT`, or `TRUNCATE`                       |
| letterSpacingValue  | number | No       | Letter spacing value                                                                   |
| letterSpacingUnit   | string | No       | Letter spacing unit: `PIXELS` or `PERCENT`                                             |
| lineHeightValue     | number | No       | Line height value                                                                      |
| lineHeightUnit      | string | No       | Line height unit: `PIXELS`, `PERCENT`, or `AUTO`                                       |
| textCase            | string | No       | Text case: `ORIGINAL`, `UPPER`, `LOWER`, `TITLE`, `SMALL_CAPS`, or `SMALL_CAPS_FORCED` |
| textDecoration      | string | No       | Text decoration: `NONE`, `UNDERLINE`, or `STRIKETHROUGH`                               |
| channel             | string | No       | Target a specific connected file by channel id.                                        |

### create_instance

Create an instance of a component, optionally placing it in a parent, positioning/sizing it, and setting variant and exposed-instance properties.

| Name                   | Type   | Required | Description                                                                       |
| ---------------------- | ------ | -------- | --------------------------------------------------------------------------------- |
| componentId            | string | Yes      | Source COMPONENT node ID in colon format                                          |
| componentKey           | string | No       | Library component key, used to resolve the component if it must be imported first |
| parentId               | string | No       | Parent node ID for the instance. Defaults to the current page.                    |
| index                  | number | No       | Insertion index within the parent's children                                      |
| x                      | number | No       | X position of the instance                                                        |
| y                      | number | No       | Y position of the instance                                                        |
| width                  | number | No       | Width to resize the instance to                                                   |
| height                 | number | No       | Height to resize the instance to                                                  |
| layoutSizingHorizontal | string | No       | Horizontal sizing inside an auto-layout parent: `FIXED`, `HUG`, or `FILL`         |
| layoutSizingVertical   | string | No       | Vertical sizing inside an auto-layout parent: `FIXED`, `HUG`, or `FILL`           |
| variantProperties      | object | No       | Variant property map e.g. `{"State": "Default", "Size": "Large"}`                 |
| properties             | object | No       | Exposed instance/text property map e.g. `{"Label#1:0": "Submit"}`                 |
| channel                | string | No       | Target a specific connected file by channel id.                                   |

### create_component

Convert an existing node (frame, group, or shape) into a reusable local COMPONENT. The node is replaced in place by the new component.

| Name    | Type   | Required | Description                                                                      |
| ------- | ------ | -------- | -------------------------------------------------------------------------------- |
| nodeId  | string | Yes      | Node ID of the frame, group, or shape to convert into a component                |
| name    | string | No       | Optional name for the component. Defaults to the node's current name.            |
| channel | string | No       | Target a specific connected file by channel id.                                  |

### create_vector

Create an editable VECTOR node with optional `vectorPaths`, size, fill color, and parent.

| Name        | Type     | Required | Description                                                   |
| ----------- | -------- | -------- | ------------------------------------------------------------- |
| x           | number   | No       | X position (default 0)                                        |
| y           | number   | No       | Y position (default 0)                                        |
| width       | number   | No       | Width in pixels (default 100)                                 |
| height      | number   | No       | Height in pixels (default 100)                                |
| name        | string   | No       | Vector node name                                              |
| vectorPaths | object[] | No       | Figma VectorPath objects                                      |
| fillColor   | string   | No       | Solid fill color as hex e.g. `#3B82F6`                        |
| parentId    | string   | No       | Parent node ID. Defaults to current page.                     |
| channel     | string   | No       | Target a specific connected file by channel id.               |

### create_slice

Create a Slice node for export regions.

| Name     | Type   | Required | Description                                      |
| -------- | ------ | -------- | ------------------------------------------------ |
| x        | number | No       | X position (default 0)                           |
| y        | number | No       | Y position (default 0)                           |
| width    | number | No       | Width in pixels (default 100)                    |
| height   | number | No       | Height in pixels (default 100)                   |
| name     | string | No       | Slice name                                       |
| parentId | string | No       | Parent node ID. Defaults to current page.        |
| channel  | string | No       | Target a specific connected file by channel id.  |

### create_page_divider

Create a page divider node using `figma.createPageDivider`. The optional name must be divider-only text, such as `---` or `***`.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| name    | string | No       | All hyphens, asterisks, spaces, en dashes, or em dashes |
| channel | string | No       | Target a specific connected file by channel id. |

### create_text_path

Create a TextPath node from an existing vector-like node (`VECTOR`, `RECTANGLE`, `ELLIPSE`, `POLYGON`, `STAR`, or `LINE`).

| Name          | Type   | Required | Description                                     |
| ------------- | ------ | -------- | ----------------------------------------------- |
| nodeId        | string | Yes      | Vector-like node ID in colon format             |
| startSegment  | number | No       | Non-negative vector path segment index          |
| startPosition | number | No       | Normalized start position on the segment, 0 to 1 |
| name          | string | No       | Text path node name                             |
| channel       | string | No       | Target a specific connected file by channel id. |

### create_section

Create a Figma Section node on the current page. Sections are the modern way to organize frames and groups on a page.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| name    | string | No       | Section name (default `Section`)                |
| x       | number | No       | X position (default 0)                          |
| y       | number | No       | Y position (default 0)                          |
| width   | number | No       | Width in pixels                                 |
| height  | number | No       | Height in pixels                                |
| channel | string | No       | Target a specific connected file by channel id. |

### import_image

Import an image from disk, raw base64, or a remote URL into Figma as a new image-fill rectangle. The server reads `imagePath` from the local filesystem and base64-encodes it automatically; `imageUrl` uses Figma's `createImageAsync`. Returns the new node ID and bounds.

| Name        | Type   | Required | Description                                                                              |
| ----------- | ------ | -------- | ---------------------------------------------------------------------------------------- |
| imagePath   | string | No       | Local image file path on the server's filesystem. Preferred over `imageData`.                 |
| imageData   | string | No       | Raw base64-encoded image data. Use only when `imagePath` is unavailable.                 |
| imageUrl    | string | No       | Remote image URL loaded by Figma's `createImageAsync`                                   |
| x           | number | No       | X position (default 0)                                                                   |
| y           | number | No       | Y position (default 0)                                                                   |
| width       | number | No       | Width in pixels (default 200)                                                            |
| height      | number | No       | Height in pixels (default 200)                                                           |
| name        | string | No       | Node name shown in the layers panel                                                      |
| scaleMode   | string | No       | Image fill scale mode: `FILL` (default), `FIT`, `CROP`, or `TILE`                        |
| rotation    | number | No       | Image fill rotation for `FILL`/`FIT`/`TILE`                                               |
| scalingFactor | number | No     | Tile density / repeat scale for `TILE`                                                    |
| imageTransform | array | No     | 2x3 crop transform matrix for `CROP`                                                      |
| exposure, contrast, saturation, temperature, tint, highlights, shadows | number | No | ImageFilters fields, each -1..1 |
| parentId    | string | No       | Parent node ID. Defaults to current page.                                                |
| channel     | string | No       | Target a specific connected file by channel id.                                          |

### create_video

Create a rectangle with a VIDEO fill from `videoPath` or base64 `videoData` using Figma's `createVideoAsync`.

| Name        | Type   | Required | Description                                                     |
| ----------- | ------ | -------- | --------------------------------------------------------------- |
| videoPath   | string | No       | Local video file path; the server reads and base64-encodes it   |
| videoData   | string | No       | Base64-encoded video bytes                                      |
| x           | number | No       | X position (default 0)                                          |
| y           | number | No       | Y position (default 0)                                          |
| width       | number | No       | Width in pixels (default 200)                                   |
| height      | number | No       | Height in pixels (default 200)                                  |
| name        | string | No       | Node name                                                       |
| scaleMode   | string | No       | Video scale mode: `FILL` (default), `FIT`, `CROP`, or `TILE`    |
| rotation    | number | No       | Video fill rotation for `FILL`/`FIT`/`TILE`                     |
| scalingFactor | number | No     | Tile density / repeat scale for `TILE`                          |
| videoTransform | array | No     | 2x3 video crop transform matrix for `CROP`                       |
| exposure, contrast, saturation, temperature, tint, highlights, shadows | number | No | Video filter fields, each -1..1 |
| parentId    | string | No       | Parent node ID. Defaults to current page.                       |
| channel     | string | No       | Target a specific connected file by channel id.                 |

### create_gif

Create a FigJam GIF media node from an existing `imageHash` using `figma.createGif`. This API is host/editor dependent.

| Name      | Type   | Required | Description                                     |
| --------- | ------ | -------- | ----------------------------------------------- |
| imageHash | string | Yes      | Image hash to convert to a GIF media node       |
| x         | number | No       | X position (default 0)                          |
| y         | number | No       | Y position (default 0)                          |
| width     | number | No       | Width in pixels                                 |
| height    | number | No       | Height in pixels                                |
| name      | string | No       | Node name                                       |
| parentId  | string | No       | Parent node ID. Defaults to current page.       |
| channel   | string | No       | Target a specific connected file by channel id. |

### create_link_preview

Create a FigJam link preview node from a URL using `figma.createLinkPreviewAsync`. This API is host/editor dependent.

| Name     | Type   | Required | Description                                     |
| -------- | ------ | -------- | ----------------------------------------------- |
| url      | string | Yes      | URL to render as a link preview                 |
| x        | number | No       | X position (default 0)                          |
| y        | number | No       | Y position (default 0)                          |
| name     | string | No       | Node name                                       |
| parentId | string | No       | Parent node ID. Defaults to current page.       |
| channel  | string | No       | Target a specific connected file by channel id. |

### clone_node

Clone an existing node, optionally repositioning it or placing it in a new parent.

| Name     | Type   | Required | Description                                                      |
| -------- | ------ | -------- | ---------------------------------------------------------------- |
| nodeId   | string | Yes      | Source node ID                                                   |
| x        | number | No       | X position of the clone                                          |
| y        | number | No       | Y position of the clone                                          |
| parentId | string | No       | Parent node ID for the clone. Defaults to same parent as source. |
| channel  | string | No       | Target a specific connected file by channel id.                  |

---

## Write — Modify

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### set_file_thumbnail

Set or clear the current file thumbnail node. Omit `nodeId` to clear the thumbnail.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| nodeId  | string | No       | FRAME, COMPONENT, COMPONENT_SET, or SECTION ID  |
| channel | string | No       | Target a specific connected file by channel id. |

### add_dev_resource

Attach a Dev Mode resource link to a node using `addDevResourceAsync`.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| nodeId  | string | Yes      | Node ID in colon format                         |
| url     | string | Yes      | Resource URL                                    |
| name    | string | No       | Optional display name                           |
| channel | string | No       | Target a specific connected file by channel id. |

### edit_dev_resource

Edit a node Dev Mode resource link by its current URL. Provide `url`, `name`, or both as the replacement.

| Name       | Type   | Required | Description                                     |
| ---------- | ------ | -------- | ----------------------------------------------- |
| nodeId     | string | Yes      | Node ID in colon format                         |
| currentUrl | string | Yes      | Existing resource URL to edit                   |
| url        | string | No       | Replacement URL                                 |
| name       | string | No       | Replacement display name                        |
| channel    | string | No       | Target a specific connected file by channel id. |

### delete_dev_resource

Delete a Dev Mode resource link from a node by URL.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| nodeId  | string | Yes      | Node ID in colon format                         |
| url     | string | Yes      | Resource URL to delete                          |
| channel | string | No       | Target a specific connected file by channel id. |

### rename_node [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Rename a single node by ID. Returns the updated node with its new name. Use `batch_rename_nodes` to rename multiple nodes at once.

| Name    | Type   | Required | Description                                                                     |
| ------- | ------ | -------- | ------------------------------------------------------------------------------- |
| nodeId  | string | Yes      | Node ID in colon format                                                         |
| name    | string | Yes      | New name. Figma supports slash-separated path notation e.g. `Icons/Arrow/Left`. |

### delete_nodes

Delete one or more nodes. This cannot be undone via MCP — use with care. Returns a per-node result array of `{nodeId, deleted}` or `{nodeId, error}` and does NOT abort when one node is un-removable (e.g. an instance child, which Figma natively refuses); that node's error carries an intent-ordered hint — delete on the master to propagate, or `detach_instance` then delete the resulting frame, to remove; `swap_component` to replace; `set_visible:false` only hides.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to delete                              |
| channel | string   | No       | Target a specific connected file by channel id. |

### lock_nodes [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Lock one or more nodes to prevent accidental edits in Figma.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to lock                                |

### unlock_nodes [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Unlock one or more nodes, allowing them to be edited again.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to unlock                              |

### set_visible

Show or hide one or more nodes by setting their visibility.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs                                        |
| visible | boolean  | Yes      | `true` to show, `false` to hide                 |
| channel | string   | No       | Target a specific connected file by channel id. |

### move_nodes

Move one or more nodes to an absolute canvas position. The same x/y is applied to every node independently (not a relative offset from current position).

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs                                        |
| x       | number   | No       | Target X position                               |
| y       | number   | No       | Target Y position                               |
| channel | string   | No       | Target a specific connected file by channel id. |

### reparent_nodes

Move one or more nodes to a different parent frame, group, or section. By default preserves each node's absolute canvas position after reparenting (counter-acts the new parent's transform).

| Name                    | Type     | Required | Description                                                                                      |
| ----------------------- | -------- | -------- | ------------------------------------------------------------------------------------------------ |
| nodeIds                 | string[] | Yes      | Node IDs to move                                                                                 |
| parentId                | string   | Yes      | Target parent node ID                                                                            |
| preserveAbsolutePosition | boolean | No       | Keep each node's absolute canvas position after reparenting (default true)                       |
| channel                 | string   | No       | Target a specific connected file by channel id.                                                  |

### ungroup_nodes [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Ungroup one or more GROUP nodes, moving their children to the parent and removing the group.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | GROUP node IDs                                  |

### group_nodes

Group two or more nodes into a GROUP. All nodes must share the same parent.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to group (minimum 2)                   |
| name    | string   | No       | Optional name for the new group                 |
| channel | string   | No       | Target a specific connected file by channel id. |

### reorder_nodes [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Change the z-order (layer stack position) of one or more nodes.

| Name    | Type     | Required | Description                                                     |
| ------- | -------- | -------- | --------------------------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs                                                        |
| order   | string   | Yes      | `bringToFront`, `sendToBack`, `bringForward`, or `sendBackward` |

### batch_rename_nodes

Rename multiple nodes using find/replace, regex substitution, or prefix/suffix addition.

| Name       | Type     | Required | Description                                                           |
| ---------- | -------- | -------- | --------------------------------------------------------------------- |
| nodeIds    | string[] | Yes      | Node IDs                                                              |
| find       | string   | No       | String (or regex when `useRegex=true`) to search for in the node name |
| replace    | string   | No       | Replacement string. Required when `find` is provided.                 |
| useRegex   | boolean  | No       | Treat `find` as a regular expression (default false)                  |
| regexFlags | string   | No       | Regex flags e.g. `gi` (default `g`). Only used when `useRegex=true`.  |
| prefix     | string   | No       | String to prepend to the node name                                    |
| suffix     | string   | No       | String to append to the node name                                     |
| channel    | string   | No       | Target a specific connected file by channel id.                       |

### boolean_operation [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Combine two or more vector nodes using a boolean operation, producing a new merged vector shape. The source nodes are consumed. Flatten first if the inputs are not already vectors.

| Name      | Type     | Required | Description                                                                                  |
| --------- | -------- | -------- | -------------------------------------------------------------------------------------------- |
| nodeIds   | string[] | Yes      | Two or more vector node IDs to combine                                                       |
| operation | string   | Yes      | `UNION`, `SUBTRACT`, `INTERSECT`, `EXCLUDE`, or `FLATTEN`                                   |
| name      | string   | No       | Name for the resulting node                                                                  |
| parentId  | string   | No       | Parent node ID for the result. Defaults to the parent of the first input node.               |

---

## Write — Components

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### swap_component

Swap the main component of an existing INSTANCE node, replacing it with a different component while keeping position and size. Uses Figma's override-preserving `swapComponent()` (not `mainComponent=`), so text and variant overrides survive the swap.

| Name        | Type   | Required | Description                                            |
| ----------- | ------ | -------- | ------------------------------------------------------ |
| nodeId      | string | Yes      | INSTANCE node ID                                       |
| componentId | string | Yes      | Target COMPONENT node ID (from `get_local_components`) |
| channel     | string | No       | Target a specific connected file by channel id.        |

### detach_instance [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Detach one or more component instances, converting them to plain frames. The link to the main component is broken; all visual properties are preserved.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | INSTANCE node IDs                               |

### set_instance_properties

Set variant, boolean, text, and instance-swap properties on a component INSTANCE. Use `resetOverrides=true` to restore defaults before applying. SLOT-type keys are auto-filtered (passing them throws `cannotSetSlotProperty`, which would poison the whole update) and reported back as `droppedSlotKeys`; fonts for TEXT properties are preloaded automatically.

| Name           | Type    | Required | Description                                                                         |
| -------------- | ------- | -------- | ----------------------------------------------------------------------------------- |
| nodeId         | string  | Yes      | INSTANCE node ID                                                                    |
| properties     | object  | Yes      | Property map e.g. `{"State": "On", "Label#1:0": "Save"}`                            |
| resetOverrides | boolean | No       | Reset the instance to component defaults before applying properties (default false) |
| channel        | string  | No       | Target a specific connected file by channel id.                                     |

### import_component_by_key

Import a component (or component set) from a subscribed library by its key, making it available to instantiate. Component keys must be full 40-char lowercase hex published keys, not node IDs. For a COMPONENT_SET key pass `assetType='COMPONENT_SET'`; if the key was seen in a cached `fetch_library_catalog` result, the server injects the correct `assetType` automatically.

| Name      | Type   | Required | Description                                                     |
| --------- | ------ | -------- | --------------------------------------------------------------- |
| key       | string | Yes      | Published library component key: 40-char lowercase hex, not a node ID |
| assetType | string | No       | Asset type hint: `COMPONENT` (default) or `COMPONENT_SET`       |
| channel   | string | No       | Target a specific connected file by channel id.                 |

---

## Write — Styles

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### apply_style_to_node

Apply an existing local style (paint, text, effect, or grid) to a node, linking the node to that style.

| Name    | Type   | Required | Description                                                   |
| ------- | ------ | -------- | ------------------------------------------------------------- |
| nodeId  | string | Yes      | Target node ID                                                |
| styleId | string | Yes      | Style ID to apply (from `get_styles`)                         |
| target  | string | No       | For paint styles only — apply to `fill` (default) or `stroke` |
| channel | string | No       | Target a specific connected file by channel id.               |

### set_fills

Set the fill color on a single node. Use `mode='append'` to stack a new fill on top of existing fills. Supports token binding via `variableId`. Pass `paints[]` for full control over multiple fills (gradient, image, etc.) — takes precedence over `color` when both are provided.

| Name       | Type     | Required | Description                                                                       |
| ---------- | -------- | -------- | --------------------------------------------------------------------------------- |
| nodeId     | string   | Yes      | Node ID                                                                           |
| color      | string   | No       | Fill color as hex: `#RRGGBB` or `#RRGGBBAA` for alpha. Required unless `paints` is given. |
| opacity    | number   | No       | Fill opacity 0–1 (default 1)                                                      |
| mode       | string   | No       | `replace` (default) overwrites all existing fills; `append` stacks on top         |
| variableId | string   | No       | Design variable ID to bind the fill color to (token-driven, hex acts as fallback) |
| paints     | object[] | No       | Full paint array (Figma Paint objects). Takes precedence over `color` when provided. Supports solid, gradient, and image fills. |
| channel    | string   | No       | Target a specific connected file by channel id.                                   |

### set_strokes

Set the stroke color and weight on a single node. Use `mode='append'` to stack. Supports token binding via `variableId`. Pass `paints[]` for full control over multiple strokes — takes precedence over `color` when both are provided.

| Name         | Type     | Required | Description                                                                        |
| ------------ | -------- | -------- | ---------------------------------------------------------------------------------- |
| nodeId       | string   | Yes      | Node ID                                                                            |
| color        | string   | No       | Stroke color as hex e.g. `#000000`. Required unless `paints` is given.             |
| strokeWeight | number   | No       | Stroke weight in pixels (default 1)                                                |
| mode         | string   | No       | `replace` (default) overwrites all strokes; `append` stacks                        |
| variableId   | string   | No       | Design variable ID to bind the stroke color to                                     |
| paints       | object[] | No       | Full paint array (Figma Paint objects). Takes precedence over `color` when provided. |
| channel      | string   | No       | Target a specific connected file by channel id.                                    |

### set_effects

Apply one or more effects directly to a node. Supports shadows, normal/progressive blurs, and native `GLASS`, `NOISE`, and `TEXTURE` effects. Replaces all existing effects. Pass an empty array to clear all effects.

| Name    | Type     | Required | Description                                                                                                                                                                                                 |
| ------- | -------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| nodeId  | string   | Yes      | Target node ID                                                                                                                                                                                              |
| effects | object[] | Yes      | Array of effect objects. `type`: `DROP_SHADOW`, `INNER_SHADOW`, `LAYER_BLUR`, `BACKGROUND_BLUR`, `GLASS`, `NOISE`, or `TEXTURE`. Shadows use `color`, `opacity`, `offsetX`, `offsetY`, `spread`, and `radius`. Blurs use `radius`, optional `blurType:"PROGRESSIVE"`, `startRadius`, `startOffset`, and `endOffset`. Native effects use their Figma fields; `NOISE`/`TEXTURE` support `noiseSizeVector:{x,y}`. |
| channel | string   | No       | Target a specific connected file by channel id.                                                                                                                                                             |

### set_blend_mode [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Set the blend mode of one or more nodes.

| Name      | Type     | Required | Description                                                                                                                                                                                                       |
| --------- | -------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| nodeIds   | string[] | Yes      | Node IDs                                                                                                                                                                                                          |
| blendMode | string   | Yes      | `NORMAL`, `MULTIPLY`, `SCREEN`, `OVERLAY`, `DARKEN`, `LIGHTEN`, `COLOR_DODGE`, `COLOR_BURN`, `HARD_LIGHT`, `SOFT_LIGHT`, `DIFFERENCE`, `EXCLUSION`, `HUE`, `SATURATION`, `COLOR`, `LUMINOSITY`, or `PASS_THROUGH` |

### set_opacity

Set the opacity of one or more nodes (0 = fully transparent, 1 = fully opaque).

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs                                        |
| opacity | number   | Yes      | Opacity value between 0 and 1                   |
| channel | string   | No       | Target a specific connected file by channel id. |

### set_corner_radius [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Set corner radius on one or more nodes. Provide a uniform `cornerRadius` or per-corner values (`topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`). When both uniform and per-corner values are supplied, per-corner values take precedence.

| Name              | Type     | Required | Description                                     |
| ----------------- | -------- | -------- | ----------------------------------------------- |
| nodeIds           | string[] | Yes      | Node IDs                                        |
| cornerRadius      | number   | No       | Uniform corner radius applied to all corners    |
| topLeftRadius     | number   | No       | Top-left corner radius                          |
| topRightRadius    | number   | No       | Top-right corner radius                         |
| bottomLeftRadius  | number   | No       | Bottom-left corner radius                       |
| bottomRightRadius | number   | No       | Bottom-right corner radius                      |

### set_constraints

Set layout constraints (pinning behaviour) on one or more nodes relative to their parent. Also available as a `batch` op.

| Name       | Type     | Required | Description                                                  |
| ---------- | -------- | -------- | ------------------------------------------------------------ |
| nodeIds    | string[] | Yes      | Node IDs                                                     |
| horizontal | string   | No       | `MIN` (left), `MAX` (right), `CENTER`, `STRETCH`, or `SCALE` |
| vertical   | string   | No       | `MIN` (top), `MAX` (bottom), `CENTER`, `STRETCH`, or `SCALE` |
| channel    | string   | No       | Target a specific connected file by channel id.              |

### rotate_nodes [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Rotate one or more nodes to an absolute angle in degrees.

| Name     | Type     | Required | Description                                                       |
| -------- | -------- | -------- | ----------------------------------------------------------------- |
| nodeIds  | string[] | Yes      | Node IDs                                                          |
| rotation | number   | Yes      | Rotation angle in degrees (positive = counter-clockwise in Figma) |

### resize_nodes

Resize one or more nodes and/or set their sizing-within-parent (FILL/HUG). FILL/HUG requires the node to be inside an auto-layout parent.

| Name                   | Type     | Required | Description                                                                |
| ---------------------- | -------- | -------- | -------------------------------------------------------------------------- |
| nodeIds                | string[] | Yes      | Node IDs                                                                   |
| width                  | number   | No       | New width in pixels                                                        |
| height                 | number   | No       | New height in pixels                                                       |
| minWidth               | number/null | No    | Auto-layout child minimum width; pass `null` to clear                      |
| maxWidth               | number/null | No    | Auto-layout child maximum width; pass `null` to clear                      |
| minHeight              | number/null | No    | Auto-layout child minimum height; pass `null` to clear                     |
| maxHeight              | number/null | No    | Auto-layout child maximum height; pass `null` to clear                     |
| layoutSizingHorizontal | string   | No       | `FIXED`, `HUG`, or `FILL`                                                  |
| layoutSizingVertical   | string   | No       | `FIXED`, `HUG`, or `FILL`                                                  |
| layoutGrow             | number   | No       | Grow factor along the parent's main axis                                   |
| layoutAlign            | string   | No       | Cross-axis self-alignment: `MIN`, `CENTER`, `MAX`, `STRETCH`, or `INHERIT` |
| layoutPositioning      | string   | No       | `AUTO` or `ABSOLUTE`                                                       |
| channel                | string   | No       | Target a specific connected file by channel id.                            |

### set_auto_layout

Set or update auto-layout (flex) properties on an existing frame.

| Name                  | Type   | Required | Description                                                         |
| --------------------- | ------ | -------- | ------------------------------------------------------------------- |
| nodeId                | string | Yes      | Frame node ID                                                       |
| layoutMode            | string | No       | `HORIZONTAL`, `VERTICAL`, `GRID`, or `NONE`                         |
| gridRowCount          | number | No       | Number of rows when `layoutMode` is `GRID`                          |
| gridColumnCount       | number | No       | Number of columns when `layoutMode` is `GRID`                       |
| gridRowGap            | number | No       | Row gap when `layoutMode` is `GRID`                                 |
| gridColumnGap         | number | No       | Column gap when `layoutMode` is `GRID`                              |
| paddingTop            | number | No       | Top padding                                                         |
| paddingRight          | number | No       | Right padding                                                       |
| paddingBottom         | number | No       | Bottom padding                                                      |
| paddingLeft           | number | No       | Left padding                                                        |
| itemSpacing           | number | No       | Gap between children                                                |
| primaryAxisAlignItems | string | No       | `MIN`, `CENTER`, `MAX`, or `SPACE_BETWEEN`                          |
| counterAxisAlignItems | string | No       | `MIN`, `CENTER`, `MAX`, or `BASELINE`                               |
| counterAxisAlignContent | string | No     | Wrapped-track distribution: `AUTO` or `SPACE_BETWEEN`               |
| primaryAxisSizingMode | string | No       | `FIXED` or `AUTO` (hug)                                             |
| counterAxisSizingMode | string | No       | `FIXED` or `AUTO` (hug)                                             |
| layoutWrap            | string | No       | `NO_WRAP` or `WRAP`                                                 |
| overflowDirection     | string | No       | `NONE`, `HORIZONTAL`, `VERTICAL`, or `BOTH`                         |
| strokesIncludedInLayout | boolean | No    | Include strokes in auto-layout sizing                               |
| itemReverseZIndex     | boolean | No      | Reverse child stacking order                                        |
| minWidth              | number/null | No    | Frame minimum width; pass `null` to clear                           |
| maxWidth              | number/null | No    | Frame maximum width; pass `null` to clear                           |
| minHeight             | number/null | No    | Frame minimum height; pass `null` to clear                          |
| maxHeight             | number/null | No    | Frame maximum height; pass `null` to clear                          |
| counterAxisSpacing      | number | No       | Gap between wrapped rows/columns (only when `layoutWrap` is `WRAP`) |
| paddingTopVariableId    | string | No       | Variable ID to bind to `paddingTop` instead of a raw value          |
| paddingRightVariableId  | string | No       | Variable ID to bind to `paddingRight` instead of a raw value        |
| paddingBottomVariableId | string | No       | Variable ID to bind to `paddingBottom` instead of a raw value       |
| paddingLeftVariableId   | string | No       | Variable ID to bind to `paddingLeft` instead of a raw value         |
| itemSpacingVariableId   | string | No       | Variable ID to bind to `itemSpacing` instead of a raw value         |
| counterAxisSpacingVariableId | string | No | Variable ID to bind to `counterAxisSpacing`                         |
| gridRowGapVariableId    | string | No       | Variable ID to bind to `gridRowGap` when `layoutMode` is `GRID`     |
| gridColumnGapVariableId | string | No       | Variable ID to bind to `gridColumnGap` when `layoutMode` is `GRID`  |
| channel               | string | No       | Target a specific connected file by channel id.                     |

### set_text

Update the content and/or styling of an existing TEXT node. Provide `text` to change content; provide any styling param to restyle. At least one is required. Font-dependent changes load the font automatically.

| Name                | Type   | Required | Description                                                                 |
| ------------------- | ------ | -------- | --------------------------------------------------------------------------- |
| nodeId              | string | Yes      | TEXT node ID                                                                |
| text                | string | No       | New text content (omit to restyle without changing the text)                |
| fontSize            | number | No       | Font size in pixels                                                         |
| fontFamily          | string | No       | Font family to switch to                                                    |
| fontStyle           | string | No       | Font style e.g. `Regular`, `Medium`, `Bold`                                 |
| textAlignHorizontal | string | No       | `LEFT`, `CENTER`, `RIGHT`, or `JUSTIFIED`                                   |
| textAlignVertical   | string | No       | `TOP`, `CENTER`, or `BOTTOM`                                                |
| textAutoResize      | string | No       | `NONE`, `HEIGHT`, `WIDTH_AND_HEIGHT`, or `TRUNCATE`                         |
| letterSpacingValue  | number | No       | Letter spacing value                                                        |
| letterSpacingUnit   | string | No       | `PIXELS` or `PERCENT`                                                       |
| lineHeightValue     | number | No       | Line height value                                                           |
| lineHeightUnit      | string | No       | `PIXELS`, `PERCENT`, or `AUTO`                                              |
| textCase            | string | No       | `ORIGINAL`, `UPPER`, `LOWER`, `TITLE`, `SMALL_CAPS`, or `SMALL_CAPS_FORCED` |
| textDecoration      | string | No       | `NONE`, `UNDERLINE`, or `STRIKETHROUGH`                                     |
| channel             | string | No       | Target a specific connected file by channel id.                             |

### set_text_range

Apply styling to a character range within a TEXT node. Offsets are zero-based and use `[startOffset, endOffset)`.

| Name               | Type   | Required | Description                                                                            |
| ------------------ | ------ | -------- | -------------------------------------------------------------------------------------- |
| nodeId             | string | Yes      | TEXT node ID                                                                           |
| startOffset        | number | Yes      | Start character index, inclusive                                                       |
| endOffset          | number | Yes      | End character index, exclusive                                                         |
| fontFamily         | string | No       | Font family for the range                                                              |
| fontStyle          | string | No       | Font style for the range                                                               |
| fontSize           | number | No       | Font size in pixels for the range                                                      |
| color              | string | No       | Text color for the range as hex                                                        |
| textCase           | string | No       | `ORIGINAL`, `UPPER`, `LOWER`, `TITLE`, `SMALL_CAPS`, or `SMALL_CAPS_FORCED`            |
| textDecoration     | string | No       | `NONE`, `UNDERLINE`, or `STRIKETHROUGH`                                                |
| letterSpacingValue | number | No       | Letter spacing value                                                                   |
| letterSpacingUnit  | string | No       | `PIXELS` or `PERCENT`                                                                  |
| lineHeightValue    | number | No       | Line height value                                                                      |
| lineHeightUnit     | string | No       | `PIXELS`, `PERCENT`, or `AUTO`                                                         |
| hyperlink          | object/null | No  | `{url}` or `{nodeId}` hyperlink for the range; pass `null` to clear                    |
| listOptions        | object | No       | `{type:"ORDERED"|"UNORDERED"|"NONE"}`                                                  |
| indentation        | number | No       | Indentation level for the range                                                        |
| channel            | string | No       | Target a specific connected file by channel id.                                        |

### create_paint_style

Create a new local paint style. Pass `paints[]` for gradients or multi-stop fills; pass `color` for a simple solid. `paints` takes precedence when both are provided.

| Name        | Type     | Required | Description                                                                                          |
| ----------- | -------- | -------- | ---------------------------------------------------------------------------------------------------- |
| name        | string   | Yes      | Style name e.g. `Brand/Primary`                                                                      |
| color       | string   | No       | Solid fill color as hex e.g. `#FF5733`. Required unless `paints` is given.                           |
| paints      | object[] | No       | Full paint array (Figma Paint objects). Takes precedence over `color`. Supports gradients and images. |
| description | string   | No       | Optional style description                                                                           |
| channel     | string   | No       | Target a specific connected file by channel id.                                                      |

### create_text_style

Create a new local text style (typography preset). Returns the new style's ID.

| Name               | Type   | Required | Description                                     |
| ------------------ | ------ | -------- | ----------------------------------------------- |
| name               | string | Yes      | Style name e.g. `Heading/H1`, `Body/Regular`    |
| fontSize           | number | No       | Font size in pixels (default 16)                |
| fontFamily         | string | No       | Font family name (default Inter)                |
| fontStyle          | string | No       | Font style variant (default Regular)            |
| textDecoration     | string | No       | `NONE`, `UNDERLINE`, or `STRIKETHROUGH`         |
| lineHeightValue    | number | No       | Line height value                               |
| lineHeightUnit     | string | No       | `PIXELS` or `PERCENT`                           |
| letterSpacingValue | number | No       | Letter spacing value                                                            |
| letterSpacingUnit  | string | No       | `PIXELS` or `PERCENT`                                                           |
| textCase           | string | No       | Text case: `ORIGINAL`, `UPPER`, `LOWER`, `TITLE`, `SMALL_CAPS`, or `SMALL_CAPS_FORCED` |
| paragraphSpacing   | number | No       | Space after each paragraph in pixels                                            |
| paragraphIndent    | number | No       | First-line indent of each paragraph in pixels                                   |
| description        | string | No       | Optional style description                                                      |
| channel            | string | No       | Target a specific connected file by channel id.                                 |

### create_effect_style

Create a new local effect style. Supports shadows, normal/progressive blurs, and native `GLASS`, `NOISE`, and `TEXTURE` effects. Pass `effects[]` for full control over multiple effects; it takes precedence over the single-effect shorthand params.

| Name                | Type     | Required | Description                                                                                                                                                  |
| ------------------- | -------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| name                | string   | Yes      | Style name e.g. `Shadow/Card`                                                                                                                                |
| type                | string   | No       | `DROP_SHADOW` (default), `INNER_SHADOW`, `LAYER_BLUR`, `BACKGROUND_BLUR`, `GLASS`, `NOISE`, or `TEXTURE`                                                     |
| color               | string   | No       | Shadow color as hex (default `#000000`, shadows only)                                                                                                        |
| opacity             | number   | No       | Shadow color opacity 0–1 (default 0.25, shadows only)                                                                                                        |
| radius              | number   | No       | Shadow, blur, or native effect radius in pixels                                                                                                              |
| offsetX             | number   | No       | Shadow X offset (shadows only)                                                                                                                               |
| offsetY             | number   | No       | Shadow Y offset (default 4, shadows only)                                                                                                                    |
| spread              | number   | No       | Shadow spread (default 0, shadows only)                                                                                                                      |
| blurType            | string   | No       | For `LAYER_BLUR`/`BACKGROUND_BLUR`: `NORMAL` or `PROGRESSIVE`                                                                                                |
| startRadius         | number   | No       | Start radius for `PROGRESSIVE` blur                                                                                                                          |
| startOffset         | object   | No       | Normalized `{x,y}` start point for `PROGRESSIVE` blur                                                                                                        |
| endOffset           | object   | No       | Normalized `{x,y}` end point for `PROGRESSIVE` blur                                                                                                          |
| lightIntensity      | number   | No       | `GLASS` light intensity, 0-1                                                                                                                                  |
| lightAngle          | number   | No       | `GLASS` light angle in degrees                                                                                                                               |
| refraction          | number   | No       | `GLASS` refraction, 0-1                                                                                                                                       |
| depth               | number   | No       | `GLASS` depth                                                                                                                                                |
| dispersion          | number   | No       | `GLASS` dispersion, 0-1                                                                                                                                       |
| noiseType           | string   | No       | `NOISE` type: `MONOTONE`, `DUOTONE`, or `MULTITONE`                                                                                                          |
| secondaryColor      | string   | No       | Secondary color for `NOISE` duotone effects                                                                                                                  |
| noiseSize           | number   | No       | `NOISE`/`TEXTURE` noise size                                                                                                                                  |
| noiseSizeVector     | object   | No       | Optional anisotropic `{x,y}` noise size for `NOISE`/`TEXTURE`                                                                                                |
| density             | number   | No       | `NOISE` density, 0-1                                                                                                                                          |
| clipToShape         | boolean  | No       | `TEXTURE` clip-to-shape flag                                                                                                                                  |
| effects             | object[] | No       | Full effect array. Each object supports the shadow, blur, `GLASS`, `NOISE`, and `TEXTURE` fields above.                                                       |
| description         | string   | No       | Optional style description                                                                                                                                   |
| channel             | string   | No       | Target a specific connected file by channel id.                                                                                                              |

### create_grid_style

Create a new local layout grid style.

| Name        | Type   | Required | Description                                               |
| ----------- | ------ | -------- | --------------------------------------------------------- |
| name        | string | Yes      | Style name e.g. `Grid/Desktop`                            |
| pattern     | string | No       | `GRID` (default), `COLUMNS`, or `ROWS`                    |
| count       | number | No       | Number of columns or rows (COLUMNS/ROWS only, default 12) |
| gutterSize  | number | No       | Gutter size in pixels (COLUMNS/ROWS only, default 16)     |
| offset      | number | No       | Margin/offset in pixels (COLUMNS/ROWS only, default 0)    |
| alignment   | string | No       | `STRETCH` (default), `CENTER`, `MIN`, or `MAX`            |
| sectionSize | number | No       | Grid cell size in pixels (GRID only, default 8)           |
| color       | string | No       | Grid line color as hex (GRID only)                        |
| opacity     | number | No       | Grid line opacity 0–1 (GRID only, default 0.1)            |
| description | string | No       | Optional style description                                |
| channel     | string | No       | Target a specific connected file by channel id.           |

### update_paint_style

Update an existing paint style's name, color, or description. Only paint styles support in-place updates — to modify text, effect, or grid styles, use `delete_style` and recreate them. Pass `paints[]` to replace with a gradient or multi-stop fill.

| Name        | Type     | Required | Description                                                                                          |
| ----------- | -------- | -------- | ---------------------------------------------------------------------------------------------------- |
| styleId     | string   | Yes      | Paint style ID                                                                                       |
| name        | string   | No       | New style name                                                                                       |
| color       | string   | No       | New fill color as hex                                                                                |
| paints      | object[] | No       | Full paint array (Figma Paint objects). Takes precedence over `color`. Supports gradients and images. |
| description | string   | No       | New style description                                                                                |
| channel     | string   | No       | Target a specific connected file by channel id.                                                      |

### reorder_local_style

Move a local paint/text/effect/grid style after another style of the same type. Omit `afterStyleId` to move the target first.

| Name         | Type   | Required | Description                                     |
| ------------ | ------ | -------- | ----------------------------------------------- |
| styleType    | string | Yes      | `PAINT`, `TEXT`, `EFFECT`, or `GRID`            |
| styleId      | string | Yes      | Target local style ID to move                   |
| afterStyleId | string | No       | Reference style ID of the same type             |
| channel      | string | No       | Target a specific connected file by channel id. |

### reorder_local_style_folder

Move a local style folder after another folder for paint/text/effect/grid styles. Omit `afterFolder` to move the target first.

| Name        | Type   | Required | Description                                     |
| ----------- | ------ | -------- | ----------------------------------------------- |
| styleType   | string | Yes      | `PAINT`, `TEXT`, `EFFECT`, or `GRID`            |
| folder      | string | Yes      | Target folder path/name                         |
| afterFolder | string | No       | Reference folder path/name of the same type     |
| channel     | string | No       | Target a specific connected file by channel id. |

### delete_style [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Delete a style (paint, text, effect, or grid) by its ID.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| styleId | string | Yes      | Style ID to delete                              |

### import_style_by_key

Import a paint, text, or effect style from a subscribed library by its key, making it available to apply to nodes.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| key     | string | Yes      | Published library style key: 40-char lowercase hex, not a node ID |
| channel | string | No       | Target a specific connected file by channel id. |

---

## Write — Variables

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### create_variable_collection

Create a new local variable collection with an optional initial mode name. Note: Figma free plan limits each collection to 1 mode. For multi-mode theming on the free plan, use the name-prefix workaround: prefix each variable name with its mode e.g. `light/color-bg` and `dark/color-bg`.

| Name            | Type   | Required | Description                                     |
| --------------- | ------ | -------- | ----------------------------------------------- |
| name            | string | Yes      | Collection name                                 |
| initialModeName | string | No       | Name for the initial mode (default `Mode 1`)    |
| channel         | string | No       | Target a specific connected file by channel id. |

### create_variable

Create a new variable (design token) inside an existing collection. Returns the new variable's ID. The response includes the collection's modes so you can map modeIds for subsequent `set_variable_value` calls.

| Name         | Type   | Required | Description                                                                                                                                           |
| ------------ | ------ | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| name         | string | Yes      | Variable name — use slash notation to group e.g. `Color/Primary`, `Spacing/MD`                                                                        |
| collectionId | string | Yes      | ID of the variable collection (from `get_variable_defs`)                                                                                              |
| type         | string | Yes      | `COLOR`, `FLOAT`, `STRING`, or `BOOLEAN`                                                                                                              |
| value        | string | No       | Initial value for the first (default) mode. COLOR: hex. FLOAT: number. STRING: text. BOOLEAN: `true` or `false`. Use `values` to set multiple modes.  |
| values       | object | No       | Map of `{modeId: value}` to set values for multiple modes at creation time. Takes precedence over `value`. Use `get_variable_defs` to obtain modeIds. |
| channel      | string | No       | Target a specific connected file by channel id.                                                                                                       |

### create_variable_alias

Create a variable alias value from an existing variable ID using `createVariableAliasByIdAsync`. Use the returned alias as a variable value.

| Name       | Type   | Required | Description                                     |
| ---------- | ------ | -------- | ----------------------------------------------- |
| variableId | string | Yes      | Variable ID to alias                            |
| channel    | string | No       | Target a specific connected file by channel id. |

### add_variable_mode

Add a new mode to an existing variable collection (e.g. Light/Dark, Desktop/Mobile). Requires a paid Figma plan — free plan returns `Limited to 1 modes only`.

| Name         | Type   | Required | Description                                     |
| ------------ | ------ | -------- | ----------------------------------------------- |
| collectionId | string | Yes      | Variable collection ID                          |
| modeName     | string | Yes      | Name for the new mode                           |
| channel      | string | No       | Target a specific connected file by channel id. |

### set_variable_mode

Pin a node to a specific mode of a variable collection (e.g. switch a frame to Dark mode) via `setExplicitVariableModeForCollection`.

| Name         | Type   | Required | Description                                      |
| ------------ | ------ | -------- | ------------------------------------------------ |
| nodeId       | string | Yes      | Node ID to pin                                   |
| collectionId | string | Yes      | Variable collection ID                           |
| modeId       | string | Yes      | Mode ID within the collection to pin the node to |
| channel      | string | No       | Target a specific connected file by channel id.  |

### set_variable_value

Set a variable's value for a specific mode. The `modeId` is validated against the variable's collection — an unknown modeId throws with the collection's valid mode IDs listed.

| Name       | Type   | Required | Description                                                                        |
| ---------- | ------ | -------- | ---------------------------------------------------------------------------------- |
| variableId | string | Yes      | Variable ID                                                                        |
| modeId     | string | Yes      | Mode ID within the collection                                                      |
| value      | string | Yes      | Value to set. COLOR: hex. FLOAT: number. STRING: text. BOOLEAN: `true` or `false`. |
| channel    | string | No       | Target a specific connected file by channel id.                                    |

### update_variable

Update an existing variable's metadata. This does not change values; use `set_variable_value` for per-mode values.

| Name                 | Type     | Required | Description                                                            |
| -------------------- | -------- | -------- | ---------------------------------------------------------------------- |
| variableId           | string   | Yes      | Variable ID to update                                                  |
| name                 | string   | No       | New variable name; slash notation groups variables                     |
| scopes               | string[] | No       | Publishing scopes such as `ALL_SCOPES`, `ALL_FILLS`, `GAP`, `FONT_SIZE` |
| hiddenFromPublishing | boolean  | No       | Hide this variable when the file is published as a library             |
| codeSyntax           | object   | No       | Per-platform code names: `{WEB?, ANDROID?, iOS?}`                      |
| removeCodeSyntax     | string[] | No       | Code syntax platforms to remove: `WEB`, `ANDROID`, or `iOS`            |
| channel              | string   | No       | Target a specific connected file by channel id.                        |

### update_variable_collection

Update a variable collection: rename it, hide it from publishing, rename a mode, or remove a mode. A collection must keep at least one mode.

| Name                 | Type    | Required | Description                                      |
| -------------------- | ------- | -------- | ------------------------------------------------ |
| collectionId         | string  | Yes      | Variable collection ID to update                 |
| name                 | string  | No       | New collection name                              |
| hiddenFromPublishing | boolean | No       | Hide this collection when published as a library |
| renameMode           | object  | No       | Rename a mode: `{modeId, newName}`               |
| removeMode           | string  | No       | Mode ID to remove; cannot remove the last mode   |
| channel              | string  | No       | Target a specific connected file by channel id.  |

### bind_variable_to_node

Bind a local variable to a node property so the property is driven by the variable's value. For `fillColor`/`strokeColor` the binding is applied to paint index 0 while preserving the remaining paints (no collapse); the base paint at index 0 must be SOLID.

| Name       | Type   | Required | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------- | ------ | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| nodeId     | string | Yes      | Target node ID                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| variableId | string | Yes      | Variable ID to bind (from `get_variable_defs`)                                                                                                                                                                                                                                                                                                                                                                                                               |
| field      | string | Yes      | Property to bind: `fillColor`, `strokeColor`, `visible`, `characters`, `opacity`, `width`, `height`, `minWidth`, `maxWidth`, `minHeight`, `maxHeight`, `topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`, `strokeWeight`, `strokeTopWeight`, `strokeRightWeight`, `strokeBottomWeight`, `strokeLeftWeight`, `itemSpacing`, `counterAxisSpacing`, `gridRowGap`, `gridColumnGap`, `paddingTop`, `paddingRight`, `paddingBottom`, `paddingLeft`. NOT bindable: `cornerRadius`, `rotation`, `x`, `y`. |
| channel    | string | No       | Target a specific connected file by channel id.                                                                                                                                                                                                                                                                                                                                                                                                              |

### bind_variable_to_effect

Bind a variable to a field on an Effect object using `setBoundVariableForEffect`. This is Figma's pure object helper: it returns an updated effect object and does not mutate any node or style by itself. Apply the returned object with `set_effects` or `create_effect_style`.

| Name       | Type   | Required | Description                                     |
| ---------- | ------ | -------- | ----------------------------------------------- |
| effect     | object | Yes      | Effect object to bind                           |
| field      | string | Yes      | Effect field to bind, e.g. `radius` or `color`  |
| variableId | string | Yes      | Variable ID to bind                             |
| channel    | string | No       | Target a specific connected file by channel id. |

### bind_variable_to_layout_grid

Bind a variable to a field on a LayoutGrid object using `setBoundVariableForLayoutGrid`. This is Figma's pure object helper: it returns an updated grid object and does not mutate any node or style by itself. Apply the returned object with layout-grid style/node APIs.

| Name       | Type   | Required | Description                                                |
| ---------- | ------ | -------- | ---------------------------------------------------------- |
| layoutGrid | object | Yes      | LayoutGrid object to bind                                  |
| field      | string | Yes      | Grid field to bind, e.g. `sectionSize`, `gutterSize`, or `color` |
| variableId | string | Yes      | Variable ID to bind                                        |
| channel    | string | No       | Target a specific connected file by channel id.            |

### delete_variable [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Delete a single variable (provide `variableId`) or an entire collection (provide `collectionId`). Provide exactly one of the two.

| Name         | Type   | Required | Description                                                       |
| ------------ | ------ | -------- | ----------------------------------------------------------------- |
| variableId   | string | No       | Variable ID to delete                                             |
| collectionId | string | No       | Collection ID to delete (removes all variables in the collection) |

### get_remote_variable_collection

Look up a remote (subscribed-library) variable collection by ID to discover its modes — uses `getVariableCollectionByIdAsync`, which local-only lookups miss.

| Name         | Type   | Required | Description                                     |
| ------------ | ------ | -------- | ----------------------------------------------- |
| collectionId | string | Yes      | Variable collection ID to resolve               |
| channel      | string | No       | Target a specific connected file by channel id. |

### list_library_variable_collections

List all variable collections available from subscribed libraries, including their IDs and modes.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### import_variable_by_key

Import a design variable from a subscribed library by its key, making it available to bind to node properties.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| key     | string | Yes      | Library variable key from the catalog; bare node IDs are rejected |
| channel | string | No       | Target a specific connected file by channel id. |

### get_library_variables

Get all variables in a subscribed library collection by its key. Returns name, resolvedType, and valuesByMode for every variable — use this to read design tokens (colors, spacing, typography) from a subscribed library without opening the library file in Figma.

| Name    | Type   | Required | Description                                                      |
| ------- | ------ | -------- | ---------------------------------------------------------------- |
| key     | string | Yes      | Variable collection key from `list_library_variable_collections` |
| channel | string | No       | Target a specific connected file by channel id.                  |

---

## Write — Prototype

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### set_reactions

Set prototype reactions on a node. Use `mode="replace"` (default) to overwrite all reactions, or `"append"` to add to existing ones.

Supported triggers: `ON_CLICK`, `ON_HOVER`, `ON_PRESS`, `ON_DRAG`, `AFTER_TIMEOUT` (`timeout` ms), `MOUSE_ENTER`, `MOUSE_LEAVE`, `MOUSE_UP`, `MOUSE_DOWN` (`delay` ms), `ON_KEY_DOWN` (`device`, `keyCodes[]`), `ON_MEDIA_HIT` (`mediaHitTime`), `ON_MEDIA_END`

Supported action types: `NODE` (navigation), `BACK`, `CLOSE`, `URL` (`openInNewTab?`), `SET_VARIABLE` (`variableId`), `SET_VARIABLE_MODE` (`variableCollectionId`, `variableModeId`), `CONDITIONAL` (`conditionalBlocks[]`), `UPDATE_MEDIA_RUNTIME` (`mediaAction`)

- `NODE` navigation: `NAVIGATE`, `OVERLAY`, `SCROLL_TO`, `SWAP`, `CHANGE_TO`. Requires `destinationId` (`navigation` defaults to `NAVIGATE`). Optional: `overlayRelativePosition`, `resetScrollPosition`, `resetVideoPosition`, `resetInteractiveComponents`.
- Transitions: `DISSOLVE`, `SMART_ANIMATE`, `SCROLL_ANIMATE`, and directional `MOVE_IN`/`MOVE_OUT`/`PUSH`/`SLIDE_IN`/`SLIDE_OUT` (require `direction` + `matchLayers`). Easings include `EASE_*`, `LINEAR`, `GENTLE`, `QUICK`, `BOUNCY`, `SLOW`, `CUSTOM_CUBIC_BEZIER`, `CUSTOM_SPRING`.
- **Overlay appearance is read-only** in the Plugin API (`overlayPositionType`/`overlayBackground`/`overlayBackgroundInteraction` are configured in the Figma UI on the destination frame). `set_reactions` can open an overlay but not place or style it.

Unknown/future action types pass through unvalidated (forward-compatibility).

| Name      | Type     | Required | Description                                                                               |
| --------- | -------- | -------- | ----------------------------------------------------------------------------------------- |
| nodeId    | string   | Yes      | Node ID                                                                                   |
| reactions | object[] | Yes      | Array of reaction objects. Each has a `trigger` and an `actions` array of Action objects. |
| mode      | string   | No       | `replace` (default) or `append`                                                           |
| channel   | string   | No       | Target a specific connected file by channel id.                                           |

### set_prototype_start

Set the prototype flow starting point(s) for the page containing the given frame(s). `prototypeStartNode` is read-only in the Plugin API; the start of a prototype is controlled through the page's flow starting points. All nodeIds must be on the same page.

| Name    | Type     | Required | Description                                                                  |
| ------- | -------- | -------- | ---------------------------------------------------------------------------- |
| nodeIds | string[] | No\*     | Frame node IDs to set as flow starting points (all on the same page). \*Required unless `mode:"clear"`. |
| names   | string[] | No       | Optional flow names, parallel to nodeIds. Defaults to each frame's name.     |
| mode    | string   | No       | `replace` (default) overwrites the page's starting points; `append` adds new; `remove` drops the given frames and keeps the rest; `clear` removes all (nodeIds optional — uses the page of the first nodeId if given, else the current page) |
| channel | string   | No       | Target a specific connected file by channel id.                              |

### remove_reactions [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Remove prototype reactions from a node. Omit `indices` to remove all reactions. Provide zero-based indices to remove specific reactions (use `get_reactions` first to see current indices).

| Name    | Type     | Required | Description                                                                 |
| ------- | -------- | -------- | --------------------------------------------------------------------------- |
| nodeId  | string   | Yes      | Node ID                                                                     |
| indices | number[] | No       | Zero-based indices of reactions to remove. Omit or pass `[]` to remove all. |

### find_replace_text

Find and replace text content across all TEXT nodes in a subtree. Searches the entire current page if no `nodeId` is given.

| Name       | Type    | Required | Description                                                            |
| ---------- | ------- | -------- | ---------------------------------------------------------------------- |
| find       | string  | Yes      | Text string (or regex when `useRegex=true`) to search for              |
| replace    | string  | Yes      | Replacement string (use empty string to delete matches)                |
| nodeId     | string  | No       | Root node ID to scope the search. Defaults to the entire current page. |
| useRegex   | boolean | No       | Treat `find` as a regular expression (default false)                   |
| regexFlags | string  | No       | Regex flags e.g. `gi` (default `g`)                                    |
| channel    | string  | No       | Target a specific connected file by channel id.                        |

---

## Write — Page

> **Core profile:** every op in this section is a **`batch` op type**, not a top-level tool. Invoke inside `batch(ops:[{ "type": "<op>", … }])` (discover via `search_batch_ops` → `get_batch_op_spec` → `batch(validateOnly:true)`). Set `FIGMA_MCP_TOOL_PROFILE=full` to expose them as full-profile top-level tools.

### navigate_to_page

Switch the active Figma page. Provide either `pageId` or `pageName`.

| Name     | Type   | Required | Description                                     |
| -------- | ------ | -------- | ----------------------------------------------- |
| pageId   | string | No       | Page node ID in colon format e.g. `0:1`         |
| pageName | string | No       | Exact page name to navigate to                  |
| channel  | string | No       | Target a specific connected file by channel id. |

### add_page

Add a new page to the Figma document.

| Name    | Type   | Required | Description                                                               |
| ------- | ------ | -------- | ------------------------------------------------------------------------- |
| name    | string | No       | Name for the new page (default `Page`)                                    |
| index   | number | No       | Position index to insert the page (0 = first). Defaults to last position. |
| channel | string | No       | Target a specific connected file by channel id.                           |

### rename_page [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Rename an existing page in the Figma document.

| Name     | Type   | Required | Description                                         |
| -------- | ------ | -------- | --------------------------------------------------- |
| pageId   | string | No       | Page node ID in colon format                        |
| pageName | string | No       | Current page name to find (alternative to `pageId`) |
| newName  | string | Yes      | New name for the page                               |

### delete_page [BATCH OP]

> **Catalog-backed batch op.** Hidden from top-level tool surfaces where profiles omit it; invoke as a `batch` op `type`. Use `get_batch_op_spec` for the authoritative schema. Pass `channel` on the outer `batch` call, not inside this op's params.

Delete a page from the Figma document. Cannot delete the only remaining page.

| Name     | Type   | Required | Description                                         |
| -------- | ------ | -------- | --------------------------------------------------- |
| pageId   | string | No       | Page node ID in colon format                        |
| pageName | string | No       | Exact page name to delete (alternative to `pageId`) |

---

## Library

> **Core profile:** `fetch_library_catalog` and `list_library_variable_collections` are **core top-level tools**. The `import_*` ops below (`import_component_by_key`, `import_style_by_key`, `import_variable_by_key`, `import_image`) are **`batch` op types** in `core` — invoke via `batch(ops:[…])`, or set `FIGMA_MCP_TOOL_PROFILE=full` for the full top-level surface.

### fetch_library_catalog

Fetch a Figma library's full published catalog via the REST API without needing the file open in Figma. Returns components, component_sets, styles, variables, and variableCollections. Variables require Figma Enterprise plan — a 403 is surfaced as `variablesError`, not a fatal error.

Requires `FIGMA_TOKEN` env (read-only PAT, auto-loaded from `.env`). Writes the full catalog JSON to `outPath`; returns a small handle `{outPath, ndjsonPath, counts, sample}` — query the files with jq/grep, not inline. `ndjsonPath` is a line-per-record `.ndjson` sidecar written beside `outPath` for grep/jq. Fetched component/component-set key types are cached in-process so later `import_component_by_key` calls can route COMPONENT_SET keys without the slow component-first fallback.

| Name    | Type   | Required | Description                                                                                         |
| ------- | ------ | -------- | --------------------------------------------------------------------------------------------------- |
| fileKey | string | Yes      | Figma file key — the segment after `/design/` in the file URL                                       |
| outPath | string | Yes      | File path to write the full catalog JSON to                                                         |
| scope   | string | No       | Which endpoints to fetch: `all` (default), `components`, `component_sets`, `styles`, or `variables` |

---

## Batch

### search_batch_ops

Search the validated `BatchOpCatalog` without loading every op's full schema.
Use this when you know the capability but not the exact op name.

| Name     | Type    | Required | Description                                                            |
| -------- | ------- | -------- | ---------------------------------------------------------------------- |
| query    | string  | No       | Name/description substring to search                                   |
| category | string  | No       | Category filter: `read`, `create`, `modify`, `styles`, `variables`, ... |
| readOnly | boolean | No       | `true` for read-only ops, `false` for non-read-only ops                |
| mutates  | boolean | No       | `true` for mutating ops, `false` for non-mutating ops                  |
| limit    | number  | No       | Max matches. Default 20, max 100                                       |

Returns `{matches, count, total}` with compact op metadata.

### get_batch_op_spec

Return the structured schema for one batch/FigmaPlan op. This is the
authoritative contract for hidden/core-profile write ops and batch-only
ops.

| Name            | Type    | Required | Description                                               |
| --------------- | ------- | -------- | --------------------------------------------------------- |
| op              | string  | Yes      | Batch op name, e.g. `create_frame`, `rename_node`, `map`  |
| includeExamples | boolean | No       | Include example payloads when available. Default false    |

Returns `{name, category, readOnly, mutates, description, paramKeys,
inputSchema}`.

### batch

Execute many ops (writes AND reads) in ONE plugin round-trip. `ops` is an ordered array of `{type, nodeIds?, params}`, where `type` is any `BatchOpCatalog` op name.

Use when you have a known multi-step sequence, a bulk apply, a read chain, or want to write-then-verify inline. In the default `core` profile, most low-level writes are batch/FigmaPlan ops rather than top-level tools. Use core read tools for open-ended exploration, then compose and validate the write plan.

Reads inside a batch are always live and bypass the singleflight cache. Do not use batch as a bypass for heavy catalog reads — use `fetch_library_catalog` or `get_local_components` directly.

**Safety caps.** `batch` fails fast before plugin execution when top-level ops exceed `FIGMA_MCP_BATCH_MAX_OPS` (default `200`) or encoded `ops` exceed `FIGMA_MCP_BATCH_MAX_BYTES` (default `2097152`). Split large work into logical sections; raise caps only for controlled local runs.

**`$N.field` ref resolution.** A string value of the form `$N.field.subfield` in `nodeIds` or `params` resolves to op N's result data at that path before the op runs. Refs may only point to earlier ops (N < current index). Array indices use dot notation: `$0.nodes.0.id`.

**Channel routing.** Put `channel` on the outer `batch` call only. Do not include `channel` in `ops[*].params`; per-op params are validated against `BatchOpCatalog` and `channel` is not part of any op schema.

**Agent presence (2.3.0+).** `origin` is a required enum label (`wolfgang`, `grace`, `theo`, `sunho`, `zoe`, `taewon`, `emma`, `alex`, `rick`) the acting agent stamps on plugin-facing tools so the plugin's multi-agent "Watch agent" panel can show who is working where (avatar + last action + status). Use `wolfgang` for the orchestrator/self; workers use their assigned roster name. Manual `status` and `task` go through `set_presence`, not `batch`; most statuses are auto-derived from the op and cost nothing. Pass the SAME `origin` on every call from one agent. Both are display-only — not routing metadata. (A server predating 2.3.0 rejects unknown top-level params, so only send these to a 2.3.0+ server.) See `references/multi-agent.md § 7`.

**Stop policy.** If any op uses a `$N` ref, the batch stops at the first failure (dependent chain). With no refs, it continues past failures (independent bulk). Override with `continueOnError`.

**`map` op (per-item-varying params).** Use `{type:"map", over, as, do}` to run an inner op once per item of a collection: `over` is a ref to an array (e.g. `$0.matchingNodes`) or a literal array, `as` names the loop binding, and `do` is the op template referencing `$item`/`$index`. Named bindings substitute only as whole-value refs (`"$item.name"`); `"Title $index"` is literal text. `map.do` cannot be another `map`. Capped at 500 items. Use this when each iteration needs _different_ params (vs `[*]` which applies the same value to all).

**`[*]` projection.** A ref like `$0.matchingNodes[*].id` fans an array out as a flat list — e.g. feed a scan's results into one bulk setter, applying the same params to every matched node.

**NOT transactional.** A batch is not atomic — there is no rollback. Earlier ops that succeeded stay applied when a later op fails; resend from the failed index to continue.

**Example — create, style, verify:**

```json
{
  "ops": [
    { "type": "create_frame", "params": { "name": "Card", "width": 320, "height": 200 } },
    { "type": "set_fills", "nodeIds": ["$0.id"], "params": { "color": "#FFFFFF" } },
    { "type": "get_node", "nodeIds": ["$0.id"], "params": { "depth": 1 } }
  ]
}
```

**Example — search then read:**

```json
{
  "ops": [
    { "type": "search_nodes", "params": { "query": "Header", "limit": 1 } },
    { "type": "get_node", "nodeIds": ["$0.nodes.0.id"], "params": { "depth": 2 } }
  ]
}
```

| Name            | Type     | Required | Description                                                                                                                                                            |
| --------------- | -------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ops             | object[] | Yes      | Ordered ops. Each: `{type: string, nodeIds?: string[], params?: object}`. Use `"$N.field"` strings in nodeIds/params to reference op N's result data.                  |
| continueOnError | boolean  | No       | Override the default stop policy: `true` = run all ops and report failures; `false` = stop at first failure. Default: stop when ops use `$N` refs, continue otherwise. |
| validateOnly    | boolean  | No       | Validate the plan and return a report without sending anything to the plugin.                                                                                          |
| channel         | string   | No       | Target a specific connected file by channel id.                                                                                                                        |
| origin          | string   | Yes      | Origin: orchestrator/self=`wolfgang`; workers use assigned roster name (`grace`/`theo`/`sunho`/`zoe`/`taewon`/`emma`/`alex`/`rick`). Pass the same value on every call from one agent. (2.3.0+) |

Returns `{results: [{i, type, data}|{i, type, error}], okCount, failCount, failedAt}`. Large aggregate results spill to disk via the response gate.
