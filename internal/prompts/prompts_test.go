package prompts

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterAll_NoPanic(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1")
	RegisterAll(s) // must register all prompts without panicking
}
