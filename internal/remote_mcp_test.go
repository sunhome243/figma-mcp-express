package internal

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestRemoteFollowerMCP_supportsInitializeAndToolsList(t *testing.T) {
	// Given
	node := NewRemoteFollowerNode("https://design-mac.example.ts.net", &http.Client{}, "test")
	mcpServer := NewFigmaMCPServer("test", node)
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(mcpServer))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	httpTransport, err := transport.NewStreamableHTTP(httpServer.URL + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHTTP: %v", err)
	}
	c := client.NewClient(httpTransport)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// When
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test-client", Version: "0.0.1"}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}
	initResult, err := c.Initialize(ctx, initReq)

	// Then
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initResult.ServerInfo.Name != "figma-mcp-express" {
		t.Fatalf("server name = %q, want figma-mcp-express", initResult.ServerInfo.Name)
	}

	// When
	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})

	// Then
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsResult.Tools) == 0 {
		t.Fatal("expected registered tools")
	}
}

func TestRemoteFollowerMCP_supportsPromptsListAndClientNotifications(t *testing.T) {
	// Given
	rec := newRemoteLeaderRecorder(t)
	c := newRemoteMCPClient(t, rec.server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initializeRemoteMCPClient(t, ctx, c)

	// When
	promptsResult, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})

	// Then
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(promptsResult.Prompts) == 0 {
		t.Fatal("expected registered prompts")
	}

	// When
	if err := c.RootListChanges(ctx); err != nil {
		t.Fatalf("RootListChanges notification: %v", err)
	}
}

func TestRemoteFollowerMCP_doesNotAddPermissiveCORS(t *testing.T) {
	// Given
	node := NewRemoteFollowerNode("https://design-mac.example.ts.net", &http.Client{}, "test")
	mcpServer := NewFigmaMCPServer("test", node)
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(mcpServer))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	req, err := http.NewRequest(http.MethodOptions, httpServer.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")

	// When
	resp, err := http.DefaultClient.Do(req)

	// Then
	if err != nil {
		t.Fatalf("OPTIONS /mcp: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want no permissive browser CORS", got)
	}
}

func TestRunRemoteFollower_stopsServingWhenContextCancelled(t *testing.T) {
	// Given
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(proxy.Close)

	listen := "127.0.0.1:" + freeRemoteFollowerTCPPort(t)
	cfg, err := NewRemoteFollowerConfig("https://design-mac.example.ts.net", proxy.URL, listen)
	if err != nil {
		t.Fatalf("NewRemoteFollowerConfig: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunRemoteFollower(ctx, cfg, "test")
	}()

	c := waitForRemoteFollowerMCP(t, "http://"+listen+"/mcp")
	if err := c.Close(); err != nil {
		t.Fatalf("Close MCP client: %v", err)
	}

	// When
	cancel()

	// Then
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunRemoteFollower: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunRemoteFollower did not stop after context cancellation")
	}
	assertRemoteFollowerEndpointDown(t, "http://"+listen+"/mcp")
}

func TestRemoteFollowerMCP_toolsCallReachesRemoteLeaderRPC(t *testing.T) {
	// Given
	rec := newRemoteLeaderRecorder(t)
	c := newRemoteMCPClient(t, rec.server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initializeRemoteMCPClient(t, ctx, c)

	// When
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_metadata",
			Arguments: map[string]any{
				"origin": "grace",
			},
		},
	})

	// Then
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", extractResultText(result))
	}
	if text := extractResultText(result); !containsCI(text, "Remote File") {
		t.Fatalf("result text = %q, want remote data", text)
	}
	req := rec.lastRequest(t)
	if req.Tool != "get_metadata" {
		t.Fatalf("forwarded tool = %q, want get_metadata", req.Tool)
	}
	if req.Params["origin"] != "grace" {
		t.Fatalf("forwarded origin = %#v, want grace", req.Params["origin"])
	}
	if _, ok := req.Params["sessionId"].(string); !ok {
		t.Fatal("remote follower did not inject sessionId")
	}
}

func TestRemoteFollowerMCP_listChannelsUsesRemoteLeaderChannels(t *testing.T) {
	// Given
	rec := newRemoteLeaderRecorder(t)
	c := newRemoteMCPClient(t, rec.server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initializeRemoteMCPClient(t, ctx, c)

	// When
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "list_channels"},
	})

	// Then
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_channels returned error: %s", extractResultText(result))
	}
	text := extractResultText(result)
	if !containsCI(text, "chan-1") || !containsCI(text, "Remote File") {
		t.Fatalf("list_channels text = %q", text)
	}
}

func TestRemoteFollowerMCP_schemaValidationRejectsBeforeForwarding(t *testing.T) {
	// Given
	rec := newRemoteLeaderRecorder(t)
	c := newRemoteMCPClient(t, rec.server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initializeRemoteMCPClient(t, ctx, c)

	// When
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_node",
			Arguments: map[string]any{
				"nodeId": "bad",
				"origin": "grace",
			},
		},
	})

	// Then
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected validation error result")
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("invalid RPC reached remote leader %d time(s), want 0", got)
	}
}

type remoteLeaderRecorder struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []RPCRequest
}

func newRemoteLeaderRecorder(t *testing.T) *remoteLeaderRecorder {
	t.Helper()
	rec := &remoteLeaderRecorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc":
			var req RPCRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode RPC request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			rec.mu.Lock()
			rec.requests = append(rec.requests, req)
			rec.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RPCResponse{Data: map[string]any{"name": "Remote File"}}) //nolint:errcheck
		case "/channels":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]ChannelInfo{{ //nolint:errcheck
				Channel:  "chan-1",
				FileName: "Remote File",
			}})
		case "/ping":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": "test"}) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

func (rec *remoteLeaderRecorder) count() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.requests)
}

func (rec *remoteLeaderRecorder) lastRequest(t *testing.T) RPCRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.requests) == 0 {
		t.Fatal("expected at least one RPC request")
	}
	return rec.requests[len(rec.requests)-1]
}

func freeRemoteFollowerTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free TCP port: %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	return port
}

func waitForRemoteFollowerMCP(t *testing.T, endpoint string) *client.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var lastErr error
	for ctx.Err() == nil {
		httpTransport, err := transport.NewStreamableHTTP(endpoint)
		if err != nil {
			t.Fatalf("NewStreamableHTTP: %v", err)
		}
		c := client.NewClient(httpTransport)
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "test-client", Version: "0.0.1"}
		initReq.Params.Capabilities = mcp.ClientCapabilities{}
		if _, err := c.Initialize(ctx, initReq); err == nil {
			return c
		} else {
			lastErr = err
			_ = c.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("remote follower MCP did not initialize: %v", lastErr)
	return nil
}

func assertRemoteFollowerEndpointDown(t *testing.T, endpoint string) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodPost, endpoint, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("remote follower endpoint still accepted connections after shutdown")
}

func newRemoteMCPClient(t *testing.T, leaderURL string) *client.Client {
	t.Helper()
	node := NewRemoteFollowerNode(leaderURL, &http.Client{}, "test")
	mcpServer := NewFigmaMCPServer("test", node)
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(mcpServer))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	httpTransport, err := transport.NewStreamableHTTP(httpServer.URL + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHTTP: %v", err)
	}
	c := client.NewClient(httpTransport)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func initializeRemoteMCPClient(t *testing.T, ctx context.Context, c *client.Client) {
	t.Helper()
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test-client", Version: "0.0.1"}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
}
