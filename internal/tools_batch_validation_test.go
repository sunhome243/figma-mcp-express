package internal

import (
	"encoding/json"
	"testing"
)

// Regression for issue #35: the create_variable batch op rejected payloads with
// "missing string `type`" even when the type field was present. Validate the exact
// payload from the issue through the batch(validateOnly:true) path, which runs the
// full validateBatchParamsAgainstSchema chain, and assert it passes.
func TestBatchValidateOnly_CreateVariableWithTypePasses(t *testing.T) {
	s, _ := newTestServer(t)

	ops := []any{
		map[string]any{
			"type": "create_variable",
			"params": map[string]any{
				"name":         "spacing/custom",
				"type":         "FLOAT",
				"collectionId": "VariableCollectionId:123:456",
			},
		},
	}

	res := callToolResult(t, s, "batch", map[string]any{
		"ops":          ops,
		"origin":       "sunho",
		"validateOnly": true,
	})
	if res.IsError {
		t.Fatalf("batch(validateOnly) on create_variable returned error: %s", resultText(t, res))
	}

	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("batch(validateOnly) structuredContent = %T", res.StructuredContent)
	}
	if valid, _ := structured["valid"].(bool); !valid {
		raw, _ := json.Marshal(structured)
		t.Fatalf("create_variable with type=FLOAT must validate as valid:true, got %s", raw)
	}
}

// batchValidateOnlyValid runs an op list through batch(validateOnly:true) — the real
// runtime gate — and returns whether it validated as valid (plus the raw structured
// result for diagnostics). This exercises validateBatchParamsAgainstSchema, which
// ValidateRPC-only tests bypass.
func batchValidateOnlyValid(t *testing.T, op map[string]any) (bool, string) {
	t.Helper()
	s, _ := newTestServer(t)
	res := callToolResult(t, s, "batch", map[string]any{
		"ops":          []any{op},
		"origin":       "sunho",
		"validateOnly": true,
	})
	if res.IsError {
		return false, resultText(t, res)
	}
	structured, _ := res.StructuredContent.(map[string]any)
	raw, _ := json.Marshal(structured)
	valid, _ := structured["valid"].(bool)
	return valid, string(raw)
}

// Regression: LINEAR_BURN / LINEAR_DODGE must pass the BATCH schema gate, not just
// ValidateRPC. set_blend_mode is batch-only, so the catalog enum (not ValidateRPC) is
// the authoritative runtime check — they were missing from it.
func TestBatchValidateOnly_SetBlendMode_LinearModes(t *testing.T) {
	for _, bm := range []string{"LINEAR_BURN", "LINEAR_DODGE"} {
		valid, raw := batchValidateOnlyValid(t, map[string]any{
			"type":    "set_blend_mode",
			"nodeIds": []any{"1:1"},
			"params":  map[string]any{"blendMode": bm},
		})
		if !valid {
			t.Errorf("set_blend_mode %s must validate through the batch gate, got %s", bm, raw)
		}
	}
}

// Regression: optional params documented as "pass null to clear" must survive the
// batch schema gate (null was rejected by the number/object type assertions).
func TestBatchValidateOnly_NullClearsOptionalParam(t *testing.T) {
	cases := []map[string]any{
		{"type": "set_text", "nodeIds": []any{"1:1"}, "params": map[string]any{"maxLines": nil}},
		{"type": "set_auto_layout", "nodeIds": []any{"1:1"}, "params": map[string]any{"minWidth": nil}},
		{"type": "set_text_range", "nodeIds": []any{"1:1"}, "params": map[string]any{"startOffset": float64(0), "endOffset": float64(3), "hyperlink": nil}},
	}
	for _, op := range cases {
		valid, raw := batchValidateOnlyValid(t, op)
		if !valid {
			t.Errorf("op %v with a null optional param must validate, got %s", op["type"], raw)
		}
	}
}

func TestBatchValidateOnly_CreateFrameGridGapVariableIds(t *testing.T) {
	valid, raw := batchValidateOnlyValid(t, map[string]any{
		"type": "create_frame",
		"params": map[string]any{
			"layoutMode":              "GRID",
			"gridRowCount":            float64(2),
			"gridColumnCount":         float64(3),
			"gridRowGapVariableId":    "VariableID:1:2",
			"gridColumnGapVariableId": "VariableID:1:3",
		},
	})
	if !valid {
		t.Fatalf("create_frame GRID gap variable ids must validate through the batch gate, got %s", raw)
	}
}
