package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func variableValuePropertySchema() map[string]any {
	return map[string]any{
		"description": "Value to set. COLOR: hex or RGBA object. FLOAT: number. STRING: text. BOOLEAN: true/false. VARIABLE_ALIAS: object returned by create_variable_alias.",
		"anyOf": []map[string]any{
			{"type": "string"},
			{"type": "number"},
			{"type": "boolean"},
			{"type": "object"},
		},
	}
}

func registerWriteVariableTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("create_variable_collection",
		mcp.WithDescription("Create a new local variable collection with an optional initial mode name. "+
			"NOTE — Figma free plan limits each collection to 1 mode. If you need Light/Dark (or any multi-mode) "+
			"theming and the user is on the free plan, do NOT try to call add_variable_mode; instead use the "+
			"name-prefix workaround: create all variables in a single collection and prefix each variable name "+
			"with its mode, e.g. 'light/color-bg' and 'dark/color-bg'. Inform the user of this limitation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Collection name"),
		),
		mcp.WithString("initialModeName", mcp.Description("Name for the initial mode (default 'Mode 1')")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_variable_collection", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("add_variable_mode",
		mcp.WithDescription("Add a new mode to an existing variable collection (e.g. Light/Dark, Desktop/Mobile). "+
			"IMPORTANT — Figma free plan only allows 1 mode per collection; calling this tool on a free-plan "+
			"account will return the error 'Limited to 1 modes only'. If that error occurs, stop retrying and "+
			"switch to the name-prefix workaround: keep the single default mode and create variables prefixed "+
			"by mode, e.g. 'light/color-bg' and 'dark/color-bg' in the same collection. Tell the user that "+
			"native multi-mode variables require a paid Figma plan (Professional or above)."),
		mcp.WithString("collectionId",
			mcp.Required(),
			mcp.Description("Variable collection ID"),
		),
		mcp.WithString("modeName",
			mcp.Required(),
			mcp.Description("Name for the new mode"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "add_variable_mode", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_variable",
		mcp.WithDescription("Create a new variable (design token) inside an existing collection. Returns the new variable's ID. Use get_variable_defs to find collection IDs, set_variable_value to set values per mode, and bind_variable_to_node to apply the variable to a node property."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Variable name — use slash notation to group e.g. 'Color/Primary', 'Spacing/MD'"),
		),
		mcp.WithString("collectionId",
			mcp.Required(),
			mcp.Description("ID of the variable collection to add this variable to (from get_variable_defs)"),
		),
		mcp.WithString("type",
			mcp.Required(),
			mcp.Description("Variable type: COLOR (hex color), FLOAT (numeric dimension/spacing), STRING (text), or BOOLEAN (true/false toggle)"),
		),
		mcp.WithString("value", mcp.Description("Initial value for the first (default) mode. COLOR: hex e.g. #FF5733. FLOAT: number e.g. 16. STRING: text. BOOLEAN: true or false. Use 'values' instead to set multiple modes at once.")),
		mcp.WithObject("values", mcp.Description("Optional map of {modeId: value} to set values for multiple modes at creation time. Takes precedence over 'value' when provided. Use get_variable_defs or create_variable_collection to obtain valid modeIds.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_variable", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_variable_alias",
		mcp.WithDescription("Create a variable alias value from an existing variable ID using createVariableAliasByIdAsync. Use the returned alias as a variable value."),
		mcp.WithString("variableId", mcp.Required(), mcp.Description("Variable ID to alias.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_variable_alias", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	setVariableValueTool := mcp.NewTool("set_variable_value",
		mcp.WithDescription("Set a variable's value for a specific mode."),
		mcp.WithString("variableId",
			mcp.Required(),
			mcp.Description("Variable ID"),
		),
		mcp.WithString("modeId",
			mcp.Required(),
			mcp.Description("Mode ID within the collection"),
		),
		mcp.WithString("value",
			mcp.Required(),
			mcp.Description("Value to set. COLOR: hex e.g. #FF5733. FLOAT: number e.g. 16. STRING: text. BOOLEAN: true or false. VARIABLE_ALIAS: object returned by create_variable_alias."),
		),
		channelParam(),
	)
	setVariableValueTool.InputSchema.Properties["value"] = variableValuePropertySchema()
	s.AddTool(setVariableValueTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "set_variable_value", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — delete_variable is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "delete_variable" to the untouched plugin handler. Uncomment to restore.
	// s.AddTool(mcp.NewTool("delete_variable",
	// 	mcp.WithDescription("Delete a single variable (provide variableId) or an entire collection and all its variables (provide collectionId). Provide exactly one of the two — not both."),
	// 	mcp.WithString("variableId", mcp.Description("Variable ID to delete")),
	// 	mcp.WithString("collectionId", mcp.Description("Collection ID to delete (removes all variables in the collection)")),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	params := req.GetArguments()
	// 	resp, err := node.Send(ctx, "delete_variable", nil, withChannel(req, params))
	// 	return renderResponse(resp, err)
	// })

	s.AddTool(mcp.NewTool("update_variable",
		mcp.WithDescription("Update an existing variable's metadata: rename, set publishing scopes, hide from publishing, set per-platform code syntax, or remove code syntax platforms. Does not change the variable's value (use set_variable_value)."),
		mcp.WithString("variableId",
			mcp.Required(),
			mcp.Description("Variable ID to update (from get_variable_defs)"),
		),
		mcp.WithString("name", mcp.Description("New variable name (use slash notation to group, e.g. 'color/primary')")),
		mcp.WithArray("scopes",
			mcp.Description("Publishing scopes restricting where the variable is offered. Values: ALL_SCOPES, TEXT_CONTENT, CORNER_RADIUS, WIDTH_HEIGHT, GAP, ALL_FILLS, FRAME_FILL, SHAPE_FILL, TEXT_FILL, STROKE_COLOR, STROKE_FLOAT, EFFECT_FLOAT, EFFECT_COLOR, OPACITY, FONT_FAMILY, FONT_STYLE, FONT_WEIGHT, FONT_SIZE, LINE_HEIGHT, LETTER_SPACING, PARAGRAPH_SPACING, PARAGRAPH_INDENT."),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("hiddenFromPublishing", mcp.Description("Hide this variable when the file is published as a library")),
		mcp.WithObject("codeSyntax", mcp.Description("Per-platform code names: {WEB?, ANDROID?, iOS?}. Each provided platform is set; others are left unchanged.")),
		mcp.WithArray("removeCodeSyntax",
			mcp.Description("Code syntax platforms to remove: WEB, ANDROID, or iOS."),
			mcp.WithStringItems(),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "update_variable", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("update_variable_collection",
		mcp.WithDescription("Update a variable collection: rename it, hide it from publishing, rename a mode, or remove a mode. A collection must always keep at least one mode."),
		mcp.WithString("collectionId",
			mcp.Required(),
			mcp.Description("Variable collection ID to update"),
		),
		mcp.WithString("name", mcp.Description("New collection name")),
		mcp.WithBoolean("hiddenFromPublishing", mcp.Description("Hide this collection when the file is published as a library")),
		mcp.WithObject("renameMode", mcp.Description("Rename a mode: {modeId, newName}")),
		mcp.WithString("removeMode", mcp.Description("modeId of a mode to remove (cannot remove the last remaining mode)")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "update_variable_collection", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})
}
