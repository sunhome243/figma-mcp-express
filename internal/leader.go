package internal

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

var leaderLogger = log.New(os.Stderr, "[leader] ", 0)

// Leader owns the WebSocket bridge to the Figma plugin and exposes
// HTTP endpoints for health checks and follower RPC proxying.
//
// Endpoints:
//
//	/ws   — WebSocket upgrade for the Figma plugin
//	/ping — Health check (GET)
//	/rpc  — JSON RPC for follower tool calls (POST)
type Leader struct {
	ip      string
	port    int
	bridge  *Bridge
	server  *http.Server
	version string
}

// NewLeader creates a Leader. Call Start() to bind the ip:port.
func NewLeader(ip string, port int, version string) *Leader {
	return &Leader{
		ip:      ip,
		port:    port,
		bridge:  NewBridge(),
		version: version,
	}
}

// GetBridge returns the underlying Bridge so Node can use it directly.
func (l *Leader) GetBridge() *Bridge {
	return l.bridge
}

// Start binds the port and begins serving. Returns an error immediately
// if the port is already in use (EADDRINUSE → caller detects another leader).
func (l *Leader) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", l.ip, l.port))
	if err != nil {
		return err // includes EADDRINUSE
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", l.handlePing)
	// /rpc and /channels carry the full follower-proxied payload (node trees etc.),
	// which compress ~6–14× — gzip them when the client accepts it. The follower's
	// Go http.Client auto-negotiates Accept-Encoding and transparently decompresses,
	// so no follower-side change is needed. /ws (WebSocket upgrade) and /ping (tiny)
	// are NOT wrapped — gzipping a hijacked upgrade would break it.
	mux.HandleFunc("/rpc", withGzip(l.handleRPC))
	mux.HandleFunc("/ws", l.handleWS)
	mux.HandleFunc("/channels", withGzip(l.handleChannels))

	srv := &http.Server{Handler: mux}
	l.server = srv

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			leaderLogger.Printf("serve error: %v", err)
		}
	}()

	leaderLogger.Printf("listening on %s:%d", l.ip, l.port)
	return nil
}

// Stop shuts down the HTTP server and closes the bridge.
func (l *Leader) Stop() {
	if l.server != nil {
		l.server.Shutdown(context.Background())
		l.server = nil
	}
	l.bridge.Close()
}

// handlePing responds to health checks from followers.
func (l *Leader) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": l.version,
	})
	if err != nil {
		leaderLogger.Printf("encode ping response error: %v", err)
	}
}

// handleWS upgrades the connection to WebSocket for the Figma plugin.
func (l *Leader) handleWS(w http.ResponseWriter, r *http.Request) {
	l.bridge.HandleUpgrade(w, r)
}

// handleChannels returns the connected plugin channels (for followers + list_channels).
func (l *Leader) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(l.bridge.ListChannels()); err != nil {
		leaderLogger.Printf("encode channels response error: %v", err)
	}
}

// handleRPC handles JSON RPC calls from follower processes.
func (l *Leader) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: "failed to read body"})
		return
	}

	var req RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: "invalid JSON"})
		return
	}

	leaderLogger.Printf("rpc %s nodeIDs=%v from %s", req.Tool, req.NodeIDs, r.RemoteAddr)
	normalizeRPCNodeReferences(req.NodeIDs, req.Params)

	if req.Tool == "batch" {
		if err := validateAndPrepareBatchParams(req.Params); err != nil {
			leaderLogger.Printf("rpc %s validation error: %s", req.Tool, err)
			l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: err.Error()})
			return
		}
	} else {
		if validationErr := ValidateRPC(req.Tool, req.NodeIDs, req.Params); validationErr != "" {
			leaderLogger.Printf("rpc %s validation error: %s", req.Tool, validationErr)
			l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: validationErr})
			return
		}
		if req.Tool == "import_component_by_key" {
			prepareImportComponentByKeyParams(req.Params)
		}
	}

	resp, err := l.bridge.Send(r.Context(), req.Tool, req.NodeIDs, req.Params)
	if err != nil {
		leaderLogger.Printf("rpc %s bridge error: %v", req.Tool, err)
		l.sendJSON(w, http.StatusOK, RPCResponse{Error: err.Error()})
		return
	}

	if resp.Error != "" {
		leaderLogger.Printf("rpc %s plugin error: %s", req.Tool, resp.Error)
		l.sendJSON(w, http.StatusOK, RPCResponse{Error: resp.Error})
		return
	}

	l.sendJSON(w, http.StatusOK, RPCResponse{Data: resp.Data})
}

// gzipResponseWriter pipes the response body through a gzip.Writer. Header() and
// WriteHeader() pass through to the underlying writer unchanged — Content-Encoding
// is set by withGzip before the handler runs, so it is present when headers flush.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

// acceptsGzip reports whether an Accept-Encoding header offers gzip with a non-zero
// quality. Tokenizes on commas and honours an explicit `gzip;q=0` refusal, rather
// than a bare substring match (which would also match `gzip;q=0` and `x-gzip`).
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.TrimSpace(coding) != "gzip" {
			continue
		}
		// Reject an explicit q=0 (RFC 9110 "not acceptable"); any other q means yes.
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			switch strings.TrimSpace(v) {
			case "0", "0.0", "0.00", "0.000":
				return false
			}
		}
		return true
	}
	return false
}

// withGzip gzips a JSON handler's response when the request advertises gzip support.
// Used only for /rpc and /channels (never the /ws upgrade). When the client does not
// accept gzip, the handler runs unwrapped and the output is byte-identical to before.
func withGzip(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			h(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer func() {
			if err := gz.Close(); err != nil {
				leaderLogger.Printf("gzip close error: %v", err)
			}
		}()
		h(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	}
}

func (l *Leader) sendJSON(w http.ResponseWriter, status int, body RPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		leaderLogger.Printf("encode response error: %v", err)
	}
}
