package internal

import (
	"strings"
	"testing"
)

func TestHintFor_TimeoutHeavyRead(t *testing.T) {
	h := hintFor("get_node", "request timed out")
	if !strings.Contains(h, "narrower") || !strings.Contains(h, "limit") {
		t.Errorf("heavy-read timeout hint missing retry-narrower guidance: %q", h)
	}
}

func TestHintFor_LocalComponentsTimeout(t *testing.T) {
	h := hintFor("get_local_components", "request timed out")
	if !strings.Contains(h, "pageId") || !strings.Contains(h, "local-master recovery") {
		t.Errorf("get_local_components timeout hint should distinguish pageId scoping from recovery: %q", h)
	}
}

func TestHintFor_NotConnectedCatalogMentionsREST(t *testing.T) {
	h := hintFor("get_local_components", "plugin not connected")
	if !strings.Contains(h, "fetch_library_catalog") || !strings.Contains(h, "FIGMA_TOKEN") {
		t.Errorf("catalog not-connected hint should offer the REST path: %q", h)
	}
}

func TestHintFor_NotConnectedCanvasNoREST(t *testing.T) {
	h := hintFor("get_node", "plugin not connected")
	if !strings.Contains(h, "list_channels") {
		t.Errorf("not-connected hint should point at the plugin/list_channels: %q", h)
	}
	if strings.Contains(h, "fetch_library_catalog") {
		t.Errorf("canvas read must NOT be offered a REST fallback it doesn't have: %q", h)
	}
}

func TestHintFor_WriteDrop(t *testing.T) {
	h := hintFor("create_frame", "send: connection reset")
	if !strings.Contains(strings.ToLower(h), "re-run") {
		t.Errorf("write socket-drop hint should advise re-running the plugin: %q", h)
	}
}

func TestHintFor_StaleId(t *testing.T) {
	h := hintFor("set_fills", "Node not found")
	if !strings.Contains(h, "stale") || !strings.Contains(h, "get_metadata") {
		t.Errorf("not-found hint should flag a stale id + re-confirm: %q", h)
	}
}

func TestHintFor_VersionMismatchForKnownOp(t *testing.T) {
	// An op the local catalog knows, rejected as unknown by a forwarded-to leader,
	// means a version mismatch — the hint must name that, not echo "get_batch_op_spec".
	err := `ops[0]: unknown op type "get_prototype"; call search_batch_ops/get_batch_op_spec first`
	h := hintFor("batch", err)
	if !strings.Contains(h, "version") || !strings.Contains(h, "1994") {
		t.Errorf("known-op unknown-type hint should flag a leader version mismatch: %q", h)
	}
}

func TestHintFor_NoVersionHintForTrulyUnknownOp(t *testing.T) {
	// A genuinely unknown op (typo) is NOT in the catalog → no version hint.
	err := `ops[0]: unknown op type "frobnicate_node"; call search_batch_ops/get_batch_op_spec first`
	if h := hintFor("batch", err); strings.Contains(h, "version") {
		t.Errorf("a truly unknown op must not get a version-mismatch hint, got %q", h)
	}
}

func TestHintFor_NoHintForOrdinary(t *testing.T) {
	if h := hintFor("set_fills", ""); h != "" {
		t.Errorf("no error text should yield no hint, got %q", h)
	}
	if h := hintFor("create_frame", "some unrecognised failure"); h != "" {
		t.Errorf("unmatched error should yield no hint, got %q", h)
	}
}

func TestHintForDesignContextDetail_MinimalAndCompact(t *testing.T) {
	for _, detail := range []string{"minimal", "compact"} {
		h := hintForDesignContextDetail(detail)
		if !strings.Contains(h, "typography") {
			t.Errorf("detail %q hint should mention typography, got %q", detail, h)
		}
		if !strings.Contains(h, "codegen") {
			t.Errorf("detail %q hint should mention codegen, got %q", detail, h)
		}
	}
}

func TestHintForDesignContextDetail_NoHintForFullOrCodegen(t *testing.T) {
	for _, detail := range []string{"full", "codegen", ""} {
		if h := hintForDesignContextDetail(detail); h != "" {
			t.Errorf("detail %q should yield no hint, got %q", detail, h)
		}
	}
}
