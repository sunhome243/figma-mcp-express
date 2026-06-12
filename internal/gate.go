package internal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// parseSpillThreshold reads FIGMA_MCP_SPILL_BYTES from the environment and
// returns it as an int. Falls back to 25000 if the env var is unset or invalid.
func parseSpillThreshold() int {
	return envInt("FIGMA_MCP_SPILL_BYTES", 25000)
}

// spillPreviewBytes caps how much of the (potentially huge) spilled payload is
// scanned to build the 600-rune preview. 600 runes are at most 2400 UTF-8 bytes,
// so a 2400-byte prefix always contains the first 600 runes intact; any partial
// rune at the cut sits past index 600 and is discarded. This avoids the
// full-buffer []rune(text) copy that Lever 6B-2 is removing — taking it over the
// whole payload would silently re-introduce the copy on the largest path.
const spillPreviewBytes = 2400

// gateResponse implements the server-side size-gate on a marshaled response.
//
//   - If len(raw) <= threshold, returns (string(raw), false, nil) — passthrough.
//   - Otherwise: writes raw to "<label>-<shortHash>.json" inside dir, also writes
//     a query-optimized .ndjson sidecar (Lever 8) when the payload is a recognized
//     collection or node tree, appends a provenance record to index.ndjson, and
//     returns a small JSON handle (carrying path + optional indexPath) plus
//     (true, nil). On any I/O error writing the canonical file, returns ("", false, err);
//     sidecar + manifest writes are best-effort and never fail the call.
//
// raw is the marshaled bytes (operated on directly — no redundant string copy on
// the hot/spill path). data is the already-parsed payload (resp.Data) reused for
// sidecar/summary derivation so the gate never re-unmarshals the large blob.
func gateResponse(raw []byte, data interface{}, label, dir string, threshold int) (string, bool, error) {
	if len(raw) <= threshold {
		return string(raw), false, nil
	}

	// Create the spill directory if needed.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("gate mkdir: %w", err)
	}

	// Lever 6B-4 — age-based + size-cap eviction, at most once per process, on the
	// first spill (best-effort; never deletes a current-process file).
	evictSpillCacheOnce(dir)

	// Derive a short, stable filename from the content hash.
	// filepath.Base strips any path separators from label to prevent traversal.
	safeLabel := filepath.Base(label)
	hash := sha256.Sum256(raw)
	shortHash := fmt.Sprintf("%x", hash[:4]) // 8 hex chars
	filename := safeLabel + "-" + shortHash + ".json"
	absPath := filepath.Join(dir, filename)

	// Check whether this exact canonical file was already written. If so, the
	// provenance manifest entry also already exists — skip the append to prevent
	// unbounded duplicate lines on cache-hit re-gates of the same spilled payload.
	_, statErr := os.Stat(absPath)
	alreadySpilled := statErr == nil

	// Write the full payload (overwrite if already exists — idempotent).
	// 0o600: owner read/write only — shared-account safety.
	if err := os.WriteFile(absPath, raw, 0o600); err != nil {
		return "", false, fmt.Errorf("gate write: %w", err)
	}

	// Lever 8 — query-optimized NDJSON sidecar (best-effort, never fails the call).
	indexPath, records := writeSpillSidecar(absPath, data)

	// Lever 8 — provenance manifest so a spill stays findable by INTENT after a
	// context compaction forgets the exact <label>-<hash> filename (best-effort).
	// Skip when the same canonical file was already present — the first spill already
	// recorded the provenance; a re-gate of an identical payload must not grow
	// index.ndjson with duplicate entries.
	if !alreadySpilled {
		appendSpillManifest(dir, manifestRecord{
			Label:     safeLabel,
			Path:      absPath,
			IndexPath: indexPath,
			Bytes:     len(raw),
			Records:   records,
			Summary:   summarizePayload(data),
		})
	}

	// Lever 6B-5 — opportunistic OS-memory release AFTER a large spill (never on
	// the hot/passthrough path). No-op unless FIGMA_MCP_FREE_OS_MEM is set.
	freeOSMemory()

	hint := "large output saved to disk — read it with jq/grep; do not expect inline data"
	if indexPath != "" {
		hint = "large output saved to disk. Query the .ndjson sidecar (indexPath) line-by-line with rg/grep/`jq -c`/duckdb instead of full-parsing the .json. " +
			"don't know which spill file you need? grep .figma-mcp-cache/index.ndjson by tool/summary, then read its .ndjson sidecar."
	} else {
		hint += ". don't know which spill file you need? grep .figma-mcp-cache/index.ndjson by tool/summary, then read the .json."
	}

	handleData := map[string]any{
		"spilled": true,
		"path":    absPath,
		"bytes":   len(raw),
		"preview": spillPreview(raw),
		"hint":    hint,
	}
	if indexPath != "" {
		handleData["indexPath"] = indexPath
	}

	handle, err := json.Marshal(handleData)
	if err != nil {
		return "", false, fmt.Errorf("gate marshal handle: %w", err)
	}
	return string(handle), true, nil
}

// spillPreview builds the ≤600-rune preview from a bounded prefix of raw, never
// copying the whole payload (see spillPreviewBytes).
func spillPreview(raw []byte) string {
	head := raw
	if len(head) > spillPreviewBytes {
		head = head[:spillPreviewBytes]
	}
	runes := []rune(string(head))
	previewLen := 600
	if len(runes) < previewLen {
		previewLen = len(runes)
	}
	return string(runes[:previewLen])
}
