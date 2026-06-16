package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWritePrototypeTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("set_reactions",
		mcp.WithDescription(`Set prototype reactions on a node. Use mode "replace" (default) to overwrite all reactions, or "append" to add to existing ones.

Supported triggers: ON_CLICK, ON_HOVER, ON_PRESS, ON_DRAG, AFTER_TIMEOUT (timeout ms), MOUSE_ENTER, MOUSE_LEAVE, MOUSE_UP, MOUSE_DOWN (delay ms), ON_KEY_DOWN (device, keyCodes[]), ON_MEDIA_HIT (mediaHitTime), ON_MEDIA_END
Supported action types: NODE (navigation), BACK, CLOSE, URL, SET_VARIABLE (variableId, variableValue?), SET_VARIABLE_MODE (variableCollectionId, variableModeId), CONDITIONAL (conditionalBlocks[]), UPDATE_MEDIA_RUNTIME (mediaAction)
  NODE navigation values: NAVIGATE, OVERLAY, SCROLL_TO, SWAP, CHANGE_TO
  NODE optional fields: overlayRelativePosition (OVERLAY only; takes effect when destination's overlayPositionType is MANUAL), resetScrollPosition, resetVideoPosition, resetInteractiveComponents
  URL optional field: openInNewTab (bool)
Transition types: DISSOLVE, SMART_ANIMATE, SCROLL_ANIMATE, MOVE_IN, MOVE_OUT, PUSH, SLIDE_IN, SLIDE_OUT
  DISSOLVE / SMART_ANIMATE / SCROLL_ANIMATE: {"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}}
  Directional (PUSH, MOVE_IN, MOVE_OUT, SLIDE_IN, SLIDE_OUT): also require "direction" (LEFT|RIGHT|TOP|BOTTOM) and "matchLayers" (bool):
    {"type":"PUSH","direction":"LEFT","matchLayers":false,"duration":0.3,"easing":{"type":"EASE_OUT"}}
  Easing types: EASE_IN, EASE_OUT, EASE_IN_AND_OUT, LINEAR, EASE_IN_BACK, EASE_OUT_BACK, EASE_IN_AND_OUT_BACK, GENTLE, QUICK, BOUNCY, SLOW, CUSTOM_CUBIC_BEZIER (easingFunctionCubicBezier), CUSTOM_SPRING (easingFunctionSpring)
NOTE: frame overlay appearance (overlayPositionType / overlayBackground / overlayBackgroundInteraction) is READ-ONLY in the plugin API — it must be configured in the Figma UI on the destination frame; only navigation:"OVERLAY" + overlayRelativePosition are settable here.

Each reaction has a "trigger" and an "actions" array (plural). Each action in the array is an Action object.

Example — on-click navigate with dissolve:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"NODE","destinationId":"1:3","navigation":"NAVIGATE","transition":{"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}

Example — on-click navigate with push (directional transition):
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"NODE","destinationId":"1:3","navigation":"NAVIGATE","transition":{"type":"PUSH","direction":"LEFT","matchLayers":false,"duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}

Example — open URL on hover:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_HOVER"},"actions":[{"type":"URL","url":"https://example.com"}]}]}

Example — auto-advance after 3 seconds:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"AFTER_TIMEOUT","timeout":3000},"actions":[{"type":"NODE","destinationId":"1:4","navigation":"NAVIGATE","transition":{"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}

Example — go back on click:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"BACK"}]}]}`),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithArray("reactions",
			mcp.Required(),
			mcp.Description("Array of reaction objects. Each has a 'trigger' and an 'actions' array (plural) of Action objects."),
			mcp.Items(map[string]any{"type": "object"}),
		),
		mcp.WithString("mode",
			mcp.Description(`"replace" (default) overwrites all existing reactions; "append" adds to them`),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]any{
			"reactions": args["reactions"],
		}
		if mode, ok := args["mode"].(string); ok && mode != "" {
			params["mode"] = mode
		}
		resp, err := node.Send(ctx, "set_reactions", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_prototype_start",
		mcp.WithDescription(`Set the prototype flow starting point(s) for the page that contains the given frame(s). The start of a prototype is controlled through the page's flow starting points — prototypeStartNode is read-only and cannot be set directly.

Pass one or more frame nodeIds (all must be on the same page). Each becomes a flow starting point. Provide "names" (parallel array) to label the flows; otherwise each flow takes the frame's name. mode "replace" (default) overwrites the page's starting points; "append" adds frames not already present; "remove" drops the given frames from the page's start points and keeps the rest (remove one without re-listing the others; tolerates an already-deleted frame, so it also clears dangling start points); "clear" removes ALL start points (nodeIds optional — uses the page of the first nodeId if given, else the current page).

Example — make frame 1:2 the start of an "Onboarding" flow:
{"nodeId":"1:2","names":["Onboarding"]}
Example — remove just frame 1:2 from the page's start points:
{"nodeId":"1:2","mode":"remove"}
Example — clear every flow starting point on the current page:
{"mode":"clear"}`),
		mcp.WithArray("nodeIds",
			mcp.Description("Frame node IDs to set as flow starting points. All must be on the same page. Colon format e.g. '4029:12345'. Required unless mode is 'clear'."),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithArray("names",
			mcp.Description("Optional flow names, parallel to nodeIds. Defaults to each frame's name."),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("mode",
			mcp.Description(`"replace" (default) overwrites the page's flow starting points; "append" adds frames not already present; "remove" drops the given frames and keeps the rest; "clear" removes all start points`),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		raw, _ := args["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		for i, id := range nodeIDs {
			nodeIDs[i] = NormalizeNodeID(id)
		}
		params := map[string]any{}
		if names := args["names"]; names != nil {
			params["names"] = names
		}
		if mode, ok := args["mode"].(string); ok && mode != "" {
			params["mode"] = mode
		}
		resp, err := node.Send(ctx, "set_prototype_start", nodeIDs, withChannel(req, params))
		return renderResponse(resp, err)
	})

	// LEVER 4 (tool demotion) — remove_reactions is DEMOTED to a batch-only op. Registration commented out (off tools/list); batch relays type "remove_reactions" to the untouched plugin handler. Uncomment to restore.
	// s.AddTool(mcp.NewTool("remove_reactions",
	// 	mcp.WithDescription("Remove prototype reactions from a node. Omit indices to remove all reactions. Provide a zero-based indices array to remove specific reactions (use get_reactions first to see current indices)."),
	// 	mcp.WithString("nodeId",
	// 		mcp.Required(),
	// 		mcp.Description("Node ID in colon format e.g. '4029:12345'"),
	// 	),
	// 	mcp.WithArray("indices",
	// 		mcp.Description("Zero-based indices of reactions to remove. Omit or pass [] to remove all."),
	// 		mcp.Items(map[string]any{"type": "number"}),
	// 	),
	// 	channelParam(),
	// ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 	args := req.GetArguments()
	// 	nodeID, _ := args["nodeId"].(string)
	// 	nodeID = NormalizeNodeID(nodeID)
	// 	params := map[string]any{}
	// 	// Pass indices as-is; the plugin handles both []any and JSON string.
	// 	if indices := args["indices"]; indices != nil {
	// 		params["indices"] = indices
	// 	}
	// 	resp, err := node.Send(ctx, "remove_reactions", []string{nodeID}, withChannel(req, params))
	// 	return renderResponse(resp, err)
	// })
}
