package internal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// TestCapabilitySeedInjectedOnInitialize proves the seed actually flows through
// the MCP initialize handshake (WithInstructions -> InitializeResult.instructions),
// which is the once-per-session injection point — not just that BuildCapabilitySeed
// renders.
func TestCapabilitySeedInjectedOnInitialize(t *testing.T) {
	s := server.NewMCPServer("figma-mcp-express", "test",
		server.WithInstructions(BuildCapabilitySeed()))

	msg := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}`
	resp := s.HandleMessage(context.Background(), []byte(msg))
	if resp == nil {
		t.Fatal("initialize returned nil")
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal initialize response: %v", err)
	}
	var parsed struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal initialize response: %v", err)
	}
	for _, want := range []string{
		"search_batch_ops(category)", "category:effects", "category:prototype",
		"glass, noise, texture", "figma-design-patterns",
	} {
		if !strings.Contains(parsed.Result.Instructions, want) {
			t.Fatalf("initialize.instructions missing %q; got %q", want, parsed.Result.Instructions)
		}
	}
	t.Logf("initialize.instructions = %d bytes injected at handshake", len(parsed.Result.Instructions))
}
