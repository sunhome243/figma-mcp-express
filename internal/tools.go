package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sunhome243/figma-mcp-express/internal/prompts"
)

// spillCacheDir returns the path for the spill cache relative to workDir.
func spillCacheDir(workDir string) string {
	return filepath.Join(workDir, ".figma-mcp-cache")
}

// RegisterTools registers all MCP tools on the server.
func RegisterTools(s *server.MCPServer, node *Node) {
	registerReadTools(s, node)
	registerWriteTools(s, node)
	registerLibraryTools(s, node)
	registerChannelTools(s, node)
	registerBatchTools(s, node)
	registerBatchCatalogTools(s)
	// Snapshot every tool's declared param keys so the schema-derived unknown-param
	// allowlist (registeredParamKeys) is available to ValidateRPC + the batch loop.
	recordToolParamKeys(s)
	syncBatchCatalogFromRegisteredTools(s)
	compactToolSchemas(s)
	applyToolProfile(s)
}

// registerChannelTools registers tools for inspecting connected plugin channels.
func registerChannelTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("list_channels",
		mcp.WithDescription("List connected Figma plugin channels — one entry per open file. Returns channel id, fileName, fileKey, and pageName for each. When more than one file is connected, pass a channel id as the `channel` param on any other tool to target that specific file (or match by fileName first)."),
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		infos, err := node.ListChannels(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		text, err := json.Marshal(infos)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal channels: %v", err)), nil
		}
		return mcp.NewToolResultText(string(text)), nil
	})
}

// RegisterPrompts registers MCP prompts on the server.
func RegisterPrompts(s *server.MCPServer) {
	prompts.RegisterAll(s)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// makeHandler creates a simple tool handler with no parameters (other than the
// universal optional `channel` routing param, injected into params when present).
func makeHandler(node *Node, command string, nodeIDs []string, params map[string]interface{}) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p := withChannel(req, params)
		resp, err := node.Send(ctx, command, nodeIDs, p)
		return renderResponse(resp, err)
	}
}

// withChannel returns params with the request's `channel` arg injected (if any),
// cloning so the base map (often shared/nil) is never mutated. The bridge strips
// `channel` before sending to the plugin. Use this in every tool handler.
func withChannel(req mcp.CallToolRequest, params map[string]interface{}) map[string]interface{} {
	ch, _ := req.GetArguments()["channel"].(string)
	if ch == "" {
		return params
	}
	out := make(map[string]interface{}, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out["channel"] = ch
	return out
}

// channelParam is the universal optional routing param every tool exposes.
func channelParam() mcp.ToolOption {
	return mcp.WithString("channel", mcp.Description("Optional. Target a specific connected file by its channel id (from list_channels). Omit when only one file is connected."))
}

// skipInvisibleChildrenParam is the optional per-op perf toggle exposed on
// traversal reads. When true, the plugin sets figma.skipInvisibleInstanceChildren
// so every .children walk + findAll* skips the subtree of hidden instances —
// faster on instance-heavy enterprise files, at the cost of those hidden internals
// being absent from the result. Omitted → false (current, non-breaking semantics).
func skipInvisibleChildrenParam() mcp.ToolOption {
	return mcp.WithBoolean("skipInvisibleInstanceChildren",
		mcp.Description("Optional. true = skip hidden instances' children during traversal (faster on instance-heavy files; hidden internals omitted from the result). Default false. Turn on for heavy full-page/scan reads where hidden component internals are not needed; leave off when you need full component anatomy or hidden state."),
	)
}

// applySkipInvisible folds the optional skipInvisibleInstanceChildren arg into the
// plugin params (only when explicitly true) — the passthrough half of the param.
func applySkipInvisible(req mcp.CallToolRequest, params map[string]interface{}) {
	if v, ok := req.GetArguments()["skipInvisibleInstanceChildren"].(bool); ok && v {
		params["skipInvisibleInstanceChildren"] = true
	}
}

// textStyleKeys are the optional text-styling params shared by set_text + create_text.
var textStyleKeys = []string{
	"textAlignHorizontal", "textAlignVertical", "textAutoResize",
	"fontSize", "fontFamily", "fontStyle",
	"letterSpacingValue", "letterSpacingUnit",
	"lineHeightValue", "lineHeightUnit",
	"textCase", "textDecoration",
}

// createTextKeys is the full allowlist of params create_text accepts. Anything
// else is silently dropped by the plugin, so ValidateRPC rejects unknown keys
// loudly (see createTextHints for the common Plugin-API-name mistakes).
var createTextKeys = func() map[string]bool {
	m := map[string]bool{
		"text": true, "x": true, "y": true, "fillColor": true,
		"name": true, "parentId": true, "channel": true,
	}
	for _, k := range textStyleKeys {
		m[k] = true
	}
	return m
}()

// createTextHints maps a commonly-mistaken Plugin-API field name to the discrete
// tool's actual param, so the error tells the caller exactly what to use.
var createTextHints = map[string]string{
	"characters": "text",
	"fills":      "fillColor (hex string, e.g. \"#FFFFFF\")",
	"fill":       "fillColor",
	"color":      "fillColor",
	"content":    "text",
	"lineHeight": "lineHeightValue + lineHeightUnit",
	"width":      "(no width param — create with textAutoResize:\"HEIGHT\", then resize_nodes to set the wrap width)",
}

// toolParamHints maps a tool to its commonly-mistaken-param → correct-param hints,
// used to make an unknown-param error actionable. The allowlist itself is DERIVED
// from each tool's live MCP registration (registeredParamKeys) rather than
// hand-maintained, so it can never drift from the real schema. Add an entry here
// only to upgrade the message for a tool with a notorious Plugin-API-name confusion.
var toolParamHints = map[string]map[string]string{
	"create_text":       createTextHints,
	"set_text":          createTextHints,
	"set_corner_radius": {"radius": "cornerRadius"},
}

// registeredParamKeys maps each registered tool to the set of param keys its MCP
// schema declares (every mcp.With* option, incl. the universal `channel`). Populated
// once by recordToolParamKeys from the live server, so the unknown-param allowlist is
// the schema itself and cannot drift. Drives rejection on BOTH paths: the direct RPC
// (ValidateRPC, leader.go) AND each op inside a `batch` (which bypasses ValidateRPC).
// The plugin silently ignores unrecognized params, so a Plugin-API-name typo
// (characters, fills, padding) would otherwise produce an empty/default node with no
// signal. Catalog-only batch ops are guarded by BatchOpCatalog schemas.
var registeredParamKeys = map[string]map[string]bool{}

// recordToolParamKeys snapshots every registered tool's declared param keys into
// registeredParamKeys. Called once at the end of RegisterTools.
func recordToolParamKeys(s *server.MCPServer) {
	registeredParamKeys = map[string]map[string]bool{}
	for name, st := range s.ListTools() {
		if st == nil {
			continue
		}
		keys := make(map[string]bool, len(st.Tool.InputSchema.Properties))
		for k := range st.Tool.InputSchema.Properties {
			keys[k] = true
		}
		registeredParamKeys[name] = keys
	}
}

// rejectUnknownToolParams rejects any param key the tool's registered schema does not
// declare. Returns "" for an unregistered tool (catalog-only batch ops use
// rejectUnknownBatchOpParams) or when the registry is empty (pure unit tests that
// skip RegisterTools) — a safe no-op.
func rejectUnknownToolParams(tool string, params map[string]interface{}) string {
	allowed, ok := registeredParamKeys[tool]
	if !ok {
		return ""
	}
	return rejectUnknownParams(tool, params, allowed, toolParamHints[tool])
}

// layoutSizingKeys are the optional sizing-within-parent params shared by
// resize_nodes + create_frame (FILL/HUG/grow/align/positioning).
var layoutSizingKeys = []string{
	"layoutSizingHorizontal", "layoutSizingVertical",
	"layoutGrow", "layoutAlign", "layoutPositioning",
}

// copyParams forwards any of the listed keys present on the request into params.
func copyParams(req mcp.CallToolRequest, params map[string]interface{}, keys []string) {
	args := req.GetArguments()
	for _, k := range keys {
		if v, ok := args[k]; ok {
			params[k] = v
		}
	}
}

// textFontParams declares the font trio (size/family/style). Kept separate from
// textStyleParams because create_text already declares these inline — only set_text
// needs them added.
func textFontParams() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithNumber("fontSize", mcp.Description("Font size in pixels")),
		mcp.WithString("fontFamily", mcp.Description("Font family to switch to (loaded automatically); pair with fontStyle")),
		mcp.WithString("fontStyle", mcp.Description("Font style e.g. Regular, Medium, Bold (loaded automatically)")),
	}
}

// textStyleParams declares the optional non-font text-styling params shared by
// set_text + create_text (alignment / auto-resize / spacing / case / decoration).
func textStyleParams() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("textAlignHorizontal", mcp.Description("Horizontal text alignment: LEFT, CENTER, RIGHT, or JUSTIFIED")),
		mcp.WithString("textAlignVertical", mcp.Description("Vertical text alignment: TOP, CENTER, or BOTTOM")),
		mcp.WithString("textAutoResize", mcp.Description("Auto-resize: NONE (fixed), HEIGHT (wrap + grow down), WIDTH_AND_HEIGHT (hug), or TRUNCATE")),
		mcp.WithNumber("letterSpacingValue", mcp.Description("Letter spacing value (unit via letterSpacingUnit)")),
		mcp.WithString("letterSpacingUnit", mcp.Description("Letter spacing unit: PIXELS (default) or PERCENT")),
		mcp.WithNumber("lineHeightValue", mcp.Description("Line height value (unit via lineHeightUnit)")),
		mcp.WithString("lineHeightUnit", mcp.Description("Line height unit: PIXELS (default), PERCENT, or AUTO (no value needed)")),
		mcp.WithString("textCase", mcp.Description("Text case: ORIGINAL, UPPER, LOWER, TITLE, SMALL_CAPS, or SMALL_CAPS_FORCED")),
		mcp.WithString("textDecoration", mcp.Description("Text decoration: NONE, UNDERLINE, or STRIKETHROUGH")),
	}
}

// layoutSizingParams declares the optional sizing-within-parent tool params
// (resize_nodes, create_frame). These require the node to live in an auto-layout frame.
func layoutSizingParams() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("layoutSizingHorizontal", mcp.Description("Horizontal sizing inside an auto-layout parent: FIXED, HUG, or FILL")),
		mcp.WithString("layoutSizingVertical", mcp.Description("Vertical sizing inside an auto-layout parent: FIXED, HUG, or FILL")),
		mcp.WithNumber("layoutGrow", mcp.Description("Grow factor along the parent's main axis (0 = don't grow, 1 = fill remaining)")),
		mcp.WithString("layoutAlign", mcp.Description("Cross-axis self-alignment in an auto-layout parent: MIN, CENTER, MAX, STRETCH, or INHERIT")),
		mcp.WithString("layoutPositioning", mcp.Description("AUTO (in-flow) or ABSOLUTE (free position inside an auto-layout parent)")),
	}
}

// renderResponse converts a BridgeResponse into an MCP tool result.
// Large responses are automatically spilled to .figma-mcp-cache/ and a small
// handle is returned instead (controlled by FIGMA_MCP_SPILL_BYTES env, default 25000).
func renderResponse(resp BridgeResponse, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if resp.Error != "" {
		return mcp.NewToolResultError(resp.Error), nil
	}
	// C1 queue visibility: when this request waited for the per-channel serial slot,
	// surface queueWaitMs/queueDepth into the response so the LLM can tell it was
	// queued (not hung). Non-breaking: only inject when Data is already a map (don't
	// reshape array/string/scalar responses) and only when there was a measurable wait.
	//
	// SAFETY: resp.Data is the SAME map aliased by singleflight followers and the
	// read-cache (all callers receive the same underlying map[string]interface{}).
	// Mutating it in place triggers "fatal: concurrent map writes" when N goroutines
	// render the same response concurrently. Shallow-copy the map and inject into the
	// copy; the original (and every other caller's view of it) is never touched.
	if resp.QueueWaitMs > 0 {
		if data, ok := resp.Data.(map[string]interface{}); ok {
			m2 := make(map[string]interface{}, len(data)+2)
			for k, v := range data {
				m2[k] = v
			}
			if _, present := m2["queueWaitMs"]; !present {
				m2["queueWaitMs"] = resp.QueueWaitMs
			}
			if _, present := m2["queueDepth"]; !present {
				m2["queueDepth"] = resp.QueueDepth
			}
			resp.Data = m2
		}
	}
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal response: %v", err)), nil
	}

	// Operate on the marshaled []byte directly (Lever 6B-2 — no redundant
	// string(raw) copy alongside the live `raw` buffer). The gate produces the
	// single string conversion: string(raw) on passthrough, or a small handle on
	// spill (writing raw straight to disk). Only the rare getwd/gate-error path
	// falls back to a bare string(raw).
	workDir, wdErr := os.Getwd()
	if wdErr == nil {
		label := resp.Type
		if label == "" {
			label = "resp"
		}
		threshold := parseSpillThreshold()
		if gated, _, gateErr := gateResponse(raw, resp.Data, label, spillCacheDir(workDir), threshold); gateErr == nil {
			return mcp.NewToolResultText(gated), nil
		}
		// On gate error, fall through to the raw text (never fail the call).
	}

	return mcp.NewToolResultText(string(raw)), nil
}

// toStringSlice converts []interface{} to []string.
func toStringSlice(raw []interface{}) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ── save_screenshots ─────────────────────────────────────────────────────────

type saveItem struct {
	NodeID     string  `json:"nodeId"`
	OutputPath string  `json:"outputPath"`
	Format     string  `json:"format,omitempty"`
	Scale      float64 `json:"scale,omitempty"`
}

type saveResult struct {
	Index        int     `json:"index"`
	NodeID       string  `json:"nodeId"`
	NodeName     string  `json:"nodeName,omitempty"`
	OutputPath   string  `json:"outputPath"`
	Format       string  `json:"format,omitempty"`
	Width        float64 `json:"width,omitempty"`
	Height       float64 `json:"height,omitempty"`
	BytesWritten int     `json:"bytesWritten,omitempty"`
	Success      bool    `json:"success"`
	Error        string  `json:"error,omitempty"`
}

func executeSaveScreenshots(ctx context.Context, node *Node, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawItems, _ := req.GetArguments()["items"].([]interface{})
	defaultFormat, _ := req.GetArguments()["format"].(string)
	defaultScale, _ := req.GetArguments()["scale"].(float64)

	workDir, err := os.Getwd()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getwd: %v", err)), nil
	}

	results := make([]saveResult, 0, len(rawItems))
	succeeded, failed := 0, 0

	for i, rawItem := range rawItems {
		item, err := parseSaveItem(rawItem)
		if err != nil {
			results = append(results, saveResult{Index: i, Error: err.Error()})
			failed++
			continue
		}

		r := saveScreenshotItem(ctx, node, req, item, i, workDir, defaultFormat, defaultScale)
		results = append(results, r)
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	out, err := json.Marshal(map[string]interface{}{
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    failed,
		"hasErrors": failed > 0,
		"results":   results,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal results: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func saveScreenshotItem(ctx context.Context, node *Node, req mcp.CallToolRequest, item saveItem, index int, workDir, defaultFormat string, defaultScale float64) saveResult {
	resolvedPath, err := resolveOutputPath(item.OutputPath, workDir)
	if err != nil {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: item.OutputPath, Error: err.Error()}
	}

	format := coalesce(item.Format, defaultFormat)
	inferredFormat := inferFormat(resolvedPath)
	if format == "" {
		format = inferredFormat
	}
	if format == "" {
		format = "PNG"
	}
	if inferredFormat != "" && format != inferredFormat {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath,
			Error: fmt.Sprintf("format %s conflicts with file extension %s", format, inferredFormat)}
	}

	scale := item.Scale
	if scale <= 0 {
		scale = defaultScale
	}

	params := map[string]interface{}{"format": format}
	if scale > 0 {
		params["scale"] = scale
	}

	resp, err := node.Send(ctx, "get_screenshot", []string{item.NodeID}, withChannel(req, params))
	if err != nil {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, Error: err.Error()}
	}
	if resp.Error != "" {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, Error: resp.Error}
	}

	export, err := extractScreenshotExport(resp.Data)
	if err != nil {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, Error: err.Error()}
	}

	bytes, err := writeBase64(export.Base64, resolvedPath)
	if err != nil {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, Error: err.Error()}
	}

	return saveResult{
		Index:        index,
		NodeID:       export.NodeID,
		NodeName:     export.NodeName,
		OutputPath:   resolvedPath,
		Format:       format,
		Width:        export.Width,
		Height:       export.Height,
		BytesWritten: bytes,
		Success:      true,
	}
}

type screenshotExport struct {
	NodeID   string  `json:"nodeId"`
	NodeName string  `json:"nodeName"`
	Base64   string  `json:"base64"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

func extractScreenshotExport(data interface{}) (screenshotExport, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return screenshotExport{}, err
	}
	var wrapper struct {
		Exports []screenshotExport `json:"exports"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return screenshotExport{}, err
	}
	if len(wrapper.Exports) == 0 {
		return screenshotExport{}, errors.New("no screenshot export returned by plugin")
	}
	return wrapper.Exports[0], nil
}

func writeBase64(b64, outputPath string) (int, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, fmt.Errorf("base64 decode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return 0, fmt.Errorf("file already exists at outputPath: %s", outputPath)
		}
		return 0, err
	}
	defer f.Close()
	n, err := f.Write(data)
	return n, err
}

func resolveOutputPath(outputPath, workDir string) (string, error) {
	if filepath.IsAbs(outputPath) {
		return mustBeInsideDir(filepath.Clean(outputPath), workDir)
	}
	return mustBeInsideDir(filepath.Join(workDir, outputPath), workDir)
}

func mustBeInsideDir(resolved, workDir string) (string, error) {
	rel, err := filepath.Rel(workDir, resolved)
	if err != nil {
		return "", fmt.Errorf("outputPath must be inside the working directory: %s", workDir)
	}
	// Convert to forward slashes before prefix check so Windows paths like
	// "C:\.." don't bypass the ".." detection.
	if strings.HasPrefix(filepath.ToSlash(rel), "..") {
		return "", fmt.Errorf("outputPath must be inside the working directory: %s", workDir)
	}
	return resolved, nil
}

func inferFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "PNG"
	case ".svg":
		return "SVG"
	case ".jpg", ".jpeg":
		return "JPG"
	case ".pdf":
		return "PDF"
	}
	return ""
}

func parseSaveItem(raw interface{}) (saveItem, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return saveItem{}, err
	}
	var item saveItem
	if err := json.Unmarshal(b, &item); err != nil {
		return saveItem{}, err
	}
	return item, nil
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
