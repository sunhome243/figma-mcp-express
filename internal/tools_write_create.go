package internal

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var imagePaintKeys = []string{
	"x", "y", "width", "height", "name", "scaleMode", "rotation", "scalingFactor",
	"imageTransform", "exposure", "contrast", "saturation", "temperature", "tint",
	"highlights", "shadows", "parentId",
}

var videoPaintKeys = []string{
	"x", "y", "width", "height", "name", "scaleMode", "rotation", "scalingFactor",
	"videoTransform", "exposure", "contrast", "saturation", "temperature", "tint",
	"highlights", "shadows", "parentId",
}

func readLocalFileBase64(toolName, argName, filePath string) (string, error) {
	workDir, wdErr := os.Getwd()
	if wdErr != nil {
		return "", fmt.Errorf("%s: cannot determine working directory: %w", toolName, wdErr)
	}
	confined, confErr := resolveOutputPath(filePath, workDir)
	if confErr != nil {
		return "", fmt.Errorf("%s: %s must be inside the working directory: %w", toolName, argName, confErr)
	}
	raw, err := os.ReadFile(confined)
	if err != nil {
		return "", fmt.Errorf("%s: cannot read %s %q: %w", toolName, argName, filePath, err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func copyOptionalArgs(args map[string]interface{}, params map[string]interface{}, keys []string) {
	for _, k := range keys {
		if v, ok := args[k]; ok {
			params[k] = v
		}
	}
}

func registerWriteCreateTools(s *server.MCPServer, node *Node) {
	createFrameOpts := []mcp.ToolOption{
		mcp.WithDescription("Create a new frame on the current page or inside a parent node. Optional layout-sizing params (FILL/HUG) size the frame within an auto-layout parent."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Frame name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FFFFFF")),
		mcp.WithNumber("cornerRadius", mcp.Description("Corner radius in pixels")),
		mcp.WithBoolean("clipsContent", mcp.Description("Whether the frame clips its children to its bounds (default true)")),
		mcp.WithNumber("opacity", mcp.Description("Opacity from 0 to 1 (default 1)")),
		mcp.WithString("layoutMode", mcp.Description("Auto-layout direction: HORIZONTAL, VERTICAL, GRID, or NONE. GRID = CSS-grid layout (use gridRowCount/gridColumnCount/gridRowGap/gridColumnGap).")),
		mcp.WithNumber("gridRowCount", mcp.Description("Number of rows (GRID layoutMode only)")),
		mcp.WithNumber("gridColumnCount", mcp.Description("Number of columns (GRID layoutMode only)")),
		mcp.WithNumber("gridRowGap", mcp.Description("Gap between grid rows (GRID layoutMode only)")),
		mcp.WithNumber("gridColumnGap", mcp.Description("Gap between grid columns (GRID layoutMode only)")),
		mcp.WithString("gridRowGapVariableId", mcp.Description("Design variable ID to bind to gridRowGap (GRID layoutMode only).")),
		mcp.WithString("gridColumnGapVariableId", mcp.Description("Design variable ID to bind to gridColumnGap (GRID layoutMode only).")),
		mcp.WithNumber("paddingTop", mcp.Description("Auto-layout top padding (raw pixels)")),
		mcp.WithNumber("paddingRight", mcp.Description("Auto-layout right padding (raw pixels)")),
		mcp.WithNumber("paddingBottom", mcp.Description("Auto-layout bottom padding (raw pixels)")),
		mcp.WithNumber("paddingLeft", mcp.Description("Auto-layout left padding (raw pixels)")),
		mcp.WithNumber("itemSpacing", mcp.Description("Auto-layout gap between children (raw pixels)")),
		mcp.WithString("paddingTopVariableId", mcp.Description("Design variable ID to bind to paddingTop (e.g. 'VariableID:1234:5'). When set, binds the variable instead of a raw pixel value.")),
		mcp.WithString("paddingRightVariableId", mcp.Description("Design variable ID to bind to paddingRight.")),
		mcp.WithString("paddingBottomVariableId", mcp.Description("Design variable ID to bind to paddingBottom.")),
		mcp.WithString("paddingLeftVariableId", mcp.Description("Design variable ID to bind to paddingLeft.")),
		mcp.WithString("itemSpacingVariableId", mcp.Description("Design variable ID to bind to itemSpacing (gap between children).")),
		mcp.WithString("primaryAxisAlignItems", mcp.Description("Main-axis alignment: MIN, CENTER, MAX, or SPACE_BETWEEN")),
		mcp.WithString("counterAxisAlignItems", mcp.Description("Cross-axis alignment: MIN, CENTER, MAX, or BASELINE")),
		mcp.WithString("primaryAxisSizingMode", mcp.Description("Main-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("counterAxisSizingMode", mcp.Description("Cross-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("layoutWrap", mcp.Description("Wrap behaviour: NO_WRAP or WRAP")),
		mcp.WithNumber("counterAxisSpacing", mcp.Description("Gap between wrapped rows/columns (only when layoutWrap is WRAP)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	}
	createFrameOpts = append(createFrameOpts, layoutSizingParams()...)
	createFrameOpts = append(createFrameOpts, channelParam())
	s.AddTool(mcp.NewTool("create_frame", createFrameOpts...),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			params := req.GetArguments()
			resp, err := node.Send(ctx, "create_frame", nil, withChannel(req, params))
			return renderResponse(resp, err)
		})

	s.AddTool(mcp.NewTool("create_rectangle",
		mcp.WithDescription("Create a new rectangle on the current page or inside a parent node. Use cornerRadius for uniform rounding or the per-corner params for asymmetric rounding."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Rectangle name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FF5733")),
		mcp.WithNumber("cornerRadius", mcp.Description("Uniform corner radius in pixels (shorthand for all four corners)")),
		mcp.WithNumber("topLeftRadius", mcp.Description("Top-left corner radius in pixels (overrides cornerRadius for this corner)")),
		mcp.WithNumber("topRightRadius", mcp.Description("Top-right corner radius in pixels (overrides cornerRadius for this corner)")),
		mcp.WithNumber("bottomLeftRadius", mcp.Description("Bottom-left corner radius in pixels (overrides cornerRadius for this corner)")),
		mcp.WithNumber("bottomRightRadius", mcp.Description("Bottom-right corner radius in pixels (overrides cornerRadius for this corner)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_rectangle", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_ellipse",
		mcp.WithDescription("Create a new ellipse (circle/oval) on the current page or inside a parent node."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Ellipse name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #3B82F6")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_ellipse", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	createTextOpts := []mcp.ToolOption{
		mcp.WithDescription("Create a new text node on the current page or inside a parent node. The font is loaded automatically before insertion. Optional styling params (alignment, autoResize, spacing, case, decoration) can be set at creation. Returns the created node ID and bounds. Use set_text to update an existing text node."),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("Text content to display"),
		),
		mcp.WithNumber("x", mcp.Description("X position in pixels (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position in pixels (default 0)")),
		mcp.WithNumber("fontSize", mcp.Description("Font size in pixels (default 14)")),
		mcp.WithString("fontFamily", mcp.Description("Font family name e.g. 'Inter', 'Roboto', 'SF Pro Display' (default Inter). Must be a font installed in Figma.")),
		mcp.WithString("fontStyle", mcp.Description("Font style variant e.g. 'Regular', 'Bold', 'Italic', 'Medium', 'SemiBold' (default Regular). Must match an available style for the chosen fontFamily.")),
		mcp.WithString("fillColor", mcp.Description("Text color as hex e.g. #000000 (default black)")),
		mcp.WithString("name", mcp.Description("Node name shown in the layers panel (defaults to the text content)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	}
	createTextOpts = append(createTextOpts, textStyleParams()...)
	createTextOpts = append(createTextOpts, channelParam())
	s.AddTool(mcp.NewTool("create_text", createTextOpts...),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			params := req.GetArguments()
			resp, err := node.Send(ctx, "create_text", nil, withChannel(req, params))
			return renderResponse(resp, err)
		})

	s.AddTool(mcp.NewTool("import_image",
		mcp.WithDescription("Import an image into Figma as a rectangle with an image fill. Provide imagePath (local PNG/JPG; server reads + base64-encodes), imageData (base64 PNG/JPG), or imageUrl (remote URL via figma.createImageAsync). imagePath wins over imageData; imageUrl is used only when no local/base64 input is provided."),
		mcp.WithString("imagePath",
			mcp.Description("Local file path to a PNG/JPG on this machine. The server reads and base64-encodes it — no need to inline base64. Preferred over imageData for on-disk assets (logos, exported PNGs)."),
		),
		mcp.WithString("imageData",
			mcp.Description("Base64-encoded image data (PNG or JPG). Use imagePath instead when the image is a file on disk."),
		),
		mcp.WithString("imageUrl",
			mcp.Description("Remote image URL loaded by Figma's createImageAsync. Use only for fetchable remote images; local files should use imagePath."),
		),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 200)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 200)")),
		mcp.WithString("name", mcp.Description("Node name")),
		mcp.WithString("scaleMode", mcp.Description("Image scale mode: FILL (default), FIT, CROP, or TILE")),
		mcp.WithNumber("rotation", mcp.Description("Image rotation within the fill, in increments of +90 (FILL/FIT/TILE only; automatic for CROP)")),
		mcp.WithNumber("scalingFactor", mcp.Description("Tile density / repeat scale (TILE scaleMode only)")),
		mcp.WithArray("imageTransform",
			mcp.Description("2×3 affine transform matrix [[a,b,tx],[c,d,ty]] controlling crop position/zoom (CROP scaleMode only)"),
			mcp.Items(map[string]any{"type": "array", "items": map[string]any{"type": "number"}}),
		),
		mcp.WithNumber("exposure", mcp.Description("Image filter: exposure, -1 to 1 (default 0)")),
		mcp.WithNumber("contrast", mcp.Description("Image filter: contrast, -1 to 1 (default 0)")),
		mcp.WithNumber("saturation", mcp.Description("Image filter: saturation, -1 to 1 (default 0)")),
		mcp.WithNumber("temperature", mcp.Description("Image filter: temperature, -1 to 1 (default 0)")),
		mcp.WithNumber("tint", mcp.Description("Image filter: tint, -1 to 1 (default 0)")),
		mcp.WithNumber("highlights", mcp.Description("Image filter: highlights, -1 to 1 (default 0)")),
		mcp.WithNumber("shadows", mcp.Description("Image filter: shadows, -1 to 1 (default 0)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		imageData, _ := args["imageData"].(string)
		imageURL, _ := args["imageUrl"].(string)
		if imagePath, _ := args["imagePath"].(string); imagePath != "" {
			encoded, err := readLocalFileBase64("import_image", "imagePath", imagePath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			imageData = encoded
			imageURL = ""
		}
		if imageData != "" {
			imageURL = ""
		}
		if imageData == "" && imageURL == "" {
			return mcp.NewToolResultError("import_image: provide imagePath (a local file), imageData (base64), or imageUrl (remote URL)"), nil
		}
		params := map[string]interface{}{}
		if imageData != "" {
			params["imageData"] = imageData
		}
		if imageURL != "" {
			params["imageUrl"] = imageURL
		}
		copyOptionalArgs(args, params, imagePaintKeys)
		resp, err := node.Send(ctx, "import_image", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_video",
		mcp.WithDescription("Create a rectangle with a VIDEO fill from local videoPath or base64 videoData using Figma's createVideoAsync."),
		mcp.WithString("videoPath", mcp.Description("Local video file path. The server reads and base64-encodes it. Preferred over inline videoData.")),
		mcp.WithString("videoData", mcp.Description("Base64-encoded video bytes. Use videoPath for files on disk.")),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 200)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 200)")),
		mcp.WithString("name", mcp.Description("Node name")),
		mcp.WithString("scaleMode", mcp.Description("Video scale mode: FILL (default), FIT, CROP, or TILE")),
		mcp.WithNumber("rotation", mcp.Description("Video rotation within the fill, in increments of +90 (FILL/FIT/TILE only; automatic for CROP)")),
		mcp.WithNumber("scalingFactor", mcp.Description("Tile density / repeat scale (TILE scaleMode only)")),
		mcp.WithArray("videoTransform",
			mcp.Description("2×3 affine transform matrix [[a,b,tx],[c,d,ty]] controlling video crop position/zoom (CROP scaleMode only)"),
			mcp.Items(map[string]any{"type": "array", "items": map[string]any{"type": "number"}}),
		),
		mcp.WithNumber("exposure", mcp.Description("Video filter: exposure, -1 to 1 (default 0)")),
		mcp.WithNumber("contrast", mcp.Description("Video filter: contrast, -1 to 1 (default 0)")),
		mcp.WithNumber("saturation", mcp.Description("Video filter: saturation, -1 to 1 (default 0)")),
		mcp.WithNumber("temperature", mcp.Description("Video filter: temperature, -1 to 1 (default 0)")),
		mcp.WithNumber("tint", mcp.Description("Video filter: tint, -1 to 1 (default 0)")),
		mcp.WithNumber("highlights", mcp.Description("Video filter: highlights, -1 to 1 (default 0)")),
		mcp.WithNumber("shadows", mcp.Description("Video filter: shadows, -1 to 1 (default 0)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		videoData, _ := args["videoData"].(string)
		if videoPath, _ := args["videoPath"].(string); videoPath != "" {
			encoded, err := readLocalFileBase64("create_video", "videoPath", videoPath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			videoData = encoded
		}
		if videoData == "" {
			return mcp.NewToolResultError("create_video: provide videoPath (a local file) or videoData (base64)"), nil
		}
		params := map[string]interface{}{"videoData": videoData}
		copyOptionalArgs(args, params, videoPaintKeys)
		resp, err := node.Send(ctx, "create_video", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_gif",
		mcp.WithDescription("Create a FigJam GIF media node from an existing imageHash using Figma's createGif. This API is host/editor dependent."),
		mcp.WithString("imageHash", mcp.Required(), mcp.Description("Image hash to convert to a GIF media node.")),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels")),
		mcp.WithNumber("height", mcp.Description("Height in pixels")),
		mcp.WithString("name", mcp.Description("Node name")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_gif", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_link_preview",
		mcp.WithDescription("Create a FigJam link preview node from a URL using Figma's createLinkPreviewAsync. This API is host/editor dependent."),
		mcp.WithString("url", mcp.Required(), mcp.Description("URL to render as a link preview.")),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithString("name", mcp.Description("Node name")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_link_preview", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_component",
		mcp.WithDescription("Convert an existing node (frame, group, or shape) into a reusable local COMPONENT in place, preserving ALL properties, bound variables (tokens), and effects (native figma.createComponentFromNode). Rejects nodes that are already a COMPONENT/COMPONENT_SET or an INSTANCE."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID to convert, in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("name", mcp.Description("Optional name for the component. Defaults to the node's current name.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{}
		if name, ok := req.GetArguments()["name"].(string); ok && name != "" {
			params["name"] = name
		}
		resp, err := node.Send(ctx, "create_component", []string{nodeID}, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_vector",
		mcp.WithDescription("Create an editable VECTOR node with optional vectorPaths, size, fill color, and parent."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Vector node name")),
		mcp.WithArray("vectorPaths",
			mcp.Description("Optional Figma VectorPath objects, e.g. [{data:'M 0 0 L 100 0 L 100 100 Z', windingRule:'NONZERO'}]."),
			mcp.Items(map[string]any{"type": "object"}),
		),
		mcp.WithString("fillColor", mcp.Description("Solid fill color as hex e.g. #3B82F6")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_vector", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_slice",
		mcp.WithDescription("Create a Slice node for export regions."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Slice name")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_slice", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_page_divider",
		mcp.WithDescription("Create a page divider node using Figma's createPageDivider. The optional name must be divider-only text, such as --- or ***."),
		mcp.WithString("name", mcp.Description("Optional divider-only name: all hyphens, asterisks, spaces, en dashes, or em dashes.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_page_divider", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_text_path",
		mcp.WithDescription("Create a TextPath node from an existing vector-like node (VECTOR, RECTANGLE, ELLIPSE, POLYGON, STAR, or LINE)."),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Vector-like node ID in colon format.")),
		mcp.WithNumber("startSegment", mcp.Description("Non-negative vector path segment index to start on (default 0)")),
		mcp.WithNumber("startPosition", mcp.Description("Normalized start position on the segment, 0 to 1 (default 0)")),
		mcp.WithString("name", mcp.Description("Text path node name")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{"nodeId": nodeID}
		copyOptionalArgs(args, params, []string{"startSegment", "startPosition", "name"})
		resp, err := node.Send(ctx, "create_text_path", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_section",
		mcp.WithDescription("Create a Figma Section node on the current page or inside a specified parent. Sections are the modern way to organize frames and groups on a page."),
		mcp.WithString("name", mcp.Description("Section name (default 'Section')")),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels")),
		mcp.WithNumber("height", mcp.Description("Height in pixels")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page. The parent must support children (e.g. a frame or page).")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if name, ok := req.GetArguments()["name"].(string); ok && name != "" {
			params["name"] = name
		}
		if x, ok := req.GetArguments()["x"].(float64); ok {
			params["x"] = x
		}
		if y, ok := req.GetArguments()["y"].(float64); ok {
			params["y"] = y
		}
		if w, ok := req.GetArguments()["width"].(float64); ok {
			params["width"] = w
		}
		if h, ok := req.GetArguments()["height"].(float64); ok {
			params["height"] = h
		}
		if pid, ok := req.GetArguments()["parentId"].(string); ok && pid != "" {
			params["parentId"] = pid
		}
		resp, err := node.Send(ctx, "create_section", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_line",
		mcp.WithDescription("Create a straight line (LineNode) on the current page or inside a parent. Lines render via their stroke — a visible 1px black stroke is applied by default. Useful for dividers and rules."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("length", mcp.Description("Line length in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Line name")),
		mcp.WithNumber("strokeWeight", mcp.Description("Stroke thickness in pixels (default 1)")),
		mcp.WithString("strokeColor", mcp.Description("Stroke color as hex e.g. #000000 (default black)")),
		mcp.WithString("strokeCap", mcp.Description("Line end cap: NONE (default), ROUND, SQUARE, ARROW_LINES, ARROW_EQUILATERAL, DIAMOND_FILLED, TRIANGLE_FILLED, or CIRCLE_FILLED")),
		mcp.WithNumber("rotation", mcp.Description("Rotation in degrees (default 0 = horizontal). 90 = vertical.")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_line", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_polygon",
		mcp.WithDescription("Create a regular polygon (PolygonNode) with a configurable number of sides on the current page or inside a parent."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithNumber("pointCount", mcp.Description("Number of sides/points, minimum 3 (default 3 = triangle)")),
		mcp.WithString("name", mcp.Description("Polygon name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #3B82F6")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_polygon", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_star",
		mcp.WithDescription("Create a star (StarNode) with a configurable number of points and inner-radius ratio on the current page or inside a parent."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithNumber("pointCount", mcp.Description("Number of star points, minimum 3 (default 5)")),
		mcp.WithNumber("innerRadius", mcp.Description("Inner-radius ratio 0–1 (depth of the points; default 0.5). Smaller = spikier.")),
		mcp.WithString("name", mcp.Description("Star name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FBBF24")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_star", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("import_svg",
		mcp.WithDescription("Create Figma vector nodes from raw SVG markup (figma.createNodeFromSvg). Returns a FrameNode wrapping the imported vectors. The simplest way to add custom icons/illustrations that have no library component."),
		mcp.WithString("svg",
			mcp.Required(),
			mcp.Description("Raw SVG markup string e.g. '<svg ...>...</svg>'"),
		),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithString("name", mcp.Description("Name for the wrapping frame")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "import_svg", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_table",
		mcp.WithDescription("Create a Table node (figma.createTable) with the given rows and columns. Optionally fill cells with text. Returns the table ID plus numRows/numColumns. Tables may be unavailable in some editor types."),
		mcp.WithNumber("numRows",
			mcp.Required(),
			mcp.Description("Number of rows, minimum 1"),
		),
		mcp.WithNumber("numColumns",
			mcp.Required(),
			mcp.Description("Number of columns, minimum 1"),
		),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithString("name", mcp.Description("Table name")),
		mcp.WithArray("cells",
			mcp.Description("Optional 2D array of cell text, indexed [row][column] e.g. [[\"A\",\"B\"],[\"1\",\"2\"]]. Out-of-range entries are ignored."),
			mcp.Items(map[string]any{"type": "array", "items": map[string]any{"type": "string"}}),
		),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
		channelParam(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_table", nil, withChannel(req, params))
		return renderResponse(resp, err)
	})
}
