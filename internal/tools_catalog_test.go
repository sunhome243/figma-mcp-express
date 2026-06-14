package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// ── fake fetcher ──────────────────────────────────────────────────────────────

// fakeFetcherFunc is a catalogFetcher that returns canned data without network.
func fakeFetcherFunc(ctx context.Context, fileKey, scope string) (map[string]any, error) {
	return map[string]any{
		"components": []any{
			map[string]any{"key": "comp1key", "name": "Button", "node_id": "10:1"},
			map[string]any{"key": "comp2key", "name": "Card", "node_id": "10:2"},
			map[string]any{"key": "comp3key", "name": "Input", "node_id": "10:3"},
			map[string]any{"key": "comp4key", "name": "Table", "node_id": "10:4"},
		},
		"component_sets": []any{
			map[string]any{"key": "cs1key", "name": "ButtonSet", "node_id": "20:1"},
			map[string]any{"key": "cs2key", "name": "CardSet", "node_id": "20:2"},
			map[string]any{"key": "cs3key", "name": "InputSet", "node_id": "20:3"},
			map[string]any{"key": "cs4key", "name": "TableSet", "node_id": "20:4"},
		},
		"styles": []any{
			map[string]any{"key": "st1key", "name": "Primary", "node_id": "30:1"},
		},
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// getResultText extracts the text from the first TextContent in a CallToolResult.
func getResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// catalogHandleResult mirrors the small handle returned by fetch_library_catalog.
type catalogHandleResult struct {
	OutPath string `json:"outPath"`
	Counts  struct {
		Components    int `json:"components"`
		ComponentSets int `json:"componentSets"`
		Styles        int `json:"styles"`
	} `json:"counts"`
	Sample []map[string]any `json:"sample"`
}

// runCatalog invokes executeFetchCatalog with the fake fetcher and returns the
// parsed handle. Fails the test if there is a Go error or IsError result.
func runCatalog(t *testing.T, fileKey, outPath, workDir string) (*catalogHandleResult, string) {
	t.Helper()
	result, err := executeFetchCatalog(context.Background(), fakeFetcherFunc, fileKey, "all", outPath, workDir)
	if err != nil {
		t.Fatalf("executeFetchCatalog Go error: %v", err)
	}
	if result.IsError {
		text := getResultText(t, result)
		t.Fatalf("executeFetchCatalog returned IsError=true: %s", text)
	}
	raw := getResultText(t, result)
	var handle catalogHandleResult
	if err := json.Unmarshal([]byte(raw), &handle); err != nil {
		t.Fatalf("handle JSON unmarshal: %v\nraw: %s", err, raw)
	}
	return &handle, raw
}

// ── handler unit tests ────────────────────────────────────────────────────────

// TestFetchCatalog_WritesFullCatalogToFile asserts that executeFetchCatalog
// writes the full arrays (components, component_sets, styles) to outPath.
func TestFetchCatalog_WritesFullCatalogToFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "catalog.json")

	handle, _ := runCatalog(t, "FKEY123", outPath, dir)
	_ = handle // checked below

	// File must exist and contain the full arrays.
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var catalogFile map[string]any
	if err := json.Unmarshal(raw, &catalogFile); err != nil {
		t.Fatalf("catalog file is not valid JSON: %v", err)
	}

	comps, _ := catalogFile["components"].([]any)
	if len(comps) != 4 {
		t.Errorf("components count = %d, want 4", len(comps))
	}
	sets, _ := catalogFile["component_sets"].([]any)
	if len(sets) != 4 {
		t.Errorf("component_sets count = %d, want 4", len(sets))
	}
	styles, _ := catalogFile["styles"].([]any)
	if len(styles) != 1 {
		t.Errorf("styles count = %d, want 1", len(styles))
	}

	// Verify byKey map is present.
	byKey, ok := catalogFile["byKey"].(map[string]any)
	if !ok || len(byKey) == 0 {
		t.Error("byKey map must be present and non-empty in the written file")
	}
	// Spot-check known keys.
	for _, k := range []string{"comp1key", "cs1key", "st1key"} {
		if _, ok := byKey[k]; !ok {
			t.Errorf("byKey must contain %q", k)
		}
	}
}

func TestFetchCatalog_RemembersComponentSetHintsForImportRouting(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "catalog.json")

	runCatalog(t, "FKEY123", outPath, dir)

	if assetType, ok := lookupLibraryCatalogAssetType("comp1key"); ok || assetType != "" {
		t.Fatalf("component key should not be cached for import routing, got %q, %v", assetType, ok)
	}
	if assetType, ok := lookupLibraryCatalogAssetType("cs1key"); !ok || assetType != "COMPONENT_SET" {
		t.Fatalf("component set assetType = %q, %v; want COMPONENT_SET, true", assetType, ok)
	}
	if assetType, ok := lookupLibraryCatalogAssetType("st1key"); ok || assetType != "" {
		t.Fatalf("style key should not be cached for import routing, got %q, %v", assetType, ok)
	}
}

// TestFetchCatalog_HandleContainsCountsAndSample verifies counts+sample+outPath.
func TestFetchCatalog_HandleContainsCountsAndSample(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "catalog.json")

	handle, _ := runCatalog(t, "FKEY123", outPath, dir)

	if handle.Counts.Components != 4 {
		t.Errorf("counts.components = %d, want 4", handle.Counts.Components)
	}
	if handle.Counts.ComponentSets != 4 {
		t.Errorf("counts.componentSets = %d, want 4", handle.Counts.ComponentSets)
	}
	if handle.Counts.Styles != 1 {
		t.Errorf("counts.styles = %d, want 1", handle.Counts.Styles)
	}

	// Sample: first 3 component_sets (name+key).
	if len(handle.Sample) != 3 {
		t.Errorf("sample length = %d, want 3", len(handle.Sample))
	}
	if len(handle.Sample) > 0 {
		first := handle.Sample[0]
		if _, ok := first["name"]; !ok {
			t.Error("sample[0] must have 'name'")
		}
		if _, ok := first["key"]; !ok {
			t.Error("sample[0] must have 'key'")
		}
	}

	if handle.OutPath != outPath {
		t.Errorf("handle.outPath = %q, want %q", handle.OutPath, outPath)
	}
}

// TestFetchCatalog_HandleDoesNotContainFullArrays verifies the handle text does
// not contain component data beyond the sample (comp4key is the 4th component,
// not included in the sample of first-3 component_sets).
func TestFetchCatalog_HandleDoesNotContainFullArrays(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "catalog.json")

	_, raw := runCatalog(t, "FKEY123", outPath, dir)

	// comp4key is from components (not component_sets), so it should not be in the handle.
	if strings.Contains(raw, "comp4key") {
		t.Error("handle must not contain full component array keys")
	}
	// st1key is from styles, not in sample.
	if strings.Contains(raw, "st1key") {
		t.Error("handle must not contain style keys inline")
	}
}

// TestFetchCatalog_PathContainment ensures outPath must be inside workDir.
func TestFetchCatalog_PathContainment(t *testing.T) {
	dir := t.TempDir()
	result, err := executeFetchCatalog(
		context.Background(), fakeFetcherFunc,
		"KEY", "all", "/etc/passwd", dir,
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for path outside workDir")
	}
}

// ── fileKey live-path validation ──────────────────────────────────────────────

// fetcherMustNotBeCalled is a catalogFetcher that fails the test if invoked.
func fetcherMustNotBeCalled(t *testing.T) catalogFetcher {
	t.Helper()
	return func(_ context.Context, _, _ string) (map[string]any, error) {
		t.Error("fetcher must NOT be called for an invalid fileKey")
		return nil, nil
	}
}

// TestFetchCatalog_InvalidFileKey_PathTraversal verifies that a fileKey with
// path-traversal characters is rejected before the fetcher is invoked.
func TestFetchCatalog_InvalidFileKey_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	result, err := executeFetchCatalog(
		context.Background(),
		fetcherMustNotBeCalled(t),
		"a/../b", "all", "catalog.json", dir,
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for fileKey with path-traversal characters")
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "fileKey must be alphanumeric") {
		t.Errorf("error message should mention fileKey validation, got: %s", text)
	}
}

// TestFetchCatalog_InvalidFileKey_QueryString verifies that a fileKey with a
// query-string injection character is rejected before the fetcher is invoked.
func TestFetchCatalog_InvalidFileKey_QueryString(t *testing.T) {
	dir := t.TempDir()
	result, err := executeFetchCatalog(
		context.Background(),
		fetcherMustNotBeCalled(t),
		"KEY?x=", "all", "catalog.json", dir,
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for fileKey with query-string injection characters")
	}
}

// TestFetchCatalog_ValidFileKey_ProceedsToFetcher verifies that a valid
// 22-char alphanumeric key reaches the fetcher without being rejected.
func TestFetchCatalog_ValidFileKey_ProceedsToFetcher(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "catalog.json")

	called := false
	fetcher := func(ctx context.Context, fileKey, scope string) (map[string]any, error) {
		called = true
		return fakeFetcherFunc(ctx, fileKey, scope)
	}

	result, err := executeFetchCatalog(context.Background(), fetcher, "ABCDEFGHIJKLMNOPQRSTUVwx", "all", outPath, dir)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success for valid fileKey, got: %s", getResultText(t, result))
	}
	if !called {
		t.Error("expected fetcher to be called for valid fileKey")
	}
}

// ── schema validation tests ───────────────────────────────────────────────────

func TestFetchCatalog_MissingFileKey(t *testing.T) {
	msg := ValidateRPC("fetch_library_catalog", nil, map[string]any{
		"outPath": "catalog.json",
	})
	if msg == "" {
		t.Error("expected validation error for missing fileKey")
	}
	if !strings.Contains(strings.ToLower(msg), "filekey") {
		t.Errorf("error should mention fileKey, got: %s", msg)
	}
}

func TestFetchCatalog_MissingOutPath(t *testing.T) {
	msg := ValidateRPC("fetch_library_catalog", nil, map[string]any{
		"fileKey": "SOMEKEY",
	})
	if msg == "" {
		t.Error("expected validation error for missing outPath")
	}
	if !strings.Contains(strings.ToLower(msg), "outpath") {
		t.Errorf("error should mention outPath, got: %s", msg)
	}
}

func TestFetchCatalog_ValidSchema(t *testing.T) {
	msg := ValidateRPC("fetch_library_catalog", nil, map[string]any{
		"fileKey": "SOMEKEY",
		"outPath": "catalog.json",
	})
	if msg != "" {
		t.Errorf("unexpected validation error: %s", msg)
	}
}

// TestFetchCatalog_ToolRegistered verifies the tool appears in the MCP server.
func TestFetchCatalog_ToolRegistered(t *testing.T) {
	resp := listTools(t)
	found := false
	for _, tool := range resp.Result.Tools {
		if tool.Name == "fetch_library_catalog" {
			found = true
			break
		}
	}
	if !found {
		t.Error("fetch_library_catalog must be registered in the MCP server")
	}
}

// ── catalog file permissions (Part 1.3) ──────────────────────────────────────

// TestFetchCatalog_CatalogFilePerms asserts the written catalog JSON is 0o600.
func TestFetchCatalog_CatalogFilePerms(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "catalog.json")

	_, err := executeFetchCatalog(context.Background(), fakeFetcherFunc, "FKEY123", "all", outPath, dir)
	if err != nil {
		t.Fatalf("executeFetchCatalog Go error: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat catalog file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("catalog file perm = %04o, want 0600", got)
	}
}

// ── NDJSON sidecar projection (Layer 4A) ────────────────────────────────────────

func TestBuildCatalogNDJSON(t *testing.T) {
	byKey := map[string]any{
		"kB": map[string]any{"name": "Button", "type": "COMPONENT_SET", "nodeId": "1:2"},
		"kA": map[string]any{"name": "Dropdown", "type": "COMPONENT_SET", "nodeId": "3:4"},
	}
	out, err := buildCatalogNDJSON(byKey)
	if err != nil {
		t.Fatalf("buildCatalogNDJSON: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d", len(lines))
	}
	// Key-sorted: kA before kB.
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 0 is not valid JSON: %v", err)
	}
	if first["key"] != "kA" || first["name"] != "Dropdown" || first["nodeId"] != "3:4" {
		t.Errorf("first record wrong: %v", first)
	}
	// Each line must be a complete, independently-parseable record (the whole point).
	for i, ln := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Errorf("line %d not self-contained JSON: %v", i, err)
		}
	}
}
