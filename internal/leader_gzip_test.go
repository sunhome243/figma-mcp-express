package internal

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// acceptsGzip must honour the q-value: a bare "gzip" yes, "gzip;q=0" no, and
// non-matching codings ("x-gzip", "deflate") no.
func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"gzip":                  true,
		"gzip, deflate, br":     true,
		"deflate, gzip;q=0.5":   true,
		"":                      false,
		"deflate":               false,
		"x-gzip":                false, // substring match would wrongly accept this
		"gzip;q=0":              false, // explicit refusal
		"deflate, gzip;q=0.000": false,
	}
	for header, want := range cases {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

// The real handler interaction: a handler that calls WriteHeader(status) then
// json.NewEncoder(w).Encode(...) (exactly what sendJSON does) must round-trip through
// withGzip with the status preserved and the body recoverable; an http.Error path
// must also survive.
func TestWithGzip_RealHandlerPaths(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"id": "1:2", "name": "Frame", "type": "FRAME"}}

	t.Run("WriteHeader+Encode (sendJSON shape) gzipped", func(t *testing.T) {
		h := withGzip(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(payload)
		})
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("missing Content-Encoding: gzip")
		}
		gr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("gzip.NewReader: %v", err)
		}
		var got map[string]any
		if err := json.NewDecoder(gr).Decode(&got); err != nil {
			t.Fatalf("decode gunzip: %v", err)
		}
		if data, ok := got["data"].(map[string]any); !ok || data["id"] != "1:2" {
			t.Fatalf("decoded payload wrong: %v", got)
		}
	})

	t.Run("http.Error path survives gzip", func(t *testing.T) {
		h := withGzip(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		})
		req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rec.Code)
		}
		gr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("gzip.NewReader: %v", err)
		}
		body, _ := io.ReadAll(gr)
		if !strings.Contains(string(body), "method not allowed") {
			t.Fatalf("error body not recoverable: %q", body)
		}
	})
}

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
