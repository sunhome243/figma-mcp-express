package prompts

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterAll_NoPanic(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1")
	RegisterAll(s) // must register all prompts without panicking
}

// newTestServer builds an MCPServer with prompts registered and returns an
// initialised in-process client so tests can query the live prompt list.
func newTestServer(t *testing.T) *client.Client {
	t.Helper()
	s := server.NewMCPServer("test", "0.0.1", server.WithPromptCapabilities(true))
	RegisterAll(s)
	c, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// TestRegisterAll_GeneratorsPresent confirms the four parameterised generator
// prompts are still registered after the guidance-only prompts were deprecated.
func TestRegisterAll_GeneratorsPresent(t *testing.T) {
	c := newTestServer(t)
	result, err := c.ListPrompts(context.Background(), mcp.ListPromptsRequest{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	got := make(map[string]bool, len(result.Prompts))
	for _, p := range result.Prompts {
		got[p.Name] = true
	}
	want := []string{
		"design_token_generation_strategy",
		"generate_color_palette",
		"generate_type_scale",
		"generate_component_variants",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("expected generator prompt %q to be registered", name)
		}
	}
}

// TestRegisterAll_DeprecatedAbsent confirms that guidance-only prompts
// consolidated into skill references are no longer registered.
func TestRegisterAll_DeprecatedAbsent(t *testing.T) {
	c := newTestServer(t)
	result, err := c.ListPrompts(context.Background(), mcp.ListPromptsRequest{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	got := make(map[string]bool, len(result.Prompts))
	for _, p := range result.Prompts {
		got[p.Name] = true
	}
	deprecated := []string{
		"read_design_strategy",
		"design_strategy",
		"style_audit_strategy",
		"bulk_rename_strategy",
		"text_replacement_strategy",
		"annotation_conversion_strategy",
		"reaction_to_connector_strategy",
		"swap_overrides_instances",
	}
	for _, name := range deprecated {
		if got[name] {
			t.Errorf("deprecated prompt %q must not be registered", name)
		}
	}
}
