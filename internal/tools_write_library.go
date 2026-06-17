package internal

import (
	"context"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerLibraryTools registers Track A library tools: importing components,
// variables, and styles by key from a subscribed library, instantiating
// components, configuring instance properties, and reading/setting variable
// modes (e.g. theme pinning) on remote collections.
func registerLibraryTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("import_component_by_key",
		mcp.WithDescription("Import a component (or component set) from a subscribed library by its key, making it available to instantiate. For a COMPONENT_SET key pass assetType='COMPONENT_SET'."),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Library component key (from the library catalog, not a node ID)"),
		),
		mcp.WithString("assetType",
			mcp.Description("Optional asset type hint: 'COMPONENT' (default) or 'COMPONENT_SET'"),
			mcp.Enum("COMPONENT", "COMPONENT_SET"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{}
		if key, ok := args["key"].(string); ok {
			params["key"] = key
		}
		if at, ok := args["assetType"].(string); ok && at != "" {
			params["assetType"] = at
		}
		resp, err := node.Send(ctx, "import_component_by_key", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("import_variable_by_key",
		mcp.WithDescription("Import a design variable from a subscribed library by its key, making it available to bind to node properties."),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Library variable key (from the library catalog)"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if key, ok := req.GetArguments()["key"].(string); ok {
			params["key"] = key
		}
		resp, err := node.Send(ctx, "import_variable_by_key", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("import_style_by_key",
		mcp.WithDescription("Import a paint, text, or effect style from a subscribed library by its key, making it available to apply to nodes."),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Library style key (from the library catalog)"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if key, ok := req.GetArguments()["key"].(string); ok {
			params["key"] = key
		}
		resp, err := node.Send(ctx, "import_style_by_key", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_instance",
		mcp.WithDescription("Create an instance of a component, optionally placing it in a parent, positioning/sizing it, and setting variant and exposed-instance properties."),
		mcp.WithString("componentId",
			mcp.Required(),
			mcp.Description("Source COMPONENT node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("componentKey", mcp.Description("Optional library component key, used to resolve the component if it must be imported first")),
		mcp.WithString("parentId", mcp.Description("Parent node ID for the instance in colon format. Defaults to the current page.")),
		mcp.WithNumber("index", mcp.Description("Insertion index within the parent's children")),
		mcp.WithNumber("x", mcp.Description("X position of the instance")),
		mcp.WithNumber("y", mcp.Description("Y position of the instance")),
		mcp.WithNumber("width", mcp.Description("Width to resize the instance to")),
		mcp.WithNumber("height", mcp.Description("Height to resize the instance to")),
		mcp.WithString("layoutSizingHorizontal", mcp.Description("Horizontal sizing inside an auto-layout parent: FIXED, HUG, or FILL (set after appendChild)")),
		mcp.WithString("layoutSizingVertical", mcp.Description("Vertical sizing inside an auto-layout parent: FIXED, HUG, or FILL (set after appendChild)")),
		mcp.WithObject("variantProperties", mcp.Description("Variant property map e.g. {\"State\":\"Default\",\"Size\":\"Large\"}")),
		mcp.WithObject("properties", mcp.Description("Exposed instance/text property map e.g. {\"Label#1:0\":\"Submit\"}")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{}
		if componentID, ok := args["componentId"].(string); ok && componentID != "" {
			params["componentId"] = NormalizeNodeID(componentID)
		}
		if ck, ok := args["componentKey"].(string); ok && ck != "" {
			params["componentKey"] = ck
		}
		if pid, ok := args["parentId"].(string); ok && pid != "" {
			params["parentId"] = NormalizeNodeID(pid)
		}
		for _, k := range []string{"index", "x", "y", "width", "height"} {
			if v, ok := args[k].(float64); ok {
				params[k] = v
			}
		}
		for _, k := range []string{"layoutSizingHorizontal", "layoutSizingVertical"} {
			if v, ok := args[k].(string); ok && v != "" {
				params[k] = v
			}
		}
		if v, ok := args["variantProperties"].(map[string]interface{}); ok {
			params["variantProperties"] = v
		}
		if v, ok := args["properties"].(map[string]interface{}); ok {
			params["properties"] = v
		}
		resp, err := node.Send(ctx, "create_instance", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_instance_properties",
		mcp.WithDescription("Set variant, boolean, text, and instance-swap properties on a component INSTANCE (this top-level tool takes one nodeId). Returns {results:[{nodeId,…}]}, a 1-element array. Use resetOverrides=true to restore defaults before applying. "+
			"Also a `batch` op type — and in `batch` it accepts nodeIds[] (all→all bulk: apply the same property map to every instance) and returns {results:[{nodeId,…}]}. Fan a scan into it with nodeIds:[\"$0.matchingNodes[*].id\"] (scan_nodes_by_types → matchingNodes; search_nodes → nodes); read back with $N.results[*].nodeId."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("INSTANCE node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithObject("properties",
			mcp.Required(),
			mcp.Description("Property map to apply e.g. {\"State\":\"On\",\"Label#1:0\":\"Save\"}"),
		),
		mcp.WithBoolean("resetOverrides", mcp.Description("Reset the instance to component defaults before applying properties (default false)")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{}
		if v, ok := args["properties"].(map[string]interface{}); ok {
			params["properties"] = v
		}
		if v, ok := args["resetOverrides"].(bool); ok {
			params["resetOverrides"] = v
		}
		resp, err := node.Send(ctx, "set_instance_properties", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_variable_mode",
		mcp.WithDescription("Pin a node to a specific mode of a variable collection (e.g. switch a frame's collection to Dark mode) via setExplicitVariableModeForCollection."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("collectionId",
			mcp.Required(),
			mcp.Description("Variable collection ID"),
		),
		mcp.WithString("modeId",
			mcp.Required(),
			mcp.Description("Mode ID within the collection to pin the node to"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{}
		if v, ok := args["collectionId"].(string); ok {
			params["collectionId"] = v
		}
		if v, ok := args["modeId"].(string); ok {
			params["modeId"] = v
		}
		resp, err := node.Send(ctx, "set_variable_mode", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_remote_variable_collection",
		mcp.WithDescription("Look up a remote (subscribed-library) variable collection by ID to discover its modes — getVariableCollectionByIdAsync, which local-only lookups miss."),
		mcp.WithString("collectionId",
			mcp.Required(),
			mcp.Description("Variable collection ID to resolve"),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if v, ok := req.GetArguments()["collectionId"].(string); ok {
			params["collectionId"] = v
		}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_remote_variable_collection", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("list_library_variable_collections",
		mcp.WithDescription("List all variable collections available from subscribed libraries, including their IDs and modes."),
		channelParam(),
		originParam(),
	), makeHandler(node, "list_library_variable_collections", nil, nil))

	s.AddTool(mcp.NewTool("get_library_variables",
		mcp.WithDescription("Get all variables in a subscribed library collection by its key (from list_library_variable_collections). Returns name, resolvedType, and valuesByMode for every variable — use this to read design tokens (colors, spacing, typography) from a library that is subscribed but not open in Figma. Does NOT require the library file to be open."),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Variable collection key from list_library_variable_collections, e.g. '544ed2a7248b18a0cc0a3213fa3f7ae95e9f5a21'"),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, _ := req.GetArguments()["key"].(string)
		params := map[string]interface{}{"key": key}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_library_variables", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("fetch_library_catalog",
		mcp.WithDescription("Fetch a Figma library's FULL published catalog via the REST API WITHOUT needing the file open in Figma. "+
			"Returns components, component_sets, styles, variables, and variableCollections (variables require Figma Enterprise plan — a 403 is surfaced as variablesError, not a fatal error). "+
			"Requires FIGMA_TOKEN env (read-only PAT, auto-loaded from .env). "+
			"Extract fileKey from any Figma URL: https://figma.com/design/<fileKey>/... "+
			"Writes the full catalog JSON to outPath; returns a small handle {outPath, counts, sample} — query the file with jq, do NOT expect inline data. "+
			"Use this to read design tokens from a variables-only library (e.g. HAE-DL UI Elements) or to build a component index without opening the file."),
		mcp.WithString("fileKey",
			mcp.Required(),
			mcp.Description("Figma file key — the path segment after /design/ in the file URL, e.g. 'IPHh6N4oJhcUSubEoPlCWP' from https://figma.com/design/IPHh6N4oJhcUSubEoPlCWP/..."),
		),
		mcp.WithString("outPath",
			mcp.Required(),
			mcp.Description("File path to write the full catalog JSON to (relative to working directory or absolute inside it)"),
		),
		mcp.WithString("scope",
			mcp.Description("Which endpoints to fetch: all (default), components, component_sets, styles, or variables"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		fileKey, _ := args["fileKey"].(string)
		outPath, _ := args["outPath"].(string)
		scope, _ := args["scope"].(string)
		if scope == "" {
			scope = "all"
		}
		workDir, err := os.Getwd()
		if err != nil {
			return mcp.NewToolResultError("getwd: " + err.Error()), nil
		}
		return executeFetchCatalog(ctx, httpCatalogFetcher, fileKey, scope, outPath, workDir)
	})
}
