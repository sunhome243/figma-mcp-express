# TOOLS.md — figma-mcp-express tool catalog

All 70 live tools exposed by this fork (16 additional tools available as batch-only demoted ops — see ARCHITECTURE.md). Tools marked **[NEW]** are additions to the upstream base; **[ENHANCED]** means the upstream tool has new params or behavior.

Every tool accepts an optional `channel` param (omitted from most param tables for brevity — see the Channel section). Node IDs are always in colon format, e.g. `4029:12345`, never hyphens.

---

## Channel [NEW]

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

### get_node [ENHANCED]

Get a single node by ID with full detail. Optional `depth` limits traversal depth to avoid MB-scale payloads. Use `get_nodes_info` to fetch multiple nodes in one round-trip. INSTANCE nodes include `mainComponent:{key,name,remote}` and `componentProperties` (the resolved variant map).

| Name                          | Type    | Required | Description                                                                                                         |
| ----------------------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------------------- |
| nodeId                        | string  | Yes      | Node ID in colon format e.g. `4029:12345`                                                                           |
| depth                         | number  | No       | How many levels deep to traverse. `0` = node only (no children), `1` = node + direct children. Default 50 (bounded, not unbounded) — a node returned with `childCount` but no `children` was truncated; request a larger depth. |
| skipInvisibleInstanceChildren | boolean | No       | Skip hidden instances' children. Default false.                                                                     |
| channel                       | string  | No       | Target a specific connected file by channel id.                                                                     |

### get_nodes_info [ENHANCED]

Get full details for multiple nodes by ID in one round-trip. Prefer this over calling `get_node` repeatedly. Large responses spill to disk. INSTANCE nodes include `mainComponent:{key,name,remote}` and `componentProperties` (the resolved variant map).

| Name                          | Type     | Required | Description                                     |
| ----------------------------- | -------- | -------- | ----------------------------------------------- |
| nodeIds                       | string[] | Yes      | List of node IDs in colon format                |
| skipInvisibleInstanceChildren | boolean  | No       | Skip hidden instances' children. Default false. |
| channel                       | string   | No       | Target a specific connected file by channel id. |

### get_design_context [ENHANCED]

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

### get_local_components [ENHANCED]

Get all components defined in the current Figma file. For large libraries, pass `pageId` to scan one page (avoids timeout/jam). Large results spill to disk. componentSets entries include a `defaultVariantKey` (import THAT, not the SET key); component entries include `variantProperties`.

| Name    | Type   | Required | Description                                                                                   |
| ------- | ------ | -------- | --------------------------------------------------------------------------------------------- |
| pageId  | string | No       | Scope scan to a single page by its node ID (colon format e.g. `0:1`). Omit to scan all pages. |
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
| layoutMode             | string | No       | Auto-layout direction: `HORIZONTAL`, `VERTICAL`, or `NONE`                    |
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

### create_instance [NEW]

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

### import_image [NEW]

Import an image file from disk (or raw base64 data) into Figma as a new image-fill rectangle. The server reads `imagePath` from the local filesystem and base64-encodes it automatically — prefer this over passing raw `imageData`. Returns the new node ID and bounds.

| Name        | Type   | Required | Description                                                                              |
| ----------- | ------ | -------- | ---------------------------------------------------------------------------------------- |
| imagePath   | string | No       | Absolute path to an image file on the server's filesystem (PNG, JPG, GIF, SVG, WEBP). Preferred over `imageData`. |
| imageData   | string | No       | Raw base64-encoded image data. Use only when `imagePath` is unavailable.                 |
| x           | number | No       | X position (default 0)                                                                   |
| y           | number | No       | Y position (default 0)                                                                   |
| width       | number | No       | Width in pixels (default 200)                                                            |
| height      | number | No       | Height in pixels (default 200)                                                           |
| name        | string | No       | Node name shown in the layers panel                                                      |
| scaleMode   | string | No       | Image fill scale mode: `FILL` (default), `FIT`, `CROP`, or `TILE`                        |
| parentId    | string | No       | Parent node ID. Defaults to current page.                                                |
| channel     | string | No       | Target a specific connected file by channel id.                                          |

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

### rename_node [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Rename a single node by ID. Returns the updated node with its new name. Use `batch_rename_nodes` to rename multiple nodes at once.

| Name    | Type   | Required | Description                                                                     |
| ------- | ------ | -------- | ------------------------------------------------------------------------------- |
| nodeId  | string | Yes      | Node ID in colon format                                                         |
| name    | string | Yes      | New name. Figma supports slash-separated path notation e.g. `Icons/Arrow/Left`. |
| channel | string | No       | Target a specific connected file by channel id.                                 |

### delete_nodes

Delete one or more nodes. This cannot be undone via MCP — use with care. Returns a per-node result array of `{nodeId, deleted}` or `{nodeId, error}` and does NOT abort when one node is un-removable (e.g. an instance child, which Figma natively refuses); that node's error carries an intent-ordered hint — delete on the master to propagate, or `detach_instance` then delete the resulting frame, to remove; `swap_component` to replace; `set_visible:false` only hides.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to delete                              |
| channel | string   | No       | Target a specific connected file by channel id. |

### lock_nodes [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Lock one or more nodes to prevent accidental edits in Figma.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to lock                                |
| channel | string   | No       | Target a specific connected file by channel id. |

### unlock_nodes [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Unlock one or more nodes, allowing them to be edited again.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to unlock                              |
| channel | string   | No       | Target a specific connected file by channel id. |

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

### ungroup_nodes [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Ungroup one or more GROUP nodes, moving their children to the parent and removing the group.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | GROUP node IDs                                  |
| channel | string   | No       | Target a specific connected file by channel id. |

### group_nodes

Group two or more nodes into a GROUP. All nodes must share the same parent.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs to group (minimum 2)                   |
| name    | string   | No       | Optional name for the new group                 |
| channel | string   | No       | Target a specific connected file by channel id. |

### reorder_nodes [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Change the z-order (layer stack position) of one or more nodes.

| Name    | Type     | Required | Description                                                     |
| ------- | -------- | -------- | --------------------------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs                                                        |
| order   | string   | Yes      | `bringToFront`, `sendToBack`, `bringForward`, or `sendBackward` |
| channel | string   | No       | Target a specific connected file by channel id.                 |

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

### boolean_operation [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Combine two or more vector nodes using a boolean operation, producing a new merged vector shape. The source nodes are consumed. Flatten first if the inputs are not already vectors.

| Name      | Type     | Required | Description                                                                                  |
| --------- | -------- | -------- | -------------------------------------------------------------------------------------------- |
| nodeIds   | string[] | Yes      | Two or more vector node IDs to combine                                                       |
| operation | string   | Yes      | `UNION`, `SUBTRACT`, `INTERSECT`, `EXCLUDE`, or `FLATTEN`                                   |
| name      | string   | No       | Name for the resulting node                                                                  |
| parentId  | string   | No       | Parent node ID for the result. Defaults to the parent of the first input node.               |
| channel   | string   | No       | Target a specific connected file by channel id.                                              |

---

## Write — Components [NEW/ENHANCED]

### swap_component

Swap the main component of an existing INSTANCE node, replacing it with a different component while keeping position and size. Uses Figma's override-preserving `swapComponent()` (not `mainComponent=`), so text and variant overrides survive the swap.

| Name        | Type   | Required | Description                                            |
| ----------- | ------ | -------- | ------------------------------------------------------ |
| nodeId      | string | Yes      | INSTANCE node ID                                       |
| componentId | string | Yes      | Target COMPONENT node ID (from `get_local_components`) |
| channel     | string | No       | Target a specific connected file by channel id.        |

### detach_instance [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Detach one or more component instances, converting them to plain frames. The link to the main component is broken; all visual properties are preserved.

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | INSTANCE node IDs                               |
| channel | string   | No       | Target a specific connected file by channel id. |

### set_instance_properties [NEW]

Set variant, boolean, text, and instance-swap properties on a component INSTANCE. Use `resetOverrides=true` to restore defaults before applying. SLOT-type keys are auto-filtered (passing them throws `cannotSetSlotProperty`, which would poison the whole update) and reported back as `droppedSlotKeys`; fonts for TEXT properties are preloaded automatically.

| Name           | Type    | Required | Description                                                                         |
| -------------- | ------- | -------- | ----------------------------------------------------------------------------------- |
| nodeId         | string  | Yes      | INSTANCE node ID                                                                    |
| properties     | object  | Yes      | Property map e.g. `{"State": "On", "Label#1:0": "Save"}`                            |
| resetOverrides | boolean | No       | Reset the instance to component defaults before applying properties (default false) |
| channel        | string  | No       | Target a specific connected file by channel id.                                     |

### import_component_by_key [NEW]

Import a component (or component set) from a subscribed library by its key, making it available to instantiate. For a COMPONENT_SET key pass `assetType='COMPONENT_SET'`.

| Name      | Type   | Required | Description                                                     |
| --------- | ------ | -------- | --------------------------------------------------------------- |
| key       | string | Yes      | Library component key (from the library catalog, not a node ID) |
| assetType | string | No       | Asset type hint: `COMPONENT` (default) or `COMPONENT_SET`       |
| channel   | string | No       | Target a specific connected file by channel id.                 |

---

## Write — Styles

### apply_style_to_node

Apply an existing local style (paint, text, effect, or grid) to a node, linking the node to that style.

| Name    | Type   | Required | Description                                                   |
| ------- | ------ | -------- | ------------------------------------------------------------- |
| nodeId  | string | Yes      | Target node ID                                                |
| styleId | string | Yes      | Style ID to apply (from `get_styles`)                         |
| target  | string | No       | For paint styles only — apply to `fill` (default) or `stroke` |
| channel | string | No       | Target a specific connected file by channel id.               |

### set_fills [ENHANCED]

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

### set_strokes [ENHANCED]

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

Apply one or more effects (drop shadow, inner shadow, layer blur, background blur) directly to a node. Replaces all existing effects. Pass an empty array to clear all effects.

| Name    | Type     | Required | Description                                                                                                                                                 |
| ------- | -------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| nodeId  | string   | Yes      | Target node ID                                                                                                                                              |
| effects | object[] | Yes      | Array of effect objects. Each has: `type`, `radius`, `color` (hex, shadows only), `opacity` (0–1), `offsetX`, `offsetY`, `spread`, `visible` (default true) |
| channel | string   | No       | Target a specific connected file by channel id.                                                                                                             |

### set_blend_mode [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Set the blend mode of one or more nodes.

| Name      | Type     | Required | Description                                                                                                                                                                                                       |
| --------- | -------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| nodeIds   | string[] | Yes      | Node IDs                                                                                                                                                                                                          |
| blendMode | string   | Yes      | `NORMAL`, `MULTIPLY`, `SCREEN`, `OVERLAY`, `DARKEN`, `LIGHTEN`, `COLOR_DODGE`, `COLOR_BURN`, `HARD_LIGHT`, `SOFT_LIGHT`, `DIFFERENCE`, `EXCLUSION`, `HUE`, `SATURATION`, `COLOR`, `LUMINOSITY`, or `PASS_THROUGH` |
| channel   | string   | No       | Target a specific connected file by channel id.                                                                                                                                                                   |

### set_opacity

Set the opacity of one or more nodes (0 = fully transparent, 1 = fully opaque).

| Name    | Type     | Required | Description                                     |
| ------- | -------- | -------- | ----------------------------------------------- |
| nodeIds | string[] | Yes      | Node IDs                                        |
| opacity | number   | Yes      | Opacity value between 0 and 1                   |
| channel | string   | No       | Target a specific connected file by channel id. |

### set_corner_radius [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Set corner radius on one or more nodes. Provide a uniform `cornerRadius` or per-corner values (`topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`). When both uniform and per-corner values are supplied, per-corner values take precedence.

| Name              | Type     | Required | Description                                     |
| ----------------- | -------- | -------- | ----------------------------------------------- |
| nodeIds           | string[] | Yes      | Node IDs                                        |
| cornerRadius      | number   | No       | Uniform corner radius applied to all corners    |
| topLeftRadius     | number   | No       | Top-left corner radius                          |
| topRightRadius    | number   | No       | Top-right corner radius                         |
| bottomLeftRadius  | number   | No       | Bottom-left corner radius                       |
| bottomRightRadius | number   | No       | Bottom-right corner radius                      |
| channel           | string   | No       | Target a specific connected file by channel id. |

### set_constraints [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Set layout constraints (pinning behaviour) on one or more nodes relative to their parent.

| Name       | Type     | Required | Description                                                  |
| ---------- | -------- | -------- | ------------------------------------------------------------ |
| nodeIds    | string[] | Yes      | Node IDs                                                     |
| horizontal | string   | No       | `MIN` (left), `MAX` (right), `CENTER`, `STRETCH`, or `SCALE` |
| vertical   | string   | No       | `MIN` (top), `MAX` (bottom), `CENTER`, `STRETCH`, or `SCALE` |
| channel    | string   | No       | Target a specific connected file by channel id.              |

### rotate_nodes [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Rotate one or more nodes to an absolute angle in degrees.

| Name     | Type     | Required | Description                                                       |
| -------- | -------- | -------- | ----------------------------------------------------------------- |
| nodeIds  | string[] | Yes      | Node IDs                                                          |
| rotation | number   | Yes      | Rotation angle in degrees (positive = counter-clockwise in Figma) |
| channel  | string   | No       | Target a specific connected file by channel id.                   |

### resize_nodes [ENHANCED]

Resize one or more nodes and/or set their sizing-within-parent (FILL/HUG). FILL/HUG requires the node to be inside an auto-layout parent.

| Name                   | Type     | Required | Description                                                                |
| ---------------------- | -------- | -------- | -------------------------------------------------------------------------- |
| nodeIds                | string[] | Yes      | Node IDs                                                                   |
| width                  | number   | No       | New width in pixels                                                        |
| height                 | number   | No       | New height in pixels                                                       |
| layoutSizingHorizontal | string   | No       | `FIXED`, `HUG`, or `FILL`                                                  |
| layoutSizingVertical   | string   | No       | `FIXED`, `HUG`, or `FILL`                                                  |
| layoutGrow             | number   | No       | Grow factor along the parent's main axis                                   |
| layoutAlign            | string   | No       | Cross-axis self-alignment: `MIN`, `CENTER`, `MAX`, `STRETCH`, or `INHERIT` |
| layoutPositioning      | string   | No       | `AUTO` or `ABSOLUTE`                                                       |
| channel                | string   | No       | Target a specific connected file by channel id.                            |

### set_auto_layout [ENHANCED]

Set or update auto-layout (flex) properties on an existing frame.

| Name                  | Type   | Required | Description                                                         |
| --------------------- | ------ | -------- | ------------------------------------------------------------------- |
| nodeId                | string | Yes      | Frame node ID                                                       |
| layoutMode            | string | No       | `HORIZONTAL`, `VERTICAL`, or `NONE`                                 |
| paddingTop            | number | No       | Top padding                                                         |
| paddingRight          | number | No       | Right padding                                                       |
| paddingBottom         | number | No       | Bottom padding                                                      |
| paddingLeft           | number | No       | Left padding                                                        |
| itemSpacing           | number | No       | Gap between children                                                |
| primaryAxisAlignItems | string | No       | `MIN`, `CENTER`, `MAX`, or `SPACE_BETWEEN`                          |
| counterAxisAlignItems | string | No       | `MIN`, `CENTER`, `MAX`, or `BASELINE`                               |
| primaryAxisSizingMode | string | No       | `FIXED` or `AUTO` (hug)                                             |
| counterAxisSizingMode | string | No       | `FIXED` or `AUTO` (hug)                                             |
| layoutWrap            | string | No       | `NO_WRAP` or `WRAP`                                                 |
| counterAxisSpacing      | number | No       | Gap between wrapped rows/columns (only when `layoutWrap` is `WRAP`) |
| paddingTopVariableId    | string | No       | Variable ID to bind to `paddingTop` instead of a raw value          |
| paddingRightVariableId  | string | No       | Variable ID to bind to `paddingRight` instead of a raw value        |
| paddingBottomVariableId | string | No       | Variable ID to bind to `paddingBottom` instead of a raw value       |
| paddingLeftVariableId   | string | No       | Variable ID to bind to `paddingLeft` instead of a raw value         |
| itemSpacingVariableId   | string | No       | Variable ID to bind to `itemSpacing` instead of a raw value         |
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

Create a new local effect style (drop shadow, inner shadow, or blur). Pass `effects[]` for full control over multiple effects including `blendMode` and `showShadowBehindNode` — takes precedence over the individual shorthand params when provided.

| Name                | Type     | Required | Description                                                                                                                                                  |
| ------------------- | -------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| name                | string   | Yes      | Style name e.g. `Shadow/Card`                                                                                                                                |
| type                | string   | No       | `DROP_SHADOW` (default), `INNER_SHADOW`, `LAYER_BLUR`, or `BACKGROUND_BLUR`                                                                                 |
| color               | string   | No       | Shadow color as hex (default `#000000`, shadows only)                                                                                                        |
| opacity             | number   | No       | Shadow color opacity 0–1 (default 0.25, shadows only)                                                                                                        |
| radius              | number   | No       | Blur radius in pixels                                                                                                                                        |
| offsetX             | number   | No       | Shadow X offset (shadows only)                                                                                                                               |
| offsetY             | number   | No       | Shadow Y offset (default 4, shadows only)                                                                                                                    |
| spread              | number   | No       | Shadow spread (default 0, shadows only)                                                                                                                      |
| effects             | object[] | No       | Full effect array (Figma Effect objects). Takes precedence over shorthand params. Each object supports `type`, `radius`, `color`, `opacity`, `offsetX`, `offsetY`, `spread`, `visible`, `blendMode`, `showShadowBehindNode`. |
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

### delete_style [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Delete a style (paint, text, effect, or grid) by its ID.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| styleId | string | Yes      | Style ID to delete                              |
| channel | string | No       | Target a specific connected file by channel id. |

### import_style_by_key [NEW]

Import a paint, text, or effect style from a subscribed library by its key, making it available to apply to nodes.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| key     | string | Yes      | Library style key (from the library catalog)    |
| channel | string | No       | Target a specific connected file by channel id. |

---

## Write — Variables

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

### add_variable_mode

Add a new mode to an existing variable collection (e.g. Light/Dark, Desktop/Mobile). Requires a paid Figma plan — free plan returns `Limited to 1 modes only`.

| Name         | Type   | Required | Description                                     |
| ------------ | ------ | -------- | ----------------------------------------------- |
| collectionId | string | Yes      | Variable collection ID                          |
| modeName     | string | Yes      | Name for the new mode                           |
| channel      | string | No       | Target a specific connected file by channel id. |

### set_variable_mode [ENHANCED]

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

### bind_variable_to_node

Bind a local variable to a node property so the property is driven by the variable's value. For `fillColor`/`strokeColor` the binding is applied to paint index 0 while preserving the remaining paints (no collapse); the base paint at index 0 must be SOLID.

| Name       | Type   | Required | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------- | ------ | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| nodeId     | string | Yes      | Target node ID                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| variableId | string | Yes      | Variable ID to bind (from `get_variable_defs`)                                                                                                                                                                                                                                                                                                                                                                                                               |
| field      | string | Yes      | Property to bind: `fillColor`, `strokeColor`, `visible`, `characters`, `opacity`, `width`, `height`, `minWidth`, `maxWidth`, `minHeight`, `maxHeight`, `topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`, `strokeWeight`, `strokeTopWeight`, `strokeRightWeight`, `strokeBottomWeight`, `strokeLeftWeight`, `itemSpacing`, `counterAxisSpacing`, `gridRowGap`, `gridColumnGap`, `paddingTop`, `paddingRight`, `paddingBottom`, `paddingLeft`. NOT bindable: `cornerRadius`, `rotation`, `x`, `y`. |
| channel    | string | No       | Target a specific connected file by channel id.                                                                                                                                                                                                                                                                                                                                                                                                              |

### delete_variable [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Delete a single variable (provide `variableId`) or an entire collection (provide `collectionId`). Provide exactly one of the two.

| Name         | Type   | Required | Description                                                       |
| ------------ | ------ | -------- | ----------------------------------------------------------------- |
| variableId   | string | No       | Variable ID to delete                                             |
| collectionId | string | No       | Collection ID to delete (removes all variables in the collection) |
| channel      | string | No       | Target a specific connected file by channel id.                   |

### get_remote_variable_collection [NEW]

Look up a remote (subscribed-library) variable collection by ID to discover its modes — uses `getVariableCollectionByIdAsync`, which local-only lookups miss.

| Name         | Type   | Required | Description                                     |
| ------------ | ------ | -------- | ----------------------------------------------- |
| collectionId | string | Yes      | Variable collection ID to resolve               |
| channel      | string | No       | Target a specific connected file by channel id. |

### list_library_variable_collections [NEW]

List all variable collections available from subscribed libraries, including their IDs and modes.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| channel | string | No       | Target a specific connected file by channel id. |

### import_variable_by_key [NEW]

Import a design variable from a subscribed library by its key, making it available to bind to node properties.

| Name    | Type   | Required | Description                                     |
| ------- | ------ | -------- | ----------------------------------------------- |
| key     | string | Yes      | Library variable key (from the library catalog) |
| channel | string | No       | Target a specific connected file by channel id. |

### get_library_variables [NEW]

Get all variables in a subscribed library collection by its key. Returns name, resolvedType, and valuesByMode for every variable — use this to read design tokens (colors, spacing, typography) from a subscribed library without opening the library file in Figma.

| Name    | Type   | Required | Description                                                      |
| ------- | ------ | -------- | ---------------------------------------------------------------- |
| key     | string | Yes      | Variable collection key from `list_library_variable_collections` |
| channel | string | No       | Target a specific connected file by channel id.                  |

---

## Write — Prototype

### set_reactions

Set prototype reactions on a node. Use `mode="replace"` (default) to overwrite all reactions, or `"append"` to add to existing ones.

Supported triggers: `ON_CLICK`, `ON_HOVER`, `ON_PRESS`, `ON_DRAG`, `AFTER_TIMEOUT`, `MOUSE_ENTER`, `MOUSE_LEAVE`, `MOUSE_UP`, `MOUSE_DOWN`

Supported action types: `NODE` (navigation), `BACK`, `CLOSE`, `URL`

| Name      | Type     | Required | Description                                                                               |
| --------- | -------- | -------- | ----------------------------------------------------------------------------------------- |
| nodeId    | string   | Yes      | Node ID                                                                                   |
| reactions | object[] | Yes      | Array of reaction objects. Each has a `trigger` and an `actions` array of Action objects. |
| mode      | string   | No       | `replace` (default) or `append`                                                           |
| channel   | string   | No       | Target a specific connected file by channel id.                                           |

### remove_reactions [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Remove prototype reactions from a node. Omit `indices` to remove all reactions. Provide zero-based indices to remove specific reactions (use `get_reactions` first to see current indices).

| Name    | Type     | Required | Description                                                                 |
| ------- | -------- | -------- | --------------------------------------------------------------------------- |
| nodeId  | string   | Yes      | Node ID                                                                     |
| indices | number[] | No       | Zero-based indices of reactions to remove. Omit or pass `[]` to remove all. |
| channel | string   | No       | Target a specific connected file by channel id.                             |

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

### rename_page [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Rename an existing page in the Figma document.

| Name     | Type   | Required | Description                                         |
| -------- | ------ | -------- | --------------------------------------------------- |
| pageId   | string | No       | Page node ID in colon format                        |
| pageName | string | No       | Current page name to find (alternative to `pageId`) |
| newName  | string | Yes      | New name for the page                               |
| channel  | string | No       | Target a specific connected file by channel id.     |

### delete_page [BATCH-ONLY]

> **Demoted — not on `tools/list`. Invoke only as a `batch` op `type`.** Params below pass through verbatim.

Delete a page from the Figma document. Cannot delete the only remaining page.

| Name     | Type   | Required | Description                                         |
| -------- | ------ | -------- | --------------------------------------------------- |
| pageId   | string | No       | Page node ID in colon format                        |
| pageName | string | No       | Exact page name to delete (alternative to `pageId`) |
| channel  | string | No       | Target a specific connected file by channel id.     |

---

## Library [NEW]

### fetch_library_catalog

Fetch a Figma library's full published catalog via the REST API without needing the file open in Figma. Returns components, component_sets, styles, variables, and variableCollections. Variables require Figma Enterprise plan — a 403 is surfaced as `variablesError`, not a fatal error.

Requires `FIGMA_TOKEN` env (read-only PAT, auto-loaded from `.env`). Writes the full catalog JSON to `outPath`; returns a small handle `{outPath, ndjsonPath, counts, sample}` — query the files with jq/grep, not inline. `ndjsonPath` is a line-per-record `.ndjson` sidecar written beside `outPath` for grep/jq.

| Name    | Type   | Required | Description                                                                                         |
| ------- | ------ | -------- | --------------------------------------------------------------------------------------------------- |
| fileKey | string | Yes      | Figma file key — the segment after `/design/` in the file URL                                       |
| outPath | string | Yes      | File path to write the full catalog JSON to                                                         |
| scope   | string | No       | Which endpoints to fetch: `all` (default), `components`, `component_sets`, `styles`, or `variables` |

---

## Batch [NEW]

### batch

Execute many ops (writes AND reads) in ONE plugin round-trip. `ops` is an ordered array of `{type, nodeIds?, params}`, where `type` is any tool name.

Use when you have a known multi-step sequence, a bulk apply, a read chain, or want to write-then-verify inline. For single fine adjustments or open-ended exploration, call the specific tool directly.

Reads inside a batch are always live and bypass the singleflight cache. Do not use batch as a bypass for heavy catalog reads — use `fetch_library_catalog` or `get_local_components` directly.

**`$N.field` ref resolution.** A string value of the form `$N.field.subfield` in `nodeIds` or `params` resolves to op N's result data at that path before the op runs. Refs may only point to earlier ops (N < current index). Array indices use dot notation: `$0.nodes.0.id`.

**Stop policy.** If any op uses a `$N` ref, the batch stops at the first failure (dependent chain). With no refs, it continues past failures (independent bulk). Override with `continueOnError`.

**`map` op (per-item-varying params).** Use `{type:"map", over, as, do}` to run an inner op once per item of a collection: `over` is a ref to an array (e.g. `$0.matchingNodes`), `as` names the loop binding, and `do` is the op template referencing `$item`/`$index`. Capped at 500 items. Use this when each iteration needs _different_ params (vs `[*]` which applies the same value to all).

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
| channel         | string   | No       | Target a specific connected file by channel id.                                                                                                                        |

Returns `{results: [{i, type, data}|{i, type, error}], okCount, failCount, failedAt}`. Large aggregate results spill to disk via the response gate.
