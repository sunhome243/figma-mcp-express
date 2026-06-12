package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/sunhome243/figma-mcp-express/internal"
)

// version is injected at build time:
// go build -ldflags "-X main.version=1.0.0" ./cmd/figma-mcp-express
var version = "dev"

var logger = log.New(os.Stderr, "", 0)

func main() {
	ip := flag.String("ip", "127.0.0.1", "IP address to listen on (use 0.0.0.0 to accept remote connections)")
	port := flag.Int("port", 1994, "port to listen on")
	showVersion := flag.Bool("version", false, "print the build version and exit")
	flag.Parse()

	// --version lets a reloading client confirm which binary is live (the stale-binary
	// trap is invisible otherwise). Stamp the real version via `make build-go`.
	if *showVersion {
		fmt.Println(version)
		return
	}

	// Load .env from the working directory (project root) so secrets like
	// FIGMA_TOKEN can live in a gitignored .env instead of the shell. The real
	// environment always wins; values are never logged.
	if n, err := internal.LoadDotEnv(".env"); err != nil {
		logger.Printf("WARNING: .env load failed: %v", err)
	} else if n > 0 {
		logger.Printf("loaded %d var(s) from .env", n)
	}

	parsedIP := net.ParseIP(*ip)
	if parsedIP == nil {
		logger.Fatalf("invalid IP address: %q", *ip)
	}
	if !parsedIP.IsLoopback() {
		logger.Printf("WARNING: binding to %s — server will be reachable from the network with no authentication", *ip)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	node := internal.NewNode(*ip, *port, version)
	election := internal.NewElection(*ip, *port, node)

	if err := election.Start(ctx); err != nil {
		logger.Fatalf("election start: %v", err)
	}

	logger.Printf("Starting figma-mcp-express %s (role: %s)", version, node.RoleName())

	s := server.NewMCPServer("figma-mcp-express", version)
	internal.RegisterTools(s, node)
	internal.RegisterPrompts(s)

	go func() {
		<-ctx.Done()
		logger.Printf("Shutting down...")
		election.Stop()
		node.Stop()
	}()

	if err := server.ServeStdio(s); err != nil {
		logger.Fatalf("mcp serve: %v", err)
	}
}
