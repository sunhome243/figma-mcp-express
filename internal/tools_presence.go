package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPresenceTools registers set_presence — the dedicated presence tool.
//
// Presence is intentionally SEPARATED from operational tools: operational tools
// carry only the required `origin` (per-call attribution + auto-status), while the
// sticky self-narration (manual workflow `status` + one-sentence `task`) flows
// through this one tool. That removes the old "status only on batch" asymmetry.
//
// set_presence performs NO Figma mutation — the plugin records the carried
// (sessionId, origin, status, task) into its Watch-agent panel and returns ok. The
// plugin must NOT run opStatus on this command (its catch-all would falsely show
// "Building…"); it applies only the explicit status/task (absent → keep prior).
func registerPresenceTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("set_presence",
		mcp.WithDescription("Update your presence in the Figma plugin's Watch-agent panel WITHOUT performing a Figma operation. Set your manual status (workflow phase) and/or task (a one-sentence description of what you are working on). The orchestrator calls this once per agent right after dispatch to stamp the agent's task; an agent may call it again when its phase changes. Does not touch the canvas."),
		originParam(),
		statusParam(),
		taskParam(),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		applyOrigin(req, params)
		if status, ok := pickStatus(req.GetArguments()); ok {
			params["status"] = status
		}
		applyTask(req, params)
		resp, err := node.Send(ctx, "set_presence", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})
}
