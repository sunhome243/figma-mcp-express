package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// toolsListResponse mirrors the subset of the MCP tools/list JSON-RPC response
// that we need to inspect for schema correctness.
type toolsListResponse struct {
	Result struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string                    `json:"type"`
				Properties map[string]propertySchema `json:"properties"`
				Required   []string                  `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	} `json:"result"`
}

type propertySchema struct {
	Type  string          `json:"type"`
	Items json.RawMessage `json:"items"`
}

// listTools calls tools/list through the server's HandleMessage path and returns
// the parsed response.
func listTools(t *testing.T) toolsListResponse {
	t.Helper()
	raw := listToolsRaw(t)
	var resp toolsListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	return resp
}

func listToolsRaw(t *testing.T) []byte {
	t.Helper()
	s, _ := newTestServer(t)
	return listToolsRawFromServer(t, s)
}

func listToolsRawDefaultProfile(t *testing.T) []byte {
	t.Helper()
	s, _ := newTestServerDefaultProfile(t)
	return listToolsRawFromServer(t, s)
}

func listToolsRawFromServer(t *testing.T, s *server.MCPServer) []byte {
	t.Helper()
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	resp := s.HandleMessage(context.Background(), []byte(msg))
	if resp == nil {
		t.Fatal("HandleMessage returned nil for tools/list")
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	return b
}

// TestToolSchemas_ArrayItemsHaveType ensures every array-typed parameter across
// all registered tools declares an items.type.  Missing items (or items without
// a type) is the exact class of bug that causes GitHub Copilot MCP validation to
// fail (see commit af0325c).
func TestToolSchemas_ArrayItemsHaveType(t *testing.T) {
	resp := listTools(t)

	if len(resp.Result.Tools) == 0 {
		t.Fatal("tools/list returned no tools — registration may have failed")
	}

	type violation struct {
		tool, param, reason string
	}
	var violations []violation

	for _, tool := range resp.Result.Tools {
		for param, prop := range tool.InputSchema.Properties {
			if prop.Type != "array" {
				continue
			}

			if len(prop.Items) == 0 || string(prop.Items) == "null" {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: "items is missing",
				})
				continue
			}

			var items map[string]any
			if err := json.Unmarshal(prop.Items, &items); err != nil {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: fmt.Sprintf("items is not a valid JSON object: %v", err),
				})
				continue
			}

			if _, ok := items["type"]; !ok {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: "items.type is missing",
				})
			}
		}
	}

	for _, v := range violations {
		t.Errorf("tool %q param %q: %s", v.tool, v.param, v.reason)
	}
}

// TestToolSchemas_AllToolsRegistered asserts the expected tool count so that
// accidentally dropped registrations are caught.
func TestToolSchemas_AllToolsRegistered(t *testing.T) {
	resp := listTools(t)
	// Full profile preserves the legacy top-level tools, plus the two compact
	// catalog meta-tools used for progressive discovery. Count includes the
	// prototype additions get_prototype + set_prototype_start.
	const want = 74
	got := len(resp.Result.Tools)
	if got != want {
		t.Errorf("expected %d registered tools, got %d — update the constant if tools were intentionally added or removed", want, got)
	}
}

func TestToolSchemas_DefaultCoreProfileExposesSmallSurface(t *testing.T) {
	raw := listToolsRawDefaultProfile(t)
	var resp toolsListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}

	const want = 21
	got := len(resp.Result.Tools)
	if got != want {
		t.Fatalf("default core profile tool count = %d, want %d", got, want)
	}

	names := map[string]bool{}
	for _, tool := range resp.Result.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{
		"list_channels",
		"batch",
		"search_batch_ops",
		"get_batch_op_spec",
		"get_metadata",
		"get_node",
		"get_nodes_info",
		"get_design_context",
		"get_selection",
		"get_pages",
		"search_nodes",
		"scan_nodes_by_types",
		"scan_text_nodes",
		"save_screenshots",
		"export_frames_to_pdf",
		"fetch_library_catalog",
		"get_styles",
		"get_variable_defs",
		"get_local_components",
		"list_library_variable_collections",
		"export_tokens",
	} {
		if !names[name] {
			t.Fatalf("default core profile missing core tool %q", name)
		}
	}
	for _, name := range []string{"create_frame", "create_instance", "set_fills", "set_strokes", "set_text", "create_paint_style", "set_visible", "get_screenshot", "get_document"} {
		if names[name] {
			t.Fatalf("default core profile should hide low-level tool %q", name)
		}
	}
}

func TestToolSchemas_CoreProfileKeepsBatchValidationForHiddenOps(t *testing.T) {
	newTestServerDefaultProfile(t) // populates the batch catalog before profile filtering
	if msg := rejectUnknownBatchOpParams("set_fills", map[string]interface{}{"fills": "#fff"}); msg == "" {
		t.Fatal("hidden set_fills must still use its registered param allowlist inside batch")
	}
}

func TestToolSchemas_DefaultCompactModeShrinksToolsList(t *testing.T) {
	t.Setenv("FIGMA_MCP_TOOL_SCHEMA_MODE", "verbose")
	verbose := listToolsRaw(t)

	t.Setenv("FIGMA_MCP_TOOL_SCHEMA_MODE", "")
	compact := listToolsRaw(t)
	t.Logf("tools/list bytes: compact=%d verbose=%d", len(compact), len(verbose))

	if len(compact) >= len(verbose) {
		t.Fatalf("default compact tools/list size = %d, want smaller than verbose size %d", len(compact), len(verbose))
	}
	if len(compact) > len(verbose)*70/100 {
		t.Fatalf("default compact tools/list size = %d, want at least 30%% smaller than verbose size %d", len(compact), len(verbose))
	}
	if !strings.Contains(string(compact), "spilled") {
		t.Fatalf("compact schema must preserve spilled-response guidance")
	}
	if !strings.Contains(string(compact), "pageId") {
		t.Fatalf("compact schema must preserve pageId scoping guidance")
	}
}

func TestToolSchemas_CompactModePreservesInputSchemaShape(t *testing.T) {
	t.Setenv("FIGMA_MCP_TOOL_SCHEMA_MODE", "verbose")
	verbose := listTools(t)

	t.Setenv("FIGMA_MCP_TOOL_SCHEMA_MODE", "compact")
	compact := listTools(t)

	compactByName := map[string]struct {
		InputSchema struct {
			Type       string                    `json:"type"`
			Properties map[string]propertySchema `json:"properties"`
			Required   []string                  `json:"required"`
		} `json:"inputSchema"`
	}{}
	for _, tool := range compact.Result.Tools {
		compactByName[tool.Name] = struct {
			InputSchema struct {
				Type       string                    `json:"type"`
				Properties map[string]propertySchema `json:"properties"`
				Required   []string                  `json:"required"`
			} `json:"inputSchema"`
		}{InputSchema: tool.InputSchema}
	}

	for _, verboseTool := range verbose.Result.Tools {
		compactTool, ok := compactByName[verboseTool.Name]
		if !ok {
			t.Fatalf("compact schema missing tool %q", verboseTool.Name)
		}
		if compactTool.InputSchema.Type != verboseTool.InputSchema.Type {
			t.Fatalf("%s input schema type changed: compact=%q verbose=%q", verboseTool.Name, compactTool.InputSchema.Type, verboseTool.InputSchema.Type)
		}
		if strings.Join(sortedStrings(compactTool.InputSchema.Required), ",") != strings.Join(sortedStrings(verboseTool.InputSchema.Required), ",") {
			t.Fatalf("%s required params changed: compact=%v verbose=%v", verboseTool.Name, compactTool.InputSchema.Required, verboseTool.InputSchema.Required)
		}
		for name, verboseProp := range verboseTool.InputSchema.Properties {
			compactProp, ok := compactTool.InputSchema.Properties[name]
			if !ok {
				t.Fatalf("%s compact schema missing param %q", verboseTool.Name, name)
			}
			if compactProp.Type != verboseProp.Type {
				t.Fatalf("%s.%s type changed: compact=%q verbose=%q", verboseTool.Name, name, compactProp.Type, verboseProp.Type)
			}
			if string(compactProp.Items) != string(verboseProp.Items) {
				t.Fatalf("%s.%s array item schema changed: compact=%s verbose=%s", verboseTool.Name, name, compactProp.Items, verboseProp.Items)
			}
		}
		if len(compactTool.InputSchema.Properties) != len(verboseTool.InputSchema.Properties) {
			t.Fatalf("%s compact schema param count changed: compact=%d verbose=%d", verboseTool.Name, len(compactTool.InputSchema.Properties), len(verboseTool.InputSchema.Properties))
		}
	}
}

func TestCompactDescriptionsRecursesAndTruncates(t *testing.T) {
	schema := map[string]any{
		"description": strings.Repeat("root ", 40),
		"nested": map[string]any{
			"description": strings.Repeat("nested ", 40),
			"items": []any{
				map[string]any{"description": strings.Repeat("array ", 40)},
			},
		},
	}

	compactDescriptions(schema, 24)

	if got := schema["description"].(string); len(got) > 27 || !strings.HasSuffix(got, "...") {
		t.Fatalf("root description not compacted as expected: %q", got)
	}
	nested := schema["nested"].(map[string]any)
	if got := nested["description"].(string); len(got) > 27 || !strings.HasSuffix(got, "...") {
		t.Fatalf("nested description not compacted as expected: %q", got)
	}
	items := nested["items"].([]any)
	item := items[0].(map[string]any)
	if got := item["description"].(string); len(got) > 27 || !strings.HasSuffix(got, "...") {
		t.Fatalf("array child description not compacted as expected: %q", got)
	}
	if got := compactText("short text", 24); got != "short text" {
		t.Fatalf("short text should remain unchanged, got %q", got)
	}
}

func TestToolSchemas_CoreBatchDescriptionPointsToCatalogContract(t *testing.T) {
	raw := listToolsRawDefaultProfile(t)
	var resp toolsListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	var desc string
	for _, tool := range resp.Result.Tools {
		if tool.Name == "batch" {
			desc = tool.Description
			break
		}
	}
	if desc == "" {
		t.Fatal("core profile missing batch tool description")
	}
	for _, want := range []string{"BatchOpCatalog", "get_batch_op_spec", "validateOnly"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("batch description must mention %q, got %q", want, desc)
		}
	}
	for _, forbidden := range []string{"any tool name", "`type` is any tool name"} {
		if strings.Contains(desc, forbidden) {
			t.Fatalf("batch description should describe catalog op names, not hidden top-level tools; found %q in %q", forbidden, desc)
		}
	}
}

// TestToolSchemas_DescriptionSpillGuidance asserts that key tools include
// spill/usage guidance text in their descriptions (Part 3).
func TestToolSchemas_DescriptionSpillGuidance(t *testing.T) {
	resp := listTools(t)

	descOf := func(name string) string {
		for _, tool := range resp.Result.Tools {
			if tool.Name == name {
				return tool.Description
			}
		}
		return ""
	}

	// get_local_components must mention pageId guidance.
	glcDesc := descOf("get_local_components")
	if !strings.Contains(glcDesc, "pageId") {
		t.Errorf("get_local_components description must mention pageId, got: %q", glcDesc)
	}

	// get_design_context must mention spill guidance.
	gdcDesc := descOf("get_design_context")
	if !strings.Contains(gdcDesc, "spilled") {
		t.Errorf("get_design_context description must mention spilled, got: %q", gdcDesc)
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
