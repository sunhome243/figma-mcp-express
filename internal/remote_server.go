package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"
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
	httpServer := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))

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
