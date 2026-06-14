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
		mcp.WithDescription("Execute many ops (writes AND reads) in ONE plugin round-trip — far fewer round-trips than calling each tool separately. `ops` is an ordered array of {type, nodeIds?, params}, where `type` is any tool name (create_frame, set_fills, get_node, search_nodes, …).\n\n"+
			"WHEN TO USE: a KNOWN multi-step/dependent sequence (create→append→style), bulk (N× the same op), a read CHAIN (search_nodes→get_node on the found id), or build-then-VERIFY (writes then a get_node read-back, inline — the structural check before any screenshot). For a single fine adjustment (one fill, one radius, one label), or when you're still exploring and need to react to each read before the next, call the SPECIFIC tool directly. Keep a batch to one logical section you can verify in one pass (~a few dozen ops), not a whole screen.\n\n"+
			"READS in a batch are always LIVE and bypass the read singleflight/cache (the plugin slot is held for the whole batch). Use them for dependency chains and write→read verification — NOT as a cache bypass for heavy bulk catalog reads (get_local_components / fetch_library_catalog have REST + on-disk cache + dedup; call those directly).\n\n"+
			"Cross-op data flow: a later op references an earlier op's result with a \"$N.field\" string. Examples — verify after build, and a read chain:\n"+
			"ops=[{\"type\":\"create_frame\",\"params\":{\"name\":\"Card\"}}, {\"type\":\"set_fills\",\"nodeIds\":[\"$0.id\"],\"params\":{...}}, {\"type\":\"get_node\",\"nodeIds\":[\"$0.id\"],\"params\":{\"depth\":2}}].\n"+
			"ops=[{\"type\":\"search_nodes\",\"params\":{\"query\":\"Header\"}}, {\"type\":\"get_node\",\"nodeIds\":[\"$0.nodes.0.id\"],\"params\":{\"depth\":1}}].\n"+
			"Refs may point to EARLIER ops only ($N with N < the current op index); a forward/self ref is rejected. Nested paths use dot notation incl. array index ($0.bounds.width, $0.nodes.0.id) — brackets are not supported.\n\n"+
			"Stop policy: if any op uses a $N ref the batch is a dependent chain and STOPS at the first failing op (downstream refs would break). With no refs it is independent bulk and CONTINUES, reporting every op. Set `continueOnError` to override.\n"+
			"NOT transactional — Figma has no rollback; write ops before a failure stay applied. On partial failure, fix the failing op and re-send FROM that index.\n"+
			"Returns {results:[{i,type,data}|{i,type,error}], okCount, failCount, failedAt}. A large aggregate (e.g. several get_node results) spills to disk via the response gate — read the spilled path.\n\n"+
			"Demoted op types + their full params/enums: references/batch-recipes.md § Demoted op types."),
		mcp.WithArray("ops",
			mcp.Required(),
			mcp.Description("Ordered ops. Each: {type: string (read or write tool name), nodeIds?: string[], params?: object}. Use \"$N.field\" strings anywhere in nodeIds/params to reference op N's result data."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":    map[string]any{"type": "string", "description": "tool name, e.g. create_frame, set_fills, get_node"},
					"nodeIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"params":  map[string]any{"type": "object"},
				},
				"required": []any{"type"},
			}),
		),
		mcp.WithBoolean("continueOnError",
			mcp.Description("Override the default stop policy: true = run all ops and report failures; false = stop at first failure. Default: stop when ops use $N refs, continue otherwise."),
		),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawOps, ok := req.GetArguments()["ops"].([]interface{})
		if !ok || len(rawOps) == 0 {
			return mcp.NewToolResultError("batch requires a non-empty `ops` array"), nil
		}

		// Coarse Go-side gate: every op must name a type (read or write), and a
		// batch cannot nest. The plugin does the fine-grained unknown-type check.
		for i, raw := range rawOps {
			op, ok := raw.(map[string]interface{})
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("ops[%d] must be an object {type, nodeIds?, params?}", i)), nil
			}
			t, _ := op["type"].(string)
			if t == "" {
				return mcp.NewToolResultError(fmt.Sprintf("ops[%d] missing string `type`", i)), nil
			}
			if t == "batch" {
				return mcp.NewToolResultError(fmt.Sprintf("ops[%d]: batch cannot be nested", i)), nil
			}
			// Reads are allowed (dependency chains, write→read verification);
			// they bypass singleflight since the slot is held for the whole batch.
			// Normalize hyphen-format ids LLMs sometimes emit in this op's nodeIds.
			if nids, ok := op["nodeIds"].([]interface{}); ok {
				for j, v := range nids {
					if s, ok := v.(string); ok {
						nids[j] = NormalizeNodeID(s)
					}
				}
			}
			// Reject Plugin-API-name param typos PER OP — a batch otherwise bypasses
			// ValidateRPC, so without this the silent-drop bug (e.g. create_text
			// `characters` → empty invisible node) survives inside batch. Uses the same
			// schema-derived allowlist as the direct path; the key check ignores values,
			// so it is safe against $N refs in param values.
			if opParams, ok := op["params"].(map[string]interface{}); ok {
				if msg := rejectUnknownToolParams(t, opParams); msg != "" {
					return mcp.NewToolResultError(fmt.Sprintf("ops[%d]: %s", i, msg)), nil
				}
			}
		}

		params := map[string]interface{}{"ops": rawOps}
		if v, ok := req.GetArguments()["continueOnError"].(bool); ok {
			params["continueOnError"] = v
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
