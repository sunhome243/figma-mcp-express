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
	Type        string           `json:"type"`
	Description string           `json:"description"`
	Enum        []string         `json:"enum"`
	Items       json.RawMessage  `json:"items"`
	AnyOf       []propertySchema `json:"anyOf"`
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
	// prototype additions get_prototype + set_prototype_start, plus set_presence,
	// and the node-creation additions create_line/create_polygon/create_star/import_svg/create_table,
	// plus set_text_range, update_variable, update_variable_collection, promoted set_constraints,
	// and the API-gap media/dev-resource/style-variable helper surface.
	const want = 105
	got := len(resp.Result.Tools)
	if got != want {
		t.Errorf("expected %d registered tools, got %d — update the constant if tools were intentionally added or removed", want, got)
	}
}

func TestToolSchemas_CreateEffectStyleExposesAdvancedShorthandParams(t *testing.T) {
	resp := listTools(t)
	var props map[string]propertySchema
	for _, tool := range resp.Result.Tools {
		if tool.Name == "create_effect_style" {
			props = tool.InputSchema.Properties
			break
		}
	}
	if props == nil {
		t.Fatal("create_effect_style tool not found")
	}

	want := map[string]string{
		"blurType":        "string",
		"startRadius":     "number",
		"startOffset":     "object",
		"endOffset":       "object",
		"lightIntensity":  "number",
		"lightAngle":      "number",
		"refraction":      "number",
		"depth":           "number",
		"dispersion":      "number",
		"noiseType":       "string",
		"secondaryColor":  "string",
		"noiseSize":       "number",
		"noiseSizeVector": "object",
		"density":         "number",
		"clipToShape":     "boolean",
	}
	for name, wantType := range want {
		prop, ok := props[name]
		if !ok {
			t.Fatalf("create_effect_style missing advanced shorthand param %q", name)
		}
		if prop.Type != wantType {
			t.Fatalf("create_effect_style.%s type = %q, want %q", name, prop.Type, wantType)
		}
	}
}

func TestToolSchemas_SetVariableValueAcceptsAliasPayload(t *testing.T) {
	resp := listTools(t)
	var value propertySchema
	found := false
	for _, tool := range resp.Result.Tools {
		if tool.Name != "set_variable_value" {
			continue
		}
		var ok bool
		value, ok = tool.InputSchema.Properties["value"]
		if !ok {
			t.Fatal("set_variable_value missing value property")
		}
		found = true
		break
	}
	if !found {
		t.Fatal("set_variable_value tool not found")
	}
	if value.Type == "string" {
		t.Fatal("set_variable_value.value must accept VARIABLE_ALIAS objects, not only strings")
	}
	gotTypes := map[string]bool{}
	for _, alt := range value.AnyOf {
		gotTypes[alt.Type] = true
	}
	for _, typ := range []string{"string", "number", "boolean", "object"} {
		if !gotTypes[typ] {
			t.Fatalf("set_variable_value.value anyOf missing %q: %#v", typ, value.AnyOf)
		}
	}
}

func TestToolSchemas_AutoInjectsRequiredOriginOnPluginFacingTools(t *testing.T) {
	resp := listTools(t)

	if len(resp.Result.Tools) == 0 {
		t.Fatal("tools/list returned no tools — registration may have failed")
	}

	wantExemptTools := map[string]bool{
		"list_channels":         true,
		"search_batch_ops":      true,
		"get_batch_op_spec":     true,
		"fetch_library_catalog": true,
	}
	assertBoolMapsEqual(t, originExemptTools, wantExemptTools)

	wantRoster := append([]string(nil), rosterOrigins...)
	sort.Strings(wantRoster)

	var missing []string
	var notRequired []string
	var wrongShape []string
	var unexpected []string
	var wrongDefault []string
	var weakDescription []string
	for _, tool := range resp.Result.Tools {
		origin, hasOrigin := tool.InputSchema.Properties["origin"]
		if originExemptTool(tool.Name) {
			if hasOrigin {
				unexpected = append(unexpected, tool.Name)
			}
			continue
		}
		if !hasOrigin {
			missing = append(missing, tool.Name)
			continue
		}
		if !schemaRequires(tool.InputSchema.Required, "origin") {
			notRequired = append(notRequired, tool.Name)
		}
		gotRoster := append([]string(nil), origin.Enum...)
		sort.Strings(gotRoster)
		if origin.Type != "string" || !stringSlicesEqual(gotRoster, wantRoster) {
			wrongShape = append(wrongShape, fmt.Sprintf("%s(type=%q enum=%v)", tool.Name, origin.Type, origin.Enum))
		}
		if len(origin.Enum) == 0 || origin.Enum[0] != "wolfgang" {
			wrongDefault = append(wrongDefault, fmt.Sprintf("%s(enum=%v)", tool.Name, origin.Enum))
		}
		if !strings.Contains(origin.Description, "orchestrator/self=wolfgang") {
			weakDescription = append(weakDescription, fmt.Sprintf("%s(description=%q)", tool.Name, origin.Description))
		}
	}

	if len(missing) > 0 || len(notRequired) > 0 || len(wrongShape) > 0 || len(unexpected) > 0 || len(wrongDefault) > 0 || len(weakDescription) > 0 {
		sort.Strings(missing)
		sort.Strings(notRequired)
		sort.Strings(wrongShape)
		sort.Strings(unexpected)
		sort.Strings(wrongDefault)
		sort.Strings(weakDescription)
		t.Fatalf("origin auto-injection failed:\nnon-exempt tools without auto-filled origin: %v\norigin not marked required: %v\nwrong origin schema shape: %v\norigin enum must default to orchestrator: %v\norigin description missing orchestrator guidance: %v\norigin unexpectedly present on exempt tools: %v", missing, notRequired, wrongShape, wrongDefault, weakDescription, unexpected)
	}
}

func assertBoolMapsEqual(t *testing.T, got, want map[string]bool) {
	t.Helper()
	var extra []string
	var missing []string
	for key := range got {
		if !want[key] {
			extra = append(extra, key)
		}
	}
	for key := range want {
		if !got[key] {
			missing = append(missing, key)
		}
	}
	if len(extra) > 0 || len(missing) > 0 {
		sort.Strings(extra)
		sort.Strings(missing)
		t.Fatalf("origin exemption set drifted: extra=%v missing=%v", extra, missing)
	}
}

func schemaRequires(required []string, name string) bool {
	for _, value := range required {
		if value == name {
			return true
		}
	}
	return false
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestToolSchemas_DefaultCoreProfileExposesSmallSurface(t *testing.T) {
	raw := listToolsRawDefaultProfile(t)
	var resp toolsListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}

	const want = 22
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
		"set_presence",
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
	// Compact truncates tool/param DESCRIPTIONS but keeps every param name + the JSON
	// structure, so param-heavy tools (set_text_range, import_image) erode the ratio as
	// the surface grows. 75% still guarantees a substantial (~25%+) reduction.
	if len(compact) > len(verbose)*75/100 {
		t.Fatalf("default compact tools/list size = %d, want at least 25%% smaller than verbose size %d", len(compact), len(verbose))
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

func TestCompactToolDescriptionsCoverCoreSurface(t *testing.T) {
	for name := range coreToolSurface {
		desc, ok := compactToolDescriptions[name]
		if !ok {
			t.Fatalf("core tool %q must have a curated compact description", name)
		}
		if strings.Contains(desc, "\n") {
			t.Fatalf("core tool %q compact description must be one line: %q", name, desc)
		}
		if len(desc) > 160 {
			t.Fatalf("core tool %q compact description is %d chars, want <= 160: %q", name, len(desc), desc)
		}
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

	// get_local_components must mention pageId guidance and the file-local recovery use case.
	glcDesc := descOf("get_local_components")
	if !strings.Contains(glcDesc, "pageId") {
		t.Errorf("get_local_components description must mention pageId, got: %q", glcDesc)
	}
	if !strings.Contains(glcDesc, "file-local") || !strings.Contains(glcDesc, "create_instance") {
		t.Errorf("get_local_components description must mention file-local create_instance recovery, got: %q", glcDesc)
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
