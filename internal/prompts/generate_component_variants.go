package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addGenerateComponentVariants(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("generate_component_variants",
		mcp.WithPromptDescription("Generate design variants of an existing component or frame (size, color, state, theme)"),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"Generate design variants of an existing component or frame",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(`# Generate Component Variants

Given an existing frame or component, produce a set of visual variants (e.g. sizes, color themes,
states) by cloning and mutating it. Arrange the variants in a tidy grid for review.

## Input

Ask the user:
- Source node ID (the base component or frame to clone)
- What variants to generate — choose one or more:
  a) **Sizes** — Small, Medium, Large (scale width/height, adjust font size and padding)
  b) **Color themes** — e.g. Primary, Secondary, Danger, Success, Warning
  c) **States** — Default, Hover, Pressed, Disabled, Loading
  d) **Dark mode** — duplicate with inverted background/text colors
- Arrange output on same page or new frame? (default: new container frame)

## Steps

### 1. Inspect the source
Call get_node(sourceNodeId) to understand:
- Width, height, position
- Fill colors (note hex values)
- Text content and sizes
- Child structure

### 2. Plan the variant grid
Calculate layout:
- Each clone = source width × source height
- Gap between clones = 24px
- Label each clone with its variant name (create_text node below each)
- Total container width = (cloneWidth + 24) × columns

### 3. Create container frame (if requested)
Use a validated batch op create_frame for the variant container. Prefer variable-bound
spacing params when available; run batch(validateOnly:true) before mutation.

### 4. For each variant

**Sizes:**
- Clone source: batch op clone_node with parentId=containerId
- Compute scale factor (SM=0.75, MD=1.0, LG=1.5)
- batch op resize_nodes to new dimensions
- For TEXT children: batch op set_text can adjust content and text style params including fontSize, fontFamily, and fontStyle
- batch op rename_node to "ComponentName/SM" etc.

**Color themes:**
- Clone source: batch op clone_node with parentId=containerId
- For each fill-bearing child: batch op set_fills with variableId when possible
- Color mapping suggestion:
  - Primary   → use the primary design variable
  - Secondary → use the secondary design variable
  - Danger / Success / Warning → use existing semantic variables, or create variables first with user approval
- batch op rename_node to "ComponentName/Primary" etc.

**States:**
- Clone source: batch op clone_node with parentId=containerId
- Disabled: bind to an existing disabled/background variable; reduce opacity only if that is the approved design-system behavior
- Hover: slightly lighten the primary fill
- batch op rename_node to "ComponentName/Hover" etc.

**Dark mode:**
- Clone source: batch op clone_node with parentId=containerId
- Prefer set_variable_mode on the top-level wrapper. If explicit swaps are needed, bind existing dark-mode variables.
- batch op rename_node to "ComponentName/Dark"

### 5. Summarize
Report all created node IDs and names. Ask the user if they want further adjustments.

## Rules
- Always inspect the source node before cloning.
- Never modify the original source node.
- Keep all variants on the same page unless the user requests otherwise.
- Add a text label below each variant showing its name.
`),
				),
			},
		), nil
	})
}
