package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteStyleTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("create_paint_style",
		mcp.WithDescription("Create a new local paint style. Supply either a solid color shorthand (color hex) or a paints[] array for gradients, images, or multiple paints."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Style name e.g. 'Brand/Primary'"),
		),
		mcp.WithString("color",
			mcp.Description("Solid fill color as hex e.g. #FF5733 (ignored when paints[] is provided)"),
		),
		mcp.WithArray("paints",
			mcp.Description("Full Paint objects array (solid/gradient/image). When provided, takes precedence over color. Each paint must be a valid Figma Paint object (e.g. {type:'GRADIENT_LINEAR', gradientStops:[…], gradientTransform:[…]})."),
			mcp.Items(map[string]any{"type": "object"}),
		),
		mcp.WithString("description", mcp.Description("Optional style description")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_paint_style", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_text_style",
		mcp.WithDescription("Create a new local text style (typography preset). Returns the new style's ID. Apply it to nodes with apply_style_to_node. Use get_styles to list existing text styles."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Style name — use slash notation to organise into groups e.g. 'Heading/H1', 'Body/Regular'"),
		),
		mcp.WithNumber("fontSize", mcp.Description("Font size in pixels (default 16)")),
		mcp.WithString("fontFamily", mcp.Description("Font family name e.g. 'Inter', 'Roboto' (default Inter). Must be installed in Figma.")),
		mcp.WithString("fontStyle", mcp.Description("Font style variant e.g. 'Regular', 'Bold', 'Medium', 'SemiBold' (default Regular)")),
		mcp.WithString("textDecoration", mcp.Description("Text decoration: NONE (default), UNDERLINE, or STRIKETHROUGH")),
		mcp.WithString("textCase", mcp.Description("Text case transform: ORIGINAL (default), UPPER, LOWER, TITLE, or SMALL_CAPS")),
		mcp.WithNumber("paragraphSpacing", mcp.Description("Space after a paragraph in pixels (default 0)")),
		mcp.WithNumber("paragraphIndent", mcp.Description("First-line indent for a paragraph in pixels (default 0)")),
		mcp.WithNumber("lineHeightValue", mcp.Description("Line height value (unit set by lineHeightUnit)")),
		mcp.WithString("lineHeightUnit", mcp.Description("Line height unit: PIXELS (default) or PERCENT")),
		mcp.WithNumber("letterSpacingValue", mcp.Description("Letter spacing value (unit set by letterSpacingUnit)")),
		mcp.WithString("letterSpacingUnit", mcp.Description("Letter spacing unit: PIXELS (default) or PERCENT")),
		mcp.WithString("description", mcp.Description("Optional human-readable description shown in the Figma style panel")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_text_style", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_effect_style",
		mcp.WithDescription("Create a new local effect style (drop shadow, inner shadow, or blur). Supply either a single-effect shorthand (type + color/radius/etc.) or an effects[] array for multi-effect styles."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Style name e.g. 'Shadow/Card'"),
		),
		mcp.WithArray("effects",
			mcp.Description("Array of effect objects for multi-effect styles. When provided, takes precedence over the single-effect shorthand fields. Each effect: {type, color?, opacity?, offsetX?, offsetY?, radius?, spread?, visible?, blendMode?, showShadowBehindNode?}"),
			mcp.Items(map[string]any{"type": "object"}),
		),
		mcp.WithString("type", mcp.Description("Single-effect shorthand: DROP_SHADOW (default), INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR. Ignored when effects[] is provided.")),
		mcp.WithString("color", mcp.Description("Shadow color as hex e.g. #000000 (default #000000, shadows only)")),
		mcp.WithNumber("opacity", mcp.Description("Shadow color opacity 0–1 (default 0.25, shadows only)")),
		mcp.WithNumber("radius", mcp.Description("Blur radius in pixels (default 8 for shadows, 4 for blurs)")),
		mcp.WithNumber("offsetX", mcp.Description("Shadow X offset in pixels (default 0, shadows only)")),
		mcp.WithNumber("offsetY", mcp.Description("Shadow Y offset in pixels (default 4, shadows only)")),
		mcp.WithNumber("spread", mcp.Description("Shadow spread in pixels (default 0, shadows only)")),
		mcp.WithString("description", mcp.Description("Optional style description")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_effect_style", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_grid_style",
		mcp.WithDescription("Create a new local layout grid style."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Style name e.g. 'Grid/Desktop'"),
		),
		mcp.WithString("pattern", mcp.Description("Grid pattern: GRID (default), COLUMNS, or ROWS")),
		mcp.WithNumber("count", mcp.Description("Number of columns or rows (COLUMNS/ROWS only, default 12)")),
		mcp.WithNumber("gutterSize", mcp.Description("Gutter size in pixels (COLUMNS/ROWS only, default 16)")),
		mcp.WithNumber("offset", mcp.Description("Margin/offset in pixels (COLUMNS/ROWS only, default 0)")),
		mcp.WithString("alignment", mcp.Description("Alignment: STRETCH (default), CENTER, MIN, or MAX (COLUMNS/ROWS only)")),
		mcp.WithNumber("sectionSize", mcp.Description("Grid cell size in pixels (GRID only, default 8)")),
		mcp.WithString("color", mcp.Description("Grid line color as hex e.g. #FF0000 (GRID only, default #FF0000)")),
		mcp.WithNumber("opacity", mcp.Description("Grid line opacity 0–1 (GRID only, default 0.1)")),
		mcp.WithString("description", mcp.Description("Optional style description")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_grid_style", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("update_paint_style",
		mcp.WithDescription("Update an existing paint style's name, color/paints, or description. Supports solid color shorthand (color hex) or paints[] for gradients and multi-paint styles. Only paint styles support in-place updates — to modify text, effect, or grid styles, use delete_style and recreate them."),
		mcp.WithString("styleId",
			mcp.Required(),
			mcp.Description("Paint style ID"),
		),
		mcp.WithString("name", mcp.Description("New style name")),
		mcp.WithString("color", mcp.Description("New solid fill color as hex e.g. #FF5733 (ignored when paints[] is provided)")),
		mcp.WithArray("paints",
			mcp.Description("Full Paint objects array (solid/gradient/image). When provided, takes precedence over color."),
			mcp.Items(map[string]any{"type": "object"}),
		),
		mcp.WithString("description", mcp.Description("New style description")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "update_paint_style", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — delete_style is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "delete_style" to the untouched plugin handler. Uncomment to restore.
	// s.AddTool(mcp.NewTool("delete_style",
	// 	mcp.WithDescription("Delete a style (paint, text, effect, or grid) by its ID."),
	// 	mcp.WithString("styleId",
	// 		mcp.Required(),
	// 		mcp.Description("Style ID to delete"),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	params := req.GetArguments()
	// 	resp, err := node.Send(ctx, "delete_style", nil, withChannel(req, params))
	// 	return renderResponse(resp, err)
	// })

	s.AddTool(mcp.NewTool("apply_style_to_node",
		mcp.WithDescription("Apply an existing local style (paint, text, effect, or grid) to a node, linking the node to that style."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Target node ID in colon format e.g. 4029:12345"),
		),
		mcp.WithString("styleId",
			mcp.Required(),
			mcp.Description("Style ID to apply (from get_styles)"),
		),
		mcp.WithString("target", mcp.Description("For paint styles only — apply to 'fill' (default) or 'stroke'")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{
			"styleId": args["styleId"],
		}
		if t, ok := args["target"]; ok {
			params["target"] = t
		}
		resp, err := node.Send(ctx, "apply_style_to_node", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_effects",
		mcp.WithDescription("Apply one or more effects (drop shadow, inner shadow, layer blur, background blur) directly to one or more nodes. Replaces all existing effects on each target node. Pass an empty array to clear all effects. Returns {results:[{nodeId,effectCount}]} for each nodeId."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Primary target node ID in colon format e.g. 4029:12345. Also accepts nodeIds[] in batch mode for bulk application."),
		),
		mcp.WithArray("effects",
			mcp.Required(),
			mcp.Description("Array of effect objects. Each has: type (DROP_SHADOW | INNER_SHADOW | LAYER_BLUR | BACKGROUND_BLUR), radius, color (hex, shadows only), opacity (0–1, shadows only), offsetX, offsetY (shadows only), spread (shadows only), visible (default true), showShadowBehindNode (DROP_SHADOW only, default false)"),
			mcp.Items(map[string]any{"type": "object"}),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{
			"effects": args["effects"],
		}
		resp, err := node.Send(ctx, "set_effects", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("bind_variable_to_node",
		mcp.WithDescription("Bind a local variable to a node property so the property is driven by the variable's value (this top-level tool takes one nodeId). Returns {results:[{nodeId,…}]}, a 1-element array. COLOR variables: use fillColor or strokeColor. BOOLEAN variables: use visible. STRING variables: use characters. FLOAT variables: use opacity, width, height, minWidth, maxWidth, minHeight, maxHeight, topLeftRadius, topRightRadius, bottomLeftRadius, bottomRightRadius, strokeWeight, strokeTopWeight, strokeRightWeight, strokeBottomWeight, strokeLeftWeight, itemSpacing, counterAxisSpacing, gridRowGap, gridColumnGap, paddingTop, paddingRight, paddingBottom, paddingLeft. NOTE: cornerRadius/rotation/x/y are NOT bindable — for radius use the per-corner fields. "+
			"Also a `batch` op type — and in `batch` it accepts nodeIds[] (all→all bulk: bind the same variable+field on every node) and returns {results:[{nodeId,…}]}. Fan a scan into it with nodeIds:[\"$0.matchingNodes[*].id\"] (scan_nodes_by_types → matchingNodes; search_nodes → nodes); read back with $N.results[*].nodeId."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Target node ID in colon format e.g. 4029:12345"),
		),
		mcp.WithString("variableId",
			mcp.Required(),
			mcp.Description("Variable ID to bind (from get_variable_defs)"),
		),
		mcp.WithString("field",
			mcp.Required(),
			mcp.Description("Property to bind: fillColor | strokeColor | visible | characters | opacity | width | height | minWidth | maxWidth | minHeight | maxHeight | topLeftRadius | topRightRadius | bottomLeftRadius | bottomRightRadius | strokeWeight | strokeTopWeight | strokeRightWeight | strokeBottomWeight | strokeLeftWeight | itemSpacing | counterAxisSpacing | gridRowGap | gridColumnGap | paddingTop | paddingRight | paddingBottom | paddingLeft (NOT bindable: cornerRadius/rotation/x/y)"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{
			"variableId": args["variableId"],
			"field":      args["field"],
		}
		resp, err := node.Send(ctx, "bind_variable_to_node", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})
}
