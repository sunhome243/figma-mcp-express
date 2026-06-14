package internal

import (
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/server"
)

const toolSchemaModeEnv = "FIGMA_MCP_TOOL_SCHEMA_MODE"

var compactToolDescriptions = map[string]string{
	"get_document":          "Full current-page tree. Very large; prefer get_design_context or targeted reads. Large output may be spilled.",
	"get_metadata":          "Lightweight file/page map. Use first to find ids, then target get_node/get_nodes_info/get_design_context.",
	"get_node":              "Read one node by id. depth defaults to 50; use smaller depth for cheap inspection. Large output may be spilled.",
	"get_nodes_info":        "Read multiple known node ids in one round trip. Prefer over repeated get_node calls. Large output may be spilled.",
	"get_design_context":    "Token-efficient selection/page tree. detail=minimal|compact|full|codegen; depth default 2; dedupe_components reduces repeated instances. Large output may be spilled.",
	"search_nodes":          "Find nodes by name substring, optional type filter. Scope with nodeId and limit to avoid broad page scans.",
	"scan_text_nodes":       "Return TEXT nodes and copy inside a scoped subtree. Large output may be spilled.",
	"scan_nodes_by_types":   "Return all nodes of requested Figma types in a scoped subtree. Large output may be spilled.",
	"get_local_components":  "List local components/component sets. Pass pageId for large libraries to scan one page. Large output may be spilled.",
	"get_screenshot":        "Export selected/provided nodes as base64 image data. Prefer save_screenshots to write files without inlining base64.",
	"save_screenshots":      "Export node screenshots directly to local files. Returns metadata only, not base64.",
	"export_frames_to_pdf":  "Export multiple frames as one PDF payload.",
	"fetch_library_catalog": "Fetch a published Figma library catalog through REST. Requires FIGMA_TOKEN.",
	"batch":                 "Run ordered BatchOpCatalog ops in one plugin round trip. Use get_batch_op_spec and validateOnly before unfamiliar mutations.",
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
		compactDescriptions(tool.InputSchema.Properties, 72)
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
