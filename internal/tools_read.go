package internal

import "github.com/mark3labs/mcp-go/server"

func registerReadTools(s *server.MCPServer, node *Node) {
	registerReadDocumentTools(s, node)
	registerReadStyleTools(s, node)
	registerReadExportTools(s, node)
}
