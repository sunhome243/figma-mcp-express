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
