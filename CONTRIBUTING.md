# Contributing

## Before you start

Read [DEV-SETUP.md](DEV-SETUP.md) — it covers the full build environment, how the Go server and Figma plugin relate, and how to reload changes without restarting everything.

## What's in scope

- Bug fixes and stability improvements
- New MCP tools (Go side) or new batch op types (plugin side)
- Skill and documentation improvements
- Performance and multi-agent reliability

If you're unsure whether something fits, open an issue first.

## Branch strategy

```
main         ← production; protected; no direct push
feature/*    ← all work branches; cut from main; merged via PR
```

- Branch off `main`: `git checkout -b feature/my-thing`
- One concern per branch — don't mix a bug fix with a refactor
- Delete your branch after merge

## Versioning and releases

This project follows [Semantic Versioning](https://semver.org/):

| Change | Bump |
|--------|------|
| Breaking API or tool change | `v1.0.0` → `v2.0.0` |
| New tool or non-breaking feature | `v1.0.0` → `v1.1.0` |
| Bug fix or patch | `v1.0.0` → `v1.0.1` |

**Tagging rules:**
- Tags are cut from `main` HEAD only — never from a feature branch
- Tag format: `vX.Y.Z` (e.g. `v1.2.0`)
- Pushing a `v*` tag triggers the release workflow automatically (binary builds → npm publish → MCP Registry)

**Changelog:**
- Keep `CHANGELOG.md` updated under `## [Unreleased]` as you work
- Before tagging, rename `[Unreleased]` to the new version + date
- GitHub Release Notes are generated automatically from PR titles — write clear PR titles

## Workflow

1. Fork the repo and create a `feature/*` branch off `main`
2. Make your changes — Go server in `internal/` + `cmd/`, plugin in `plugin/src/`
3. Run tests: `go test ./...` and `cd plugin && bun test`
4. Open a PR with a clear description of what changed and why

## PR expectations

- One concern per PR — don't mix a bug fix with a refactor
- If you're adding an op, register it in `BatchOpCatalog` (the authoritative contract `search_batch_ops` / `get_batch_op_spec` expose) and update [TOOLS.md](TOOLS.md) with the parameter table. If the op is batch-only in the `core` profile (not a top-level tool), add it under the relevant `## Write —*` section so it inherits that section's batch-op banner — don't present it as a top-level tool. Only add to the core-21 list (in TOOLS.md + [ARCHITECTURE.md](ARCHITECTURE.md)) if it's genuinely a new top-level core tool.
- If you're changing server behavior, update [ARCHITECTURE.md](ARCHITECTURE.md) if it affects the documented design
- Keep `CHANGELOG.md` updated under `[Unreleased]`

## Code style

Go: standard `gofmt`. Plugin (TypeScript): existing style, `bun run build` must pass clean.

## License

By contributing you agree your changes are released under the [MIT License](LICENSE).
