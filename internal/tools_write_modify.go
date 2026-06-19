package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteModifyTools(s *server.MCPServer, node *Node) {
	setTextOpts := []mcp.ToolOption{
		mcp.WithDescription("Update the content and/or styling of an existing TEXT node. Provide `text` to change content; provide any styling param (alignment, autoResize, font, spacing, case, decoration) to restyle. At least one is required. Font-dependent changes load the font automatically."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("TEXT node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("text",
			mcp.Description("New text content (optional — omit to restyle without changing the text)"),
		),
	}
	setTextOpts = append(setTextOpts, textFontParams()...)
	setTextOpts = append(setTextOpts, textStyleParams()...)
	setTextOpts = append(setTextOpts, channelParam())
	s.AddTool(mcp.NewTool("set_text", setTextOpts...),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			nodeID, _ := req.GetArguments()["nodeId"].(string)
			params := map[string]interface{}{}
			if t, ok := req.GetArguments()["text"]; ok {
				params["text"] = t
			}
			copyParams(req, params, textStyleKeys)
			resp, err := node.Send(ctx, "set_text", []string{nodeID}, withChannel(req, params))
			return renderResponse(resp, err)
		})

	s.AddTool(mcp.NewTool("set_text_range",
		mcp.WithDescription("Apply styling to a CHARACTER RANGE within a TEXT node (per-span formatting): mixed fonts/sizes, per-span color, hyperlinks, lists, indentation, decoration. Offsets are character indices with 0 <= startOffset < endOffset <= text length. Fonts covering the range are loaded automatically. Use set_text for whole-node changes."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("TEXT node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithNumber("startOffset",
			mcp.Required(),
			mcp.Description("Start character index (inclusive, 0-based)"),
		),
		mcp.WithNumber("endOffset",
			mcp.Required(),
			mcp.Description("End character index (exclusive). Must be > startOffset and <= text length."),
		),
		mcp.WithString("fontFamily", mcp.Description("Font family for the range (e.g. 'Inter'). Loaded automatically.")),
		mcp.WithString("fontStyle", mcp.Description("Font style for the range (e.g. 'Bold', 'Italic'). Loaded automatically.")),
		mcp.WithNumber("fontSize", mcp.Description("Font size in pixels for the range")),
		mcp.WithString("color", mcp.Description("Text color for the range as hex e.g. #FF0000 (sets a solid fill on the span)")),
		mcp.WithString("textCase", mcp.Description("Text case for the range: ORIGINAL, UPPER, LOWER, TITLE, SMALL_CAPS, or SMALL_CAPS_FORCED")),
		mcp.WithString("textDecoration", mcp.Description("Decoration for the range: NONE, UNDERLINE, or STRIKETHROUGH")),
		mcp.WithNumber("letterSpacingValue", mcp.Description("Letter spacing value for the range (unit via letterSpacingUnit)")),
		mcp.WithString("letterSpacingUnit", mcp.Description("Letter spacing unit: PIXELS (default) or PERCENT")),
		mcp.WithNumber("lineHeightValue", mcp.Description("Line height value for the range (unit via lineHeightUnit)")),
		mcp.WithString("lineHeightUnit", mcp.Description("Line height unit: PIXELS (default), PERCENT, or AUTO")),
		mcp.WithObject("hyperlink",
			mcp.Description("Hyperlink for the range: {url:\"https://…\"} for a web link, or {nodeId:\"1:23\"} for an in-file link. Omit to leave unchanged; pass null to clear."),
		),
		mcp.WithObject("listOptions",
			mcp.Description("List formatting for the range: {type:\"ORDERED\"|\"UNORDERED\"|\"NONE\"}"),
		),
		mcp.WithNumber("indentation", mcp.Description("Indentation level for the range (0-based)")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{}
		for _, k := range setTextRangeKeys {
			if v, ok := args[k]; ok {
				params[k] = v
			}
		}
		resp, err := node.Send(ctx, "set_text_range", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_fills",
		mcp.WithDescription("Set the fill color on a single node (this top-level tool takes one nodeId, not an array). Returns {results:[{nodeId,…}]}, a 1-element array. PREFER variableId over a raw hex — bind a design token, don't bake a raw color (the project invariant). Use mode='append' to stack a new fill on top of existing fills instead of replacing them. "+
			"Also a `batch` op type — and in `batch` it accepts nodeIds[] (all→all bulk: the same fill applied to every node) and returns {results:[{nodeId,…}]}. Fan a scan into it in ONE round-trip with the projection ref nodeIds:[\"$0.matchingNodes[*].id\"]."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("color",
			mcp.Description("Solid fill color as hex: #RRGGBB e.g. #FF5733 or #RRGGBBAA e.g. #FF573380 for 50% alpha. Required unless paints[] is given."),
		),
		mcp.WithNumber("opacity", mcp.Description("Fill opacity 0–1 (default 1). Combines multiplicatively with any alpha in the color hex.")),
		mcp.WithString("mode", mcp.Description("'replace' (default) overwrites all existing fills; 'append' stacks the new fill(s) on top of existing ones")),
		mcp.WithString("variableId", mcp.Description("Optional design variable ID to bind the fill color to (setBoundVariableForPaint). When provided, the fill is token-driven and the color hex acts as a fallback.")),
		mcp.WithArray("paints",
			mcp.Description("Full Paint objects array for advanced fills (gradient/image/mixed solids), applied to the node verbatim and taking precedence over color. For a REUSABLE/tokenized fill prefer create_paint_style(paints[]) + apply_style_to_node; this is the one-off direct path. Each item is a valid Figma Paint object e.g. {type:'GRADIENT_LINEAR', gradientStops:[…], gradientTransform:[…]}."),
			mcp.Items(map[string]any{"type": "object"}),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{}
		if c, ok := req.GetArguments()["color"]; ok {
			params["color"] = c
		}
		if op, ok := req.GetArguments()["opacity"].(float64); ok {
			params["opacity"] = op
		}
		if m, ok := req.GetArguments()["mode"].(string); ok {
			params["mode"] = m
		}
		if vid, ok := req.GetArguments()["variableId"].(string); ok && vid != "" {
			params["variableId"] = vid
		}
		if paints, ok := req.GetArguments()["paints"]; ok {
			params["paints"] = paints
		}
		resp, err := node.Send(ctx, "set_fills", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_strokes",
		mcp.WithDescription("Set the stroke color and weight on a single node (this top-level tool takes one nodeId, not an array). Returns {results:[{nodeId,…}]}, a 1-element array. PREFER variableId over a raw hex — bind a design token. Use mode='append' to stack a new stroke on top of existing strokes instead of replacing them. "+
			"Also a `batch` op type — and in `batch` it accepts nodeIds[] (all→all bulk: the same stroke applied to every node) and returns {results:[{nodeId,…}]}. Fan a scan into it in ONE round-trip with the projection ref nodeIds:[\"$0.matchingNodes[*].id\"]."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("color",
			mcp.Description("Solid stroke color as hex e.g. #000000. Required unless paints[] is given."),
		),
		mcp.WithNumber("strokeWeight", mcp.Description("Stroke weight in pixels (default 1)")),
		mcp.WithString("mode", mcp.Description("'replace' (default) overwrites all strokes; 'append' stacks on top of existing strokes")),
		mcp.WithString("variableId", mcp.Description("Optional design variable ID to bind the stroke color to (setBoundVariableForPaint). When provided, the stroke is token-driven and the color hex acts as a fallback.")),
		mcp.WithArray("paints",
			mcp.Description("Full Paint objects array for advanced strokes (gradient/image/mixed solids), applied verbatim and taking precedence over color. For a reusable/tokenized stroke prefer create_paint_style(paints[]) + apply_style_to_node(target:'stroke'); this is the one-off direct path."),
			mcp.Items(map[string]any{"type": "object"}),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{}
		if c, ok := req.GetArguments()["color"]; ok {
			params["color"] = c
		}
		if sw, ok := req.GetArguments()["strokeWeight"].(float64); ok {
			params["strokeWeight"] = sw
		}
		if m, ok := req.GetArguments()["mode"].(string); ok {
			params["mode"] = m
		}
		if vid, ok := req.GetArguments()["variableId"].(string); ok && vid != "" {
			params["variableId"] = vid
		}
		if paints, ok := req.GetArguments()["paints"]; ok {
			params["paints"] = paints
		}
		resp, err := node.Send(ctx, "set_strokes", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("move_nodes",
		mcp.WithDescription("Move one or more nodes to an absolute canvas position. The same x/y is applied to every node independently (not a relative offset from current position). Note: x/y are parent-local coordinates, not absolute canvas coordinates."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("x", mcp.Description("Target X position")),
		mcp.WithNumber("y", mcp.Description("Target Y position")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if x, ok := req.GetArguments()["x"].(float64); ok {
			params["x"] = x
		}
		if y, ok := req.GetArguments()["y"].(float64); ok {
			params["y"] = y
		}
		resp, err := node.Send(ctx, "move_nodes", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	resizeOpts := []mcp.ToolOption{
		mcp.WithDescription("Resize one or more nodes and/or set their sizing-within-parent (FILL/HUG). Width/height is applied to every node independently. Provide width, height, a layout-sizing param, or any combination — FILL/HUG needs the node to be inside an auto-layout parent."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("width", mcp.Description("New width in pixels")),
		mcp.WithNumber("height", mcp.Description("New height in pixels")),
	}
	resizeOpts = append(resizeOpts, layoutSizingParams()...)
	resizeOpts = append(resizeOpts, channelParam())
	s.AddTool(mcp.NewTool("resize_nodes", resizeOpts...),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			raw, _ := req.GetArguments()["nodeIds"].([]interface{})
			nodeIDs := toStringSlice(raw)
			params := map[string]interface{}{}
			if w, ok := req.GetArguments()["width"].(float64); ok {
				params["width"] = w
			}
			if h, ok := req.GetArguments()["height"].(float64); ok {
				params["height"] = h
			}
			copyParams(req, params, layoutSizingKeys)
			resp, err := node.Send(ctx, "resize_nodes", nodeIDs, withChannel(req, params))
			return renderResponse(resp, err)
		})

	// LEVER 4 (tool demotion) — rename_node is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "rename_node" to the untouched plugin handler. Uncomment to restore.
	// s.AddTool(mcp.NewTool("rename_node",
	// 	mcp.WithDescription("Rename a single node by ID. Returns the updated node with its new name. Use batch_rename_nodes to rename multiple nodes at once or to apply find/replace patterns across many nodes. Also a `batch` op type."),
	// 	mcp.WithString("nodeId",
	// 		mcp.Required(),
	// 		mcp.Description("Node ID in colon format e.g. '4029:12345'"),
	// 	),
	// 	mcp.WithString("name",
	// 		mcp.Required(),
	// 		mcp.Description("New name for the node. Figma supports slash-separated path notation e.g. 'Icons/Arrow/Left' to organise nodes in component panels."),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	nodeID, _ := req.GetArguments()["nodeId"].(string)
	// 	name, _ := req.GetArguments()["name"].(string)
	// 	resp, err := node.Send(ctx, "rename_node", []string{nodeID}, withChannel(req, map[string]interface{}{"name": name}))
	// 	return renderResponse(resp, err)
	// })

	s.AddTool(mcp.NewTool("clone_node",
		mcp.WithDescription("Clone an existing node, optionally repositioning it or placing it in a new parent."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Source node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithNumber("x", mcp.Description("X position of the clone")),
		mcp.WithNumber("y", mcp.Description("Y position of the clone")),
		mcp.WithString("parentId", mcp.Description("Parent node ID for the clone. Defaults to same parent as source.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{}
		if x, ok := req.GetArguments()["x"].(float64); ok {
			params["x"] = x
		}
		if y, ok := req.GetArguments()["y"].(float64); ok {
			params["y"] = y
		}
		if pid, ok := req.GetArguments()["parentId"].(string); ok && pid != "" {
			params["parentId"] = pid
		}
		resp, err := node.Send(ctx, "clone_node", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_file_thumbnail",
		mcp.WithDescription("Set or clear the current file thumbnail node. Omit nodeId to clear the thumbnail."),
		mcp.WithString("nodeId", mcp.Description("FRAME, COMPONENT, COMPONENT_SET, or SECTION node ID to use as the file thumbnail. Omit to clear.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if id, ok := req.GetArguments()["nodeId"].(string); ok && id != "" {
			params["nodeId"] = NormalizeNodeID(id)
		}
		resp, err := node.Send(ctx, "set_file_thumbnail", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("add_dev_resource",
		mcp.WithDescription("Attach a Dev Mode resource link to a node using addDevResourceAsync."),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Node ID in colon format e.g. '4029:12345'.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Resource URL.")),
		mcp.WithString("name", mcp.Description("Optional display name for the resource.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{
			"nodeId": NormalizeNodeID(args["nodeId"].(string)),
			"url":    args["url"],
		}
		if name, ok := args["name"].(string); ok && name != "" {
			params["name"] = name
		}
		resp, err := node.Send(ctx, "add_dev_resource", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("edit_dev_resource",
		mcp.WithDescription("Edit a node Dev Mode resource link by its current URL. Provide url, name, or both as the replacement."),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Node ID in colon format e.g. '4029:12345'.")),
		mcp.WithString("currentUrl", mcp.Required(), mcp.Description("Existing resource URL to edit.")),
		mcp.WithString("url", mcp.Description("Replacement URL.")),
		mcp.WithString("name", mcp.Description("Replacement display name.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{
			"nodeId":     NormalizeNodeID(args["nodeId"].(string)),
			"currentUrl": args["currentUrl"],
		}
		copyOptionalArgs(args, params, []string{"url", "name"})
		resp, err := node.Send(ctx, "edit_dev_resource", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("delete_dev_resource",
		mcp.WithDescription("Delete a Dev Mode resource link from a node by URL."),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Node ID in colon format e.g. '4029:12345'.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Resource URL to delete.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{
			"nodeId": NormalizeNodeID(args["nodeId"].(string)),
			"url":    args["url"],
		}
		resp, err := node.Send(ctx, "delete_dev_resource", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_opacity",
		mcp.WithDescription("Set the opacity of one or more nodes (nodeIds[]) — 0 = fully transparent, 1 = fully opaque. Bulk-apply: returns {results:[{nodeId,…}]}, one entry per node. "+
			"Also a `batch` op type — fan a scan into it in ONE round-trip with the projection ref nodeIds:[\"$0.matchingNodes[*].id\"]; read its bulk output back with $N.results[*].nodeId."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("opacity",
			mcp.Required(),
			mcp.Description("Opacity value between 0 and 1"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		opacity, _ := req.GetArguments()["opacity"].(float64)
		resp, err := node.Send(ctx, "set_opacity", nodeIDs, withChannel(req, map[string]interface{}{"opacity": opacity}))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — set_corner_radius DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays the type to the untouched plugin handler. Uncomment to restore.
	//
	// s.AddTool(mcp.NewTool("set_corner_radius",
	// 	mcp.WithDescription("Set corner radius on one or more nodes. Provide a uniform cornerRadius or individual per-corner values. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	mcp.WithNumber("cornerRadius", mcp.Description("Uniform corner radius applied to all corners")),
	// 	mcp.WithNumber("topLeftRadius", mcp.Description("Top-left corner radius")),
	// 	mcp.WithNumber("topRightRadius", mcp.Description("Top-right corner radius")),
	// 	mcp.WithNumber("bottomLeftRadius", mcp.Description("Bottom-left corner radius")),
	// 	mcp.WithNumber("bottomRightRadius", mcp.Description("Bottom-right corner radius")),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	params := map[string]interface{}{}
	// 	if v, ok := req.GetArguments()["cornerRadius"].(float64); ok {
	// 		params["cornerRadius"] = v
	// 	}
	// 	if v, ok := req.GetArguments()["topLeftRadius"].(float64); ok {
	// 		params["topLeftRadius"] = v
	// 	}
	// 	if v, ok := req.GetArguments()["topRightRadius"].(float64); ok {
	// 		params["topRightRadius"] = v
	// 	}
	// 	if v, ok := req.GetArguments()["bottomLeftRadius"].(float64); ok {
	// 		params["bottomLeftRadius"] = v
	// 	}
	// 	if v, ok := req.GetArguments()["bottomRightRadius"].(float64); ok {
	// 		params["bottomRightRadius"] = v
	// 	}
	// 	resp, err := node.Send(ctx, "set_corner_radius", nodeIDs, withChannel(req, params))
	// 	return renderResponse(resp, err)
	// })

	s.AddTool(mcp.NewTool("set_auto_layout",
		mcp.WithDescription("Set or update auto-layout (flex) properties on an existing frame."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Frame node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("layoutMode", mcp.Description("Auto-layout direction: HORIZONTAL, VERTICAL, GRID, or NONE. GRID = CSS-grid layout (use gridRowCount/gridColumnCount/gridRowGap/gridColumnGap).")),
		mcp.WithNumber("paddingTop", mcp.Description("Top padding")),
		mcp.WithNumber("paddingRight", mcp.Description("Right padding")),
		mcp.WithNumber("paddingBottom", mcp.Description("Bottom padding")),
		mcp.WithNumber("paddingLeft", mcp.Description("Left padding")),
		mcp.WithNumber("itemSpacing", mcp.Description("Gap between children")),
		mcp.WithString("paddingTopVariableId", mcp.Description("Design variable ID to bind to paddingTop (e.g. 'VariableID:1234:5'). When set, binds the variable instead of a raw pixel value.")),
		mcp.WithString("paddingRightVariableId", mcp.Description("Design variable ID to bind to paddingRight.")),
		mcp.WithString("paddingBottomVariableId", mcp.Description("Design variable ID to bind to paddingBottom.")),
		mcp.WithString("paddingLeftVariableId", mcp.Description("Design variable ID to bind to paddingLeft.")),
		mcp.WithString("itemSpacingVariableId", mcp.Description("Design variable ID to bind to itemSpacing (gap between children).")),
		mcp.WithString("primaryAxisAlignItems", mcp.Description("Main-axis alignment: MIN, CENTER, MAX, or SPACE_BETWEEN")),
		mcp.WithString("counterAxisAlignItems", mcp.Description("Cross-axis alignment: MIN, CENTER, MAX, or BASELINE")),
		mcp.WithString("primaryAxisSizingMode", mcp.Description("Main-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("counterAxisSizingMode", mcp.Description("Cross-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("layoutWrap", mcp.Description("Wrap behaviour: NO_WRAP or WRAP")),
		mcp.WithNumber("counterAxisSpacing", mcp.Description("Gap between wrapped rows/columns (only when layoutWrap is WRAP)")),
		mcp.WithString("counterAxisSpacingVariableId", mcp.Description("Design variable ID to bind to counterAxisSpacing (wrapped-track gap; only when layoutWrap is WRAP).")),
		mcp.WithString("counterAxisAlignContent", mcp.Description("Wrapped-track distribution: AUTO or SPACE_BETWEEN (only when layoutWrap is WRAP)")),
		mcp.WithNumber("gridRowCount", mcp.Description("Number of rows (GRID layoutMode only)")),
		mcp.WithNumber("gridColumnCount", mcp.Description("Number of columns (GRID layoutMode only)")),
		mcp.WithNumber("gridRowGap", mcp.Description("Gap between grid rows (GRID layoutMode only)")),
		mcp.WithNumber("gridColumnGap", mcp.Description("Gap between grid columns (GRID layoutMode only)")),
		mcp.WithString("gridRowGapVariableId", mcp.Description("Design variable ID to bind to gridRowGap (GRID layoutMode only).")),
		mcp.WithString("gridColumnGapVariableId", mcp.Description("Design variable ID to bind to gridColumnGap (GRID layoutMode only).")),
		mcp.WithNumber("minWidth", mcp.Description("Minimum frame width in px (null clears). Responsive constraint on auto-layout frames.")),
		mcp.WithNumber("maxWidth", mcp.Description("Maximum frame width in px (null clears).")),
		mcp.WithNumber("minHeight", mcp.Description("Minimum frame height in px (null clears).")),
		mcp.WithNumber("maxHeight", mcp.Description("Maximum frame height in px (null clears).")),
		mcp.WithString("overflowDirection", mcp.Description("Scroll overflow: NONE, HORIZONTAL, VERTICAL, or BOTH")),
		mcp.WithBoolean("strokesIncludedInLayout", mcp.Description("Whether strokes count toward layout size (auto-layout frames only)")),
		mcp.WithBoolean("itemReverseZIndex", mcp.Description("Reverse the stacking order of children (auto-layout frames only)")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := req.GetArguments()
		resp, err := node.Send(ctx, "set_auto_layout", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("delete_nodes",
		mcp.WithDescription("Delete one or more nodes. This cannot be undone via MCP — use with care."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs to delete in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		resp, err := node.Send(ctx, "delete_nodes", nodeIDs, withChannel(req, nil))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_visible",
		mcp.WithDescription("Show or hide one or more nodes (nodeIds[]) by setting their visibility. Bulk-apply: returns {results:[{nodeId,…}]}, one entry per node. "+
			"Also a `batch` op type — fan a scan into it in ONE round-trip with the projection ref nodeIds:[\"$0.matchingNodes[*].id\"]; read its bulk output back with $N.results[*].nodeId."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("visible",
			mcp.Required(),
			mcp.Description("true to show the node, false to hide it"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		visible, _ := req.GetArguments()["visible"].(bool)
		resp, err := node.Send(ctx, "set_visible", nodeIDs, withChannel(req, map[string]interface{}{"visible": visible}))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — lock_nodes, unlock_nodes, rotate_nodes, reorder_nodes, set_blend_mode are DEMOTED to batch-only ops. Registrations commented out (off tools/list); batch relays each type to the untouched plugin handlers. Uncomment to restore. (set_constraints was promoted back to a top-level tool.)
	// s.AddTool(mcp.NewTool("lock_nodes",
	// 	mcp.WithDescription("Lock one or more nodes to prevent accidental edits in Figma. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	resp, err := node.Send(ctx, "lock_nodes", nodeIDs, withChannel(req, nil))
	// 	return renderResponse(resp, err)
	// })

	// s.AddTool(mcp.NewTool("unlock_nodes",
	// 	mcp.WithDescription("Unlock one or more nodes, allowing them to be edited again. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	resp, err := node.Send(ctx, "unlock_nodes", nodeIDs, withChannel(req, nil))
	// 	return renderResponse(resp, err)
	// })

	// s.AddTool(mcp.NewTool("rotate_nodes",
	// 	mcp.WithDescription("Rotate one or more nodes to an absolute angle in degrees. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	mcp.WithNumber("rotation",
	// 		mcp.Required(),
	// 		mcp.Description("Rotation angle in degrees (positive = counter-clockwise in Figma)"),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	rotation, _ := req.GetArguments()["rotation"].(float64)
	// 	resp, err := node.Send(ctx, "rotate_nodes", nodeIDs, withChannel(req, map[string]interface{}{"rotation": rotation}))
	// 	return renderResponse(resp, err)
	// })

	// s.AddTool(mcp.NewTool("reorder_nodes",
	// 	mcp.WithDescription("Change the z-order (layer stack position) of one or more nodes. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	mcp.WithString("order",
	// 		mcp.Required(),
	// 		mcp.Description("Order operation: bringToFront, sendToBack, bringForward, or sendBackward"),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	order, _ := req.GetArguments()["order"].(string)
	// 	resp, err := node.Send(ctx, "reorder_nodes", nodeIDs, withChannel(req, map[string]interface{}{"order": order}))
	// 	return renderResponse(resp, err)
	// })

	// s.AddTool(mcp.NewTool("set_blend_mode",
	// 	mcp.WithDescription("Set the blend mode of one or more nodes (e.g. MULTIPLY, SCREEN, OVERLAY). Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	mcp.WithString("blendMode",
	// 		mcp.Required(),
	// 		mcp.Description("Blend mode: NORMAL, MULTIPLY, SCREEN, OVERLAY, DARKEN, LIGHTEN, COLOR_DODGE, COLOR_BURN, HARD_LIGHT, SOFT_LIGHT, DIFFERENCE, EXCLUSION, HUE, SATURATION, COLOR, LUMINOSITY, PASS_THROUGH"),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	blendMode, _ := req.GetArguments()["blendMode"].(string)
	// 	resp, err := node.Send(ctx, "set_blend_mode", nodeIDs, withChannel(req, map[string]interface{}{"blendMode": blendMode}))
	// 	return renderResponse(resp, err)
	// })

	s.AddTool(mcp.NewTool("set_constraints",
		mcp.WithDescription("Set layout constraints (pinning behaviour) on one or more nodes relative to their parent — how a node resizes/repositions when its parent resizes. For non-auto-layout parents. Also a `batch` op type."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("horizontal", mcp.Description("Horizontal constraint: MIN (left), MAX (right), CENTER, STRETCH, or SCALE")),
		mcp.WithString("vertical", mcp.Description("Vertical constraint: MIN (top), MAX (bottom), CENTER, STRETCH, or SCALE")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if h, ok := req.GetArguments()["horizontal"].(string); ok && h != "" {
			params["horizontal"] = h
		}
		if v, ok := req.GetArguments()["vertical"].(string); ok && v != "" {
			params["vertical"] = v
		}
		resp, err := node.Send(ctx, "set_constraints", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("reparent_nodes",
		mcp.WithDescription("Move one or more nodes to a different parent frame, group, or section. By default (preserveAbsolutePosition=true) the node's canvas position is preserved after reparenting by adjusting its parent-local x/y. Set preserveAbsolutePosition=false to keep the raw x/y values unchanged (node will visually jump to a new location relative to the new parent)."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs to move in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("parentId",
			mcp.Required(),
			mcp.Description("Target parent node ID in colon format e.g. '4029:99'"),
		),
		mcp.WithBoolean("preserveAbsolutePosition",
			mcp.Description("When true (default), the node's absolute canvas position is preserved after reparenting by recomputing its local x/y relative to the new parent. Set to false to keep the current parent-local x/y unchanged (the node will visually jump)."),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		parentID, _ := req.GetArguments()["parentId"].(string)
		parentID = NormalizeNodeID(parentID)
		params := map[string]interface{}{"parentId": parentID}
		if v, ok := req.GetArguments()["preserveAbsolutePosition"].(bool); ok {
			params["preserveAbsolutePosition"] = v
		}
		resp, err := node.Send(ctx, "reparent_nodes", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("batch_rename_nodes",
		mcp.WithDescription("Rename multiple nodes using find/replace, regex substitution, or prefix/suffix addition."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("find", mcp.Description("String (or regex pattern when useRegex=true) to search for in the node name")),
		mcp.WithString("replace", mcp.Description("Replacement string. Required when find is provided.")),
		mcp.WithBoolean("useRegex", mcp.Description("Treat find as a regular expression (default false)")),
		mcp.WithString("regexFlags", mcp.Description("Regex flags e.g. 'gi' (default 'g'). Only used when useRegex=true.")),
		mcp.WithString("prefix", mcp.Description("String to prepend to the node name")),
		mcp.WithString("suffix", mcp.Description("String to append to the node name")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		for _, k := range []string{"find", "replace", "regexFlags", "prefix", "suffix"} {
			if v, ok := req.GetArguments()[k].(string); ok {
				params[k] = v
			}
		}
		if v, ok := req.GetArguments()["useRegex"].(bool); ok {
			params["useRegex"] = v
		}
		resp, err := node.Send(ctx, "batch_rename_nodes", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("find_replace_text",
		mcp.WithDescription("Find and replace text content across all TEXT nodes in a subtree. Searches the entire current page if no nodeId is given."),
		mcp.WithString("find",
			mcp.Required(),
			mcp.Description("Text string (or regex pattern when useRegex=true) to search for"),
		),
		mcp.WithString("replace",
			mcp.Required(),
			mcp.Description("Replacement string (use empty string to delete matches)"),
		),
		mcp.WithString("nodeId", mcp.Description("Root node ID to scope the search. Defaults to the entire current page.")),
		mcp.WithBoolean("useRegex", mcp.Description("Treat find as a regular expression (default false)")),
		mcp.WithString("regexFlags", mcp.Description("Regex flags e.g. 'gi' (default 'g'). Only used when useRegex=true.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{
			"find":    req.GetArguments()["find"],
			"replace": req.GetArguments()["replace"],
		}
		if v, ok := req.GetArguments()["useRegex"].(bool); ok {
			params["useRegex"] = v
		}
		if v, ok := req.GetArguments()["regexFlags"].(string); ok && v != "" {
			params["regexFlags"] = v
		}
		var nodeIDs []string
		if nodeID, ok := req.GetArguments()["nodeId"].(string); ok && nodeID != "" {
			nodeID = NormalizeNodeID(nodeID)
			nodeIDs = []string{nodeID}
		}
		resp, err := node.Send(ctx, "find_replace_text", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — boolean_operation is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "boolean_operation" to the untouched plugin handler (write-vector.ts). Uncomment to restore.
	// s.AddTool(mcp.NewTool("boolean_operation",
	// 	mcp.WithDescription("Combine or flatten vector shapes. UNION/SUBTRACT/INTERSECT/EXCLUDE merge 2+ nodes into a boolean-operation node; FLATTEN rasterises 1+ nodes into a single vector. Use to build custom marks (diamonds, bars) the library lacks. The result is placed in the first node's parent unless parentId is given. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("Node IDs to operate on, in colon format e.g. ['4:1','4:2']. Boolean ops need 2+; FLATTEN accepts 1+."),
	// 		mcp.WithStringItems(),
	// 	),
	// 	mcp.WithString("operation",
	// 		mcp.Required(),
	// 		mcp.Description("UNION, SUBTRACT, INTERSECT, EXCLUDE, or FLATTEN"),
	// 	),
	// 	mcp.WithString("parentId", mcp.Description("Optional parent node ID for the result. Defaults to the first node's parent.")),
	// 	mcp.WithString("name", mcp.Description("Optional name for the resulting node")),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	params := map[string]interface{}{
	// 		"operation": req.GetArguments()["operation"],
	// 	}
	// 	if pid, ok := req.GetArguments()["parentId"].(string); ok && pid != "" {
	// 		params["parentId"] = pid
	// 	}
	// 	if n, ok := req.GetArguments()["name"].(string); ok && n != "" {
	// 		params["name"] = n
	// 	}
	// 	resp, err := node.Send(ctx, "boolean_operation", nodeIDs, withChannel(req, params))
	// 	return renderResponse(resp, err)
	// })
}
