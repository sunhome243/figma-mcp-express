package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addDesignTokenGenerationStrategy(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("design_token_generation_strategy",
		mcp.WithPromptDescription("Extract raw values from an existing design and build a structured variable + style token system"),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"Extract raw values from an existing design and build a structured variable + style token system",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(`# Design Token Generation Strategy

Scan an existing design to discover all unique colors, font sizes, spacing values, and radii,
then create a structured variable collection and named styles, and finally link nodes to them.

## Steps

### Phase 1 — Discovery

	1. Call get_styles() to check what styles already exist (avoid duplicating them).
	2. Call get_variable_defs() to check existing variables.
	3. Scope discovery to a target page/frame first. Use search_nodes / scan_nodes_by_types / scan_text_nodes for broad value collection, then get_node or get_design_context(detail="compact", depth:<needed>) for targeted inspection.
	4. Collect unique values:
	   - **Colors**: all unique hex fills and stroke colors across nodes.
	   - **Font sizes**: all unique fontSize values on TEXT nodes.
   - **Spacing**: all unique itemSpacing, paddingTop/Right/Bottom/Left values on FRAME nodes.
   - **Radii**: all unique cornerRadius values.

### Phase 2 — Token naming

Map discovered values to semantic token names. Use this hierarchy:

**Colors** (variable collection "Primitives"):
- Sort colors by hue/lightness.
- Assign names like "Blue/100", "Blue/200", … "Blue/900", "Neutral/50", "Neutral/900", etc.
- Also create a "Semantic" collection with aliases: "Color/Primary", "Color/Background", "Color/Text", etc.

**Spacing** (variable collection "Spacing"):
- Name by scale: "Spacing/0" (0), "Spacing/1" (4px), "Spacing/2" (8px), "Spacing/3" (12px), …

**Radius** (variable collection "Radius"):
- Name: "Radius/None" (0), "Radius/SM" (4), "Radius/MD" (8), "Radius/LG" (16), "Radius/Full" (9999)

**Typography** (local text styles):
- Name: "Heading/H1", "Heading/H2", "Body/Regular", "Body/Small", "Label/Medium", etc.

Present the full token plan to the user for approval before creating anything.

### Phase 3 — Creation

	Use the live BatchOpCatalog for exact params: search_batch_ops → get_batch_op_spec → batch(validateOnly:true) → batch.
	Do not duplicate op schemas from memory.
	- Create each variable collection once, then create approved variables inside that collection and set values per mode.
	- COLOR/FLOAT variables: batch op create_variable_collection once per collection → create_variable → set_variable_value.
	- Text styles: inspect create_text_style with get_batch_op_spec; use the returned typography params, not shorthand names such as lineHeight or letterSpacing.
	- Paint styles: inspect create_paint_style with get_batch_op_spec; prefer variables for themeable colors.

### Phase 4 — Linking (optional, ask user)

After creating tokens, offer to link existing nodes:
- For nodes with raw fill colors that match a new variable → batch op bind_variable_to_node with field "fillColor"
- For TEXT nodes with matching font styles → batch op apply_style_to_node with the matching styleId

## Multi-mode / Theming (Light & Dark)

### Paid plan (Professional / Organization / Enterprise)
Use Figma's native multi-mode variables:
1. batch op create_variable_collection — creates the collection with one default mode (e.g. "Light").
2. batch op add_variable_mode — add the second mode (e.g. "Dark").
3. batch op create_variable — create each variable once; it exists in all modes automatically.
4. batch op set_variable_value — call once per mode to set the Light value, then again with the Dark value.

### Free plan workaround (when add_variable_mode returns "Limited to 1 modes only")
Figma's free plan is capped at 1 mode per collection. Detect this error and fall back to
name-encoded modes: keep a **single collection with 1 mode**, and prefix every variable name
with the mode it belongs to.

Naming convention — use a slash-prefix per mode:
- light/color-bg, dark/color-bg
- light/color-text, dark/color-text
- light/color-primary, dark/color-primary

Steps:
1. batch op create_variable_collection — one collection, accept the single default mode as-is.
2. batch op create_variable with name "light/<token>" — Light variant.
3. batch op create_variable with name "dark/<token>" — Dark variant.
4. batch op set_variable_value for each variable.

Bind the active-theme variable to nodes (e.g. bind the "light/color-bg" variable for light theme).
When the user wants to switch to dark, rebind nodes to the corresponding "dark/*" variable.
Inform the user that native mode-switching requires a paid Figma plan; with this workaround
they manually choose which prefixed variable to bind.

## Rules
- Never delete or overwrite existing styles/variables — only add new ones.
- Prefer variables over paint styles for colors that will need dark-mode variants.
- Always get user approval on the token plan (Phase 2) before executing Phase 3.
- Process in batches of 20 during linking phase.
- Detect the free-plan mode limit at runtime: if add_variable_mode fails with "Limited to 1 modes only", switch to the name-encoded workaround automatically and inform the user.
`),
				),
			},
		), nil
	})
}
