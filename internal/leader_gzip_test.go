package internal

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withGzip must be transparent: the decompressed body a client receives is
// byte-identical to what the wrapped handler wrote, and compression only kicks
// in when the client advertised it.
func TestWithGzip_RoundTripIdentical(t *testing.T) {
	// A representative JSON-ish body large enough to be worth compressing.
	body := `{"data":` + strings.Repeat(`{"id":"1:23","name":"Frame","type":"FRAME","bounds":{"x":0,"y":0,"width":390,"height":844}},`, 200) + `null]}`
	inner := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
	h := withGzip(inner)

	t.Run("gzip when accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/channels", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		gr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("gzip.NewReader: %v", err)
		}
		got, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("read gunzip: %v", err)
		}
		if string(got) != body {
			t.Fatalf("decompressed body differs from original (len got=%d want=%d)", len(got), len(body))
		}
		// The whole point: the wire form is smaller than the source.
		if rec.Body.Len() >= len(body) {
			t.Fatalf("gzipped size %d not smaller than raw %d", rec.Body.Len(), len(body))
		}
	})

	t.Run("plain when not accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/channels", nil)
		rec := httptest.NewRecorder()
		h(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Content-Encoding = %q, want empty (no gzip)", got)
		}
		if rec.Body.String() != body {
			t.Fatalf("unwrapped body differs from original")
		}
	})
}
