package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── gateResponse (legacy gateResponseText behavior, new []byte+data signature) ─

func TestGateResponseText_UnderThreshold(t *testing.T) {
	dir := t.TempDir()
	text := `{"hello":"world"}`
	out, spilled, err := gateResponse([]byte(text), nil, "test", dir, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spilled {
		t.Error("expected spilled=false for under-threshold text")
	}
	if out != text {
		t.Errorf("expected passthrough text, got %q", out)
	}
	// No file should have been written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected no files written for under-threshold, got %d", len(entries))
	}
}

func TestGateResponseText_AtThreshold(t *testing.T) {
	dir := t.TempDir()
	// exactly at threshold — should pass through unchanged
	text := strings.Repeat("x", 100)
	out, spilled, err := gateResponse([]byte(text), nil, "test", dir, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spilled {
		t.Error("expected spilled=false at exactly threshold")
	}
	if out != text {
		t.Errorf("expected passthrough at threshold, got %q", out)
	}
}

func TestGateResponseText_OverThreshold_WritesFile(t *testing.T) {
	dir := t.TempDir()
	// Generate text bigger than the threshold.
	text := strings.Repeat("a", 200)
	threshold := 50

	out, spilled, err := gateResponse([]byte(text), nil, "catalog", dir, threshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spilled=true for over-threshold text")
	}

	// Parse the handle JSON.
	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle is not valid JSON: %v\nhandle: %s", err, out)
	}

	// Check required handle fields.
	if v, ok := handle["spilled"].(bool); !ok || !v {
		t.Errorf("handle.spilled must be true, got %v", handle["spilled"])
	}
	if _, ok := handle["path"].(string); !ok {
		t.Error("handle.path must be a string")
	}
	if bytes, ok := handle["bytes"].(float64); !ok || int(bytes) != len(text) {
		t.Errorf("handle.bytes = %v, want %d", handle["bytes"], len(text))
	}
	if _, ok := handle["preview"].(string); !ok {
		t.Error("handle.preview must be a string")
	}
	if _, ok := handle["hint"].(string); !ok {
		t.Error("handle.hint must be a string")
	}

	// The written file must contain the full original text.
	writtenPath, _ := handle["path"].(string)
	got, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", writtenPath, err)
	}
	if string(got) != text {
		t.Error("file content does not match original text")
	}
}

func TestGateResponseText_OverThreshold_FullArrayInFile_NotInHandle(t *testing.T) {
	dir := t.TempDir()
	// Build a large text that is too big for inline output.
	// Use a sentinel that appears ONLY at positions > 600 runes so it can never
	// appear in the handle's preview field (which is capped at 600 runes).
	// text() pads the bigText by repeating it, so sentinel only in tail.
	prefix := strings.Repeat("x", 700) // 700 x's — past preview cap
	sentinel := `"SENTINEL_NOT_IN_PREVIEW"`
	bigText := prefix + sentinel
	threshold := 10

	out, spilled, err := gateResponse([]byte(bigText), nil, "catalog", dir, threshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spilled=true")
	}

	// The handle must NOT contain the sentinel (it's beyond the preview cap).
	if strings.Contains(out, "SENTINEL_NOT_IN_PREVIEW") {
		t.Error("handle must not contain content beyond preview cap")
	}

	// Verify written file contains the full original text including the sentinel.
	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle not valid JSON: %v", err)
	}
	writtenPath, _ := handle["path"].(string)
	fileContent, _ := os.ReadFile(writtenPath)
	if string(fileContent) != bigText {
		t.Error("written file does not contain full original text")
	}
	if !strings.Contains(string(fileContent), "SENTINEL_NOT_IN_PREVIEW") {
		t.Error("written file should contain the full text including sentinel")
	}
}

func TestGateResponseText_Preview_TruncatedAt600Runes(t *testing.T) {
	dir := t.TempDir()
	// 800-rune text to force spill and check preview length.
	text := strings.Repeat("あ", 800) // multibyte runes
	threshold := 10

	out, spilled, err := gateResponse([]byte(text), nil, "big", dir, threshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spilled=true")
	}

	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle not valid JSON: %v", err)
	}
	preview, _ := handle["preview"].(string)
	// preview should be ≤ 600 runes (it's a valid substring of the input)
	if len([]rune(preview)) > 600 {
		t.Errorf("preview rune length = %d, want ≤ 600", len([]rune(preview)))
	}
	// Preview should start with the input's first characters.
	if !strings.HasPrefix(text, preview) {
		t.Error("preview must be a prefix of the original text")
	}
}

func TestGateResponseText_FileNameContainsLabel(t *testing.T) {
	dir := t.TempDir()
	text := strings.Repeat("z", 200)
	_, spilled, err := gateResponse([]byte(text), nil, "myLabel", dir, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spill")
	}
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "myLabel-") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a file with prefix 'myLabel-' in %v", entries)
	}
}

func TestGateResponseText_FileInDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "spills")
	text := strings.Repeat("b", 200)

	out, spilled, err := gateResponse([]byte(text), nil, "t", subdir, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spill")
	}
	// dir should have been created automatically.
	if _, err := os.Stat(subdir); err != nil {
		t.Fatalf("expected subdir to be created: %v", err)
	}
	var handle map[string]any
	_ = json.Unmarshal([]byte(out), &handle)
	writtenPath, _ := handle["path"].(string)
	if !strings.HasPrefix(writtenPath, subdir) {
		t.Errorf("written path %q should be inside subdir %q", writtenPath, subdir)
	}
}

// ── parseSpillThreshold (env default) ────────────────────────────────────────

func TestParseSpillThreshold_Default(t *testing.T) {
	t.Setenv("FIGMA_MCP_SPILL_BYTES", "")
	got := parseSpillThreshold()
	if got != 25000 {
		t.Errorf("default threshold = %d, want 25000", got)
	}
}

func TestParseSpillThreshold_CustomValue(t *testing.T) {
	t.Setenv("FIGMA_MCP_SPILL_BYTES", "50000")
	got := parseSpillThreshold()
	if got != 50000 {
		t.Errorf("threshold = %d, want 50000", got)
	}
}

func TestParseSpillThreshold_InvalidFallsToDefault(t *testing.T) {
	t.Setenv("FIGMA_MCP_SPILL_BYTES", "not-a-number")
	got := parseSpillThreshold()
	if got != 25000 {
		t.Errorf("invalid env should fall to default 25000, got %d", got)
	}
}

// ── gateResponseText path-escape (Part 1.2) ──────────────────────────────────

// TestGateResponseText_PathTraversalLabelStaysInDir asserts that a label
// containing path traversal characters ("../../evil") cannot escape dir.
func TestGateResponseText_PathTraversalLabelStaysInDir(t *testing.T) {
	dir := t.TempDir()
	bigText := strings.Repeat("x", 200)
	threshold := 50

	out, spilled, err := gateResponse([]byte(bigText), nil, "../../evil", dir, threshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spilled=true for oversized text")
	}

	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle is not valid JSON: %v\nhandle: %s", err, out)
	}
	writtenPath, _ := handle["path"].(string)

	// The written file must be INSIDE dir (no traversal escape).
	rel, err := filepath.Rel(dir, writtenPath)
	if err != nil {
		t.Fatalf("filepath.Rel error: %v", err)
	}
	if strings.HasPrefix(filepath.ToSlash(rel), "..") {
		t.Errorf("spill file %q escaped dir %q (rel=%q)", writtenPath, dir, rel)
	}

	// The file must actually exist (write succeeded).
	if _, err := os.Stat(writtenPath); err != nil {
		t.Errorf("spill file must exist at %q: %v", writtenPath, err)
	}
}

// TestGateResponseText_SpillFilePerms asserts the spill file is written 0o600.
func TestGateResponseText_SpillFilePerms(t *testing.T) {
	dir := t.TempDir()
	bigText := strings.Repeat("y", 200)
	threshold := 50

	out, spilled, err := gateResponse([]byte(bigText), nil, "permtest", dir, threshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spilled=true")
	}

	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle not valid JSON: %v", err)
	}
	writtenPath, _ := handle["path"].(string)
	info, err := os.Stat(writtenPath)
	if err != nil {
		t.Fatalf("stat %q: %v", writtenPath, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("spill file perm = %04o, want 0600", got)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// text repeats s until total length >= minLen.
func text(s string, minLen int) string {
	for len(s) < minLen {
		s += s
	}
	return s[:minLen]
}
