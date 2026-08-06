//go:build e2e

package internal

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestRemoteFollowerBinaryE2E_proxiesMCPCallsThroughHTTPSLeader(t *testing.T) {
	binary := os.Getenv("FIGMA_MCP_E2E_BINARY")
	if binary == "" {
		t.Skip("set FIGMA_MCP_E2E_BINARY to run binary E2E")
	}

	caPath, cert := writeTestCAAndCert(t, "design-mac.example.ts.net")
	leader := newFakeTLSLeader(t, cert)
	proxy := newConnectProxy(t, leader.Listener.Addr().String())
	mcpListen := "127.0.0.1:" + freeTCPPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary,
		"--mode", "remote-follower",
		"--leader-url", "https://design-mac.example.ts.net",
		"--outbound-proxy", "http://"+proxy.Addr(),
		"--mcp-listen", mcpListen,
	)
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+caPath, "FIGMA_MCP_TOOL_PROFILE=full")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start binary: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	})
	go io.Copy(io.Discard, stderr) //nolint:errcheck

	c := waitForRemoteMCP(t, ctx, "http://"+mcpListen+"/mcp")

	if _, err := c.ListTools(ctx, mcp.ListToolsRequest{}); err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if _, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_metadata",
		Arguments: map[string]any{"origin": "grace"},
	}}); err != nil {
		t.Fatalf("read CallTool: %v", err)
	}
	if _, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name: "set_text",
		Arguments: map[string]any{
			"nodeId": "1:1",
			"text":   "updated",
			"origin": "grace",
		},
	}}); err != nil {
		t.Fatalf("write CallTool: %v", err)
	}

	tools := leader.Tools()
	if tools["get_metadata"] != 1 {
		t.Fatalf("get_metadata RPC count = %d, want 1", tools["get_metadata"])
	}
	if tools["set_text"] != 1 {
		t.Fatalf("set_text RPC count = %d, want 1", tools["set_text"])
	}
}

type fakeTLSLeader struct {
	*httptest.Server
	mu    sync.Mutex
	tools map[string]int
}

func newFakeTLSLeader(t *testing.T, cert tls.Certificate) *fakeTLSLeader {
	t.Helper()
	leader := &fakeTLSLeader{tools: map[string]int{}}
	leader.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": "e2e"}) //nolint:errcheck
		case "/channels":
			json.NewEncoder(w).Encode([]ChannelInfo{{Channel: "chan-1", FileName: "Remote File"}}) //nolint:errcheck
		case "/rpc":
			var req RPCRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			leader.mu.Lock()
			leader.tools[req.Tool]++
			leader.mu.Unlock()
			json.NewEncoder(w).Encode(RPCResponse{Data: map[string]any{"ok": true}}) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	leader.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	leader.StartTLS()
	t.Cleanup(leader.Close)
	return leader
}

func (l *fakeTLSLeader) Tools() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]int, len(l.tools))
	for k, v := range l.tools {
		out[k] = v
	}
	return out
}

func waitForRemoteMCP(t *testing.T, ctx context.Context, endpoint string) *client.Client {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		httpTransport, err := transport.NewStreamableHTTP(endpoint)
		if err != nil {
			t.Fatalf("NewStreamableHTTP: %v", err)
		}
		c := client.NewClient(httpTransport)
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "e2e-client", Version: "0.0.1"}
		initReq.Params.Capabilities = mcp.ClientCapabilities{}
		if _, err := c.Initialize(ctx, initReq); err == nil {
			t.Cleanup(func() { _ = c.Close() })
			return c
		} else {
			lastErr = err
			_ = c.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("remote MCP did not initialize: %v", lastErr)
	return nil
}
