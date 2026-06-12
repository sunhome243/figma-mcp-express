package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ── renderResponse ────────────────────────────────────────────────────────────

func TestRenderResponse_TransportError(t *testing.T) {
	result, err := renderResponse(BridgeResponse{}, fmt.Errorf("connection failed"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for transport error")
	}
}

func TestRenderResponse_PluginError(t *testing.T) {
	result, err := renderResponse(BridgeResponse{Error: "node not found"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for plugin error")
	}
}

func TestRenderResponse_Success(t *testing.T) {
	result, err := renderResponse(BridgeResponse{Data: map[string]any{"id": "1:1"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for successful response")
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestRenderResponse_NilData(t *testing.T) {
	result, err := renderResponse(BridgeResponse{Data: nil}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for nil data")
	}
}

// ── toStringSlice ─────────────────────────────────────────────────────────────

func TestToStringSlice(t *testing.T) {
	raw := []any{"a", "b", 42, nil, "c"}
	got := toStringSlice(raw)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("[%d] = %q, want %q", i, got[i], s)
		}
	}
}

func TestToStringSlice_Empty(t *testing.T) {
	if got := toStringSlice(nil); len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ── coalesce ─────────────────────────────────────────────────────────────────

func TestCoalesce(t *testing.T) {
	if got := coalesce("a", "b"); got != "a" {
		t.Errorf("coalesce(a,b) = %q, want a", got)
	}
	if got := coalesce("", "b"); got != "b" {
		t.Errorf("coalesce('',b) = %q, want b", got)
	}
	if got := coalesce("", ""); got != "" {
		t.Errorf("coalesce('','') = %q, want empty", got)
	}
}

// ── inferFormat ───────────────────────────────────────────────────────────────

func TestInferFormat(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"image.png", "PNG"},
		{"image.PNG", "PNG"},
		{"icon.svg", "SVG"},
		{"photo.jpg", "JPG"},
		{"photo.jpeg", "JPG"},
		{"doc.pdf", "PDF"},
		{"noext", ""},
		{"file.txt", ""},
		{"file.gif", ""},
	}
	for _, c := range cases {
		if got := inferFormat(c.path); got != c.want {
			t.Errorf("inferFormat(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// ── resolveOutputPath ─────────────────────────────────────────────────────────

func TestResolveOutputPath_Relative(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveOutputPath("output/img.png", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "output", "img.png")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolveOutputPath_AbsoluteInsideDir(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "sub", "img.png")
	got, err := resolveOutputPath(abs, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != abs {
		t.Errorf("got %s, want %s", got, abs)
	}
}

func TestResolveOutputPath_Traversal_Blocked(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveOutputPath("../outside.png", dir)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestResolveOutputPath_AbsoluteOutsideDir_Blocked(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveOutputPath("/etc/passwd", dir)
	if err == nil {
		t.Error("expected error for absolute path outside working dir")
	}
}

// ── mustBeInsideDir ───────────────────────────────────────────────────────────

func TestMustBeInsideDir_Allowed(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "a", "b", "c.txt")
	got, err := mustBeInsideDir(inner, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != inner {
		t.Errorf("got %s, want %s", got, inner)
	}
}

func TestMustBeInsideDir_Blocked(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	_, err := mustBeInsideDir(filepath.Join(parent, "evil.txt"), dir)
	if err == nil {
		t.Error("expected error for path outside directory")
	}
}

func TestMustBeInsideDir_SameDir(t *testing.T) {
	dir := t.TempDir()
	_, err := mustBeInsideDir(filepath.Join(dir, "file.txt"), dir)
	if err != nil {
		t.Fatalf("file directly in workDir should be allowed: %v", err)
	}
}

// ── parseSaveItem ─────────────────────────────────────────────────────────────

func TestParseSaveItem_Valid(t *testing.T) {
	raw := map[string]any{
		"nodeId":     "1:1",
		"outputPath": "out/img.png",
		"format":     "PNG",
		"scale":      float64(2),
	}
	item, err := parseSaveItem(raw)
	if err != nil {
		t.Fatalf("parseSaveItem: %v", err)
	}
	if item.NodeID != "1:1" {
		t.Errorf("NodeID = %q, want 1:1", item.NodeID)
	}
	if item.OutputPath != "out/img.png" {
		t.Errorf("OutputPath = %q, want out/img.png", item.OutputPath)
	}
	if item.Scale != 2 {
		t.Errorf("Scale = %v, want 2", item.Scale)
	}
}

func TestParseSaveItem_UnmarshalError(t *testing.T) {
	// A channel cannot be marshaled to JSON.
	_, err := parseSaveItem(make(chan int))
	if err == nil {
		t.Error("expected marshal error for non-JSON-serialisable value")
	}
}

// ── extractScreenshotExport ───────────────────────────────────────────────────

func TestExtractScreenshotExport_Valid(t *testing.T) {
	data := map[string]any{
		"exports": []any{
			map[string]any{
				"nodeId":   "1:1",
				"nodeName": "Frame",
				"base64":   "abc123",
				"width":    float64(100),
				"height":   float64(200),
			},
		},
	}
	export, err := extractScreenshotExport(data)
	if err != nil {
		t.Fatalf("extractScreenshotExport: %v", err)
	}
	if export.NodeID != "1:1" {
		t.Errorf("NodeID = %q, want 1:1", export.NodeID)
	}
	if export.Width != 100 || export.Height != 200 {
		t.Errorf("dimensions = %vx%v, want 100x200", export.Width, export.Height)
	}
}

func TestExtractScreenshotExport_EmptyExports(t *testing.T) {
	data := map[string]any{"exports": []any{}}
	_, err := extractScreenshotExport(data)
	if err == nil {
		t.Error("expected error for empty exports array")
	}
}

func TestExtractScreenshotExport_MissingExports(t *testing.T) {
	_, err := extractScreenshotExport(map[string]any{})
	if err == nil {
		t.Error("expected error when exports key is missing")
	}
}

// ── writeBase64 ───────────────────────────────────────────────────────────────

func TestWriteBase64_WritesFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte("hello figma")
	b64 := base64.StdEncoding.EncodeToString(data)

	path := filepath.Join(dir, "out.png")
	n, err := writeBase64(b64, path)
	if err != nil {
		t.Fatalf("writeBase64: %v", err)
	}
	if n != len(data) {
		t.Errorf("bytes written = %d, want %d", n, len(data))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Error("file content does not match original data")
	}
}

func TestWriteBase64_CreatesIntermediateDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "out.png")
	b64 := base64.StdEncoding.EncodeToString([]byte("x"))

	if _, err := writeBase64(b64, path); err != nil {
		t.Fatalf("writeBase64 should create dirs: %v", err)
	}
}

func TestWriteBase64_RejectsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.png")
	b64 := base64.StdEncoding.EncodeToString([]byte("data"))

	// Create the file first.
	if _, err := writeBase64(b64, path); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Second write must fail.
	_, err := writeBase64(b64, path)
	if err == nil {
		t.Error("expected error when file already exists")
	}
}

func TestWriteBase64_InvalidBase64(t *testing.T) {
	dir := t.TempDir()
	_, err := writeBase64("not-valid-base64!!!", filepath.Join(dir, "out.png"))
	if err == nil {
		t.Error("expected error for invalid base64 input")
	}
}

// ── CRITICAL #1: renderResponse must not mutate the shared Data map ───────────
//
// When N goroutines call renderResponse concurrently on a BridgeResponse whose
// Data map is SHARED (as happens when singleflight followers all receive the
// same fl.resp, or when the cache returns the same item.resp), the current
// in-place mutation of data["queueWaitMs"] / data["queueDepth"] triggers the
// Go runtime's "fatal: concurrent map writes".  Under -race the detector fires
// before the fatal, which shows up as a test failure.
//
// RED: this test MUST fail (race or panic) on pre-fix code where renderResponse
// mutates resp.Data in place.
// GREEN: after the fix (shallow-copy the map), it must pass cleanly.
func TestRenderResponse_ConcurrentSharedDataMapNoRace(t *testing.T) {
	// Build one Data map that is shared across all goroutines — exactly the
	// aliasing that happens via singleflight fl.resp / cache item.resp.
	sharedData := map[string]interface{}{
		"id":   "1:1",
		"name": "Frame",
	}
	resp := BridgeResponse{
		Data:        sharedData,
		QueueWaitMs: 50, // > 0 so the injection branch is taken
		QueueDepth:  2,
	}

	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			// Each goroutine renders the SAME resp (shared Data map).
			// We don't inspect the content — the -race detector or a concurrent
			// map-write fatal is what demonstrates RED on the pre-fix code.
			_, _ = renderResponse(resp, nil)
		}()
	}
	wg.Wait()

	// After all goroutines finish, the original shared map must NOT have been
	// mutated — the fix must copy before injecting.
	if _, present := sharedData["queueWaitMs"]; present {
		t.Error("renderResponse must not inject queueWaitMs into the original shared Data map")
	}
	if _, present := sharedData["queueDepth"]; present {
		t.Error("renderResponse must not inject queueDepth into the original shared Data map")
	}
}

// TestRenderResponse_QueueFieldsInjectedIntoRenderedOutput verifies that when
// QueueWaitMs > 0 the rendered JSON carries the queue fields, and that a
// non-queued response does NOT carry them.  This is a correctness companion to
// the race test above (which only confirms the original map is not mutated).
func TestRenderResponse_QueueFieldsInjectedIntoRenderedOutput(t *testing.T) {
	sharedData := map[string]interface{}{"id": "1:1"}
	resp := BridgeResponse{
		Data:        sharedData,
		QueueWaitMs: 75,
		QueueDepth:  3,
	}
	result, err := renderResponse(resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	// Decode the rendered text to verify injection happened.
	raw := extractResultText(result)
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal rendered output: %v", err)
	}
	if _, ok := out["queueWaitMs"]; !ok {
		t.Error("rendered output must contain queueWaitMs when QueueWaitMs > 0")
	}
	if _, ok := out["queueDepth"]; !ok {
		t.Error("rendered output must contain queueDepth when QueueWaitMs > 0")
	}

	// Non-queued path: QueueWaitMs == 0 → no injection.
	resp2 := BridgeResponse{Data: map[string]interface{}{"id": "2:2"}}
	result2, _ := renderResponse(resp2, nil)
	raw2 := extractResultText(result2)
	var out2 map[string]interface{}
	if err := json.Unmarshal([]byte(raw2), &out2); err != nil {
		t.Fatalf("unmarshal non-queued output: %v", err)
	}
	if _, ok := out2["queueWaitMs"]; ok {
		t.Error("non-queued response must NOT carry queueWaitMs")
	}
}
