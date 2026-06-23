package internal

import (
	"math"
	"strings"
	"testing"
)

// ── ValidNodeID ──────────────────────────────────────────────────────────────

func TestValidNodeID(t *testing.T) {
	valid := []string{
		"4029:12345",
		"0:1",
		"1:1",
		"I44:9;44:3",
		"I2167:9091;186:1579;186:1745",
	}
	for _, id := range valid {
		if !ValidNodeID(id) {
			t.Errorf("expected %q to be valid", id)
		}
	}

	invalid := []string{
		"",
		"4029-12345",
		"4029:12345:6789",
		"abc:def",
		"4029:",
		":12345",
		"4029",
	}
	for _, id := range invalid {
		if ValidNodeID(id) {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}

// ── NormalizeNodeID ───────────────────────────────────────────────────────────

func TestNormalizeNodeID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"4029-12345", "4029:12345"},
		{"4029:12345", "4029:12345"},       // already valid, no-op
		{"not-a-node-id", "not-a-node-id"}, // hyphen but not a node ID
		{"", ""},
	}
	for _, c := range cases {
		got := NormalizeNodeID(c.input)
		if got != c.want {
			t.Errorf("NormalizeNodeID(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestNormalizeRPCNodeReferences(t *testing.T) {
	nodeIDs := []string{"4029-12345", "I2167-9091;186-1579", "$0.id"}
	params := map[string]interface{}{
		"nodeId":      "1-2",
		"parentId":    "3-4",
		"pageId":      "0-1",
		"componentId": "5-6",
		"hyperlink": map[string]interface{}{
			"nodeId": "7-8",
		},
		"key": "abc-def",
	}

	normalizeRPCNodeReferences(nodeIDs, params)

	if got, want := nodeIDs[0], "4029:12345"; got != want {
		t.Fatalf("nodeIDs[0] = %q, want %q", got, want)
	}
	if got, want := nodeIDs[1], "I2167:9091;186:1579"; got != want {
		t.Fatalf("nodeIDs[1] = %q, want %q", got, want)
	}
	if got, want := nodeIDs[2], "$0.id"; got != want {
		t.Fatalf("nodeIDs[2] = %q, want %q", got, want)
	}
	for key, want := range map[string]string{
		"nodeId":      "1:2",
		"parentId":    "3:4",
		"pageId":      "0:1",
		"componentId": "5:6",
		"key":         "abc-def",
	} {
		if got, _ := params[key].(string); got != want {
			t.Fatalf("params[%q] = %q, want %q", key, got, want)
		}
	}
	hyperlink, _ := params["hyperlink"].(map[string]interface{})
	if got, _ := hyperlink["nodeId"].(string); got != "7:8" {
		t.Fatalf("hyperlink.nodeId = %q, want %q", got, "7:8")
	}
}

// ── ValidateRPC ───────────────────────────────────────────────────────────────

func TestValidateRPC_GetNode(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("get_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// hyphen format
	if msg := ValidateRPC("get_node", []string{"4029-12345"}, nil); msg == "" {
		t.Error("expected error for hyphen nodeId")
	}
	// valid — no depth
	if msg := ValidateRPC("get_node", []string{"4029:12345"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid — depth 0 (full-depth boundary)
	if msg := ValidateRPC("get_node", []string{"4029:12345"}, map[string]interface{}{"depth": float64(0)}); msg != "" {
		t.Errorf("unexpected error for depth=0: %s", msg)
	}
	// valid — depth 3
	if msg := ValidateRPC("get_node", []string{"4029:12345"}, map[string]interface{}{"depth": float64(3)}); msg != "" {
		t.Errorf("unexpected error for depth=3: %s", msg)
	}
	// invalid — depth is a string (must be rejected)
	if msg := ValidateRPC("get_node", []string{"4029:12345"}, map[string]interface{}{"depth": "2"}); msg == "" {
		t.Error("expected error for non-number depth")
	}
	// invalid — negative depth
	if msg := ValidateRPC("get_node", []string{"4029:12345"}, map[string]interface{}{"depth": float64(-1)}); msg == "" {
		t.Error("expected error for negative depth")
	}
}

func TestValidateRPC_GetNodesInfo(t *testing.T) {
	if msg := ValidateRPC("get_nodes_info", nil, nil); msg == "" {
		t.Error("expected error for empty nodeIds")
	}
	if msg := ValidateRPC("get_nodes_info", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("get_nodes_info", []string{"1:1", "2:2"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_GetScreenshot(t *testing.T) {
	// invalid format
	msg := ValidateRPC("get_screenshot", []string{"1:1"}, map[string]interface{}{"format": "GIF"})
	if msg == "" {
		t.Error("expected error for invalid format")
	}
	// valid formats
	for _, f := range []string{"PNG", "SVG", "JPG", "PDF"} {
		msg := ValidateRPC("get_screenshot", []string{"1:1"}, map[string]interface{}{"format": f})
		if msg != "" {
			t.Errorf("unexpected error for format %s: %s", f, msg)
		}
	}
}

func TestValidateRPC_CreateTextUnknownParams(t *testing.T) {
	// Plugin-API field names must be rejected loudly with the correct param name.
	cases := map[string]string{
		"characters": "text",
		"fills":      "fillColor",
		"lineHeight": "lineHeightValue",
		"width":      "resize_nodes",
	}
	for bad, wantSubstr := range cases {
		msg := ValidateRPC("create_text", nil, map[string]interface{}{"text": "hi", bad: "x"})
		if msg == "" {
			t.Errorf("expected error for unknown param %q", bad)
			continue
		}
		if !strings.Contains(msg, bad) || !strings.Contains(msg, wantSubstr) {
			t.Errorf("param %q: message %q should mention %q and %q", bad, msg, bad, wantSubstr)
		}
	}
	// All valid params pass.
	ok := map[string]interface{}{
		"text": "hi", "x": float64(0), "y": float64(0), "fontSize": float64(14),
		"fontFamily": "Inter", "fontStyle": "Bold", "fillColor": "#FFFFFF",
		"name": "T", "parentId": "1:2", "textAlignHorizontal": "LEFT",
		"textAutoResize": "HEIGHT", "lineHeightValue": float64(140), "lineHeightUnit": "PERCENT",
	}
	if msg := ValidateRPC("create_text", nil, ok); msg != "" {
		t.Errorf("unexpected error for valid create_text params: %s", msg)
	}
}

// ── A2: schema-derived generic unknown-param rejection ────────────────────────

// After RegisterTools, every registered tool must have a derived param allowlist
// (registeredParamKeys) — proving recordToolParamKeys captured the live schema across
// the WHOLE surface, so the allowlist can't drift from the schema because it IS the
// schema. Channel-routed tools must capture the shared `channel` option (REST-path
// tools and list_channels intentionally omit it).
func TestRegisteredParamKeys_CoverEveryTool(t *testing.T) {
	s, _ := newTestServer(t)
	for name := range s.ListTools() {
		if _, ok := registeredParamKeys[name]; !ok {
			t.Errorf("tool %q has no derived param allowlist", name)
		}
	}
	// Spot-check that a shared option (channel) is captured, not dropped by derivation.
	if !registeredParamKeys["set_fills"]["channel"] {
		t.Error("set_fills allowlist missing the shared `channel` option — derivation dropped it")
	}
}

// rejectUnknownToolParams rejects a param the tool's schema does not declare on ANY
// registered tool (not just create_text), keeps the actionable hint for create_text,
// never rejects a declared param, and is a safe no-op for an unregistered tool.
func TestRejectUnknownToolParams_Generic(t *testing.T) {
	newTestServer(t) // populates registeredParamKeys from the live registration

	// set_fills: `fill` is a Plugin-API-name typo (the real param is `color`) and is
	// not in set_fills' schema → must be rejected on the generic path.
	if msg := rejectUnknownToolParams("set_fills", map[string]interface{}{"color": "#fff", "fill": "x"}); msg == "" {
		t.Error("expected rejection of unknown param `fill` on set_fills")
	}
	// Declared params + the universal channel must pass untouched.
	if msg := rejectUnknownToolParams("set_fills", map[string]interface{}{"color": "#fff", "mode": "replace", "channel": "auto-1"}); msg != "" {
		t.Errorf("declared set_fills params must not be rejected: %s", msg)
	}
	// create_text keeps its actionable hint through the generic path too.
	if msg := rejectUnknownToolParams("create_text", map[string]interface{}{"text": "hi", "characters": "x"}); msg == "" || !strings.Contains(msg, "text") {
		t.Errorf("create_text `characters` should be rejected with a hint to `text`, got %q", msg)
	}
	// Direct-tool validation ignores unregistered names; batch/FigmaPlan validation
	// has its own catalog-backed param guard.
	if msg := rejectUnknownToolParams("definitely_not_a_tool", map[string]interface{}{"whatever": 1}); msg != "" {
		t.Errorf("unregistered tool must be a no-op, got %q", msg)
	}
}

func TestValidateRPC_SaveScreenshots(t *testing.T) {
	// missing items
	if msg := ValidateRPC("save_screenshots", nil, nil); msg == "" {
		t.Error("expected error for missing items")
	}
	// empty items array
	msg := ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{},
	})
	if msg == "" {
		t.Error("expected error for empty items")
	}
	// invalid nodeId in item
	msg = ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"nodeId": "bad", "outputPath": "out.png"},
		},
	})
	if msg == "" {
		t.Error("expected error for bad nodeId in item")
	}
	// missing outputPath
	msg = ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"nodeId": "1:1"},
		},
	})
	if msg == "" {
		t.Error("expected error for missing outputPath")
	}
	// valid
	msg = ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"nodeId": "1:1", "outputPath": "out.png"},
		},
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_GetDesignContext(t *testing.T) {
	// negative depth
	msg := ValidateRPC("get_design_context", nil, map[string]interface{}{"depth": float64(-1)})
	if msg == "" {
		t.Error("expected error for negative depth")
	}
	// invalid detail
	msg = ValidateRPC("get_design_context", nil, map[string]interface{}{"detail": "huge"})
	if msg == "" {
		t.Error("expected error for invalid detail")
	}
	// valid detail values
	for _, d := range []string{"minimal", "compact", "full", "codegen"} {
		msg := ValidateRPC("get_design_context", nil, map[string]interface{}{"detail": d})
		if msg != "" {
			t.Errorf("unexpected error for detail %s: %s", d, msg)
		}
	}
}

func TestValidateRPC_SearchNodes(t *testing.T) {
	// missing query
	if msg := ValidateRPC("search_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing query")
	}
	// invalid nodeId
	msg := ValidateRPC("search_nodes", nil, map[string]interface{}{
		"query":  "button",
		"nodeId": "bad",
	})
	if msg == "" {
		t.Error("expected error for bad nodeId")
	}
	// non-positive limit
	msg = ValidateRPC("search_nodes", nil, map[string]interface{}{
		"query": "button",
		"limit": float64(0),
	})
	if msg == "" {
		t.Error("expected error for zero limit")
	}
	// valid
	msg = ValidateRPC("search_nodes", nil, map[string]interface{}{"query": "button"})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateFrame(t *testing.T) {
	// zero width
	msg := ValidateRPC("create_frame", nil, map[string]interface{}{"width": float64(0)})
	if msg == "" {
		t.Error("expected error for zero width")
	}
	// invalid layoutMode
	msg = ValidateRPC("create_frame", nil, map[string]interface{}{"layoutMode": "DIAGONAL"})
	if msg == "" {
		t.Error("expected error for invalid layoutMode")
	}
	// valid
	msg = ValidateRPC("create_frame", nil, map[string]interface{}{
		"width": float64(100), "height": float64(100), "layoutMode": "VERTICAL",
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetAutoLayout_GridAndExtras(t *testing.T) {
	const nid = "1:1"
	// GRID layoutMode is now accepted.
	if msg := ValidateRPC("set_auto_layout", []string{nid}, map[string]interface{}{
		"layoutMode": "GRID", "gridRowCount": float64(2), "gridColumnCount": float64(3),
	}); msg != "" {
		t.Errorf("GRID layoutMode should be valid, got: %s", msg)
	}
	// counterAxisAlignContent enum
	if msg := ValidateRPC("set_auto_layout", []string{nid}, map[string]interface{}{
		"counterAxisAlignContent": "SPACE_BETWEEN",
	}); msg != "" {
		t.Errorf("counterAxisAlignContent SPACE_BETWEEN should be valid, got: %s", msg)
	}
	if msg := ValidateRPC("set_auto_layout", []string{nid}, map[string]interface{}{
		"counterAxisAlignContent": "WHATEVER",
	}); msg == "" {
		t.Error("expected error for invalid counterAxisAlignContent")
	}
	// overflowDirection enum
	if msg := ValidateRPC("set_auto_layout", []string{nid}, map[string]interface{}{
		"overflowDirection": "BOTH",
	}); msg != "" {
		t.Errorf("overflowDirection BOTH should be valid, got: %s", msg)
	}
	if msg := ValidateRPC("set_auto_layout", []string{nid}, map[string]interface{}{
		"overflowDirection": "DIAGONAL",
	}); msg == "" {
		t.Error("expected error for invalid overflowDirection")
	}
	if msg := ValidateRPC("set_auto_layout", []string{nid}, map[string]interface{}{
		"layoutPositioning": "ABSOLUTE",
	}); msg == "" {
		t.Error("expected set_auto_layout to reject child layoutPositioning")
	} else if !strings.Contains(msg, "resize_nodes/create_frame") {
		t.Fatalf("expected routing error for layoutPositioning, got %q", msg)
	}
}

func TestValidateRPC_LayoutMinMaxRejectsNonPositiveValues(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		nodeIDs []string
		params  map[string]interface{}
	}{
		{
			name:   "create_frame_minWidth_zero",
			tool:   "create_frame",
			params: map[string]interface{}{"minWidth": float64(0)},
		},
		{
			name:    "set_auto_layout_maxHeight_negative",
			tool:    "set_auto_layout",
			nodeIDs: []string{"1:1"},
			params:  map[string]interface{}{"maxHeight": float64(-1)},
		},
		{
			name:    "resize_nodes_minHeight_zero",
			tool:    "resize_nodes",
			nodeIDs: []string{"1:1"},
			params:  map[string]interface{}{"minHeight": float64(0)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if msg := ValidateRPC(tc.tool, tc.nodeIDs, tc.params); msg == "" {
				t.Fatalf("expected %s to reject non-positive min/max value", tc.tool)
			} else if !strings.Contains(msg, "must be positive") {
				t.Fatalf("expected positivity error, got %q", msg)
			}
		})
	}
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, map[string]interface{}{"minWidth": nil}); msg != "" {
		t.Fatalf("nil minWidth should still clear the constraint, got %q", msg)
	}
	if msg := ValidateRPC("set_auto_layout", []string{"1:1"}, map[string]interface{}{"minWidth": nil}); msg != "" {
		t.Fatalf("nil minWidth should still clear the constraint on auto-layout nodes, got %q", msg)
	}
}

func TestValidateRPC_ImportImage_Filters(t *testing.T) {
	// valid filter values within -1..1
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu", "exposure": float64(0.5), "contrast": float64(-1),
	}); msg != "" {
		t.Errorf("valid filters should pass, got: %s", msg)
	}
	// out-of-range filter
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu", "saturation": float64(2),
	}); msg == "" {
		t.Error("expected error for saturation out of range")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu", "exposure": "0.5",
	}); msg == "" {
		t.Error("expected error for non-numeric exposure")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu", "contrast": math.Inf(1),
	}); msg == "" {
		t.Error("expected error for infinite contrast")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu", "rotation": math.NaN(),
	}); msg == "" {
		t.Error("expected error for NaN rotation")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu", "scaleMode": "TILE", "scalingFactor": float64(0),
	}); msg == "" {
		t.Error("expected error for non-positive scalingFactor")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{
		"imageData": "TWFu",
		"imageTransform": []interface{}{
			[]interface{}{float64(1), float64(0)},
			[]interface{}{float64(0), float64(1), float64(0)},
		},
	}); msg == "" {
		t.Error("expected error for malformed imageTransform")
	}
	if msg := ValidateRPC("create_video", nil, map[string]interface{}{
		"videoData": "TWFu", "shadows": math.Inf(-1),
	}); msg == "" {
		t.Error("expected error for infinite create_video filter")
	}
	if msg := ValidateRPC("create_video", nil, map[string]interface{}{
		"videoData": "TWFu",
		"videoTransform": []interface{}{
			[]interface{}{float64(1), float64(0), float64(0)},
			[]interface{}{float64(0), "bad", float64(0)},
		},
	}); msg == "" {
		t.Error("expected error for non-numeric videoTransform entry")
	}
}

func TestValidateRPC_SetBlendMode_LinearModes(t *testing.T) {
	for _, bm := range []string{"LINEAR_BURN", "LINEAR_DODGE"} {
		if msg := ValidateRPC("set_blend_mode", []string{"1:1"}, map[string]interface{}{"blendMode": bm}); msg != "" {
			t.Errorf("%s should be a valid blend mode, got: %s", bm, msg)
		}
	}
}

func TestValidateRPC_ResizeNodes_MinMaxOnly(t *testing.T) {
	// resize_nodes with ONLY min/max params (no width/height/sizing) should be valid.
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, map[string]interface{}{
		"minWidth": float64(50), "maxWidth": float64(300),
	}); msg != "" {
		t.Errorf("resize_nodes with min/max only should be valid, got: %s", msg)
	}
}

func TestValidateRPC_SetTextRangeHyperlinkNodeID(t *testing.T) {
	valid := map[string]interface{}{
		"startOffset": float64(0),
		"endOffset":   float64(3),
		"hyperlink":   map[string]interface{}{"nodeId": "2:3"},
	}
	if msg := ValidateRPC("set_text_range", []string{"1:1"}, valid); msg != "" {
		t.Fatalf("valid in-file hyperlink nodeId should pass, got %q", msg)
	}
	invalid := map[string]interface{}{
		"startOffset": float64(0),
		"endOffset":   float64(3),
		"hyperlink":   map[string]interface{}{"nodeId": "not-a-node-id"},
	}
	if msg := ValidateRPC("set_text_range", []string{"1:1"}, invalid); msg == "" {
		t.Fatal("expected invalid in-file hyperlink nodeId to be rejected")
	}
}

func TestValidateRPC_SetText(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("set_text", nil, map[string]interface{}{"text": "hello"}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// missing text
	if msg := ValidateRPC("set_text", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing text")
	}
	// valid
	msg := ValidateRPC("set_text", []string{"1:1"}, map[string]interface{}{"text": "hello"})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetFills(t *testing.T) {
	// missing color
	if msg := ValidateRPC("set_fills", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing color")
	}
	// invalid mode
	msg := ValidateRPC("set_fills", []string{"1:1"}, map[string]interface{}{
		"color": "#ff0000", "mode": "overwrite",
	})
	if msg == "" {
		t.Error("expected error for invalid mode")
	}
	// valid modes
	for _, mode := range []string{"replace", "append"} {
		msg := ValidateRPC("set_fills", []string{"1:1"}, map[string]interface{}{
			"color": "#ff0000", "mode": mode,
		})
		if msg != "" {
			t.Errorf("unexpected error for mode %s: %s", mode, msg)
		}
	}
}

func TestValidateRPC_MoveNodes(t *testing.T) {
	// no x or y
	msg := ValidateRPC("move_nodes", []string{"1:1"}, nil)
	if msg == "" {
		t.Error("expected error when neither x nor y provided")
	}
	// valid with just x
	msg = ValidateRPC("move_nodes", []string{"1:1"}, map[string]interface{}{"x": float64(10)})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateVariable(t *testing.T) {
	// invalid type
	msg := ValidateRPC("create_variable", nil, map[string]interface{}{
		"name": "myVar", "collectionId": "abc", "type": "NUMBER",
	})
	if msg == "" {
		t.Error("expected error for invalid variable type")
	}
	// valid types
	for _, vt := range []string{"COLOR", "FLOAT", "STRING", "BOOLEAN"} {
		msg := ValidateRPC("create_variable", nil, map[string]interface{}{
			"name": "myVar", "collectionId": "abc", "type": vt,
		})
		if msg != "" {
			t.Errorf("unexpected error for type %s: %s", vt, msg)
		}
	}
}

func TestValidateRPC_DeleteVariable(t *testing.T) {
	// neither variableId nor collectionId
	if msg := ValidateRPC("delete_variable", nil, nil); msg == "" {
		t.Error("expected error when neither id provided")
	}
	// variableId only — valid
	msg := ValidateRPC("delete_variable", nil, map[string]interface{}{"variableId": "abc"})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SwapComponent(t *testing.T) {
	// invalid componentId format
	msg := ValidateRPC("swap_component", []string{"1:1"}, map[string]interface{}{
		"componentId": "bad-format",
	})
	if msg == "" {
		t.Error("expected error for hyphen componentId")
	}
	// valid
	msg = ValidateRPC("swap_component", []string{"1:1"}, map[string]interface{}{
		"componentId": "2:2",
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_UnknownTool(t *testing.T) {
	// unknown tools pass through with no error
	msg := ValidateRPC("unknown_tool", nil, nil)
	if msg != "" {
		t.Errorf("expected no error for unknown tool, got: %s", msg)
	}
}

func TestValidateRPC_GetReactions(t *testing.T) {
	if msg := ValidateRPC("get_reactions", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("get_reactions", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for hyphen nodeId")
	}
	if msg := ValidateRPC("get_reactions", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ScanTextNodes(t *testing.T) {
	if msg := ValidateRPC("scan_text_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("scan_text_nodes", nil, map[string]interface{}{"nodeId": "bad"}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("scan_text_nodes", nil, map[string]interface{}{"nodeId": "1:1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ScanNodesByTypes(t *testing.T) {
	if msg := ValidateRPC("scan_nodes_by_types", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// missing types
	msg := ValidateRPC("scan_nodes_by_types", nil, map[string]interface{}{"nodeId": "1:1"})
	if msg == "" {
		t.Error("expected error for missing types")
	}
	// valid
	msg = ValidateRPC("scan_nodes_by_types", nil, map[string]interface{}{
		"nodeId": "1:1",
		"types":  []interface{}{"FRAME"},
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetAutoLayout(t *testing.T) {
	if msg := ValidateRPC("set_auto_layout", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("set_auto_layout", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("set_auto_layout", []string{"1:1"}, map[string]interface{}{"layoutMode": "DIAGONAL"}); msg == "" {
		t.Error("expected error for invalid layoutMode")
	}
	if msg := ValidateRPC("set_auto_layout", []string{"1:1"}, map[string]interface{}{"layoutMode": "HORIZONTAL"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateRectangleEllipse(t *testing.T) {
	for _, tool := range []string{"create_rectangle", "create_ellipse"} {
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"width": float64(-1)}); msg == "" {
			t.Errorf("%s: expected error for negative width", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"height": float64(0)}); msg == "" {
			t.Errorf("%s: expected error for zero height", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"parentId": "bad-id"}); msg == "" {
			t.Errorf("%s: expected error for invalid parentId", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"width": float64(50), "parentId": "1:1"}); msg != "" {
			t.Errorf("%s unexpected error: %s", tool, msg)
		}
	}
}

func TestValidateRPC_CreatePolygonStarContracts(t *testing.T) {
	if msg := ValidateRPC("create_polygon", nil, map[string]interface{}{"pointCount": float64(2)}); msg == "" {
		t.Error("expected error for polygon pointCount below the Figma minimum")
	}
	if msg := ValidateRPC("create_polygon", nil, map[string]interface{}{"pointCount": float64(3)}); msg != "" {
		t.Errorf("unexpected error for polygon pointCount=3: %s", msg)
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"pointCount": float64(2)}); msg == "" {
		t.Error("expected error for star pointCount below the Figma minimum")
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"innerRadius": float64(-0.1)}); msg == "" {
		t.Error("expected error for star innerRadius below 0")
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"innerRadius": float64(1.1)}); msg == "" {
		t.Error("expected error for star innerRadius above 1")
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"pointCount": float64(3), "innerRadius": float64(1)}); msg != "" {
		t.Errorf("unexpected error for valid star bounds: %s", msg)
	}
}

func TestValidateRPC_CreateText(t *testing.T) {
	if msg := ValidateRPC("create_text", nil, nil); msg == "" {
		t.Error("expected error for missing text")
	}
	if msg := ValidateRPC("create_text", nil, map[string]interface{}{"text": "hi", "parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	if msg := ValidateRPC("create_text", nil, map[string]interface{}{"text": "hi"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetStrokes(t *testing.T) {
	if msg := ValidateRPC("set_strokes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("set_strokes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing color")
	}
	if msg := ValidateRPC("set_strokes", []string{"1:1"}, map[string]interface{}{"color": "#000", "mode": "bad"}); msg == "" {
		t.Error("expected error for invalid mode")
	}
	for _, mode := range []string{"replace", "append"} {
		if msg := ValidateRPC("set_strokes", []string{"1:1"}, map[string]interface{}{"color": "#000", "mode": mode}); msg != "" {
			t.Errorf("unexpected error for mode %s: %s", mode, msg)
		}
	}
}

func TestValidateRPC_ResizeNodes(t *testing.T) {
	if msg := ValidateRPC("resize_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("resize_nodes", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error when neither width nor height provided")
	}
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, map[string]interface{}{"width": float64(200)}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, map[string]interface{}{"layoutPositioning": "ABSOLUTE"}); msg != "" {
		t.Errorf("layoutPositioning-only resize_nodes should be valid, got: %s", msg)
	}
}

func TestValidateRPC_DeleteNodes(t *testing.T) {
	if msg := ValidateRPC("delete_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("delete_nodes", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("delete_nodes", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_RenameNode(t *testing.T) {
	if msg := ValidateRPC("rename_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("rename_node", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("rename_node", []string{"1:1"}, map[string]interface{}{"name": "Frame 1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CloneNode(t *testing.T) {
	if msg := ValidateRPC("clone_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("clone_node", []string{"1:1"}, map[string]interface{}{"parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	if msg := ValidateRPC("clone_node", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ImportImage(t *testing.T) {
	if msg := ValidateRPC("import_image", nil, nil); msg == "" {
		t.Error("expected error for missing image input")
	}
	// imagePath alone is now valid (server reads + encodes the file)
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imagePath": "/tmp/logo.png"}); msg != "" {
		t.Errorf("unexpected error for imagePath: %s", msg)
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageUrl": "https://example.com/logo.png"}); msg != "" {
		t.Errorf("unexpected error for imageUrl: %s", msg)
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageData": "b64", "scaleMode": "STRETCH"}); msg == "" {
		t.Error("expected error for invalid scaleMode")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageData": "b64", "parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	for _, sm := range []string{"FILL", "FIT", "CROP", "TILE"} {
		if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageData": "b64", "scaleMode": sm}); msg != "" {
			t.Errorf("unexpected error for scaleMode %s: %s", sm, msg)
		}
	}
}

func TestValidateRPC_APIGapCreateAndDevResourceTools(t *testing.T) {
	if msg := ValidateRPC("create_video", nil, nil); msg == "" {
		t.Error("expected error for missing video input")
	}
	if msg := ValidateRPC("create_video", nil, map[string]interface{}{
		"videoData": "b64",
		"scaleMode": "FIT",
		"exposure":  float64(0.25),
		"videoTransform": []interface{}{
			[]interface{}{float64(1), float64(0), float64(0)},
			[]interface{}{float64(0), float64(1), float64(0)},
		},
	}); msg != "" {
		t.Errorf("unexpected error for create_video: %s", msg)
	}
	if msg := ValidateRPC("create_video", nil, map[string]interface{}{"videoData": "b64", "contrast": float64(1.5)}); msg == "" {
		t.Error("expected error for out-of-range create_video filter")
	}
	if msg := ValidateRPC("create_gif", nil, map[string]interface{}{"imageHash": "abc"}); msg != "" {
		t.Errorf("unexpected error for create_gif: %s", msg)
	}
	if msg := ValidateRPC("create_link_preview", nil, map[string]interface{}{"url": "https://example.com"}); msg != "" {
		t.Errorf("unexpected error for create_link_preview: %s", msg)
	}
	if msg := ValidateRPC("create_vector", nil, map[string]interface{}{"width": float64(10), "height": float64(10)}); msg != "" {
		t.Errorf("unexpected error for create_vector: %s", msg)
	}
	if msg := ValidateRPC("create_slice", nil, map[string]interface{}{"parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid slice parentId")
	}
	if msg := ValidateRPC("create_page_divider", nil, map[string]interface{}{"name": "---"}); msg != "" {
		t.Errorf("unexpected error for create_page_divider: %s", msg)
	}
	if msg := ValidateRPC("create_page_divider", nil, map[string]interface{}{"name": "Milestone"}); msg == "" {
		t.Error("expected error for invalid create_page_divider name")
	}
	if msg := ValidateRPC("create_text_path", nil, map[string]interface{}{"nodeId": "1:1"}); msg != "" {
		t.Errorf("unexpected error for create_text_path: %s", msg)
	}
	if msg := ValidateRPC("create_text_path", nil, map[string]interface{}{"nodeId": "1:1", "startPosition": float64(1.5)}); msg == "" {
		t.Error("expected error for out-of-range text path startPosition")
	}
	if msg := ValidateRPC("create_text_path", nil, map[string]interface{}{"nodeId": "1:1", "startSegment": float64(1.25)}); msg == "" {
		t.Error("expected error for non-integer text path startSegment")
	}
	if msg := ValidateRPC("add_dev_resource", nil, map[string]interface{}{"nodeId": "1:1", "url": "https://example.com/spec"}); msg != "" {
		t.Errorf("unexpected error for add_dev_resource: %s", msg)
	}
	if msg := ValidateRPC("edit_dev_resource", nil, map[string]interface{}{"nodeId": "1:1", "currentUrl": "https://example.com/spec"}); msg == "" {
		t.Error("expected error when edit_dev_resource has no replacement url or name")
	}
	if msg := ValidateRPC("delete_dev_resource", nil, map[string]interface{}{"nodeId": "1:1", "url": "https://example.com/spec"}); msg != "" {
		t.Errorf("unexpected error for delete_dev_resource: %s", msg)
	}
}

func TestValidateRPC_CreatePaintStyle(t *testing.T) {
	if msg := ValidateRPC("create_paint_style", nil, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("create_paint_style", nil, map[string]interface{}{"name": "Primary"}); msg == "" {
		t.Error("expected error for missing color")
	}
	if msg := ValidateRPC("create_paint_style", nil, map[string]interface{}{"name": "Primary", "color": "#ff0000"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateTextStyle(t *testing.T) {
	if msg := ValidateRPC("create_text_style", nil, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("create_text_style", nil, map[string]interface{}{"name": "H1", "textDecoration": "BOLD"}); msg == "" {
		t.Error("expected error for invalid textDecoration")
	}
	if msg := ValidateRPC("create_text_style", nil, map[string]interface{}{"name": "H1", "lineHeightUnit": "EM"}); msg == "" {
		t.Error("expected error for invalid lineHeightUnit")
	}
	if msg := ValidateRPC("create_text_style", nil, map[string]interface{}{"name": "H1", "letterSpacingUnit": "PT"}); msg == "" {
		t.Error("expected error for invalid letterSpacingUnit")
	}
	if msg := ValidateRPC("create_text_style", nil, map[string]interface{}{
		"name": "H1", "textDecoration": "UNDERLINE", "lineHeightUnit": "PIXELS", "letterSpacingUnit": "PERCENT",
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateEffectStyle(t *testing.T) {
	if msg := ValidateRPC("create_effect_style", nil, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{"name": "Shadow", "type": "GLOW"}); msg == "" {
		t.Error("expected error for invalid type")
	}
	for _, et := range []string{"DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR", "GLASS", "NOISE", "TEXTURE"} {
		if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{"name": "S", "type": et}); msg != "" {
			t.Errorf("unexpected error for type %s: %s", et, msg)
		}
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name":     "Blur/Bad",
		"type":     "LAYER_BLUR",
		"blurType": "GRADUAL",
	}); msg == "" {
		t.Error("expected error for invalid blurType")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name":      "Noise/Bad",
		"type":      "NOISE",
		"noiseType": "CHROMA",
	}); msg == "" {
		t.Error("expected error for invalid noiseType")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name":  "Glass/Bad",
		"type":  "GLASS",
		"depth": float64(0),
	}); msg == "" {
		t.Error("expected error for invalid GLASS depth")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name":       "Glass/BadAngle",
		"type":       "GLASS",
		"lightAngle": "east",
	}); msg == "" {
		t.Error("expected error for invalid GLASS lightAngle")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name":            "Texture/BadVector",
		"type":            "TEXTURE",
		"noiseSizeVector": map[string]interface{}{"x": float64(2)},
	}); msg == "" {
		t.Error("expected error for incomplete noiseSizeVector")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name": "Mixed/Bad",
		"effects": []interface{}{
			map[string]interface{}{"type": "BACKGROUND_BLUR", "blurType": "PROGRESSIVE", "startOffset": map[string]interface{}{"x": float64(1.2), "y": float64(0)}},
		},
	}); msg == "" {
		t.Error("expected effects[] validation to reject out-of-range progressive blur vectors")
	}
	if msg := ValidateRPC("create_effect_style", nil, map[string]interface{}{
		"name": "Mixed/Good",
		"effects": []interface{}{
			map[string]interface{}{"type": "GLASS", "lightIntensity": float64(0.7), "depth": float64(4)},
			map[string]interface{}{"type": "NOISE", "noiseType": "DUOTONE", "noiseSizeVector": map[string]interface{}{"x": float64(2), "y": float64(5)}},
			map[string]interface{}{"type": "BACKGROUND_BLUR", "blurType": "PROGRESSIVE", "startOffset": map[string]interface{}{"x": float64(0.5), "y": float64(0)}, "endOffset": map[string]interface{}{"x": float64(0.5), "y": float64(1)}},
		},
	}); msg != "" {
		t.Errorf("unexpected error for valid advanced effects[]: %s", msg)
	}
}

func TestValidateRPC_CreateGridStyle(t *testing.T) {
	if msg := ValidateRPC("create_grid_style", nil, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("create_grid_style", nil, map[string]interface{}{"name": "Grid", "pattern": "DIAGONAL"}); msg == "" {
		t.Error("expected error for invalid pattern")
	}
	if msg := ValidateRPC("create_grid_style", nil, map[string]interface{}{"name": "Grid", "alignment": "LEFT"}); msg == "" {
		t.Error("expected error for invalid alignment")
	}
	if msg := ValidateRPC("create_grid_style", nil, map[string]interface{}{"name": "Grid", "pattern": "COLUMNS", "alignment": "CENTER"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_UpdatePaintStyle(t *testing.T) {
	if msg := ValidateRPC("update_paint_style", nil, nil); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc"}); msg == "" {
		t.Error("expected error when no fields to update")
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc", "color": "#fff"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc", "paints": []interface{}{map[string]interface{}{"type": "SOLID"}}}); msg != "" {
		t.Errorf("unexpected error for paints[] update: %s", msg)
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc", "description": "desc"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_DeleteStyle(t *testing.T) {
	if msg := ValidateRPC("delete_style", nil, nil); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("delete_style", nil, map[string]interface{}{"styleId": "S:abc"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ReorderLocalStyles(t *testing.T) {
	if msg := ValidateRPC("reorder_local_style", nil, map[string]interface{}{"styleType": "PAINT"}); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("reorder_local_style", nil, map[string]interface{}{"styleType": "PAINT", "styleId": "S:abc", "afterStyleId": "S:def"}); msg != "" {
		t.Errorf("unexpected error for reorder_local_style: %s", msg)
	}
	if msg := ValidateRPC("reorder_local_style_folder", nil, map[string]interface{}{"styleType": "TEXT", "folder": "Brand/Heading", "afterFolder": "Brand/Body"}); msg != "" {
		t.Errorf("unexpected error for reorder_local_style_folder: %s", msg)
	}
	if msg := ValidateRPC("reorder_local_style_folder", nil, map[string]interface{}{"styleType": "SHADOW", "folder": "Brand"}); msg == "" {
		t.Error("expected error for invalid styleType")
	}
}

func TestValidateRPC_CreateVariableCollection(t *testing.T) {
	if msg := ValidateRPC("create_variable_collection", nil, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("create_variable_collection", nil, map[string]interface{}{"name": "Brand"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_AddVariableMode(t *testing.T) {
	if msg := ValidateRPC("add_variable_mode", nil, nil); msg == "" {
		t.Error("expected error for missing collectionId")
	}
	if msg := ValidateRPC("add_variable_mode", nil, map[string]interface{}{"collectionId": "c1"}); msg == "" {
		t.Error("expected error for missing modeName")
	}
	if msg := ValidateRPC("add_variable_mode", nil, map[string]interface{}{"collectionId": "c1", "modeName": "Dark"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetVariableValue(t *testing.T) {
	if msg := ValidateRPC("set_variable_value", nil, nil); msg == "" {
		t.Error("expected error for missing variableId")
	}
	if msg := ValidateRPC("set_variable_value", nil, map[string]interface{}{"variableId": "v1"}); msg == "" {
		t.Error("expected error for missing modeId")
	}
	if msg := ValidateRPC("set_variable_value", nil, map[string]interface{}{"variableId": "v1", "modeId": "m1"}); msg == "" {
		t.Error("expected error for missing value")
	}
	if msg := ValidateRPC("set_variable_value", nil, map[string]interface{}{"variableId": "v1", "modeId": "m1", "value": "#fff"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ApplyStyleToNode(t *testing.T) {
	if msg := ValidateRPC("apply_style_to_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("apply_style_to_node", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("apply_style_to_node", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("apply_style_to_node", []string{"1:1"}, map[string]interface{}{"styleId": "S:abc", "target": "shadow"}); msg == "" {
		t.Error("expected error for invalid target")
	}
	for _, target := range []string{"fill", "stroke"} {
		if msg := ValidateRPC("apply_style_to_node", []string{"1:1"}, map[string]interface{}{"styleId": "S:abc", "target": target}); msg != "" {
			t.Errorf("unexpected error for target %s: %s", target, msg)
		}
	}
}

func TestValidateRPC_BindVariableToNode(t *testing.T) {
	if msg := ValidateRPC("bind_variable_to_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing variableId")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"1:1"}, map[string]interface{}{"variableId": "v1"}); msg == "" {
		t.Error("expected error for missing field")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"1:1"}, map[string]interface{}{"variableId": "v1", "field": "fill"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_EffectAndLayoutGridVariableHelpers(t *testing.T) {
	if msg := ValidateRPC("create_variable_alias", nil, map[string]interface{}{"variableId": "v1"}); msg != "" {
		t.Errorf("unexpected error for create_variable_alias: %s", msg)
	}
	if msg := ValidateRPC("update_variable", nil, map[string]interface{}{
		"variableId": "v1",
		"codeSyntax": map[string]interface{}{"WEB": "colorPrimary"},
	}); msg != "" {
		t.Errorf("unexpected error for valid codeSyntax: %s", msg)
	}
	if msg := ValidateRPC("update_variable", nil, map[string]interface{}{
		"variableId": "v1",
		"codeSyntax": map[string]interface{}{"WEB": map[string]interface{}{}},
	}); msg == "" {
		t.Error("expected error for non-string codeSyntax value")
	}
	if msg := ValidateRPC("update_variable", nil, map[string]interface{}{"variableId": "v1", "removeCodeSyntax": []interface{}{"WEB", "iOS"}}); msg != "" {
		t.Errorf("unexpected error for removeCodeSyntax: %s", msg)
	}
	if msg := ValidateRPC("update_variable", nil, map[string]interface{}{"variableId": "v1", "removeCodeSyntax": []interface{}{"MAC"}}); msg == "" {
		t.Error("expected error for invalid removeCodeSyntax platform")
	}
	if msg := ValidateRPC("update_variable_collection", nil, map[string]interface{}{
		"collectionId": "c1",
		"renameMode":   map[string]interface{}{"modeId": "m1", "newName": "Dark"},
	}); msg != "" {
		t.Errorf("unexpected error for valid renameMode: %s", msg)
	}
	if msg := ValidateRPC("update_variable_collection", nil, map[string]interface{}{
		"collectionId": "c1",
		"renameMode":   map[string]interface{}{"modeId": "m1", "newName": map[string]interface{}{}},
	}); msg == "" {
		t.Error("expected error for non-string renameMode.newName")
	}
	if msg := ValidateRPC("bind_variable_to_effect", nil, map[string]interface{}{
		"effect": map[string]interface{}{"type": "DROP_SHADOW", "radius": float64(8)}, "field": "radius", "variableId": "v1",
	}); msg != "" {
		t.Errorf("unexpected error for bind_variable_to_effect: %s", msg)
	}
	if msg := ValidateRPC("bind_variable_to_layout_grid", nil, map[string]interface{}{
		"layoutGrid": map[string]interface{}{"pattern": "GRID", "sectionSize": float64(8)}, "field": "sectionSize", "variableId": "v1",
	}); msg != "" {
		t.Errorf("unexpected error for bind_variable_to_layout_grid: %s", msg)
	}
}

func TestValidateRPC_DetachInstance(t *testing.T) {
	if msg := ValidateRPC("detach_instance", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("detach_instance", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("detach_instance", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetOpacity(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("set_opacity", nil, map[string]interface{}{"opacity": float64(0.5)}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("set_opacity", []string{"bad"}, map[string]interface{}{"opacity": float64(0.5)}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// missing opacity
	if msg := ValidateRPC("set_opacity", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing opacity")
	}
	// opacity out of range
	if msg := ValidateRPC("set_opacity", []string{"1:1"}, map[string]interface{}{"opacity": float64(1.5)}); msg == "" {
		t.Error("expected error for opacity > 1")
	}
	if msg := ValidateRPC("set_opacity", []string{"1:1"}, map[string]interface{}{"opacity": float64(-0.1)}); msg == "" {
		t.Error("expected error for opacity < 0")
	}
	// boundary values
	for _, op := range []float64{0, 0.5, 1} {
		if msg := ValidateRPC("set_opacity", []string{"1:1"}, map[string]interface{}{"opacity": op}); msg != "" {
			t.Errorf("unexpected error for opacity %v: %s", op, msg)
		}
	}
	// multiple nodeIds
	if msg := ValidateRPC("set_opacity", []string{"1:1", "2:2"}, map[string]interface{}{"opacity": float64(0.5)}); msg != "" {
		t.Errorf("unexpected error for multiple valid nodeIds: %s", msg)
	}
}

func TestValidateRPC_SetCornerRadius(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("set_corner_radius", nil, map[string]interface{}{"cornerRadius": float64(8)}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("set_corner_radius", []string{"bad"}, map[string]interface{}{"cornerRadius": float64(8)}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// no radius param provided
	if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error when no radius param provided")
	}
	// uniform cornerRadius
	if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, map[string]interface{}{"cornerRadius": float64(8)}); msg != "" {
		t.Errorf("unexpected error for cornerRadius: %s", msg)
	}
	// per-corner individually
	for _, param := range []string{"topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius"} {
		if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, map[string]interface{}{param: float64(4)}); msg != "" {
			t.Errorf("unexpected error for %s: %s", param, msg)
		}
	}
	// mixed per-corner
	if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, map[string]interface{}{
		"topLeftRadius": float64(8), "topRightRadius": float64(0),
		"bottomLeftRadius": float64(8), "bottomRightRadius": float64(0),
	}); msg != "" {
		t.Errorf("unexpected error for per-corner radii: %s", msg)
	}
}

func TestValidateRPC_GroupNodes(t *testing.T) {
	// fewer than 2 nodes
	if msg := ValidateRPC("group_nodes", nil, nil); msg == "" {
		t.Error("expected error for empty nodeIds")
	}
	if msg := ValidateRPC("group_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for single nodeId")
	}
	// invalid nodeId
	if msg := ValidateRPC("group_nodes", []string{"1:1", "bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// valid
	if msg := ValidateRPC("group_nodes", []string{"1:1", "2:2"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("group_nodes", []string{"1:1", "2:2", "3:3"}, nil); msg != "" {
		t.Errorf("unexpected error for 3 nodeIds: %s", msg)
	}
}

func TestValidateRPC_UngroupNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("ungroup_nodes", nil, nil); msg == "" {
		t.Error("expected error for empty nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("ungroup_nodes", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// valid single
	if msg := ValidateRPC("ungroup_nodes", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid multiple
	if msg := ValidateRPC("ungroup_nodes", []string{"1:1", "2:2"}, nil); msg != "" {
		t.Errorf("unexpected error for multiple nodeIds: %s", msg)
	}
}

func TestValidateRPC_NavigateToPage(t *testing.T) {
	// neither pageId nor pageName
	if msg := ValidateRPC("navigate_to_page", nil, nil); msg == "" {
		t.Error("expected error when neither pageId nor pageName provided")
	}
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{}); msg == "" {
		t.Error("expected error for empty params")
	}
	// pageId provided
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{"pageId": "0:1"}); msg != "" {
		t.Errorf("unexpected error for pageId: %s", msg)
	}
	// pageName provided
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{"pageName": "Design"}); msg != "" {
		t.Errorf("unexpected error for pageName: %s", msg)
	}
	// both provided — also valid
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{"pageId": "0:1", "pageName": "Design"}); msg != "" {
		t.Errorf("unexpected error when both provided: %s", msg)
	}
}

func TestValidateRPC_CreateComponent(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("create_component", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("create_component", []string{""}, nil); msg == "" {
		t.Error("expected error for empty nodeId")
	}
	// invalid nodeId format
	if msg := ValidateRPC("create_component", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for hyphen nodeId")
	}
	// valid
	if msg := ValidateRPC("create_component", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("create_component", []string{"1:1"}, map[string]interface{}{"name": "MyComponent"}); msg != "" {
		t.Errorf("unexpected error with name: %s", msg)
	}
}

func TestValidateRPC_ExportTokens(t *testing.T) {
	// no params — valid (defaults to json)
	if msg := ValidateRPC("export_tokens", nil, nil); msg != "" {
		t.Errorf("unexpected error for no params: %s", msg)
	}
	// valid formats
	for _, f := range []string{"json", "css"} {
		if msg := ValidateRPC("export_tokens", nil, map[string]interface{}{"format": f}); msg != "" {
			t.Errorf("unexpected error for format %s: %s", f, msg)
		}
	}
	// invalid format
	if msg := ValidateRPC("export_tokens", nil, map[string]interface{}{"format": "yaml"}); msg == "" {
		t.Error("expected error for invalid format")
	}
	if msg := ValidateRPC("export_tokens", nil, map[string]interface{}{"format": "style-dictionary"}); msg == "" {
		t.Error("expected error for unsupported format")
	}
}

func TestValidateAutoLayoutParams_InvalidValues(t *testing.T) {
	cases := []struct {
		param string
		value string
	}{
		{"primaryAxisAlignItems", "LEFT"},
		{"counterAxisAlignItems", "TOP"},
		{"primaryAxisSizingMode", "SHRINK"},
		{"counterAxisSizingMode", "SHRINK"},
		{"layoutWrap", "FLEX_WRAP"},
	}
	for _, c := range cases {
		msg := ValidateRPC("create_frame", nil, map[string]interface{}{c.param: c.value})
		if msg == "" {
			t.Errorf("expected error for invalid %s=%q", c.param, c.value)
		}
	}

	// All valid auto-layout params together
	msg := ValidateRPC("create_frame", nil, map[string]interface{}{
		"primaryAxisAlignItems": "CENTER",
		"counterAxisAlignItems": "BASELINE",
		"primaryAxisSizingMode": "AUTO",
		"counterAxisSizingMode": "FIXED",
		"layoutWrap":            "WRAP",
	})
	if msg != "" {
		t.Errorf("unexpected error for valid auto-layout params: %s", msg)
	}
}

// ── set_reactions ─────────────────────────────────────────────────────────────

func TestValidateRPC_SetReactions(t *testing.T) {
	validReaction := map[string]interface{}{
		"trigger": map[string]interface{}{"type": "ON_CLICK"},
		"action": map[string]interface{}{
			"type":          "NODE",
			"destinationId": "1:3",
			"navigation":    "NAVIGATE",
		},
	}

	// missing nodeId
	if msg := ValidateRPC("set_reactions", nil, map[string]interface{}{"reactions": []interface{}{}}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// bad nodeId format
	if msg := ValidateRPC("set_reactions", []string{"1-2"}, map[string]interface{}{"reactions": []interface{}{}}); msg == "" {
		t.Error("expected error for bad nodeId format")
	}
	// missing reactions
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{}); msg == "" {
		t.Error("expected error for missing reactions")
	}
	// reactions not an array
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{"reactions": "not-array"}); msg == "" {
		t.Error("expected error for non-array reactions")
	}
	// bad mode
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{},
		"mode":      "overwrite",
	}); msg == "" {
		t.Error("expected error for bad mode")
	}
	// valid mode replace
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{validReaction},
		"mode":      "replace",
	}); msg != "" {
		t.Errorf("unexpected error for mode=replace: %s", msg)
	}
	// valid mode append
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{validReaction},
		"mode":      "append",
	}); msg != "" {
		t.Errorf("unexpected error for mode=append: %s", msg)
	}
	// invalid trigger type
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "INVALID_TRIGGER"},
				"action":  map[string]interface{}{"type": "BACK"},
			},
		},
	}); msg == "" {
		t.Error("expected error for invalid trigger type")
	}
	// AFTER_TIMEOUT missing timeout
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "AFTER_TIMEOUT"},
				"action":  map[string]interface{}{"type": "BACK"},
			},
		},
	}); msg == "" {
		t.Error("expected error for AFTER_TIMEOUT without timeout")
	}
	// AFTER_TIMEOUT with valid timeout
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "AFTER_TIMEOUT", "timeout": float64(3000)},
				"action":  map[string]interface{}{"type": "BACK"},
			},
		},
	}); msg != "" {
		t.Errorf("unexpected error for valid AFTER_TIMEOUT: %s", msg)
	}
	// unknown action type passes (forward-compat — mirrors the plugin, which forwards
	// unknown types to setReactionsAsync rather than rejecting them)
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "FUTURE_ACTION", "someField": "x"},
			},
		},
	}); msg != "" {
		t.Errorf("unexpected error for unknown (forward-compat) action type: %s", msg)
	}
	// NODE missing destinationId (navigation is optional — plugin defaults it)
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "NODE", "navigation": "NAVIGATE"},
			},
		},
	}); msg == "" {
		t.Error("expected error for NODE without destinationId")
	}
	// NODE with only destinationId is valid (navigation defaulted by the plugin)
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "NODE", "destinationId": "1:3"},
			},
		},
	}); msg != "" {
		t.Errorf("unexpected error for minimal NODE action: %s", msg)
	}
	// URL missing url
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "URL"},
			},
		},
	}); msg == "" {
		t.Error("expected error for URL without url")
	}
	// plural `actions` array IS validated (the real API path) — SET_VARIABLE_MODE
	// missing variableModeId must error
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"actions": []interface{}{
					map[string]interface{}{"type": "SET_VARIABLE_MODE", "variableCollectionId": "VC:1"},
				},
			},
		},
	}); msg == "" {
		t.Error("expected error for SET_VARIABLE_MODE without variableModeId in plural actions array")
	}
	// valid plural actions array passes
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"actions": []interface{}{
					map[string]interface{}{"type": "NODE", "destinationId": "1:3", "navigation": "NAVIGATE"},
				},
			},
		},
	}); msg != "" {
		t.Errorf("unexpected error for valid plural actions array: %s", msg)
	}
	// empty reactions array is valid (clear all)
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{},
	}); msg != "" {
		t.Errorf("unexpected error for empty reactions: %s", msg)
	}
	// valid full reaction
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{validReaction},
	}); msg != "" {
		t.Errorf("unexpected error for valid reaction: %s", msg)
	}
}

// ── get_prototype ─────────────────────────────────────────────────────────────

func TestValidateRPC_GetPrototype(t *testing.T) {
	// Optional scope: no nodeIds reads the whole current page — valid.
	if msg := ValidateRPC("get_prototype", nil, map[string]interface{}{}); msg != "" {
		t.Errorf("unscoped get_prototype should be valid, got: %s", msg)
	}
	// Scoped to a valid node id — valid.
	if msg := ValidateRPC("get_prototype", []string{"1:2"}, map[string]interface{}{}); msg != "" {
		t.Errorf("scoped get_prototype should be valid, got: %s", msg)
	}
	// Bad node-id format — must error.
	if msg := ValidateRPC("get_prototype", []string{"1-2"}, map[string]interface{}{}); msg == "" {
		t.Error("expected error for bad nodeId format")
	}
}

// ── set_prototype_start ───────────────────────────────────────────────────────

func TestValidateRPC_SetPrototypeStart(t *testing.T) {
	// missing nodeId in a non-clear mode
	if msg := ValidateRPC("set_prototype_start", nil, map[string]interface{}{}); msg == "" {
		t.Error("expected error for missing nodeId without clear mode")
	}
	// valid single starting point
	if msg := ValidateRPC("set_prototype_start", []string{"1:2"}, map[string]interface{}{}); msg != "" {
		t.Errorf("unexpected error for a valid starting point: %s", msg)
	}
	// bad nodeId format
	if msg := ValidateRPC("set_prototype_start", []string{"1-2"}, map[string]interface{}{}); msg == "" {
		t.Error("expected error for bad nodeId format")
	}
	// bad mode
	if msg := ValidateRPC("set_prototype_start", []string{"1:2"}, map[string]interface{}{"mode": "overwrite"}); msg == "" {
		t.Error("expected error for bad mode")
	}
	// remove mode is valid with a nodeId (targeted removal)
	if msg := ValidateRPC("set_prototype_start", []string{"1:2"}, map[string]interface{}{"mode": "remove"}); msg != "" {
		t.Errorf("remove mode with a nodeId should be valid, got: %s", msg)
	}
	// remove mode still requires a nodeId (only clear may omit it)
	if msg := ValidateRPC("set_prototype_start", nil, map[string]interface{}{"mode": "remove"}); msg == "" {
		t.Error("remove mode without a nodeId should error")
	}
	// clear mode is valid WITHOUT any nodeId (the only way to remove all start points)
	if msg := ValidateRPC("set_prototype_start", nil, map[string]interface{}{"mode": "clear"}); msg != "" {
		t.Errorf("clear mode must not require a nodeId, got: %s", msg)
	}
	// clear mode still accepts a nodeId (to target a specific page)
	if msg := ValidateRPC("set_prototype_start", []string{"1:2"}, map[string]interface{}{"mode": "clear"}); msg != "" {
		t.Errorf("clear mode with a nodeId should be valid, got: %s", msg)
	}
	// names entries must be strings
	if msg := ValidateRPC("set_prototype_start", []string{"1:2"}, map[string]interface{}{
		"names": []any{42},
	}); msg == "" {
		t.Error("expected error for non-string names entry")
	}
}

// ── remove_reactions ──────────────────────────────────────────────────────────

func TestValidateRPC_RemoveReactions(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("remove_reactions", nil, map[string]interface{}{}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// bad nodeId format
	if msg := ValidateRPC("remove_reactions", []string{"1-2"}, map[string]interface{}{}); msg == "" {
		t.Error("expected error for bad nodeId format")
	}
	// non-number in indices
	if msg := ValidateRPC("remove_reactions", []string{"1:2"}, map[string]interface{}{
		"indices": []interface{}{"zero"},
	}); msg == "" {
		t.Error("expected error for non-number index")
	}
	// valid with no indices (remove all)
	if msg := ValidateRPC("remove_reactions", []string{"1:2"}, map[string]interface{}{}); msg != "" {
		t.Errorf("unexpected error for remove all: %s", msg)
	}
	// valid with numeric indices
	if msg := ValidateRPC("remove_reactions", []string{"1:2"}, map[string]interface{}{
		"indices": []interface{}{float64(0), float64(2)},
	}); msg != "" {
		t.Errorf("unexpected error for valid indices: %s", msg)
	}
}

// ── set_visible ─────────────────────────────────────────────────────

func TestValidateRPC_SetVisible(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("set_visible", nil, map[string]interface{}{"visible": true}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("set_visible", []string{"bad"}, map[string]interface{}{"visible": true}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// missing visible
	if msg := ValidateRPC("set_visible", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing visible")
	}
	// valid hide
	if msg := ValidateRPC("set_visible", []string{"1:1"}, map[string]interface{}{"visible": false}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid show
	if msg := ValidateRPC("set_visible", []string{"1:1"}, map[string]interface{}{"visible": true}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── lock_nodes / unlock_nodes ───────────────────────────────────────

func TestValidateRPC_LockUnlockNodes(t *testing.T) {
	for _, tool := range []string{"lock_nodes", "unlock_nodes"} {
		if msg := ValidateRPC(tool, nil, nil); msg == "" {
			t.Errorf("%s: expected error for missing nodeIds", tool)
		}
		if msg := ValidateRPC(tool, []string{"bad"}, nil); msg == "" {
			t.Errorf("%s: expected error for invalid nodeId", tool)
		}
		if msg := ValidateRPC(tool, []string{"1:1", "2:2"}, nil); msg != "" {
			t.Errorf("%s: unexpected error: %s", tool, msg)
		}
	}
}

// ── rotate_nodes ───────────────────────────────────────────────────

func TestValidateRPC_RotateNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("rotate_nodes", nil, map[string]interface{}{"rotation": float64(45)}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("rotate_nodes", []string{"bad"}, map[string]interface{}{"rotation": float64(45)}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// missing rotation
	if msg := ValidateRPC("rotate_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing rotation")
	}
	// valid
	if msg := ValidateRPC("rotate_nodes", []string{"1:1"}, map[string]interface{}{"rotation": float64(-90)}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── reorder_nodes ───────────────────────────────────────────────────

func TestValidateRPC_ReorderNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("reorder_nodes", nil, map[string]interface{}{"order": "bringToFront"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// invalid order
	if msg := ValidateRPC("reorder_nodes", []string{"1:1"}, map[string]interface{}{"order": "up"}); msg == "" {
		t.Error("expected error for invalid order")
	}
	// missing order (empty string falls through to default)
	if msg := ValidateRPC("reorder_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing order")
	}
	// valid orders
	for _, order := range []string{"bringToFront", "sendToBack", "bringForward", "sendBackward"} {
		if msg := ValidateRPC("reorder_nodes", []string{"1:1"}, map[string]interface{}{"order": order}); msg != "" {
			t.Errorf("unexpected error for order %q: %s", order, msg)
		}
	}
}

// ── set_blend_mode ─────────────────────────────────────────────────

func TestValidateRPC_SetBlendMode(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("set_blend_mode", nil, map[string]interface{}{"blendMode": "MULTIPLY"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// missing blendMode
	if msg := ValidateRPC("set_blend_mode", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing blendMode")
	}
	// invalid blendMode
	if msg := ValidateRPC("set_blend_mode", []string{"1:1"}, map[string]interface{}{"blendMode": "GLOW"}); msg == "" {
		t.Error("expected error for invalid blendMode")
	}
	// valid blend modes
	for _, bm := range []string{"NORMAL", "MULTIPLY", "SCREEN", "OVERLAY", "PASS_THROUGH"} {
		if msg := ValidateRPC("set_blend_mode", []string{"1:1"}, map[string]interface{}{"blendMode": bm}); msg != "" {
			t.Errorf("unexpected error for blendMode %q: %s", bm, msg)
		}
	}
}

// ── set_constraints ────────────────────────────────────────────────

func TestValidateRPC_SetConstraints(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("set_constraints", nil, map[string]interface{}{"horizontal": "CENTER"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// missing both horizontal and vertical
	if msg := ValidateRPC("set_constraints", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing constraints")
	}
	// invalid horizontal
	if msg := ValidateRPC("set_constraints", []string{"1:1"}, map[string]interface{}{"horizontal": "LEFT"}); msg == "" {
		t.Error("expected error for invalid horizontal value")
	}
	// invalid vertical
	if msg := ValidateRPC("set_constraints", []string{"1:1"}, map[string]interface{}{"vertical": "TOP"}); msg == "" {
		t.Error("expected error for invalid vertical value")
	}
	// valid horizontal only
	if msg := ValidateRPC("set_constraints", []string{"1:1"}, map[string]interface{}{"horizontal": "STRETCH"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid vertical only
	if msg := ValidateRPC("set_constraints", []string{"1:1"}, map[string]interface{}{"vertical": "CENTER"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid both
	if msg := ValidateRPC("set_constraints", []string{"1:1"}, map[string]interface{}{"horizontal": "MIN", "vertical": "MAX"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── reparent_nodes ─────────────────────────────────────────────────

func TestValidateRPC_ReparentNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("reparent_nodes", nil, map[string]interface{}{"parentId": "2:2"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// missing parentId
	if msg := ValidateRPC("reparent_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing parentId")
	}
	// invalid parentId
	if msg := ValidateRPC("reparent_nodes", []string{"1:1"}, map[string]interface{}{"parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	// valid
	if msg := ValidateRPC("reparent_nodes", []string{"1:1"}, map[string]interface{}{"parentId": "2:2"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── batch_rename_nodes ──────────────────────────────────────────────

func TestValidateRPC_BatchRenameNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("batch_rename_nodes", nil, map[string]interface{}{"prefix": "x"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// no operation provided
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for no rename operation")
	}
	// find without replace
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, map[string]interface{}{"find": "x"}); msg == "" {
		t.Error("expected error for find without replace")
	}
	// valid prefix only
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, map[string]interface{}{"prefix": "UI/"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid find+replace
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, map[string]interface{}{"find": "Btn", "replace": "Button"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── find_replace_text ───────────────────────────────────────────────

func TestValidateRPC_FindReplaceText(t *testing.T) {
	// missing find
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"replace": "x"}); msg == "" {
		t.Error("expected error for missing find")
	}
	// missing replace
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"find": "x"}); msg == "" {
		t.Error("expected error for missing replace")
	}
	// valid minimal
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"find": "x", "replace": "y"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid with empty replace (delete matches)
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"find": "x", "replace": ""}); msg != "" {
		t.Errorf("unexpected error for empty replace: %s", msg)
	}
}

// ── Page management ─────────────────────────────────────────────────

func TestValidateRPC_AddPage(t *testing.T) {
	// valid with no params
	if msg := ValidateRPC("add_page", nil, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// negative index
	if msg := ValidateRPC("add_page", nil, map[string]interface{}{"index": float64(-1)}); msg == "" {
		t.Error("expected error for negative index")
	}
	// valid with name
	if msg := ValidateRPC("add_page", nil, map[string]interface{}{"name": "Flows"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_DeletePage(t *testing.T) {
	// missing both pageId and pageName
	if msg := ValidateRPC("delete_page", nil, nil); msg == "" {
		t.Error("expected error for missing page identifier")
	}
	// valid with pageId
	if msg := ValidateRPC("delete_page", nil, map[string]interface{}{"pageId": "0:2"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid with pageName
	if msg := ValidateRPC("delete_page", nil, map[string]interface{}{"pageName": "Flows"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_RenamePage(t *testing.T) {
	// missing page identifier
	if msg := ValidateRPC("rename_page", nil, map[string]interface{}{"newName": "X"}); msg == "" {
		t.Error("expected error for missing page identifier")
	}
	// missing newName
	if msg := ValidateRPC("rename_page", nil, map[string]interface{}{"pageId": "0:2"}); msg == "" {
		t.Error("expected error for missing newName")
	}
	// valid
	if msg := ValidateRPC("rename_page", nil, map[string]interface{}{"pageId": "0:2", "newName": "Sprint 1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetEffects(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("set_effects", nil, map[string]interface{}{"effects": []interface{}{}}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// missing effects
	if msg := ValidateRPC("set_effects", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing effects")
	}
	// effects not an array
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{"effects": "shadow"}); msg == "" {
		t.Error("expected error for non-array effects")
	}
	// invalid effect type
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "GLOW"}},
	}); msg == "" {
		t.Error("expected error for invalid effect type")
	}
	// valid empty effects (clear all)
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{"effects": []interface{}{}}); msg != "" {
		t.Errorf("unexpected error for empty effects: %s", msg)
	}
	// valid drop shadow
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "DROP_SHADOW"}},
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid layer blur
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "LAYER_BLUR", "radius": float64(4)}},
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "LAYER_BLUR", "blurType": "GRADUAL"}},
	}); msg == "" {
		t.Error("expected error for invalid blurType")
	}
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "NOISE", "noiseType": "CHROMA"}},
	}); msg == "" {
		t.Error("expected error for invalid noiseType")
	}
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "TEXTURE", "noiseSizeVector": map[string]interface{}{"x": float64(2)}}},
	}); msg == "" {
		t.Error("expected error for incomplete noiseSizeVector")
	}
}

func TestValidateRPC_CreateSection(t *testing.T) {
	// valid with no params
	if msg := ValidateRPC("create_section", nil, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid with name
	if msg := ValidateRPC("create_section", nil, map[string]interface{}{"name": "Sprint 1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// invalid width
	if msg := ValidateRPC("create_section", nil, map[string]interface{}{"width": float64(-10)}); msg == "" {
		t.Error("expected error for negative width")
	}
	// invalid height
	if msg := ValidateRPC("create_section", nil, map[string]interface{}{"height": float64(0)}); msg == "" {
		t.Error("expected error for zero height")
	}
}

// ── Library tools (Track A) ───────────────────────────────────────────────────

func TestValidateRPC_ImportByKey(t *testing.T) {
	validPublishedKey := "0123456789abcdef0123456789abcdef01234567"

	for _, tool := range []string{"import_component_by_key", "import_variable_by_key", "import_style_by_key"} {
		if msg := ValidateRPC(tool, nil, nil); msg == "" {
			t.Errorf("%s: expected error for missing key", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"key": ""}); msg == "" {
			t.Errorf("%s: expected error for empty key", tool)
		}
	}

	for _, tool := range []string{"import_component_by_key", "import_style_by_key"} {
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"key": validPublishedKey}); msg != "" {
			t.Errorf("%s: unexpected error for valid published key: %s", tool, msg)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"key": "8b931898634bdc63"}); !containsCI(msg, "truncated") {
			t.Errorf("%s: truncated key should get truncated hint, got: %s", tool, msg)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"key": "410:49695"}); !containsCI(msg, "node id") {
			t.Errorf("%s: node-id key should get node-id hint, got: %s", tool, msg)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"key": "ABCDEF0123456789abcdef0123456789abcdef01"}); !containsCI(msg, "40-char hex") {
			t.Errorf("%s: uppercase/non-lowercase key should mention 40-char hex, got: %s", tool, msg)
		}
	}
	if msg := ValidateRPC("import_component_by_key", nil, map[string]interface{}{"key": validPublishedKey, "assetType": "COMPONENT"}); msg != "" {
		t.Errorf("import_component_by_key: unexpected error for valid COMPONENT assetType: %s", msg)
	}
	if msg := ValidateRPC("import_component_by_key", nil, map[string]interface{}{"key": validPublishedKey, "assetType": "COMPONENT_SET"}); msg != "" {
		t.Errorf("import_component_by_key: unexpected error for valid COMPONENT_SET assetType: %s", msg)
	}
	if msg := ValidateRPC("import_component_by_key", nil, map[string]interface{}{"key": validPublishedKey, "assetType": "STYLE"}); !containsCI(msg, "assetType") {
		t.Errorf("import_component_by_key: invalid assetType should be rejected, got: %s", msg)
	}

	if msg := ValidateRPC("import_variable_by_key", nil, map[string]interface{}{"key": "VariableID:123:456"}); msg != "" {
		t.Errorf("import_variable_by_key: unexpected error for VariableID key: %s", msg)
	}
	if msg := ValidateRPC("import_variable_by_key", nil, map[string]interface{}{"key": validPublishedKey + "/colors/brand"}); msg != "" {
		t.Errorf("import_variable_by_key: unexpected error for collection/path key: %s", msg)
	}
	if msg := ValidateRPC("import_variable_by_key", nil, map[string]interface{}{"key": "410:49695"}); !containsCI(msg, "node id") {
		t.Errorf("import_variable_by_key: node-id key should get node-id hint, got: %s", msg)
	}
}

func TestValidateRPC_CreateInstance(t *testing.T) {
	// missing componentId
	if msg := ValidateRPC("create_instance", nil, nil); msg == "" {
		t.Error("expected error for missing componentId")
	}
	// invalid componentId format
	if msg := ValidateRPC("create_instance", nil, map[string]interface{}{"componentId": "bad-format"}); msg == "" {
		t.Error("expected error for invalid componentId format")
	}
	// invalid parentId format
	if msg := ValidateRPC("create_instance", nil, map[string]interface{}{"componentId": "2:2", "parentId": "nope"}); msg == "" {
		t.Error("expected error for invalid parentId format")
	}
	// valid (no parent)
	if msg := ValidateRPC("create_instance", nil, map[string]interface{}{"componentId": "2:2"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid (with parent)
	if msg := ValidateRPC("create_instance", nil, map[string]interface{}{"componentId": "2:2", "parentId": "1:1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetInstanceProperties(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("set_instance_properties", nil, map[string]interface{}{"properties": map[string]interface{}{"State": "On"}}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// invalid nodeId
	if msg := ValidateRPC("set_instance_properties", []string{"bad-id"}, map[string]interface{}{"properties": map[string]interface{}{"State": "On"}}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// missing properties
	if msg := ValidateRPC("set_instance_properties", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing properties")
	}
	// empty properties
	if msg := ValidateRPC("set_instance_properties", []string{"1:1"}, map[string]interface{}{"properties": map[string]interface{}{}}); msg == "" {
		t.Error("expected error for empty properties")
	}
	// valid
	if msg := ValidateRPC("set_instance_properties", []string{"1:1"}, map[string]interface{}{"properties": map[string]interface{}{"State": "On"}}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetVariableMode(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("set_variable_mode", nil, map[string]interface{}{"collectionId": "c1", "modeId": "m1"}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// invalid nodeId
	if msg := ValidateRPC("set_variable_mode", []string{"bad-id"}, map[string]interface{}{"collectionId": "c1", "modeId": "m1"}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// missing collectionId
	if msg := ValidateRPC("set_variable_mode", []string{"1:1"}, map[string]interface{}{"modeId": "m1"}); msg == "" {
		t.Error("expected error for missing collectionId")
	}
	// missing modeId
	if msg := ValidateRPC("set_variable_mode", []string{"1:1"}, map[string]interface{}{"collectionId": "c1"}); msg == "" {
		t.Error("expected error for missing modeId")
	}
	// valid
	if msg := ValidateRPC("set_variable_mode", []string{"1:1"}, map[string]interface{}{"collectionId": "c1", "modeId": "m1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// TestValidateRPC_IgnoresChannel locks the follower→leader channel-routing
// contract: ValidateRPC must NOT reject the universal `channel` param (it is a
// routing key stripped later by bridge.Send). If ValidateRPC ever becomes an
// allowlist, this guards against it silently breaking multi-file routing.
func TestValidateRPC_IgnoresChannel(t *testing.T) {
	// A valid call with an extra `channel` key stays valid.
	if msg := ValidateRPC("set_variable_mode", []string{"1:1"},
		map[string]interface{}{"collectionId": "c1", "modeId": "m1", "channel": "a3f9"}); msg != "" {
		t.Errorf("channel param must be ignored, got: %s", msg)
	}
	// A param-less tool with only a channel stays valid.
	if msg := ValidateRPC("get_pages", nil, map[string]interface{}{"channel": "a3f9"}); msg != "" {
		t.Errorf("channel-only params must be valid, got: %s", msg)
	}
}

func TestValidateRPC_GetRemoteVariableCollection(t *testing.T) {
	// missing collectionId
	if msg := ValidateRPC("get_remote_variable_collection", nil, nil); msg == "" {
		t.Error("expected error for missing collectionId")
	}
	// valid
	if msg := ValidateRPC("get_remote_variable_collection", nil, map[string]interface{}{"collectionId": "c1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ListLibraryVariableCollections(t *testing.T) {
	// no params required — always valid
	if msg := ValidateRPC("list_library_variable_collections", nil, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── fetch_library_catalog fileKey injection (Part 1.1) ───────────────────────

// TestValidateRPC_FetchLibraryCatalog_FileKeyInjection asserts that injection
// attempts in fileKey are rejected before they can reach the HTTP layer.
func TestValidateRPC_FetchLibraryCatalog_FileKeyInjection(t *testing.T) {
	injectionKeys := []string{
		"KEY/components?x=",
		"a/../b",
		"../../etc/passwd",
		"key with spaces",
		"key\x00null",
		"KEY?token=abc",
		"KEY#fragment",
	}
	for _, key := range injectionKeys {
		msg := ValidateRPC("fetch_library_catalog", nil, map[string]any{
			"fileKey": key,
			"outPath": "catalog.json",
		})
		if msg == "" {
			t.Errorf("expected rejection for injection fileKey %q", key)
		}
		if msg != "" && !containsCI(msg, "fileKey") {
			t.Errorf("error for key %q should mention fileKey, got: %s", key, msg)
		}
	}
}

// TestValidateRPC_FetchLibraryCatalog_ValidFileKey asserts normal alnum keys pass.
func TestValidateRPC_FetchLibraryCatalog_ValidFileKey(t *testing.T) {
	validKeys := []string{
		"ABCDEFGHIJKLMNOPQRSTUV", // 22 alnum chars (typical Figma key)
		"abc123DEF456",           // mixed case + digits
		"key-with-hyphens",       // hyphens allowed
		"key_with_underscores",   // underscores allowed
		"MixedCase-Key_123",      // mixed with hyphens and underscores
	}
	for _, key := range validKeys {
		msg := ValidateRPC("fetch_library_catalog", nil, map[string]any{
			"fileKey": key,
			"outPath": "catalog.json",
		})
		if msg != "" {
			t.Errorf("unexpected rejection for valid fileKey %q: %s", key, msg)
		}
	}
}

// containsCI is a helper for case-insensitive string containment checks.
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		sl := []byte(s)
		subl := []byte(substr)
		for i := range sl {
			if i+len(subl) > len(sl) {
				break
			}
			match := true
			for j, b := range subl {
				sb := sl[i+j]
				if b >= 'A' && b <= 'Z' {
					b += 32
				}
				if sb >= 'A' && sb <= 'Z' {
					sb += 32
				}
				if b != sb {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
		return false
	}()
}

// ── get_local_components pageId validation (Part 2) ──────────────────────────

// TestValidateRPC_GetLocalComponents_PageIdValidation asserts that when
// pageId is provided it must be a valid colon-format node ID; absent is OK.
func TestValidateRPC_GetLocalComponents_PageIdValidation(t *testing.T) {
	// Absent pageId — always valid.
	if msg := ValidateRPC("get_local_components", nil, nil); msg != "" {
		t.Errorf("no params should be valid, got: %s", msg)
	}
	if msg := ValidateRPC("get_local_components", nil, map[string]any{}); msg != "" {
		t.Errorf("empty params should be valid, got: %s", msg)
	}
	// Present and valid page IDs.
	for _, id := range []string{"0:1", "123:456", "I44:9;44:3"} {
		if msg := ValidateRPC("get_local_components", nil, map[string]any{"pageId": id}); msg != "" {
			t.Errorf("valid pageId %q should pass, got: %s", id, msg)
		}
	}
	// Present and invalid page IDs.
	for _, id := range []string{"bad", "4029-12345", "abc:def", ""} {
		msg := ValidateRPC("get_local_components", nil, map[string]any{"pageId": id})
		if id == "" {
			// Empty string treated as absent — still valid.
			if msg != "" {
				t.Errorf("empty pageId string should be treated as absent, got: %s", msg)
			}
		} else {
			if msg == "" {
				t.Errorf("invalid pageId %q should be rejected", id)
			}
		}
	}
}
