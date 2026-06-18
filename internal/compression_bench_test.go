package internal

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"fmt"
	"testing"
)

// Compression gate for the two transport hops (WebSocket permessage-deflate +
// HTTP gzip, both DEFLATE-based).
//
// The go/no-go decision was validated on the REAL captured wire corpus
// (.figma-mcp-cache/*.json, 78 payloads, 6.0 MB): node-tree reads compress
// ~6–14× (get_nodes_info 11.8×, scan 13.6×, get_node 10.5×, get_design_context
// 9.1×), whole-corpus 6.3× (84% saved), at ~5–10 ms/MB CPU. Those payloads are
// proprietary design data and are NOT committed; this test re-proves the win on a
// structurally-faithful synthetic node tree (same key/value distribution as the
// serializer output) so it is reproducible in the public repo and guards against
// regressions in the assumption.
const compressionGate = 3.0 // adopt only if representative payload compresses >= 3×

func buildRepresentativeNodeTree(nodes int) []byte {
	types := []string{"FRAME", "TEXT", "INSTANCE", "RECTANGLE", "GROUP"}
	hex := []string{"#0f172a", "#ffffff", "#3b82f6", "#e2e8f0", "#64748b"}
	children := make([]map[string]any, 0, nodes)
	for i := 0; i < nodes; i++ {
		children = append(children, map[string]any{
			"id":   fmt.Sprintf("%d:%d", 1+i%9, 100+i),
			"name": fmt.Sprintf("Component / Item %d", i%37),
			"type": types[i%len(types)],
			"bounds": map[string]any{
				"x": i % 390, "y": (i * 13) % 844, "width": 120 + i%48, "height": 40 + i%16,
			},
			"styles": map[string]any{
				"fills":       []string{hex[i%len(hex)]},
				"strokeStyle": hex[(i+2)%len(hex)],
			},
		})
	}
	root := map[string]any{
		"id": "0:1", "name": "Page", "type": "FRAME",
		"bounds":   map[string]any{"x": 0, "y": 0, "width": 1440, "height": 4000},
		"children": children,
	}
	b, _ := json.Marshal(root)
	return b
}

func deflateRatio(t *testing.T, raw []byte) float64 {
	t.Helper()
	var buf bytes.Buffer
	zw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate.NewWriter: %v", err)
	}
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("deflate write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("deflate close: %v", err)
	}
	return float64(len(raw)) / float64(buf.Len())
}

func TestCompressionGate_RepresentativePayload(t *testing.T) {
	raw := buildRepresentativeNodeTree(300)
	ratio := deflateRatio(t, raw)
	t.Logf("representative node tree: %d bytes → deflate ratio %.1f×", len(raw), ratio)
	if ratio < compressionGate {
		t.Fatalf("compression ratio %.1f× below gate %.1f× — compression not justified", ratio, compressionGate)
	}
}

func BenchmarkDeflateRepresentative(b *testing.B) {
	raw := buildRepresentativeNodeTree(300)
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		zw, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		_, _ = zw.Write(raw)
		_ = zw.Close()
	}
}
