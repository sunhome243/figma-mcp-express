# Contributing

## Before you start

Read [DEV-SETUP.md](DEV-SETUP.md) — it covers the full build environment, how the Go server and Figma plugin relate, and how to reload changes without restarting everything.

## What's in scope

- Bug fixes and stability improvements
- New MCP tools (Go side) or new batch op types (plugin side)
- Skill and documentation improvements
- Performance and multi-agent reliability

If you're unsure whether something fits, open an issue first.

## Workflow

1. Fork the repo and create a branch off `main`
2. Make your changes — Go server in `internal/` + `cmd/`, plugin in `plugin/src/`
3. Run tests: `go test ./...` and `cd plugin && bun test`
4. Open a PR with a clear description of what changed and why

## PR expectations

- One concern per PR — don't mix a bug fix with a refactor
- If you're adding a tool, update [TOOLS.md](TOOLS.md) with the parameter table
- If you're changing server behavior, update [ARCHITECTURE.md](ARCHITECTURE.md) if it affects the documented design
- Keep `CHANGELOG.md` updated under `[Unreleased]`

## Code style

Go: standard `gofmt`. Plugin (TypeScript): existing style, `bun run build` must pass clean.

## License

By contributing you agree your changes are released under the [MIT License](LICENSE).
