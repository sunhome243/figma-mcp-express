package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

const (
	remoteMCPPath               = "/mcp"
	remoteMCPReadinessPath      = "/readyz"
	remoteMCPMaxRequestBytes    = maxRPCPayloadBytes
	remoteHTTPReadHeaderTimeout = 5 * time.Second
	remoteHTTPIdleTimeout       = 2 * time.Minute
	remoteHTTPMaxHeaderBytes    = 32 << 10
	remoteMCPReadinessTimeout   = 2 * time.Second
)

func NewFigmaMCPServer(version string, node *Node) *server.MCPServer {
	s := server.NewMCPServer("figma-mcp-express", version,
		server.WithInstructions(BuildCapabilitySeed()))
	RegisterTools(s, node)
	RegisterPrompts(s)
	return s
}

func RunRemoteFollower(ctx context.Context, cfg RemoteFollowerConfig, version string) error {
	client, err := NewRemoteFollowerHTTPClient(cfg)
	if err != nil {
		return fmt.Errorf("remote follower client: %w", err)
	}
	node := NewRemoteFollowerNode(cfg.LeaderURL, client, version)
	mcpServer := NewFigmaMCPServer(version, node)
	httpServer := newRemoteFollowerMCPServer(mcpServer, node, cfg.MCPListen)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Start(cfg.MCPListen)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("remote mcp shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("remote mcp serve: %w", err)
	}
}

// newRemoteFollowerMCPServer serves only loopback clients, but still applies
// HTTP limits and rejects browser origins so a local browser cannot drive the
// MCP endpoint through DNS rebinding.
func newRemoteFollowerMCPServer(mcpServer *server.MCPServer, node *Node, listen string) *server.StreamableHTTPServer {
	mux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:              listen,
		ReadHeaderTimeout: remoteHTTPReadHeaderTimeout,
		IdleTimeout:       remoteHTTPIdleTimeout,
		MaxHeaderBytes:    remoteHTTPMaxHeaderBytes,
	}
	transport := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath(remoteMCPPath),
		server.WithStreamableHTTPServer(httpServer),
	)
	mux.Handle(remoteMCPPath, newRemoteFollowerMCPHandler(transport))
	mux.HandleFunc(remoteMCPReadinessPath, remoteMCPReadinessHandler(node))
	httpServer.Handler = mux
	return transport
}

func newRemoteFollowerHTTPHandler(mcpTransport http.Handler, node *Node) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(remoteMCPPath, newRemoteFollowerMCPHandler(mcpTransport))
	mux.HandleFunc(remoteMCPReadinessPath, remoteMCPReadinessHandler(node))
	return mux
}

func newRemoteFollowerMCPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			http.Error(w, "browser origins are not allowed", http.StatusForbidden)
			return
		}
		if r.ContentLength > remoteMCPMaxRequestBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, remoteMCPMaxRequestBytes))
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

func remoteMCPReadinessHandler(node *Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), remoteMCPReadinessTimeout)
		defer cancel()
		channels, err := node.ListChannels(ctx)
		if err != nil || len(channels) == 0 {
			http.Error(w, "remote follower is not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Status   string `json:"status"`
			Channels int    `json:"channels"`
		}{Status: "ready", Channels: len(channels)})
	}
}
