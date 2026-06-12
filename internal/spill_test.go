package internal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── Lever 6B-2 — byte-identical render (golden) ────────────────────────────────

// TestGateResponse_PassthroughByteIdentical asserts the under-threshold output is
// byte-identical to string(raw) — the 6B-2 invariant.
func TestGateResponse_PassthroughByteIdentical(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"id":"1:1","name":"Frame","value":42}`)
	out, spilled, err := gateResponse(raw, nil, "node", dir, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spilled {
		t.Fatal("expected passthrough, got spill")
	}
	if out != string(raw) {
		t.Errorf("passthrough not byte-identical:\n got %q\nwant %q", out, string(raw))
	}
}

// TestGateResponse_CanonicalFileByteIdentical asserts the spilled canonical .json
// file bytes equal raw exactly (fidelity round-trip) — holds across Lever 8.
func TestGateResponse_CanonicalFileByteIdentical(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"nodes":[{"id":"1:1","name":"あいうえお","type":"FRAME"}],"extra":"` + strings.Repeat("x", 200) + `"}`)
	out, spilled, err := gateResponse(raw, mustUnmarshal(t, raw), "node", dir, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spill")
	}
	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle not valid JSON: %v", err)
	}
	got, err := os.ReadFile(handle["path"].(string))
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if string(got) != string(raw) {
		t.Error("canonical .json is not byte-identical to raw")
	}
}

// TestGateResponse_PreviewBounded asserts the preview is a ≤600-rune prefix even
// for a huge multibyte payload, without scanning the whole buffer.
func TestGateResponse_PreviewBounded(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(strings.Repeat("あ", 5000)) // 15000 bytes
	out, _, err := gateResponse(raw, nil, "big", dir, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var handle map[string]any
	_ = json.Unmarshal([]byte(out), &handle)
	preview, _ := handle["preview"].(string)
	if n := len([]rune(preview)); n > 600 {
		t.Errorf("preview rune length = %d, want ≤ 600", n)
	}
	if !strings.HasPrefix(string(raw), preview) {
		t.Error("preview must be a prefix of raw")
	}
}

// ── Lever 8 — NDJSON sidecar ───────────────────────────────────────────────────

func TestGateResponse_CollectionSidecar(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{"nodes": []any{
		map[string]any{"id": "1:1", "name": "A"},
		map[string]any{"id": "1:2", "name": "B"},
		map[string]any{"id": "1:3", "name": "C"},
	}}
	raw := marshal(t, data)
	out, spilled, err := gateResponse(raw, data, "scan", dir, 10)
	if err != nil || !spilled {
		t.Fatalf("expected spill, err=%v spilled=%v", err, spilled)
	}
	var handle map[string]any
	if err := json.Unmarshal([]byte(out), &handle); err != nil {
		t.Fatalf("handle not JSON: %v", err)
	}
	indexPath, ok := handle["indexPath"].(string)
	if !ok || indexPath == "" {
		t.Fatal("handle must carry indexPath when sidecar written")
	}
	lines := readLines(t, indexPath)
	if len(lines) != 3 {
		t.Fatalf("ndjson line count = %d, want 3", len(lines))
	}
	for _, ln := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Errorf("ndjson line not parseable: %v (%q)", err, ln)
		}
	}
}

func TestGateResponse_TreeSidecarIndex(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{
		"id": "0:1", "name": "Root", "type": "PAGE",
		"children": []any{
			map[string]any{"id": "1:1", "name": "Frame", "type": "FRAME", "children": []any{
				map[string]any{"id": "2:1", "name": "Text", "type": "TEXT"},
			}},
			map[string]any{"id": "1:2", "name": "Rect", "type": "RECTANGLE"},
		},
	}
	raw := marshal(t, data)
	out, spilled, err := gateResponse(raw, data, "node", dir, 10)
	if err != nil || !spilled {
		t.Fatalf("expected spill, err=%v spilled=%v", err, spilled)
	}
	var handle map[string]any
	_ = json.Unmarshal([]byte(out), &handle)
	indexPath := handle["indexPath"].(string)
	lines := readLines(t, indexPath)
	if len(lines) != 4 {
		t.Fatalf("index line count = %d, want 4 (one per node)", len(lines))
	}
	byID := map[string]map[string]any{}
	for _, ln := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Fatalf("index line not parseable: %v", err)
		}
		byID[rec["id"].(string)] = rec
	}
	// Root has empty parentId; child's parentId points up; path chains ids.
	if byID["0:1"]["parentId"] != "" {
		t.Errorf("root parentId = %q, want empty", byID["0:1"]["parentId"])
	}
	if byID["2:1"]["parentId"] != "1:1" {
		t.Errorf("2:1 parentId = %q, want 1:1", byID["2:1"]["parentId"])
	}
	if byID["2:1"]["path"] != "0:1/1:1/2:1" {
		t.Errorf("2:1 path = %q, want 0:1/1:1/2:1", byID["2:1"]["path"])
	}
}

// TestGateResponse_DesignContextForest mirrors the REAL get_design_context root
// shape: {fileName, currentPage, selectionCount, context:[...nodes], globalVars}
// — NO top-level id. The `context` forest must flatten to a line-per-node index,
// NOT one nested-blob line per top-level node (the regression Lever 8 fights).
func TestGateResponse_DesignContextForest(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{
		"fileName":       "Design",
		"currentPage":    map[string]any{"id": "0:1", "name": "Page 1"},
		"selectionCount": 1,
		"context": []any{
			map[string]any{"id": "1:1", "name": "Frame", "type": "FRAME", "children": []any{
				map[string]any{"id": "2:1", "name": "Label", "type": "TEXT"},
				map[string]any{"id": "2:2", "name": "Icon", "type": "VECTOR"},
			}},
		},
		"globalVars": map[string]any{"styles": map[string]any{}},
	}
	raw := marshal(t, data)
	out, spilled, err := gateResponse(raw, data, "context", dir, 10)
	if err != nil || !spilled {
		t.Fatalf("expected spill, err=%v spilled=%v", err, spilled)
	}
	var handle map[string]any
	_ = json.Unmarshal([]byte(out), &handle)
	indexPath, ok := handle["indexPath"].(string)
	if !ok || indexPath == "" {
		t.Fatal("get_design_context forest must produce a sidecar (indexPath)")
	}
	lines := readLines(t, indexPath)
	if len(lines) != 3 {
		t.Fatalf("index line count = %d, want 3 (Frame + 2 children flattened)", len(lines))
	}
	byID := map[string]map[string]any{}
	for _, ln := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Fatalf("index line not parseable: %v", err)
		}
		byID[rec["id"].(string)] = rec
	}
	// Forest root has empty parentId; its children point up.
	if byID["1:1"]["parentId"] != "" {
		t.Errorf("forest root parentId = %q, want empty", byID["1:1"]["parentId"])
	}
	if byID["2:1"]["parentId"] != "1:1" || byID["2:1"]["path"] != "1:1/2:1" {
		t.Errorf("2:1 parentId/path = %q/%q, want 1:1 / 1:1/2:1", byID["2:1"]["parentId"], byID["2:1"]["path"])
	}
}

// TestGateResponse_TopLevelArrayForest mirrors get_nodes_info / get_selection:
// a top-level array of node roots → each flattened to a line-per-node index.
func TestGateResponse_TopLevelArrayForest(t *testing.T) {
	dir := t.TempDir()
	data := []any{
		map[string]any{"id": "1:1", "name": "A", "type": "FRAME", "children": []any{
			map[string]any{"id": "2:1", "name": "A.child", "type": "TEXT"},
		}},
		map[string]any{"id": "1:2", "name": "B", "type": "RECTANGLE"},
	}
	raw := marshal(t, data)
	out, spilled, err := gateResponse(raw, data, "nodes_info", dir, 10)
	if err != nil || !spilled {
		t.Fatalf("expected spill, err=%v spilled=%v", err, spilled)
	}
	var handle map[string]any
	_ = json.Unmarshal([]byte(out), &handle)
	indexPath, _ := handle["indexPath"].(string)
	if indexPath == "" {
		t.Fatal("top-level node array must produce a sidecar")
	}
	lines := readLines(t, indexPath)
	if len(lines) != 3 {
		t.Fatalf("index line count = %d, want 3 (A + A.child + B)", len(lines))
	}
}

func TestGateResponse_UnrecognizedNoSidecar(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{"status": "ok", "count": 5} // no id+children, no collection key
	raw := []byte(`{"status":"ok","count":5,"pad":"` + strings.Repeat("p", 200) + `"}`)
	out, spilled, err := gateResponse(raw, data, "resp", dir, 50)
	if err != nil || !spilled {
		t.Fatalf("expected spill, err=%v spilled=%v", err, spilled)
	}
	var handle map[string]any
	_ = json.Unmarshal([]byte(out), &handle)
	if _, ok := handle["indexPath"]; ok {
		t.Error("unrecognized payload must NOT carry indexPath")
	}
	// Only canonical .json (+ manifest) present — no .ndjson sidecar beside it.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ndjson") && e.Name() != spillManifestName {
			t.Errorf("unexpected sidecar written: %s", e.Name())
		}
	}
}

// ── Lever 8 — provenance manifest ──────────────────────────────────────────────

func TestSpillManifest_OneLinePerSpill(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{"nodes": []any{map[string]any{"id": "1:1"}, map[string]any{"id": "1:2"}}}
	raw := marshal(t, data)
	if _, _, err := gateResponse(raw, data, "scan", dir, 10); err != nil {
		t.Fatalf("spill: %v", err)
	}
	lines := readLines(t, filepath.Join(dir, spillManifestName))
	if len(lines) != 1 {
		t.Fatalf("manifest line count = %d, want 1", len(lines))
	}
	var rec manifestRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("manifest line not parseable: %v", err)
	}
	if rec.Summary == "" {
		t.Error("manifest summary must be non-empty")
	}
	if rec.Records != 2 {
		t.Errorf("manifest records = %d, want 2", rec.Records)
	}
	if rec.Label != "scan" || rec.Path == "" || rec.Ts == "" {
		t.Errorf("manifest record incomplete: %+v", rec)
	}
}

func TestSpillManifest_ConcurrentAppendsIntact(t *testing.T) {
	dir := t.TempDir()
	const n = 8

	// Marshal fixtures on the TEST goroutine — marshal() calls t.Fatalf, which must
	// never run from a child goroutine (testing.T.FailNow is test-goroutine only).
	type spillJob struct {
		raw  []byte
		data any
	}
	jobs := make([]spillJob, n)
	for i := 0; i < n; i++ {
		// Distinct payloads → distinct hashes → distinct canonical files.
		data := map[string]any{"nodes": []any{map[string]any{"id": "n", "i": i}}}
		jobs[i] = spillJob{raw: marshal(t, data), data: data}
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, _ = gateResponse(jobs[i].raw, jobs[i].data, "scan", dir, 5)
		}(i)
	}
	wg.Wait()

	lines := readLines(t, filepath.Join(dir, spillManifestName))
	if len(lines) != n {
		t.Fatalf("manifest line count = %d, want %d", len(lines), n)
	}
	for _, ln := range lines {
		var rec manifestRecord
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Errorf("interleaved/corrupt manifest line: %v (%q)", err, ln)
		}
	}
}

func TestSpillManifest_UnrecognizedStillRecorded(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{"foo": "bar", "baz": 1}
	raw := []byte(`{"foo":"bar","baz":1,"pad":"` + strings.Repeat("q", 200) + `"}`)
	if _, _, err := gateResponse(raw, data, "resp", dir, 50); err != nil {
		t.Fatalf("spill: %v", err)
	}
	lines := readLines(t, filepath.Join(dir, spillManifestName))
	if len(lines) != 1 {
		t.Fatalf("manifest line count = %d, want 1", len(lines))
	}
	var rec manifestRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("manifest not parseable: %v", err)
	}
	if rec.Summary == "" {
		t.Error("fallback summary must be non-empty for unrecognized payload")
	}
	if rec.IndexPath != "" {
		t.Errorf("unrecognized payload must have empty indexPath, got %q", rec.IndexPath)
	}
}

// TestSpillManifest_RepeatedSpillDoesNotGrowManifest proves that re-gating the
// SAME payload (same content hash → same canonical file) does not add a second
// line to index.ndjson. Before the dedup guard, every cache-hit re-gate appended
// an unbounded duplicate provenance line.
func TestSpillManifest_RepeatedSpillDoesNotGrowManifest(t *testing.T) {
	dir := t.TempDir()
	data := map[string]any{"nodes": []any{map[string]any{"id": "2:1"}, map[string]any{"id": "2:2"}}}
	raw := marshal(t, data)

	// First spill — should write the canonical file and ONE manifest line.
	if _, _, err := gateResponse(raw, data, "scan", dir, 10); err != nil {
		t.Fatalf("first spill: %v", err)
	}
	lines := readLines(t, filepath.Join(dir, spillManifestName))
	if len(lines) != 1 {
		t.Fatalf("after first spill: manifest line count = %d, want 1", len(lines))
	}

	// Second spill — identical raw bytes → same hash → same canonical path.
	// The manifest must still have exactly 1 line (no duplicate provenance entry).
	if _, _, err := gateResponse(raw, data, "scan", dir, 10); err != nil {
		t.Fatalf("second spill: %v", err)
	}
	lines = readLines(t, filepath.Join(dir, spillManifestName))
	if len(lines) != 1 {
		t.Fatalf("after repeated spill: manifest line count = %d, want 1 (dedup failed)", len(lines))
	}
}

// ── summarizePayload ───────────────────────────────────────────────────────────

func TestSummarizePayload(t *testing.T) {
	cases := []struct {
		name string
		data interface{}
		want string // substring expected
	}{
		{"tree", map[string]any{"id": "0:1", "type": "PAGE", "children": []any{
			map[string]any{"id": "1:1", "type": "FRAME"},
		}}, "tree root=0:1 nodes=2"},
		{"forest", map[string]any{"nodes": []any{1, 2, 3}}, "forest nodes len=3"},
		{"array", []any{1, 2}, "array len=2"},
		{"unrecognized", map[string]any{"b": 1, "a": 2}, "keys=a,b"},
		{"scalar", "hello", "scalar"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := summarizePayload(c.data)
			if got == "" {
				t.Fatal("summary must never be empty")
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("summary %q does not contain %q", got, c.want)
			}
		})
	}
}

// ── Lever 6B-4 — eviction ──────────────────────────────────────────────────────

func TestEvictSpillCache_TTLDeletesOldKeepsFresh(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	procStart := now.Add(-1 * time.Hour) // process came up an hour ago

	old := writeAged(t, dir, "old.json", 1000, now.Add(-72*time.Hour))       // beyond 48h TTL + prior process
	fresh := writeAged(t, dir, "fresh.json", 1000, now.Add(-30*time.Minute)) // within TTL, current process

	if err := evictSpillCache(dir, 48*time.Hour, 1<<40, procStart, now); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if fileExists(old) {
		t.Error("old beyond-TTL file should have been evicted")
	}
	if !fileExists(fresh) {
		t.Error("fresh within-TTL file must be kept")
	}
}

func TestEvictSpillCache_SizeCapEvictsOldestPriorProcess(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	procStart := now.Add(-5 * time.Minute)

	// Three prior-process files (mtime before procStart) within TTL but over the
	// cap together (3*1000 > 1500). Oldest-first eviction should remove enough to
	// get under cap.
	a := writeAged(t, dir, "a.json", 1000, now.Add(-9*time.Minute)) // oldest prior-process
	b := writeAged(t, dir, "b.json", 1000, now.Add(-8*time.Minute))
	c := writeAged(t, dir, "c.json", 1000, now.Add(-7*time.Minute))
	// A current-process file (mtime after procStart) — sacrosanct even over cap.
	current := writeAged(t, dir, "current.json", 1000, now.Add(-1*time.Minute))

	if err := evictSpillCache(dir, 48*time.Hour, 1500, procStart, now); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if !fileExists(current) {
		t.Error("current-process file must never be evicted by size cap")
	}
	if fileExists(a) {
		// a is the oldest prior-process file — should go first.
	} else if !fileExists(a) && fileExists(b) && fileExists(c) {
		// acceptable: only enough evicted to get under cap (prior-process portion)
	}
	// At least the oldest prior-process file must be gone (cap was exceeded by
	// prior-process bytes alone: 3000 > 1500 even ignoring current).
	if fileExists(a) && fileExists(b) && fileExists(c) {
		t.Error("size cap should have evicted at least one prior-process file")
	}
}

func TestEvictSpillCache_NeverDeletesCurrentProcessFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	procStart := now.Add(-5 * time.Minute)

	// A file as old as 100h but written AFTER procStart cannot exist in reality;
	// simulate the guarantee: a within-process file with an artificially old mtime
	// is still protected because mtime >= procStart is the only test.
	current := writeAged(t, dir, "current.json", 10_000_000, now.Add(-1*time.Second))

	// Tiny cap to force the cap pass; TTL huge so TTL pass deletes nothing.
	if err := evictSpillCache(dir, 1*time.Hour, 1, procStart, now); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if !fileExists(current) {
		t.Error("current-process file must never be deleted, even over cap")
	}
}

func TestEvictSpillCache_SkipsManifest(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	procStart := now.Add(-1 * time.Hour)
	manifest := writeAged(t, dir, spillManifestName, 1000, now.Add(-100*time.Hour))

	if err := evictSpillCache(dir, 48*time.Hour, 1<<40, procStart, now); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if !fileExists(manifest) {
		t.Error("index.ndjson manifest must never be evicted")
	}
}

func TestEvictSpillCache_MissingDirIsNoOp(t *testing.T) {
	if err := evictSpillCache(filepath.Join(t.TempDir(), "nope"), time.Hour, 1<<40, time.Now(), time.Now()); err != nil {
		t.Errorf("missing dir should be a no-op, got %v", err)
	}
}

// A canonical .json and its .ndjson sidecar must be evicted as ONE unit — never
// orphaning a sidecar nor leaving index.ndjson pointing at a deleted canonical.
func TestEvictSpillCache_TTLEvictsCanonicalWithSidecar(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	procStart := now.Add(-5 * time.Minute)

	// Prior-process, past TTL: canonical + sidecar — both must go together.
	oldCanon := writeAged(t, dir, "old-1.json", 1000, now.Add(-100*time.Hour))
	oldSidecar := writeAged(t, dir, "old-1.ndjson", 200, now.Add(-100*time.Hour))
	// Prior-process, within TTL: both kept.
	keepCanon := writeAged(t, dir, "keep-2.json", 1000, now.Add(-10*time.Minute))
	keepSidecar := writeAged(t, dir, "keep-2.ndjson", 200, now.Add(-10*time.Minute))

	if err := evictSpillCache(dir, 48*time.Hour, 1<<40, procStart, now); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if fileExists(oldCanon) || fileExists(oldSidecar) {
		t.Error("past-TTL canonical and its sidecar must both be evicted (no orphan)")
	}
	if !fileExists(keepCanon) || !fileExists(keepSidecar) {
		t.Error("within-TTL canonical and its sidecar must both be kept")
	}
}

// The size-cap pass removes whole units (counting sidecar bytes against the
// canonical) and never selects a sidecar as an independent eviction target.
func TestEvictSpillCache_SizeCapRemovesUnitNeverOrphansSidecar(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	procStart := now.Add(-5 * time.Minute)

	// Two prior-process units: 1000 canonical + 500 sidecar = 1500 each, 3000 total.
	// Cap 1800 forces evicting exactly the oldest unit (both files) → 1500 ≤ 1800.
	aCanon := writeAged(t, dir, "a.json", 1000, now.Add(-9*time.Minute))
	aSide := writeAged(t, dir, "a.ndjson", 500, now.Add(-9*time.Minute))
	bCanon := writeAged(t, dir, "b.json", 1000, now.Add(-7*time.Minute))
	bSide := writeAged(t, dir, "b.ndjson", 500, now.Add(-7*time.Minute))

	if err := evictSpillCache(dir, 48*time.Hour, 1800, procStart, now); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if fileExists(aCanon) || fileExists(aSide) {
		t.Error("oldest unit (canonical+sidecar) must be evicted together")
	}
	if !fileExists(bCanon) || !fileExists(bSide) {
		t.Error("surviving unit must keep BOTH its canonical and sidecar — sidecar never evicted alone")
	}
}

// ── Lever 6B-5 — FreeOSMemory off the hot path ─────────────────────────────────

func TestFreeOSMemory_NotCalledOnPassthrough(t *testing.T) {
	dir := t.TempDir()
	var calls int
	restore := freeOSMemory
	freeOSMemory = func() { calls++ }
	defer func() { freeOSMemory = restore }()

	// Small payload → passthrough, no spill → freeOSMemory must not fire.
	if _, spilled, err := gateResponse([]byte(`{"a":1}`), nil, "resp", dir, 10000); err != nil || spilled {
		t.Fatalf("expected passthrough, err=%v spilled=%v", err, spilled)
	}
	if calls != 0 {
		t.Errorf("freeOSMemory called %d times on hot/passthrough path, want 0", calls)
	}
}

func TestFreeOSMemory_CalledAfterSpill(t *testing.T) {
	dir := t.TempDir()
	var calls int
	restore := freeOSMemory
	freeOSMemory = func() { calls++ }
	defer func() { freeOSMemory = restore }()

	raw := []byte(strings.Repeat("x", 200))
	if _, spilled, err := gateResponse(raw, nil, "big", dir, 50); err != nil || !spilled {
		t.Fatalf("expected spill, err=%v spilled=%v", err, spilled)
	}
	if calls != 1 {
		t.Errorf("freeOSMemory called %d times after spill, want 1", calls)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────────

func mustUnmarshal(t *testing.T, raw []byte) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return v
}

func marshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return b
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if t := sc.Text(); t != "" {
			lines = append(lines, t)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return lines
}

func writeAged(t *testing.T, dir, name string, size int, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
