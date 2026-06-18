package internal

import (
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/server"
)

const toolProfileEnv = "FIGMA_MCP_TOOL_PROFILE"

var coreToolSurface = map[string]bool{
	"list_channels":                     true,
	"set_presence":                      true,
	"batch":                             true,
	"search_batch_ops":                  true,
	"get_batch_op_spec":                 true,
	"get_metadata":                      true,
	"get_node":                          true,
	"get_nodes_info":                    true,
	"get_design_context":                true,
	"get_selection":                     true,
	"get_pages":                         true,
	"search_nodes":                      true,
	"scan_nodes_by_types":               true,
	"scan_text_nodes":                   true,
	"save_screenshots":                  true,
	"export_frames_to_pdf":              true,
	"fetch_library_catalog":             true,
	"get_styles":                        true,
	"get_variable_defs":                 true,
	"get_local_components":              true,
	"list_library_variable_collections": true,
	"export_tokens":                     true,
}

func applyToolProfile(s *server.MCPServer) {
	if strings.EqualFold(os.Getenv(toolProfileEnv), "full") {
		return
	}

	listed := s.ListTools()
	tools := make([]server.ServerTool, 0, len(coreToolSurface))
	for _, st := range listed {
		if coreToolSurface[st.Tool.Name] {
			tools = append(tools, server.ServerTool{
				Tool:    st.Tool,
				Handler: st.Handler,
			})
		}
	}
	s.SetTools(tools...)
}
