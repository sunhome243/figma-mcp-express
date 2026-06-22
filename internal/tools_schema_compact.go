package internal

import (
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/server"
)

const toolSchemaModeEnv = "FIGMA_MCP_TOOL_SCHEMA_MODE"

var compactToolDescriptions = map[string]string{
	"batch":                             "Run ordered BatchOpCatalog ops in one plugin round trip. For unfamiliar mutations, inspect get_batch_op_spec then use validateOnly.",
	"export_frames_to_pdf":              "Export selected/provided frames as one PDF payload.",
	"export_tokens":                     "Export local variables and paint styles as JSON or CSS design tokens.",
	"fetch_library_catalog":             "Fetch a published Figma library catalog through REST. Requires FIGMA_TOKEN; file need not be open.",
	"get_batch_op_spec":                 "Return the live schema for one BatchOpCatalog op. Use before composing unfamiliar batch ops.",
	"get_design_context":                "Token-efficient tree for selection/page/subtree. detail=minimal|compact|full|codegen; bound depth. Large output may be spilled.",
	"get_document":                      "Full current-page tree. Very large; prefer get_design_context or targeted reads. Large output may be spilled.",
	"get_local_components":              "List local components/sets. Omit pageId for file-local create_instance recovery; pass pageId to bound large scans. Large output may be spilled.",
	"get_metadata":                      "Lightweight file/page map. Start here to find ids before targeted reads.",
	"get_node":                          "Read one node by id. Set small depth for cheap inspection; large output may be spilled.",
	"get_nodes_info":                    "Read multiple known node ids in one call. Prefer over repeated get_node. Large output may be spilled.",
	"get_pages":                         "List pages with ids and names.",
	"get_screenshot":                    "Export selected/provided nodes as base64 image data. Prefer save_screenshots to write files.",
	"get_selection":                     "Return currently selected nodes; use to scope follow-up reads.",
	"get_styles":                        "List local paint, text, effect, and grid styles. Use variables for design tokens.",
	"get_variable_defs":                 "List local variable collections, modes, variables, and values. Local only; check libraries separately.",
	"list_channels":                     "List connected Figma files/channels. Use first in multi-file sessions; pass channel on file-specific tools.",
	"list_library_variable_collections": "List subscribed-library variable collections and modes; use when get_variable_defs is empty or local-only.",
	"save_screenshots":                  "Export node screenshots to local files. Returns paths/metadata, not base64.",
	"scan_nodes_by_types":               "List nodes of requested Figma types in a scoped subtree. Large output may be spilled.",
	"scan_text_nodes":                   "List TEXT nodes and copy in a scoped subtree. Large output may be spilled.",
	"search_batch_ops":                  "Search BatchOpCatalog by intent, op name, category, read/write flag, or param key before get_batch_op_spec.",
	"search_nodes":                      "Find nodes by name substring and optional type filter. Scope with nodeId and limit.",
	"set_presence":                      "Update Watch-agent status/task without touching Figma. Use for workflow transitions; operational tools carry only origin/channel.",
}

// compactToolSchemas keeps the live MCP schema useful while reducing the
// tools/list token cost. The detailed docs remain in source/README; clients can
// opt back into the old verbose schema with FIGMA_MCP_TOOL_SCHEMA_MODE=verbose.
func compactToolSchemas(s *server.MCPServer) {
	if strings.EqualFold(os.Getenv(toolSchemaModeEnv), "verbose") {
		return
	}

	listed := s.ListTools()
	if len(listed) == 0 {
		return
	}

	tools := make([]server.ServerTool, 0, len(listed))
	for _, st := range listed {
		tool := st.Tool
		tool.Description = compactToolDescription(tool.Name, tool.Description)
		compactDescriptions(tool.InputSchema.Properties, 64)
		tools = append(tools, server.ServerTool{
			Tool:    tool,
			Handler: st.Handler,
		})
	}
	s.SetTools(tools...)
}

func compactToolDescription(name, desc string) string {
	if v, ok := compactToolDescriptions[name]; ok {
		return v
	}
	return compactText(desc, 140)
}

func compactDescriptions(v any, max int) {
	switch x := v.(type) {
	case map[string]any:
		if desc, ok := x["description"].(string); ok {
			x["description"] = compactText(desc, max)
		}
		for _, child := range x {
			compactDescriptions(child, max)
		}
	case []any:
		for _, child := range x {
			compactDescriptions(child, max)
		}
	}
}

func compactText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := strings.LastIndex(s[:max], " ")
	if cut < max/2 {
		cut = max
	}
	return strings.TrimSpace(s[:cut]) + "..."
}
