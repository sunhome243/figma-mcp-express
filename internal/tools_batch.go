package internal

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerBatchTools registers the `batch` tool — an additive sequencing layer
// over the discrete read+write tools. It executes many ops in ONE plugin
// round-trip, with optional $N.field references so later ops can use earlier
// ops' results (e.g. create a frame, then add a child to its returned id, or
// search for a node then read it back).
func registerBatchTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("batch",
		mcp.WithDescription("Execute many ops (writes AND reads) in ONE plugin round-trip — far fewer round-trips than calling each tool separately. `ops` is an ordered array of {type, nodeIds?, params}, where `type` is any BatchOpCatalog op name (create_frame, set_fills, get_node, search_nodes, …).\n\n"+
			"WHEN TO USE: a KNOWN multi-step/dependent sequence (create→append→style), bulk (N× the same op), a read CHAIN (search_nodes→get_node on the found id), or build-then-VERIFY (writes then a get_node read-back, inline — the structural check before any screenshot). In the default core profile, low-level writes are normally validated batch/FigmaPlan ops; use search_batch_ops and get_batch_op_spec for unfamiliar params. Keep a batch to one logical section you can verify in one pass (~a few dozen ops), not a whole screen.\n\n"+
			"Safety caps: batches fail fast before plugin execution when ops exceed FIGMA_MCP_BATCH_MAX_OPS (default 200) or encoded ops exceed FIGMA_MCP_BATCH_MAX_BYTES (default 2MiB). Split large work into smaller logical sections; raise env caps only for controlled local runs.\n\n"+
			"READS in a batch are always LIVE and bypass the read singleflight/cache (the plugin slot is held for the whole batch). Use them for dependency chains and write→read verification — NOT as a cache bypass for heavy bulk catalog reads (get_local_components / fetch_library_catalog have REST + on-disk cache + dedup; call those directly).\n\n"+
			"Cross-op data flow: a later op references an earlier op's result with a \"$N.field\" string. Examples — verify after build, and a read chain:\n"+
			"ops=[{\"type\":\"create_frame\",\"params\":{\"name\":\"Card\"}}, {\"type\":\"set_fills\",\"nodeIds\":[\"$0.id\"],\"params\":{...}}, {\"type\":\"get_node\",\"nodeIds\":[\"$0.id\"],\"params\":{\"depth\":2}}].\n"+
			"ops=[{\"type\":\"search_nodes\",\"params\":{\"query\":\"Header\"}}, {\"type\":\"get_node\",\"nodeIds\":[\"$0.nodes.0.id\"],\"params\":{\"depth\":1}}].\n"+
			"Refs may point to EARLIER ops only ($N with N < the current op index); a forward/self ref is rejected. Nested paths use dot notation incl. array index ($0.bounds.width, $0.nodes.0.id) — brackets are not supported.\n\n"+
			"Stop policy: if any op uses a $N ref the batch is a dependent chain and STOPS at the first failing op (downstream refs would break). With no refs it is independent bulk and CONTINUES, reporting every op. Set `continueOnError` to override.\n"+
			"NOT transactional — Figma has no rollback; write ops before a failure stay applied. On partial failure, fix the failing op and re-send FROM that index.\n"+
			"Returns {results:[{i,type,data}|{i,type,error}], okCount, failCount, failedAt}. A large aggregate (e.g. several get_node results) spills to disk via the response gate — read the spilled path.\n\n"+
			"BatchOpCatalog is the SSOT for op contracts. The op `type` is a BatchOpCatalog op name, not proof that the op is exposed as a top-level MCP tool. Use get_batch_op_spec for exact params/enums, then batch(validateOnly:true) before generated or unfamiliar mutations."),
		mcp.WithArray("ops",
			mcp.Required(),
			mcp.Description("Ordered ops. Each: {type: BatchOpCatalog op name, nodeIds?: string[], params?: object}. Use \"$N.field\" strings anywhere in nodeIds/params to reference op N's result data."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":    map[string]any{"type": "string", "description": "BatchOpCatalog op name, e.g. create_frame, set_fills, get_node"},
					"nodeIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"params":  map[string]any{"type": "object"},
				},
				"required": []any{"type"},
			}),
		),
		mcp.WithBoolean("continueOnError",
			mcp.Description("Override the default stop policy: true = run all ops and report failures; false = stop at first failure. Default: stop when ops use $N refs, continue otherwise."),
		),
		mcp.WithBoolean("validateOnly",
			mcp.Description("Validate the declarative batch/FigmaPlan payload and return a report without sending anything to the Figma plugin."),
		),
		channelParam(),
		originParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawOps, err := batchOpsFromParams(req.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := validateBatchOps(rawOps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		normalizeBatchNodeIDs(rawOps)

		if validateOnly, _ := req.GetArguments()["validateOnly"].(bool); validateOnly {
			return mcp.NewToolResultStructuredOnly(map[string]interface{}{
				"valid":   true,
				"opCount": len(rawOps),
				"message": "batch/FigmaPlan validated; no plugin call was made",
			}), nil
		}

		prepareBatchImportParams(rawOps)

		params := map[string]interface{}{"ops": rawOps}
		if v, ok := req.GetArguments()["continueOnError"].(bool); ok {
			params["continueOnError"] = v
		}
		// Presence label: forwarded verbatim to the plugin (the bridge strips
		// only `channel`), where it attributes this write to a named agent. The
		// manual workflow `status` (and `task`) now flow through the dedicated
		// set_presence tool — NOT batch — so presence is one consistent path.
		if origin, ok := pickOrigin(req.GetArguments()); ok {
			params["origin"] = origin
		}

		resp, err := node.Send(ctx, "batch", nil, withChannel(req, params))
		// A partial batch is a SUCCESSFUL round-trip whose failures live inside
		// resp.Data, so it bypasses the node.Send hint layer. Fold the hint into
		// the result data and let renderResponse marshal + spill-gate it (a large
		// batch result must still go through the gate).
		if err == nil {
			if hint := batchFailureHint(resp); hint != "" {
				if data, ok := resp.Data.(map[string]interface{}); ok {
					data["hint"] = hint
				}
			}
		}
		return renderResponse(resp, err)
	})
}

// batchFailureHint returns a self-correction hint when a batch partially failed,
// or "" on full success.
func batchFailureHint(resp BridgeResponse) string {
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	fc, _ := data["failCount"].(float64)
	if fc <= 0 {
		return ""
	}
	failedAt, _ := data["failedAt"].(float64)
	return fmt.Sprintf("batch had %d failed op(s); first failure at op #%d. Fix that op and re-send FROM that index — earlier ops already applied (no rollback). Do not resend the whole batch.", int(fc), int(failedAt))
}
