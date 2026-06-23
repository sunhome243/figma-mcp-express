package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerReadStyleTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("get_styles",
		mcp.WithDescription("Get all local styles in the document (paint, text, effect, and grid). Returns each style's ID, name, type, and properties. Use the style ID with apply_style_to_node or update_paint_style. For design tokens (variables), use get_variable_defs instead."),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_styles", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_variable_defs",
		mcp.WithDescription("Get all local variable definitions: collections, modes, and values. Variables are Figma's design token system."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_variable_defs", nil, nil))

	s.AddTool(mcp.NewTool("get_selection_colors",
		mcp.WithDescription("Get Figma's computed colors from the current selection using getSelectionColors."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_selection_colors", nil, nil))

	s.AddTool(mcp.NewTool("get_local_components",
		mcp.WithDescription("Get all components defined in the current Figma file, including file-local masters needed by create_instance {componentId}. Omit pageId for the whole-file recovery scan (loads all pages and can find local masters missed by page traversal); pass pageId to scan ONE page in large libraries. Large results are saved to disk and returned as {spilled:true,path} — read with jq. Timeouts are server-managed; a read that times out should be re-scoped narrower, never given a longer timeout."),
		mcp.WithString("pageId",
			mcp.Description("Optional — scope scan to a single page by its node ID (colon format e.g. '0:1'). Omit to scan all pages."),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if id, ok := req.GetArguments()["pageId"].(string); ok && id != "" {
			params["pageId"] = id
		}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_local_components", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_annotations",
		mcp.WithDescription("Get dev-mode annotations in the current document or scoped to a specific node. Returns annotation objects with label text, measurement type, and the ID of the annotated node. Omit nodeId to retrieve all annotations on the current page."),
		mcp.WithString("nodeId",
			mcp.Description("Optional — scope results to annotations on this node and its descendants, colon format e.g. '4029:12345'"),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if id, ok := req.GetArguments()["nodeId"].(string); ok && id != "" {
			params["nodeId"] = id
		}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_annotations", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("export_tokens",
		mcp.WithDescription("Export all design tokens (variables and paint styles) as JSON or CSS custom properties. Ideal for bridging Figma variables into your codebase."),
		mcp.WithString("format", mcp.Description("Output format: json (default) or css")),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if f, ok := req.GetArguments()["format"].(string); ok && f != "" {
			params["format"] = f
		}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "export_tokens", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})
}
