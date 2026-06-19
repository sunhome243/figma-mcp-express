package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerReadDocumentTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("get_document",
		mcp.WithDescription("Get the full node tree of the current page (not the whole file — only the active page). Returns all nodes recursively and can be very large. Prefer get_design_context for exploration or when token efficiency matters. Large responses are saved to disk and returned as {spilled:true,path,bytes,preview} — read the file with jq/grep; do not expect the full payload inline. Timeouts are server-managed; a read that times out should be re-scoped narrower, never given a longer timeout."),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_document", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_pages",
		mcp.WithDescription("List all pages in the document with their IDs and names. Lightweight alternative to get_document."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_pages", nil, nil))

	s.AddTool(mcp.NewTool("get_metadata",
		mcp.WithDescription("Get the lightweight structure/id map of the current document: file name, pages, current page (and node skeleton). The FIRST read in the official discipline — run it to find the ids you want, then target those nodes. "+
			"PREFER A SIBLING WHEN: you have ids and want full node detail → get_node / get_nodes_info; you want a token-efficient property tree of a region → get_design_context; you're hunting nodes by name/type → search_nodes / scan_nodes_by_types; you just want the page list → get_pages. "+
			"SCOPING: returns the map only — cheap and safe to call first. Large responses spill to disk as {spilled:true,path,bytes,preview} — read with jq/grep, don't expect the full payload inline. "+
			"CHAIN: get_metadata first → then get_node/get_nodes_info/get_design_context on the specific nodes it surfaced. Also a `batch` op type."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_metadata", nil, nil))

	s.AddTool(mcp.NewTool("get_selection",
		mcp.WithDescription("Get the nodes currently selected in Figma. Returns an empty array if nothing is selected. Use get_design_context or get_node to retrieve deeper detail about a specific node by ID."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_selection", nil, nil))

	s.AddTool(mcp.NewTool("get_image_by_hash",
		mcp.WithDescription("Get image metadata and bytes for an existing Figma image hash using figma.getImageByHash. Returns null image data when the hash is not in the file."),
		mcp.WithString("hash", mcp.Required(), mcp.Description("Figma image hash from an IMAGE paint.")),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{"hash": req.GetArguments()["hash"]}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_image_by_hash", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_file_thumbnail",
		mcp.WithDescription("Get the node currently assigned as the file thumbnail, if any."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_file_thumbnail", nil, nil))

	s.AddTool(mcp.NewTool("get_dev_resources",
		mcp.WithDescription("Read Dev Mode resource links attached to a node via the node DevResourcesMixin. Optionally includes child resources."),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Node ID in colon format e.g. '4029:12345'.")),
		mcp.WithBoolean("includeChildren", mcp.Description("When true, include dev resources attached to descendants.")),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{"nodeId": NormalizeNodeID(req.GetArguments()["nodeId"].(string))}
		if v, ok := req.GetArguments()["includeChildren"].(bool); ok {
			params["includeChildren"] = v
		}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_dev_resources", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("resolve_variable_for_consumer",
		mcp.WithDescription("Resolve a variable's effective value for a specific scene node using Variable.resolveForConsumer."),
		mcp.WithString("variableId", mcp.Required(), mcp.Description("Variable ID to resolve.")),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Consumer scene node ID in colon format.")),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{
			"variableId": req.GetArguments()["variableId"],
			"nodeId":     NormalizeNodeID(req.GetArguments()["nodeId"].(string)),
		}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "resolve_variable_for_consumer", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_node",
		mcp.WithDescription("Get one node by ID with full detail (fills, strokes, text, layout, boundVariables). "+
			"PREFER A SIBLING WHEN: you need several nodes → get_nodes_info (one round-trip, not a get_node loop); you only want the structure/id map → get_metadata; you want a token-efficient overview of a whole selection/page → get_design_context; you're searching by name/type rather than a known id → search_nodes / scan_nodes_by_types. "+
			"SCOPING: node ID must be colon format e.g. '4029:12345', never hyphens. depth caps traversal — DEFAULT depth is 50 (was unbounded); pass a larger depth (or Infinity) only for a deliberate deep read. A returned node carrying a `childCount` field (with no `children`) was TRUNCATED at the depth cap — it is NOT a real leaf; request a larger `depth` to expand it (childCount:0 is a genuine leaf). Large responses spill to disk as {spilled:true,path,bytes,preview} — read with jq/grep, don't expect the full payload inline. Timeouts are server-managed; a slow read that ticks progress is progressing, not hung — the fix for a real timeout is a narrower read, not a longer timeout. "+
			"CHAIN: official discipline is get_metadata first → then get_node on the specific target nodes. Also a `batch` op type — chain it as a write→read verify (a trailing get_node read-back) or after search_nodes (get_node on $0.nodes.0.id)."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithNumber("depth",
			mcp.Description("How many levels deep to traverse (default: full depth). depth:0 returns the node only (no children), depth:1 returns node + direct children, etc."),
		),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{}
		if d, ok := req.GetArguments()["depth"].(float64); ok && d >= 0 {
			params["depth"] = d
		}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_node", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_nodes_info",
		mcp.WithDescription("Get full details for multiple nodes by ID in one round-trip. "+
			"PREFER A SIBLING WHEN: you only need ONE node → get_node (simpler); you want the structure/id map of a subtree → get_metadata; you want a token-efficient whole-selection/page tree → get_design_context; you don't yet know the ids → search_nodes / scan_nodes_by_types to discover them first. This is the right tool the moment you have 2+ known ids — prefer it over a get_node loop. "+
			"SCOPING: ids in colon format e.g. ['4029:12345','4029:67890']. depth bounds traversal per node (default 50). Large responses spill to disk — read with jq/grep. "+
			"CHAIN: official discipline is get_metadata first → then get_nodes_info on the targets. Also a `batch` op type — use it as a trailing write→read verify over several created ids in one batch."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("List of node IDs in colon format e.g. ['4029:12345', '4029:67890']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("depth",
			mcp.Description("How many levels deep to traverse per node (default 50, bounded). depth:0 returns each node only (no children), depth:1 node + direct children, etc."),
		),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if d, ok := req.GetArguments()["depth"].(float64); ok && d >= 0 {
			params["depth"] = d
		}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_nodes_info", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_design_context",
		mcp.WithDescription("Depth-limited, token-efficient node tree. Pass nodeId to scope to a subtree, else reads the current selection/page. detail=minimal|compact|full|codegen; dedupe_components collapses repeated instances. Large results spill to disk."),
		mcp.WithNumber("depth",
			mcp.Description("How many levels deep to traverse (default 2)"),
		),
		mcp.WithString("detail",
			mcp.Description("Property verbosity: minimal (id/name/type/bounds only), compact (+fills/strokes/opacity), full (everything, default), codegen (full + autoLayout + resolved design-token names + INSTANCE componentRef/Code-Connect mapping for React/Tailwind generation)"),
		),
		mcp.WithBoolean("dedupe_components",
			mcp.Description("When true, INSTANCE nodes are serialized compactly (mainComponentId + componentProperties + overrides array of differing text/nested content) and unique component definitions are collected once in a top-level componentDefs map. Highly token-efficient for screens with many repeated component instances."),
		),
		mcp.WithObject("codeConnectMap",
			mcp.Description("Optional Code-Connect map keyed by published component key → an arbitrary mapping value (e.g. {\"abc123\": {\"component\": \"Button\", \"import\": \"@/ui/button\"}}). Only used with detail=codegen: any INSTANCE whose main-component key is present gets a codeConnect field attached. Pass-through only — the plugin does not read files."),
		),
		mcp.WithString("nodeId",
			mcp.Description("Optional. Scope to this node's subtree (e.g. 4029:12345). Omit to read the current selection/page."),
		),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if d, ok := req.GetArguments()["depth"].(float64); ok && d > 0 {
			params["depth"] = d
		}
		if id, ok := req.GetArguments()["nodeId"].(string); ok && id != "" {
			params["nodeId"] = NormalizeNodeID(id)
		}
		applyOrigin(req, params)
		detail, _ := req.GetArguments()["detail"].(string)
		if detail != "" {
			params["detail"] = detail
		}
		if dd, ok := req.GetArguments()["dedupe_components"].(bool); ok && dd {
			params["dedupeComponents"] = true
		}
		applySkipInvisible(req, params)
		if cc, ok := req.GetArguments()["codeConnectMap"].(map[string]interface{}); ok && len(cc) > 0 {
			params["codeConnectMap"] = cc
		}
		resp, err := node.Send(ctx, "get_design_context", nil, withChannel(req, params))
		// Attach a just-in-time hint on successful reads at detail levels that omit
		// typography, color, and autoLayout — nudging toward detail:full or detail:codegen
		// without requiring the user to know about the dormant prompt.
		if err == nil && resp.Error == "" {
			if hint := hintForDesignContextDetail(detail); hint != "" {
				if data, ok := resp.Data.(map[string]interface{}); ok {
					m2 := make(map[string]interface{}, len(data)+1)
					for k, v := range data {
						m2[k] = v
					}
					m2["hint"] = hint
					resp.Data = m2
				}
			}
		}
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("search_nodes",
		mcp.WithDescription("Find nodes by NAME substring (optionally filtered by type) within a subtree. Use when you know (part of) a node's name. "+
			"PREFER A SIBLING WHEN: name doesn't matter and you want every node of a type → scan_nodes_by_types; you only want TEXT content → scan_text_nodes; you already have the id and want detail → get_node / get_nodes_info; you want the whole structure map → get_metadata. "+
			"SCOPING: always pass nodeId (scope to a frame, not the whole page) + types + limit (default 50) — an unscoped search can jam the single-threaded plugin. Timeouts are server-managed; fix a real timeout by narrowing scope, not requesting a longer timeout. "+
			"CHAIN: search_nodes → get_node on the found id is the canonical discovery→detail chain. Also a `batch` op type — feed its results forward with the projection ref $N.nodes[*].id (e.g. into a bulk swap_component), or single via $0.nodes.0.id."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Name substring to match (case-insensitive)"),
		),
		mcp.WithString("nodeId",
			mcp.Description("Scope search to this subtree (default: current page), colon format e.g. '4029:12345'"),
		),
		mcp.WithArray("types",
			mcp.Description("Filter by Figma node type e.g. ['TEXT', 'FRAME', 'COMPONENT']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 50)"),
		),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{
			"query": req.GetArguments()["query"],
		}
		if id, ok := req.GetArguments()["nodeId"].(string); ok && id != "" {
			params["nodeId"] = id
		}
		if raw, ok := req.GetArguments()["types"].([]interface{}); ok && len(raw) > 0 {
			params["types"] = raw
		}
		if limit, ok := req.GetArguments()["limit"].(float64); ok && limit > 0 {
			params["limit"] = limit
		}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "search_nodes", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("scan_text_nodes",
		mcp.WithDescription("Scan all TEXT nodes in a subtree and return their content. Shorthand for scan_nodes_by_types with ['TEXT'] — use when you only need the text copy of a component or frame. "+
			"PREFER A SIBLING WHEN: you want other node types too → scan_nodes_by_types (pass the types); you're matching by name → search_nodes; you already have ids and want full detail → get_nodes_info; you want to find-and-replace copy rather than read it → find_replace_text (a write). "+
			"SCOPING: scope nodeId tightly — scanning a huge subtree can jam the plugin; large results spill to disk as {spilled:true,path} (read with jq). Timeouts are server-managed; fix a real timeout by narrowing scope. "+
			"CHAIN: pair with get_metadata first to pick the subtree root. Also a `batch` op type."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Root node ID to scan from, colon format e.g. '4029:12345'"),
		),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{"nodeId": nodeID}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "scan_text_nodes", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("scan_nodes_by_types",
		mcp.WithDescription("Find all nodes of specific types in a subtree, regardless of name. The all-of-a-kind sweep (e.g. every INSTANCE / FRAME / COMPONENT under a frame). "+
			"PREFER A SIBLING WHEN: you're matching by NAME → search_nodes; you only want TEXT → scan_text_nodes (shorthand for types:['TEXT']); you already have the ids → get_nodes_info; you want the structure map first → get_metadata. "+
			"SCOPING: scope nodeId tightly + pass types — scanning a huge subtree can jam the plugin; large results spill to disk as {spilled:true,path} (read with jq). Timeouts are server-managed; fix a real timeout by narrowing scope. "+
			"CHAIN: scan → feed the results forward in a `batch` via the projection ref $N.matchingNodes[*].id (scan_nodes_by_types returns matchingNodes; e.g. swap_component / set_fills over every match in ONE round-trip). Also a `batch` op type."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Root node ID to scan from, colon format e.g. '4029:12345'"),
		),
		mcp.WithArray("types",
			mcp.Required(),
			mcp.Description("Node types to find e.g. ['FRAME', 'COMPONENT', 'INSTANCE']"),
			mcp.WithStringItems(),
		),
		skipInvisibleChildrenParam(),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		raw, _ := req.GetArguments()["types"].([]interface{})
		params := map[string]interface{}{
			"nodeId": nodeID,
			"types":  raw,
		}
		applySkipInvisible(req, params)
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "scan_nodes_by_types", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_reactions",
		mcp.WithDescription("Get the prototype reactions defined on a node. Returns an array of reaction objects — each has a trigger (e.g. ON_CLICK, ON_HOVER, AFTER_TIMEOUT) and an actions array (navigate to node, open URL, go back, etc.). Use set_reactions to add or replace reactions, remove_reactions to delete them."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_reactions", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_prototype",
		mcp.WithDescription(`Read the prototype FLOW GRAPH for a whole page (or a scoped subtree). Unlike get_reactions (one node's raw reactions), this walks every reaction-bearing node — buttons and instances included, not just top-level frames — and returns the connections between them.

Returns: { pageId, pageName, flowStartingPoints:[{nodeId,name}], prototypeStartNodeId, reactionNodeCount, edgeCount, edges:[...], overlays:[...] }.
  edges[]: { sourceId, sourceName, sourceType, trigger, actionType, navigation?, destinationId?, destinationName?, transition?, url? } — one entry per (source node, reaction, action).
  overlays[]: read-only overlay config of OVERLAY destinations { nodeId, name, overlayPositionType, overlayBackground, overlayBackgroundInteraction }. These cannot be set via the plugin API — use them to detect a dropdown/sheet still at the default CENTER position.

Pass nodeId(s) to scope to subtrees on one page; omit to read the current page. Use this to audit a prototype (dead-ends, unwired buttons, missing back, overlay placement) before or after wiring with set_reactions / set_prototype_start.`),
		mcp.WithString("nodeId",
			mcp.Description("Optional. Scope the read to this node's subtree. Omit to read the whole current page. Colon format e.g. '4029:12345'."),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var nodeIDs []string
		if nodeID, ok := req.GetArguments()["nodeId"].(string); ok && nodeID != "" {
			nodeIDs = []string{NormalizeNodeID(nodeID)}
		}
		params := map[string]interface{}{}
		applyOrigin(req, params)
		resp, err := node.Send(ctx, "get_prototype", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_viewport",
		mcp.WithDescription("Get the current Figma viewport: scroll center, zoom level, and visible bounds."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_viewport", nil, nil))

	s.AddTool(mcp.NewTool("get_fonts",
		mcp.WithDescription("List all fonts used in the current page, sorted by usage frequency. Useful for understanding typography without scanning all text nodes."),
		channelParam(),
		originParam(),
	), makeHandler(node, "get_fonts", nil, nil))
}
