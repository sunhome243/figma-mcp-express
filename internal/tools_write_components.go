package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteComponentTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("navigate_to_page",
		mcp.WithDescription("Switch the active Figma page. Provide either pageId or pageName."),
		mcp.WithString("pageId", mcp.Description("Page node ID in colon format e.g. '0:1'")),
		mcp.WithString("pageName", mcp.Description("Exact page name to navigate to")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if id, ok := req.GetArguments()["pageId"].(string); ok && id != "" {
			params["pageId"] = id
		}
		if name, ok := req.GetArguments()["pageName"].(string); ok && name != "" {
			params["pageName"] = name
		}
		resp, err := node.Send(ctx, "navigate_to_page", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("group_nodes",
		mcp.WithDescription("Group two or more nodes into a GROUP. All nodes must share the same parent."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs to group (minimum 2), in colon format e.g. ['4029:12345', '4029:12346']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("name", mcp.Description("Optional name for the new group")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if name, ok := req.GetArguments()["name"].(string); ok && name != "" {
			params["name"] = name
		}
		resp, err := node.Send(ctx, "group_nodes", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — ungroup_nodes is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "ungroup_nodes" to the untouched plugin handler. Uncomment to restore.
	// s.AddTool(mcp.NewTool("ungroup_nodes",
	// 	mcp.WithDescription("Ungroup one or more GROUP nodes, moving their children to the parent and removing the group."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("GROUP node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	resp, err := node.Send(ctx, "ungroup_nodes", nodeIDs, withChannel(req, nil))
	// 	return renderResponse(resp, err)
	// })

	s.AddTool(mcp.NewTool("swap_component",
		mcp.WithDescription("Swap the main component of an existing INSTANCE node, replacing it with a different component while keeping position and size (this top-level tool takes one nodeId). Returns {results:[{nodeId,…}]}, a 1-element array. "+
			"Also a `batch` op type — and in `batch` it accepts nodeIds[] (all→all bulk: swap EVERY instance to the same target component) and returns {results:[{nodeId,…}]}. The canonical relibrary recipe: scan_nodes_by_types → swap_component nodeIds:[\"$0.matchingNodes[*].id\"] swaps every matching instance in ONE round-trip (scan_nodes_by_types returns matchingNodes; search_nodes returns nodes); read the swapped ids back with $N.results[*].nodeId."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("INSTANCE node ID in colon format e.g. 4029:12345"),
		),
		mcp.WithString("componentId",
			mcp.Required(),
			mcp.Description("Target COMPONENT node ID in colon format (from get_local_components)"),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		componentID, _ := args["componentId"].(string)
		componentID = NormalizeNodeID(componentID)
		params := map[string]interface{}{"componentId": componentID}
		resp, err := node.Send(ctx, "swap_component", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — detach_instance is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "detach_instance" to the untouched plugin handler. Uncomment to restore.
	// s.AddTool(mcp.NewTool("detach_instance",
	// 	mcp.WithDescription("Detach one or more component instances, converting them to plain frames. The link to the main component is broken; all visual properties are preserved. Also a `batch` op type."),
	// 	mcp.WithArray("nodeIds",
	// 		mcp.Required(),
	// 		mcp.Description("INSTANCE node IDs in colon format e.g. ['4029:12345']"),
	// 		mcp.WithStringItems(),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	raw, _ := req.GetArguments()["nodeIds"].([]interface{})
	// 	nodeIDs := toStringSlice(raw)
	// 	for i, id := range nodeIDs {
	// 		nodeIDs[i] = NormalizeNodeID(id)
	// 	}
	// 	resp, err := node.Send(ctx, "detach_instance", nodeIDs, withChannel(req, nil))
	// 	return renderResponse(resp, err)
	// })
}
