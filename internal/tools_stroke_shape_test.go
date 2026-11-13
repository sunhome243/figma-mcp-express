package internal

import (
	"strings"
	"testing"
)

func TestSetStrokes_GeometryOnlyAndEnums(t *testing.T) {
	cases := []struct {
		name    string
		params  map[string]any
		wantErr string
	}{
		{"dash only restyles existing stroke", map[string]any{"dashPattern": []any{float64(4), float64(2)}}, ""},
		{"align only", map[string]any{"strokeAlign": "INSIDE"}, ""},
		{"single bottom border", map[string]any{"color": "#E5E5E5", "strokeTopWeight": float64(0), "strokeBottomWeight": float64(1)}, ""},
		{"variableId alone", map[string]any{"variableId": "VariableID:1:2"}, ""},
		{"nothing at all", map[string]any{}, "color or paints"},
		{"bad align", map[string]any{"strokeAlign": "MIDDLE"}, "strokeAlign"},
		{"bad join", map[string]any{"strokeJoin": "SHARP"}, "strokeJoin"},
		{"bad cap", map[string]any{"strokeCap": "BUTT"}, "strokeCap"},
		{"negative dash", map[string]any{"dashPattern": []any{float64(-1)}}, "dashPattern[0]"},
		{"dash not array", map[string]any{"dashPattern": "4,4"}, "dashPattern"},
		{"negative side weight", map[string]any{"strokeLeftWeight": float64(-2)}, "strokeLeftWeight"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errored, txt := batchValidateOnly(t, map[string]any{
				"type":    "set_strokes",
				"nodeIds": []any{"1:2"},
				"params":  tc.params,
			})
			if tc.wantErr == "" && errored {
				t.Fatalf("expected pass, got: %s", txt)
			}
			if tc.wantErr != "" && (!errored || !strings.Contains(txt, tc.wantErr)) {
				t.Fatalf("expected error containing %q, got errored=%v %s", tc.wantErr, errored, txt)
			}
		})
	}
}

func TestSetCornerRadius_CornerSmoothing(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type": "set_corner_radius", "nodeIds": []any{"1:2"},
		"params": map[string]any{"cornerSmoothing": 0.6},
	}); errored {
		t.Fatalf("cornerSmoothing alone should pass, got: %s", txt)
	}
	if errored, _ := batchValidateOnly(t, map[string]any{
		"type": "set_corner_radius", "nodeIds": []any{"1:2"},
		"params": map[string]any{"cornerSmoothing": 1.5},
	}); !errored {
		t.Fatal("cornerSmoothing > 1 must fail")
	}
}

func TestSetMask_Validation(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type": "set_mask", "nodeIds": []any{"1:2"},
		"params": map[string]any{"maskType": "LUMINANCE"},
	}); errored {
		t.Fatalf("set_mask should pass, got: %s", txt)
	}
	if errored, _ := batchValidateOnly(t, map[string]any{
		"type": "set_mask", "nodeIds": []any{"1:2"},
		"params": map[string]any{"maskType": "OUTLINE"},
	}); !errored {
		t.Fatal("bad maskType must fail")
	}
}

func TestSetLayoutGrids_Validation(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type": "set_layout_grids", "nodeIds": []any{"1:2"},
		"params": map[string]any{"grids": []any{
			map[string]any{"pattern": "COLUMNS", "count": float64(12), "gutterSize": float64(24), "offset": float64(80)},
		}},
	}); errored {
		t.Fatalf("12-col grid should pass, got: %s", txt)
	}
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type": "set_layout_grids", "nodeIds": []any{"1:2"},
		"params": map[string]any{"grids": []any{}},
	}); errored {
		t.Fatalf("empty grids (clear) should pass, got: %s", txt)
	}
	for name, params := range map[string]map[string]any{
		"missing grids": {},
		"bad pattern":   {"grids": []any{map[string]any{"pattern": "HEX"}}},
		"zero count":    {"grids": []any{map[string]any{"pattern": "COLUMNS", "count": float64(0)}}},
		"bad mode":      {"grids": []any{}, "mode": "merge"},
	} {
		if errored, _ := batchValidateOnly(t, map[string]any{
			"type": "set_layout_grids", "nodeIds": []any{"1:2"}, "params": params,
		}); !errored {
			t.Fatalf("%s must fail validateOnly", name)
		}
	}
}

func TestStrokeShapeOpsAreDiscoverable(t *testing.T) {
	s, _ := newTestServer(t)
	for op, category := range map[string]string{"set_mask": "vector", "set_layout_grids": "layout"} {
		spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": op})
		if spec.IsError {
			t.Fatalf("get_batch_op_spec(%s) errored: %s", op, resultText(t, spec))
		}
		if txt := resultText(t, spec); !strings.Contains(txt, category) {
			t.Fatalf("%s spec should be categorized under %s, got: %s", op, category, txt)
		}
	}
}
