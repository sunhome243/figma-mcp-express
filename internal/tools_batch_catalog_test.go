package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestBatchOpCatalogCoversPluginHandlers(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "plugin", "src", "*.ts"))
	if err != nil {
		t.Fatalf("glob plugin handlers: %v", err)
	}
	caseRE := regexp.MustCompile(`case "([a-z_]+)"`)
	pluginOps := map[string]bool{}
	for _, path := range files {
		if !strings.Contains(filepath.Base(path), "read-") && !strings.Contains(filepath.Base(path), "write-") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range caseRE.FindAllStringSubmatch(string(body), -1) {
			pluginOps[m[1]] = true
		}
	}
	if len(pluginOps) != 117 {
		t.Fatalf("plugin lowercase handler op count = %d, want 117", len(pluginOps))
	}

	var missing []string
	for op := range pluginOps {
		if _, ok := batchOpCatalog[op]; !ok {
			missing = append(missing, op)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("batch catalog missing plugin-supported ops: %v", missing)
	}
}

func TestBatchOpCatalogEveryPluginOpHasInspectableSchema(t *testing.T) {
	newTestServer(t) // syncs registered top-level tool schemas into BatchOpCatalog.

	var missing []string
	for _, op := range pluginSupportedBatchOps {
		spec := batchOpCatalog[op]
		if spec.InputSchema == nil {
			missing = append(missing, op)
			continue
		}
		if typ, _ := spec.InputSchema["type"].(string); typ != "object" {
			t.Fatalf("%s input schema type = %q, want object", op, typ)
		}
		if _, ok := spec.InputSchema["properties"].(map[string]any); !ok {
			t.Fatalf("%s input schema missing properties map: %#v", op, spec.InputSchema)
		}
		if spec.ParamKeys == nil {
			t.Fatalf("%s paramKeys is nil; use an empty slice for no-param ops", op)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("batch catalog ops missing inspectable schemas: %v", missing)
	}
}

// Regression for issue #36: get_batch_op_spec returned 0 results for valid ops
// like set_text / set_opacity. These are plugin-supported batch ops whose schemas
// are synced from the registered top-level tools; assert the live spec lookup
// returns a populated paramKeys + inputSchema for the exact reported ops.
func TestGetBatchOpSpecReturnsSchemaForReportedOps(t *testing.T) {
	s, _ := newTestServer(t)

	for _, op := range []string{"set_text", "set_opacity"} {
		op := op
		t.Run(op, func(t *testing.T) {
			spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": op})
			if spec.IsError {
				t.Fatalf("get_batch_op_spec(%s) returned error: %s", op, resultText(t, spec))
			}
			structured, ok := spec.StructuredContent.(map[string]any)
			if !ok {
				t.Fatalf("get_batch_op_spec(%s) structuredContent = %T", op, spec.StructuredContent)
			}
			paramKeys, ok := structured["paramKeys"].([]any)
			if !ok || len(paramKeys) == 0 {
				t.Fatalf("get_batch_op_spec(%s) must expose non-empty paramKeys, got %#v", op, structured["paramKeys"])
			}
			inputSchema, ok := structured["inputSchema"].(map[string]any)
			if !ok {
				t.Fatalf("get_batch_op_spec(%s) missing inputSchema: %#v", op, structured)
			}
			if props, ok := inputSchema["properties"].(map[string]any); !ok || len(props) == 0 {
				t.Fatalf("get_batch_op_spec(%s) inputSchema has no properties: %#v", op, inputSchema)
			}
		})
	}
}

func TestBatchCatalogMetaTools(t *testing.T) {
	s, _ := newTestServer(t)

	search := callToolResult(t, s, "search_batch_ops", map[string]any{
		"query": "corner",
		"limit": float64(5),
	})
	if search.IsError {
		t.Fatalf("search_batch_ops returned error: %s", resultText(t, search))
	}
	var searchOut struct {
		Matches []struct {
			Name string `json:"name"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(resultText(t, search)), &searchOut); err != nil {
		t.Fatalf("unmarshal search result: %v", err)
	}
	found := false
	for _, m := range searchOut.Matches {
		if m.Name == "set_corner_radius" {
			found = true
		}
	}
	if !found {
		t.Fatalf("search_batch_ops did not find set_corner_radius: %#v", searchOut.Matches)
	}

	spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "set_corner_radius"})
	if spec.IsError {
		t.Fatalf("get_batch_op_spec returned error: %s", resultText(t, spec))
	}
	if txt := resultText(t, spec); !strings.Contains(txt, "cornerRadius") || !strings.Contains(txt, `"type":"number"`) || !strings.Contains(txt, "paramKeys") {
		t.Fatalf("expected demoted op spec to include typed cornerRadius param schema, got %s", txt)
	}

	enumSpec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "boolean_operation"})
	if enumSpec.IsError {
		t.Fatalf("get_batch_op_spec(boolean_operation) returned error: %s", resultText(t, enumSpec))
	}
	if txt := resultText(t, enumSpec); !strings.Contains(txt, "UNION") || !strings.Contains(txt, "FLATTEN") {
		t.Fatalf("expected boolean_operation spec to include operation enum, got %s", txt)
	}
}

// search_batch_ops must match natural multi-word queries (AND over whitespace
// tokens), not just one contiguous substring — so "create frame" finds create_frame
// even though the op name uses an underscore. Regression for the usability finding.
func TestSearchBatchOps_MultiWordQueryMatches(t *testing.T) {
	s, _ := newTestServer(t)
	cases := []struct{ query, wantOp string }{
		{"create frame", "create_frame"},
		{"auto layout", "set_auto_layout"},
		{"corn", "set_corner_radius"},
		{"delete node", "delete_nodes"},
		{"delete_node op", "delete_nodes"},
		{"delete-node tool", "delete_nodes"},
		{"reorder op", "reorder_nodes"},
		{"bring forward tool", "reorder_nodes"},
		// Search-only synonyms (batchOpSearchAliases) — the op name/description say
		// "delete"/"clone", but a peer searching the common word still finds them.
		{"remove node", "delete_nodes"},
		{"duplicate", "clone_node"},
	}
	for _, tc := range cases {
		search := callToolResult(t, s, "search_batch_ops", map[string]any{"query": tc.query, "limit": float64(30)})
		if search.IsError {
			t.Fatalf("search %q errored: %s", tc.query, resultText(t, search))
		}
		var out struct {
			Matches []struct {
				Name string `json:"name"`
			} `json:"matches"`
		}
		if err := json.Unmarshal([]byte(resultText(t, search)), &out); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.query, err)
		}
		found := false
		for _, m := range out.Matches {
			if m.Name == tc.wantOp {
				found = true
			}
		}
		if !found {
			t.Fatalf("search %q did not find %q; got %#v", tc.query, tc.wantOp, out.Matches)
		}
	}
}

// A query that finds NO exact (AND) match must not dead-end — search_batch_ops
// returns ranked "closest ops" + a hint so an agent doesn't wrongly conclude the
// capability is absent. Reproduces the real failure: searched, found nothing,
// gave up.
func TestSearchBatchOps_ZeroMatchSuggestsClosest(t *testing.T) {
	s, _ := newTestServer(t)
	// "delete node" + a non-matching term → no op matches ALL tokens (AND) →
	// zero matches, while the node-specific suggestion still ranks first.
	search := callToolResult(t, s, "search_batch_ops", map[string]any{
		"query": "delete node xyzzy", "limit": float64(30),
	})
	if search.IsError {
		t.Fatalf("search errored: %s", resultText(t, search))
	}
	var out struct {
		Total       int      `json:"total"`
		Suggestions []string `json:"suggestions"`
		Hint        string   `json:"hint"`
	}
	if err := json.Unmarshal([]byte(resultText(t, search)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 0 {
		t.Fatalf("expected 0 exact matches, got %d", out.Total)
	}
	if len(out.Suggestions) == 0 {
		t.Fatal("zero-match search must surface closest-op suggestions, got none")
	}
	if out.Suggestions[0] != "delete_nodes" {
		t.Errorf("top suggestion = %q, want delete_nodes (best name relevance)", out.Suggestions[0])
	}
	if out.Hint == "" {
		t.Error("zero-match search must include a do-not-give-up hint")
	}
}

// suggestedBatchOps ranks by NAME relevance — a singular/plural typo surfaces the
// real op FIRST, not an alphabetical match buried in some description.
func TestSuggestedBatchOps_RanksByNameRelevance(t *testing.T) {
	got := suggestedBatchOps("delete_node", 5)
	if len(got) == 0 || got[0] != "delete_nodes" {
		t.Fatalf("suggestedBatchOps(\"delete_node\") = %#v, want delete_nodes first", got)
	}
	// A query with no meaningful terms (filler only) suggests nothing — never a
	// dump of arbitrary ops.
	if s := suggestedBatchOps("op tool", 5); len(s) != 0 {
		t.Errorf("filler-only query should suggest nothing, got %#v", s)
	}
}

func TestBatchCatalogMetaToolsReturnStructuredContent(t *testing.T) {
	s, _ := newTestServer(t)

	search := callToolResult(t, s, "search_batch_ops", map[string]any{
		"query": "corner",
		"limit": float64(1),
	})
	if search.IsError {
		t.Fatalf("search_batch_ops returned error: %s", resultText(t, search))
	}
	searchStructured, ok := search.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("search_batch_ops must return structuredContent map, got %T", search.StructuredContent)
	}
	if _, ok := searchStructured["matches"].([]any); !ok {
		t.Fatalf("search_batch_ops structuredContent missing matches array: %#v", searchStructured)
	}
	if _, ok := searchStructured["count"].(float64); !ok {
		t.Fatalf("search_batch_ops structuredContent missing numeric count: %#v", searchStructured)
	}
	if _, ok := searchStructured["total"].(float64); !ok {
		t.Fatalf("search_batch_ops structuredContent missing numeric total: %#v", searchStructured)
	}

	spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "set_corner_radius"})
	if spec.IsError {
		t.Fatalf("get_batch_op_spec returned error: %s", resultText(t, spec))
	}
	specStructured, ok := spec.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("get_batch_op_spec must return structuredContent map, got %T", spec.StructuredContent)
	}
	if specStructured["name"] != "set_corner_radius" {
		t.Fatalf("get_batch_op_spec structuredContent name = %#v", specStructured["name"])
	}
	if _, ok := specStructured["inputSchema"].(map[string]any); !ok {
		t.Fatalf("get_batch_op_spec structuredContent missing inputSchema: %#v", specStructured)
	}
}

func TestBatchCatalogSpecExposesImportAssetTypeEnum(t *testing.T) {
	s, _ := newTestServer(t)

	spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "import_component_by_key"})
	if spec.IsError {
		t.Fatalf("get_batch_op_spec returned error: %s", resultText(t, spec))
	}
	specStructured, ok := spec.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("get_batch_op_spec must return structuredContent map, got %T", spec.StructuredContent)
	}
	inputSchema, ok := specStructured["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("missing inputSchema: %#v", specStructured)
	}
	props, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("missing properties: %#v", inputSchema)
	}
	assetType, ok := props["assetType"].(map[string]any)
	if !ok {
		t.Fatalf("missing assetType schema: %#v", props)
	}
	enum, ok := assetType["enum"]
	if !ok {
		t.Fatalf("assetType schema should expose enum: %#v", assetType)
	}
	enumJSON, _ := json.Marshal(enum)
	if !strings.Contains(string(enumJSON), "COMPONENT") || !strings.Contains(string(enumJSON), "COMPONENT_SET") {
		t.Fatalf("assetType enum = %s, want COMPONENT and COMPONENT_SET", enumJSON)
	}
}

func TestBatchCatalogSpecKeepsVerboseDetailsWhenToolSchemaIsCompact(t *testing.T) {
	t.Setenv("FIGMA_MCP_TOOL_SCHEMA_MODE", "compact")
	s, _ := newTestServer(t)

	raw := listToolsRawFromServer(t, s)
	var tools toolsListResponse
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}

	const verbosePhrase = "Returns the created node ID and bounds"
	for _, tool := range tools.Result.Tools {
		if tool.Name == "create_text" && strings.Contains(tool.Description, verbosePhrase) {
			t.Fatalf("compact tools/list should not expose verbose create_text prose: %q", tool.Description)
		}
	}

	spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "create_text"})
	if spec.IsError {
		t.Fatalf("get_batch_op_spec returned error: %s", resultText(t, spec))
	}
	if !strings.Contains(resultText(t, spec), verbosePhrase) {
		t.Fatalf("get_batch_op_spec must preserve verbose operation details under compact tools/list mode, got %s", resultText(t, spec))
	}
}

func TestBatchCatalogSearchFilters(t *testing.T) {
	s, _ := newTestServer(t)

	search := callToolResult(t, s, "search_batch_ops", map[string]any{
		"category": "read",
		"readOnly": true,
		"mutates":  false,
		"limit":    float64(3),
	})
	if search.IsError {
		t.Fatalf("search_batch_ops returned error: %s", resultText(t, search))
	}
	var out struct {
		Matches []struct {
			Name     string `json:"name"`
			Category string `json:"category"`
			ReadOnly bool   `json:"readOnly"`
			Mutates  bool   `json:"mutates"`
		} `json:"matches"`
		Count int `json:"count"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(resultText(t, search)), &out); err != nil {
		t.Fatalf("unmarshal search result: %v", err)
	}
	if out.Count > 3 {
		t.Fatalf("limit was not applied: count=%d", out.Count)
	}
	if out.Total < out.Count {
		t.Fatalf("total must be >= count, got total=%d count=%d", out.Total, out.Count)
	}
	for _, m := range out.Matches {
		if m.Category != "read" || !m.ReadOnly || m.Mutates {
			t.Fatalf("unexpected filtered match: %#v", m)
		}
	}
}

func TestBatchCatalogSearchLimitDefaultsAndClamp(t *testing.T) {
	s, _ := newTestServer(t)

	check := func(args map[string]any, wantCount int, wantTotalAtLeast int) {
		t.Helper()
		search := callToolResult(t, s, "search_batch_ops", args)
		if search.IsError {
			t.Fatalf("search_batch_ops returned error: %s", resultText(t, search))
		}
		var out struct {
			Count int `json:"count"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal([]byte(resultText(t, search)), &out); err != nil {
			t.Fatalf("unmarshal search result: %v", err)
		}
		if out.Count != wantCount {
			t.Fatalf("count = %d, want %d (total=%d, args=%v)", out.Count, wantCount, out.Total, args)
		}
		if out.Total < wantTotalAtLeast {
			t.Fatalf("total = %d, want >= %d", out.Total, wantTotalAtLeast)
		}
	}

	check(map[string]any{}, 20, 20)
	check(map[string]any{"limit": float64(-1)}, 20, 20)
	check(map[string]any{"limit": float64(2)}, 2, 20)

	original := batchOpCatalog
	expanded := make(map[string]BatchOpSpec, len(original)+130)
	for name, spec := range original {
		expanded[name] = spec
	}
	for i := 0; i < 130; i++ {
		name := "zz_test_limit_op_" + string(rune('a'+(i%26))) + "_" + string(rune('a'+((i/26)%26))) + "_" + string(rune('a'+((i/676)%26)))
		expanded[name] = BatchOpSpec{Name: name, Category: "test", Description: "limit test op"}
	}
	batchOpCatalog = expanded
	t.Cleanup(func() { batchOpCatalog = original })

	check(map[string]any{"limit": float64(200)}, 100, 100)
}

func TestBatchCatalogSearchFindsParamKeys(t *testing.T) {
	s, _ := newTestServer(t)

	search := callToolResult(t, s, "search_batch_ops", map[string]any{
		"query": "fontSize",
		"limit": float64(20),
	})
	if search.IsError {
		t.Fatalf("search_batch_ops returned error: %s", resultText(t, search))
	}
	var out struct {
		Matches []struct {
			Name string `json:"name"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(resultText(t, search)), &out); err != nil {
		t.Fatalf("unmarshal search result: %v", err)
	}
	names := map[string]bool{}
	for _, m := range out.Matches {
		names[m.Name] = true
	}
	for _, want := range []string{"create_text", "set_text"} {
		if !names[want] {
			t.Fatalf("search by param key fontSize should find %s; got %#v", want, out.Matches)
		}
	}
}

func TestBatchCatalogSpecExamplesAndUnknownOp(t *testing.T) {
	s, _ := newTestServer(t)

	withoutExamples := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "map"})
	if withoutExamples.IsError {
		t.Fatalf("get_batch_op_spec(map) returned error: %s", resultText(t, withoutExamples))
	}
	if strings.Contains(resultText(t, withoutExamples), "examples") {
		t.Fatalf("examples should be omitted by default, got %s", resultText(t, withoutExamples))
	}
	specStructured, ok := withoutExamples.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("get_batch_op_spec(map) structuredContent = %T", withoutExamples.StructuredContent)
	}
	inputSchema, ok := specStructured["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("map spec missing inputSchema: %#v", specStructured)
	}
	props, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("map spec missing properties: %#v", inputSchema)
	}
	over, ok := props["over"].(map[string]any)
	if !ok {
		t.Fatalf("map spec missing over schema: %#v", props)
	}
	oneOf, ok := over["oneOf"].([]any)
	if !ok || len(oneOf) != 2 {
		t.Fatalf("map.over must expose string-or-array schema, got %#v", over)
	}

	withExamples := callToolResult(t, s, "get_batch_op_spec", map[string]any{
		"op":              "map",
		"includeExamples": true,
	})
	if withExamples.IsError {
		t.Fatalf("get_batch_op_spec(map, includeExamples) returned error: %s", resultText(t, withExamples))
	}
	if !strings.Contains(resultText(t, withExamples), "examples") || !strings.Contains(resultText(t, withExamples), "$item.id") {
		t.Fatalf("expected map examples when requested, got %s", resultText(t, withExamples))
	}

	unknown := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "not_a_real_op"})
	if !unknown.IsError {
		t.Fatalf("unknown op should return a tool error, got %s", resultText(t, unknown))
	}
	if txt := resultText(t, unknown); !strings.Contains(txt, "unknown batch op") {
		t.Fatalf("expected unknown-op message, got %q", txt)
	}

	singular := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": "delete_node"})
	if !singular.IsError {
		t.Fatalf("singular op alias should still be an exact-spec error, got %s", resultText(t, singular))
	}
	if txt := resultText(t, singular); !strings.Contains(txt, "Did you mean: delete_nodes") {
		t.Fatalf("expected unknown-op suggestion for singular/plural mismatch, got %q", txt)
	}
}

func TestBatchCatalogSpecsDoNotExposeOuterToolParams(t *testing.T) {
	s, _ := newTestServer(t)

	for _, op := range []string{"set_fills", "create_text", "rename_node", "set_corner_radius"} {
		op := op
		t.Run(op, func(t *testing.T) {
			spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": op})
			if spec.IsError {
				t.Fatalf("get_batch_op_spec(%s) returned error: %s", op, resultText(t, spec))
			}
			txt := resultText(t, spec)
			if strings.Contains(txt, `"channel"`) {
				t.Fatalf("batch op spec must not expose per-op channel; route channel on outer batch only: %s", txt)
			}
			if strings.Contains(txt, `"origin"`) {
				t.Fatalf("batch op spec must not expose per-op origin; route origin on outer batch only: %s", txt)
			}
		})
	}
}

func TestBatchCatalogSearchIncludesEnumVocabulary(t *testing.T) {
	s, _ := newTestServer(t)

	search := callToolResult(t, s, "search_batch_ops", map[string]any{
		"query": "flatten",
		"limit": float64(10),
	})
	if search.IsError {
		t.Fatalf("search_batch_ops returned error: %s", resultText(t, search))
	}
	var out struct {
		Matches []struct {
			Name string `json:"name"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(resultText(t, search)), &out); err != nil {
		t.Fatalf("unmarshal search result: %v", err)
	}
	for _, match := range out.Matches {
		if match.Name == "boolean_operation" {
			return
		}
	}
	t.Fatalf("query over enum vocabulary should find boolean_operation, got %#v", out.Matches)
}

func TestBatchCatalogMetaToolsExposeOutputSchema(t *testing.T) {
	s, _ := newTestServer(t)
	raw := listToolsRawFromServer(t, s)
	if !strings.Contains(string(raw), `"name":"search_batch_ops"`) {
		t.Fatal("search_batch_ops missing from tools/list")
	}
	if !strings.Contains(string(raw), `"outputSchema"`) {
		t.Fatal("catalog meta tools must expose outputSchema")
	}
}

func TestDefaultProfileCatalogMetaToolsCallable(t *testing.T) {
	s, _ := newTestServerDefaultProfile(t)
	resp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_batch_op_spec","arguments":{"op":"rename_node"}}}`))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(raw), "rename_node") {
		t.Fatalf("get_batch_op_spec should be callable in default core profile, got %s", raw)
	}
}
