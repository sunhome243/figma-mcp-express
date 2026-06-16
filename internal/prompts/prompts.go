package prompts

import "github.com/mark3labs/mcp-go/server"

// RegisterAll registers all MCP prompts on the server.
// Guidance-only prompts (read_design_strategy, design_strategy, style_audit_strategy,
// bulk_rename_strategy, text_replacement_strategy, annotation_conversion_strategy,
// reaction_to_connector_strategy, swap_overrides_instances) have been consolidated into
// skills/figma-mcp-express/references/ and are no longer registered as MCP prompts.
func RegisterAll(s *server.MCPServer) {
	addDesignTokenGenerationStrategy(s)
	addGenerateColorPalette(s)
	addGenerateTypeScale(s)
	addGenerateComponentVariants(s)
}
