package internal

import (
	"strings"
	"testing"
)

// The prototype scroll/fixed ops are batch-only (demoted, off tools/list). These guard
// that the catalog validates them on the validateOnly path — required params, enum
// vocabulary, and unknown-param rejection — without ever calling the plugin bridge.

func batchValidateOnly(t *testing.T, ops ...map[string]any) (errored bool, text string) {
	t.Helper()
	s, _ := newTestServer(t)
	raw := make([]any, len(ops))
	for i, op := range ops {
		raw[i] = op
	}
	res := callToolResult(t, s, "batch", map[string]any{"validateOnly": true, "ops": raw})
	return res.IsError, resultText(t, res)
}

func TestSetOverflow_ValidatesEnumAndClips(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type":    "set_overflow",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{"overflowDirection": "VERTICAL", "clipsContent": true},
	}); errored {
		t.Fatalf("valid set_overflow should pass validateOnly, got: %s", txt)
	}

	if errored, txt := batchValidateOnly(t, map[string]any{
		"type":    "set_overflow",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{"overflowDirection": "DIAGONAL"},
	}); !errored {
		t.Fatalf("bad overflowDirection enum must fail validateOnly, got: %s", txt)
	}

	if errored, _ := batchValidateOnly(t, map[string]any{
		"type":    "set_overflow",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{},
	}); !errored {
		t.Fatal("set_overflow missing required overflowDirection must fail validateOnly")
	}
}

func TestSetFixedChildren_RequiresCount(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type":    "set_fixed_children",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{"numberOfFixedChildren": float64(2)},
	}); errored {
		t.Fatalf("valid set_fixed_children should pass validateOnly, got: %s", txt)
	}

	if errored, _ := batchValidateOnly(t, map[string]any{
		"type":    "set_fixed_children",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{},
	}); !errored {
		t.Fatal("set_fixed_children missing required numberOfFixedChildren must fail validateOnly")
	}

	if errored, _ := batchValidateOnly(t, map[string]any{
		"type":    "set_fixed_children",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{"numberOfFixedChildren": "two"},
	}); !errored {
		t.Fatal("set_fixed_children with a non-number count must fail validateOnly")
	}
}

func TestPinChild_TakesNoParams(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type":    "pin_child",
		"nodeIds": []any{"1:2"},
	}); errored {
		t.Fatalf("pin_child with only nodeIds should pass validateOnly, got: %s", txt)
	}

	if errored, _ := batchValidateOnly(t, map[string]any{
		"type":    "pin_child",
		"nodeIds": []any{"1:2"},
		"params":  map[string]any{"numberOfFixedChildren": float64(1)},
	}); !errored {
		t.Fatal("pin_child must reject unknown params — it takes none")
	}
}

func TestSetPrototypeBackground_SetAndClear(t *testing.T) {
	if errored, txt := batchValidateOnly(t, map[string]any{
		"type":   "set_prototype_background",
		"params": map[string]any{"color": "#101014"},
	}); errored {
		t.Fatalf("set_prototype_background with color should pass validateOnly, got: %s", txt)
	}

	if errored, txt := batchValidateOnly(t, map[string]any{
		"type":   "set_prototype_background",
		"params": map[string]any{"mode": "clear"},
	}); errored {
		t.Fatalf("set_prototype_background mode=clear should pass validateOnly, got: %s", txt)
	}

	if errored, _ := batchValidateOnly(t, map[string]any{
		"type":   "set_prototype_background",
		"params": map[string]any{"mode": "wipe"},
	}); !errored {
		t.Fatal("set_prototype_background bad mode enum must fail validateOnly")
	}
}

func TestPrototypeScrollOpsAreDiscoverable(t *testing.T) {
	s, _ := newTestServer(t)
	for _, op := range []string{"set_overflow", "set_fixed_children", "pin_child", "set_prototype_background"} {
		spec := callToolResult(t, s, "get_batch_op_spec", map[string]any{"op": op})
		if spec.IsError {
			t.Fatalf("get_batch_op_spec(%s) errored: %s", op, resultText(t, spec))
		}
		if txt := resultText(t, spec); !strings.Contains(txt, op) || !strings.Contains(txt, "prototype") {
			t.Fatalf("%s spec should be categorized under prototype, got: %s", op, txt)
		}
	}
}
