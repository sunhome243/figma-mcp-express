package internal

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// makeTestPDF creates a minimal valid single-page PDF in memory using pdfcpu.
func makeTestPDF(t *testing.T) []byte {
	t.Helper()
	const minimalJSON = `{"paper":"A4P","pages":{"1":{"content":{}}}}`
	var buf bytes.Buffer
	if err := api.Create(nil, bytes.NewReader([]byte(minimalJSON)), &buf, nil); err != nil {
		t.Fatalf("makeTestPDF: %v", err)
	}
	return buf.Bytes()
}

// Regression for issue #28: save_screenshots returned a silent
// {succeeded:0,total:0} when items was empty/unparseable, with no reason. It must
// surface an actionable error instead.
func TestSaveScreenshots_EmptyItemsReturnsError(t *testing.T) {
	s, _ := newTestServer(t)

	res := callToolResult(t, s, "save_screenshots", map[string]any{
		"items": []any{},
	})
	if !res.IsError {
		t.Fatalf("save_screenshots with empty items must return an error, got: %s", resultText(t, res))
	}
	if txt := resultText(t, res); !strings.Contains(txt, "items") {
		t.Fatalf("error must explain that items is required, got: %s", txt)
	}
}

// ── extractFramePDFs ──────────────────────────────────────────────────────────

func TestExtractFramePDFs_Valid(t *testing.T) {
	pdf := makeTestPDF(t)
	b64 := base64.StdEncoding.EncodeToString(pdf)

	data := map[string]any{
		"frames": []any{
			map[string]any{"nodeId": "1:1", "nodeName": "Frame 1", "base64": b64},
			map[string]any{"nodeId": "1:2", "nodeName": "Frame 2", "base64": b64},
		},
	}

	pages, err := extractFramePDFs(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	for i, p := range pages {
		if !bytes.Equal(p, pdf) {
			t.Errorf("page %d bytes differ from input", i)
		}
	}
}

func TestExtractFramePDFs_EmptyFrames(t *testing.T) {
	data := map[string]any{"frames": []any{}}
	_, err := extractFramePDFs(data)
	if err == nil {
		t.Error("expected error for empty frames array")
	}
}

func TestExtractFramePDFs_MissingFramesKey(t *testing.T) {
	_, err := extractFramePDFs(map[string]any{})
	if err == nil {
		t.Error("expected error when frames key is absent")
	}
}

func TestExtractFramePDFs_EmptyBase64InFrame(t *testing.T) {
	data := map[string]any{
		"frames": []any{
			map[string]any{"nodeId": "1:1", "base64": ""},
		},
	}
	_, err := extractFramePDFs(data)
	if err == nil {
		t.Error("expected error for frame with empty base64")
	}
}

func TestExtractFramePDFs_InvalidBase64(t *testing.T) {
	data := map[string]any{
		"frames": []any{
			map[string]any{"nodeId": "1:1", "base64": "!!!not-valid-base64!!!"},
		},
	}
	_, err := extractFramePDFs(data)
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestExtractFramePDFs_UnmarshalError(t *testing.T) {
	_, err := extractFramePDFs(make(chan int))
	if err == nil {
		t.Error("expected marshal error for non-JSON-serialisable value")
	}
}

// ── mergePDFPages ─────────────────────────────────────────────────────────────

func TestMergePDFPages_SinglePage(t *testing.T) {
	pdf := makeTestPDF(t)
	merged, err := mergePDFPages([][]byte{pdf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged) == 0 {
		t.Fatal("merged PDF is empty")
	}
	if !bytes.HasPrefix(merged, []byte("%PDF")) {
		t.Error("merged output does not start with %PDF")
	}
}

func TestMergePDFPages_MultiplePages(t *testing.T) {
	pdf := makeTestPDF(t)
	merged, err := mergePDFPages([][]byte{pdf, pdf, pdf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged) == 0 {
		t.Fatal("merged PDF is empty")
	}
	// Validate that pdfcpu considers the output a valid PDF.
	if err := api.Validate(bytes.NewReader(merged), nil); err != nil {
		t.Errorf("merged PDF is not valid: %v", err)
	}
}

func TestMergePDFPages_EmptyInput(t *testing.T) {
	_, err := mergePDFPages(nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
	_, err = mergePDFPages([][]byte{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestMergePDFPages_InvalidPDFBytes(t *testing.T) {
	_, err := mergePDFPages([][]byte{[]byte("not a pdf")})
	if err == nil {
		t.Error("expected error for invalid PDF bytes")
	}
}
