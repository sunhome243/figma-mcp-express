package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addAnnotationConversionStrategy(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("annotation_conversion_strategy",
		mcp.WithPromptDescription("Read-only strategy for analyzing manual annotations and mapping them to likely target nodes"),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"Read-only strategy for analyzing manual annotations",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(`# Manual Annotation Analysis

## Process Overview
Analyze manual annotations (numbered/alphabetical indicators with connected descriptions)
and map each one to the likely target UI node. This prompt is read-only: the current
server can read existing native annotations with get_annotations, but it does not provide
an annotation-write op.

1. Get selected frame/component information
2. Scan and collect all annotation text nodes
3. Scan target UI elements (components, instances, frames)
4. Match annotations to appropriate UI elements
5. Report a conversion plan for a human or future annotation-write tool

## Step 1: Get Selection and Initial Setup

// Get the selected frame/component
get_selection()
// Note the selected node ID, then use get_batch_op_spec("get_annotations")
// and a validated batch op get_annotations.

## Step 2: Scan Annotation Text Nodes

// Get all text nodes in the selection
scan_text_nodes(nodeId: "selected-node-id")

// Filter and group annotation markers and descriptions
// Markers typically have these characteristics:
// - Short text content (usually single digit/letter)
// - Specific font styles (often bold)
// - Located in a container with "Marker" or "Dot" in the name
// - Have a clear naming pattern (e.g., "1", "2", "3" or "A", "B", "C")

## Step 3: Scan Target UI Elements

// Get all potential target elements that annotations might refer to
scan_nodes_by_types(nodeId: "selected-node-id", types: ["COMPONENT", "INSTANCE", "FRAME"])

## Step 4: Match Annotations to Targets

Match each annotation to its target UI element using these strategies in order of priority:

1. Path-Based Matching:
   - Look at the marker's parent container name in the Figma layer hierarchy
   - Remove any "Marker:" or "Annotation:" prefixes from the parent name
   - Find UI elements that share the same parent name or have it in their path

2. Name-Based Matching:
   - Extract key terms from the annotation description
   - Look for UI elements whose names contain these key terms
   - Particularly effective for form fields, buttons, and labeled components

3. Proximity-Based Matching (fallback):
   - Calculate the center point of the marker using its bounds
   - Find the closest UI element by measuring distances to element centers
   - Use this method when other matching strategies fail

## Step 5: Report Results

Produce a table:
| Marker | Description | Likely Target Node ID | Target Name | Confidence | Evidence |

Then verify the source state with:
batch op get_annotations for the selected node
save_screenshots(items: [{nodeId: "selected-node-id", outputPath: "/tmp/annotation-check.png"}], format: "PNG", scale: 0.5)

This strategy focuses on read-only mapping. Do not claim the annotations were written
unless a future annotation-write op exists and succeeds.`),
				),
			},
		), nil
	})
}
