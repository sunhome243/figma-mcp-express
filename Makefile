.PHONY: test test-go test-ts typecheck-ts lint-ts coverage coverage-go coverage-go-html coverage-ts build build-go build-ts run

# Version stamped into the binary via -ldflags, so `figma-mcp-express --version` reports a
# real value (git describe) instead of "dev" — the only way to confirm a reload picked
# up a fresh build rather than a stale binary. Falls back to "dev" outside a git tree.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build: build-go build-ts

build-go:
	# Output name MUST match the command path in .mcp.json (figma-mcp-express → bin/figma-mcp-express),
	# otherwise the MCP server keeps launching a stale binary after a reload.
	go build -ldflags "-X main.version=$(VERSION)" -o bin/figma-mcp-express ./cmd/figma-mcp-express

# Single-source launch: rebuild (with version stamp) then exec the fresh binary on the
# default port. The server speaks stdio JSON-RPC, so this is mainly for a leader process
# / smoke check — an MCP client normally spawns the binary itself via .mcp.json.
run: build-go
	./bin/figma-mcp-express --port 1994

build-ts:
	cd plugin && bun run build

test: test-go test-ts

test-go:
	go test ./...

test-ts:
	cd plugin && bun test

# Type-checks the new Figma Plugin API surface against @figma/plugin-typings.
# vite/esbuild builds strip types without checking, so this is the only gate that
# verifies handler property/method usage matches the real typed API.
typecheck-ts:
	cd plugin && bun run typecheck

# Figma-plugin API hygiene: fails on any deprecated sync API forbidden under
# documentAccess: "dynamic-page" (--max-warnings 0 makes the advisory rules fail too).
lint-ts:
	cd plugin && bunx eslint . --max-warnings 0

coverage: coverage-go coverage-ts

coverage-go:
	go test -coverprofile=bin/coverage.out ./... && go tool cover -func=bin/coverage.out

coverage-ts:
	cd plugin && bun test --coverage
