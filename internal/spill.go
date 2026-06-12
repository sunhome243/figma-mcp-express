package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Lever 6B-5 — opportunistic OS-memory release (OFF the hot path) ────────────

// freeOSMemory is indirected through a package var so it is (a) trivially
// swappable in tests and (b) provably never invoked on the hot/passthrough read
// path — only after a large spill. FreeOSMemory forces a STW GC, so it must never
// run per-read. Default is a no-op; opt in via FIGMA_MCP_FREE_OS_MEM (any
// non-empty value) so the STW cost is paid only when explicitly requested.
var freeOSMemory = func() {
	if os.Getenv("FIGMA_MCP_FREE_OS_MEM") != "" {
		debug.FreeOSMemory()
	}
}

// ── Lever 8 — NDJSON sidecar (query-optimized spill format) ────────────────────

// forestKeys are the map keys whose value is a large array worth a line-per-record
// NDJSON sidecar. Each is a verified real root shape: `context` =
// get_design_context's node forest (root has NO top-level id); `children` =
// get_document/get_node-style wrappers without an id; `nodes`/`matchingNodes`/
// `textNodes` = scan results; `results` = batch results. Array elements that look
// like nodes are flattened to a line-per-node index; others emit one line each.
// Order matters: `context` is checked first so get_design_context never falls to
// a generic key.
var forestKeys = []string{"context", "nodes", "matchingNodes", "textNodes", "children", "results"}

// writeSpillSidecar writes a query-optimized .ndjson sidecar beside the canonical
// .json spill when data is a recognized node tree or large collection, mirroring
// the catalog's NDJSON idiom. Returns (indexPath, recordCount); ("", 0) when no
// recognizable shape (canonical-only, no crash). Best-effort: a write error
// yields ("", 0) and never fails the call.
func writeSpillSidecar(canonicalPath string, data interface{}) (string, int) {
	lines, ok := buildSpillNDJSON(data)
	if !ok || len(lines) == 0 {
		return "", 0
	}
	ndjsonPath := strings.TrimSuffix(canonicalPath, filepath.Ext(canonicalPath)) + ".ndjson"
	if err := os.WriteFile(ndjsonPath, []byte(strings.Join(lines, "")), 0o600); err != nil {
		return "", 0
	}
	return ndjsonPath, len(lines)
}

// buildSpillNDJSON returns the NDJSON lines (each newline-terminated) for data,
// and ok=false when data has no recognizable collection/tree shape.
//
// Detection precedence (anchored to the verified real root shapes):
//  1. map with a top-level `id` → single node tree (get_node / get_document) →
//     flatten to a line-per-node index {id,name,type,parentId,path}.
//  2. map with a forest key (`context` for get_design_context, `nodes`/
//     `matchingNodes`/`textNodes` for scans, `children`, `results`) → treat each
//     array element as a forest root.
//  3. top-level array (get_nodes_info / get_selection) → each element as a root.
//  4. else → skip (canonical-only, no crash).
func buildSpillNDJSON(data interface{}) ([]string, bool) {
	switch v := data.(type) {
	case map[string]interface{}:
		if hasID(v) {
			var lines []string
			flattenNode(v, "", "", &lines)
			return lines, true
		}
		for _, k := range forestKeys {
			if arr, ok := v[k].([]interface{}); ok {
				return flattenForest(arr), true
			}
		}
		return nil, false
	case []interface{}:
		return flattenForest(v), true
	default:
		return nil, false
	}
}

// flattenForest emits NDJSON for an array of roots: each node-shaped element is
// flattened to a line-per-node index (parentId="" at its root); any non-node
// element is emitted as a single passthrough line. This covers both deep trees
// (get_design_context `context`, get_nodes_info array) and flat scan lists
// (matchingNodes/textNodes — node-shaped but childless → one line each).
func flattenForest(arr []interface{}) []string {
	var lines []string
	for _, el := range arr {
		if m, ok := el.(map[string]interface{}); ok && hasID(m) {
			flattenNode(m, "", "", &lines)
			continue
		}
		if b, err := json.Marshal(el); err == nil {
			lines = append(lines, string(b)+"\n")
		}
	}
	return lines
}

// hasID reports whether m carries a top-level string id — the marker of a
// serialized Figma node root.
func hasID(m map[string]interface{}) bool {
	_, ok := m["id"].(string)
	return ok
}

// flattenNode appends one index record per node {id,name,type,parentId,path},
// then recurses into children. path is the slash-joined chain of ids from the root.
func flattenNode(m map[string]interface{}, parentID, parentPath string, out *[]string) {
	id, _ := m["id"].(string)
	name, _ := m["name"].(string)
	typ, _ := m["type"].(string)
	path := id
	if parentPath != "" {
		path = parentPath + "/" + id
	}
	rec, err := json.Marshal(map[string]any{
		"id":       id,
		"name":     name,
		"type":     typ,
		"parentId": parentID,
		"path":     path,
	})
	if err == nil {
		*out = append(*out, string(rec)+"\n")
	}
	if children, ok := m["children"].([]interface{}); ok {
		for _, c := range children {
			if cm, ok := c.(map[string]interface{}); ok {
				flattenNode(cm, id, path, out)
			}
		}
	}
}

// ── Lever 8 — provenance manifest (findable-by-intent after compaction) ────────

const spillManifestName = "index.ndjson"

// manifestRecord is one append-only provenance line in index.ndjson. Ts is set by
// appendSpillManifest at write time.
type manifestRecord struct {
	Ts        string `json:"ts"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	IndexPath string `json:"indexPath,omitempty"`
	Bytes     int    `json:"bytes"`
	Records   int    `json:"records"`
	Summary   string `json:"summary"`
}

// appendSpillManifest appends one provenance line to dir/index.ndjson under an
// advisory flock, so concurrent spills can't interleave a partial line. The
// summary lets the AI relocate a spill by intent (grep label/summary) after a
// context compaction forgot the exact <label>-<hash> filename. Best-effort:
// never fails the call.
func appendSpillManifest(dir string, rec manifestRecord) {
	rec.Ts = time.Now().UTC().Format(time.RFC3339Nano)
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	line = append(line, '\n')

	f, err := os.OpenFile(filepath.Join(dir, spillManifestName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	unlock := lockManifest(f)
	defer unlock()
	_, _ = f.Write(line)
}

// summarizePayload derives a human/grep-friendly one-line summary from the
// payload itself (the gate is generic, so it can't assume a shape). Never empty:
// falls back to the top-level key list for unrecognized shapes.
func summarizePayload(data interface{}) string {
	switch v := data.(type) {
	case map[string]interface{}:
		if hasID(v) {
			id, _ := v["id"].(string)
			var count int
			hist := map[string]int{}
			countNodes(v, &count, hist)
			return "tree root=" + id + " nodes=" + strconv.Itoa(count) + " types=" + topTypes(hist)
		}
		for _, k := range forestKeys {
			if arr, ok := v[k].([]interface{}); ok {
				return "forest " + k + " len=" + strconv.Itoa(len(arr))
			}
		}
		return "keys=" + strings.Join(sortedKeys(v), ",")
	case []interface{}:
		return "array len=" + strconv.Itoa(len(v))
	default:
		return "scalar"
	}
}

// countNodes counts every node in the tree and tallies a type histogram.
func countNodes(m map[string]interface{}, count *int, hist map[string]int) {
	*count++
	if typ, ok := m["type"].(string); ok {
		hist[typ]++
	}
	if children, ok := m["children"].([]interface{}); ok {
		for _, c := range children {
			if cm, ok := c.(map[string]interface{}); ok {
				countNodes(cm, count, hist)
			}
		}
	}
}

// topTypes renders a small, deterministic "TYPE:n" histogram (alpha-sorted,
// capped) so the summary stays one short line.
func topTypes(hist map[string]int) string {
	keys := make([]string, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 5 {
		keys = keys[:5]
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+":"+strconv.Itoa(hist[k]))
	}
	return strings.Join(parts, ",")
}

// sortedKeys returns the map's keys alpha-sorted (deterministic summaries).
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ── Lever 6B-4 — age-based spill-cache eviction ────────────────────────────────

// spillEvictTTL is the minimum age a spill file must reach before it is eligible
// for eviction. Generous (>24h) so a multi-turn skill that still plans to read a
// spill is never robbed of it. Tunable via FIGMA_MCP_SPILL_TTL_HOURS.
const defaultSpillTTLHours = 48

// spillSizeCapBytes is the total-size ceiling for the spill cache; beyond it,
// eviction removes oldest-first — but ONLY among files already past the TTL, so
// an in-use (recent) spill is never deleted to make room. Tunable via
// FIGMA_MCP_SPILL_CAP_BYTES.
const defaultSpillCapBytes = 512 * 1024 * 1024 // 512 MiB

var evictOnce sync.Once

// processStart marks when this server process came up. Any spill file with an
// mtime at/after this instant was (or may have been) written by THIS process and
// is never evicted — the safety anchor for "never delete a current-process file"
// independent of the TTL value.
var processStart = time.Now()

// evictSpillCacheOnce runs eviction at most once per process lifetime, on first
// spill. Best-effort; never fails the call.
func evictSpillCacheOnce(dir string) {
	evictOnce.Do(func() {
		_ = evictSpillCache(dir, spillTTL(), spillCap(), processStart, time.Now())
	})
}

func spillTTL() time.Duration {
	return time.Duration(envInt("FIGMA_MCP_SPILL_TTL_HOURS", defaultSpillTTLHours)) * time.Hour
}

func spillCap() int64 {
	cap := int64(defaultSpillCapBytes)
	if raw := os.Getenv("FIGMA_MCP_SPILL_CAP_BYTES"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			cap = n
		}
	}
	return cap
}

// spillSidecarExt is the extension of the per-canonical NDJSON sidecar (see
// writeSpillSidecar). Eviction treats a canonical .json and its sidecar as ONE
// unit so it can never (a) orphan a sidecar nor (b) leave index.ndjson pointing
// at a deleted canonical — the exact grep-by-intent recovery Lever 8 exists for.
const spillSidecarExt = ".ndjson"

// removeSpillUnit deletes a canonical spill file and its sibling .ndjson sidecar
// (if any) atomically-enough for best-effort eviction. Returns true if the
// canonical was removed.
func removeSpillUnit(canonicalPath string) bool {
	removed := os.Remove(canonicalPath) == nil
	sidecar := strings.TrimSuffix(canonicalPath, filepath.Ext(canonicalPath)) + spillSidecarExt
	_ = os.Remove(sidecar) // best-effort; absent sidecar is fine
	return removed
}

// evictSpillCache deletes spill UNITS (a canonical .json + its .ndjson sidecar)
// whose mtime is older than ttl, then — if the cache still exceeds sizeCap —
// keeps deleting the oldest remaining EVICTABLE units until under the cap or none
// are left. Sidecars are never selected as independent eviction targets; they are
// only ever removed together with their canonical, and their bytes are accounted
// against that canonical's unit so the cap reflects real disk use.
//
// CRITICAL: a unit is evictable only if its canonical's mtime is before
// procStart — i.e. it was written by a PRIOR process, never the current one. This
// is the hard guarantee "never delete a file written in the current process
// lifetime", decoupled from the TTL value. The TTL is the primary age filter; the
// size cap only ever removes additional prior-process units (oldest-first). A
// within-TTL current-process spill is sacrosanct even if that means the cap is
// temporarily exceeded (safety wins). now and procStart are injected for testability.
func evictSpillCache(dir string, ttl time.Duration, sizeCap int64, procStart, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to evict yet
		}
		return err
	}

	type spillUnit struct {
		path  string // canonical .json
		mtime time.Time
		size  int64 // canonical + sidecar bytes
	}

	// First, index sidecar sizes by their unit base (path without extension) so a
	// canonical can fold in its sidecar's bytes without listing it independently.
	sidecarSize := make(map[string]int64)
	for _, e := range entries {
		if e.IsDir() || e.Name() == spillManifestName {
			continue
		}
		if filepath.Ext(e.Name()) != spillSidecarExt {
			continue
		}
		if info, err := e.Info(); err == nil {
			base := strings.TrimSuffix(filepath.Join(dir, e.Name()), spillSidecarExt)
			sidecarSize[base] = info.Size()
		}
	}

	cutoff := now.Add(-ttl)
	var survivors []spillUnit
	var total int64
	for _, e := range entries {
		if e.IsDir() || e.Name() == spillManifestName {
			continue
		}
		// Sidecars are accounted via their canonical, never independently.
		if filepath.Ext(e.Name()) == spillSidecarExt {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		base := strings.TrimSuffix(path, filepath.Ext(path))
		mtime := info.ModTime()
		unitSize := info.Size() + sidecarSize[base]
		evictable := mtime.Before(procStart) // prior-process unit only

		// Pass 1 — delete every evictable unit already past the TTL.
		if evictable && mtime.Before(cutoff) {
			removeSpillUnit(path)
			continue
		}
		survivors = append(survivors, spillUnit{path: path, mtime: mtime, size: unitSize})
		total += unitSize
	}

	// Pass 2 — enforce the size cap by removing the oldest remaining prior-process
	// units (oldest-first). Current-process units are skipped, never deleted.
	if total <= sizeCap {
		return nil
	}
	sort.Slice(survivors, func(i, j int) bool { return survivors[i].mtime.Before(survivors[j].mtime) })
	for _, f := range survivors {
		if total <= sizeCap {
			break
		}
		if !f.mtime.Before(procStart) {
			continue // current-process unit — sacrosanct
		}
		if removeSpillUnit(f.path) {
			total -= f.size
		}
	}
	return nil
}
