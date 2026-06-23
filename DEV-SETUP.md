# DEV-SETUP.md — figma-mcp-express

This guide covers building, registering, and using the fork locally.
Use the published npm package for normal installs. Build from source when developing or verifying local changes so the MCP client launches the freshly built binary and plugin bundle.

---

## Prerequisites

| Tool | Purpose |
|------|---------|
| **Go 1.21+** | Build the MCP server binary |
| **Bun** | Build and test the Figma plugin |
| **Figma Desktop** | Required — the plugin only runs in the desktop app, not the web |

---

## Architecture in 30 seconds

```
Figma Desktop
  └── Plugin (WebSocket client)
        │  channel=auto-1
        ▼
Go MCP Server  (port 1994, stdio to AI tool)
  ├── Bridge — routes requests to plugins by channel id
  ├── Gate   — spills large responses (>25KB) to .figma-mcp-cache/
  └── compact core MCP tools via stdio (full legacy surface opt-in)
```

Two processes must be running simultaneously: the **Go binary** (MCP server, speaks stdio to your AI tool) and the **Figma plugin** (WebSocket client inside Figma Desktop). The bridge links them.

---

## 1. Build

```bash
# From the repo root
cd figma-mcp-express

# Build the Go MCP server (output name MUST match the .mcp.json command below)
go build -o bin/figma-mcp-express ./cmd/figma-mcp-express   # or: make build-go

# Build the Figma plugin
cd plugin && bun run build
# Output: plugin/dist/code.js + plugin/dist/index.html
```

---

## 2. Register the MCP server

Add to your project's `.mcp.json` (or `claude_desktop_config.json` for Claude Desktop):

```json
{
  "mcpServers": {
    "figma-mcp-express": {
      "command": "/absolute/path/to/figma-mcp-express/bin/figma-mcp-express",
      "args": ["--port", "1994"]
    }
  }
}
```

> Run `figma-mcp-express --version` to confirm a reload picked up your fresh build, not a stale one.

Restart your AI tool (Claude Code: `claude mcp list` to confirm the server appears).

---

## 3. Load the plugin in Figma Desktop (one-time setup)

1. Open Figma Desktop
2. **Plugins → Development → Import plugin from manifest…**
3. Navigate to the `plugin/` folder inside the cloned repo and select `manifest.json` (not the upstream release zip — that zip is for users who don't clone)
4. Open a Figma file
5. **Plugins → Development → Figma MCP Express** → Run
6. The plugin UI shows the connection status and assigned channel ID (e.g. `Channel: auto-1`)

---

## 4. Working with multiple files simultaneously

The bridge uses channel IDs to multiplex multiple open files through the same port.

**Setup:**
1. Open **File A** in Figma Desktop and run the plugin → note `Channel: auto-1`
2. Open **File B** in a separate window/tab and run the plugin → note `Channel: auto-2`
3. Both channels coexist — connecting a second file does **not** disconnect the first

**Usage:**
```
list_channels → [
  { channel: "auto-1", fileName: "UI Kit", fileKey: "ABC123" },
  { channel: "auto-2", fileName: "Product App", fileKey: "XYZ789" }
]

# Target a specific file by passing the channel param
get_styles(channel: "auto-1")        → reads from UI Kit
batch(channel: "auto-2", ops: [
  { "type": "create_frame", "params": { "name": "Draft", "width": 320, "height": 200 } }
])                                  → writes to Product App
```

When only one file is open, omit `channel` — the single connection is used automatically.

### Reaching a subscribed library (icon kits like SF Symbols / Material Symbols, UI kits)

To **use** a published component you only need its key — the library file does **not** have to be open. The hard part is getting the keys in the first place. There are two ways, and the human action differs:

- **Route A — paste the library's file URL (nothing to open).** Give the assistant the library file's Figma URL (`https://figma.com/design/<fileKey>/...`). It runs `fetch_library_catalog` over REST and gets every component name + key with no file open. **Route A only works for a PUBLISHED team library.** A **community file** (one you duplicated into your own drafts but never published) returns an empty catalog — then use Route B. (`FIGMA_TOKEN` (§5) must also have read access to the file; a token that can't read it likewise falls back to Route B.)
- **Route B — open the kit file AND run the plugin in it.** Open the library file in Figma Desktop and run **Plugins → Development → Figma MCP Express** inside it. The assistant enumerates its components **one page at a time** (icon kits are huge — it scopes to the icon page rather than scanning the whole file) and caches the keys. You don't need to do anything beyond opening the file and running the plugin.

**Gotcha — *opening* a file is not enough.** A file is only reachable once the plugin is **running inside it** (each running plugin = one channel). So the action you need to take is either "paste the library's file URL" (Route A) or "open the kit file **and run the plugin in it**" (Route B). Confirm with `list_channels` — one entry per running plugin.

---

## 5. Environment variables

Create a `.env` file in the repo root (gitignored). The server auto-loads it on startup.

```bash
# Required for REST library catalog fetches (`fetch_library_catalog`)
FIGMA_TOKEN=figd_xxxxxxxxxxxxxxxx

# Optional tuning
FIGMA_MCP_TOOL_PROFILE=core       # default. Set full for the legacy top-level tool surface.
FIGMA_MCP_TOOL_SCHEMA_MODE=compact # compact tools/list descriptions. Set verbose for full schema docs.
FIGMA_MCP_BATCH_MAX_OPS=200       # Max top-level batch.ops entries before fail-fast rejection.
FIGMA_MCP_BATCH_MAX_BYTES=2097152 # Max encoded batch.ops payload bytes before fail-fast rejection.
FIGMA_MCP_SPILL_BYTES=25000   # Response gate threshold (bytes). Default: 25000
FIGMA_MCP_TIMEOUT=120         # Inactivity ceiling (seconds) for lightweight ops. Default: 120
FIGMA_MCP_READ_TIMEOUT=600    # Inactivity ceiling (seconds) for heavy reads + batch. Default: 600
FIGMA_MCP_STALL_THRESHOLD=45  # Seconds an op may hold a channel's slot with no progress before a NEW call is fast-rejected (ErrChannelStalled). Default: 45
```

---

## 6. Available capabilities (this fork)

In the default `core` profile, many low-level operations below are batch/FigmaPlan
op types rather than top-level MCP tools. Use `search_batch_ops` and
`get_batch_op_spec` for exact availability and params.

### Track A — Library workflow

| Operation | Purpose |
|------|---------|
| `import_component_by_key` | Import a published component or component set by key |
| `import_variable_by_key` | Import a library variable; returns local variable ID |
| `import_style_by_key` | Import text / effect / paint / grid styles |
| `create_instance` | Import a component set variant and create an instance |
| `set_instance_properties` | Set variant/slot properties on an instance (`resetOverrides` supported) |
| `set_variable_mode` | Pin a node to dark/light mode via `setExplicitVariableModeForCollection` |
| `get_remote_variable_collection` | Query a library variable collection's modes and defaults |
| `list_library_variable_collections` | Enumerate subscribed library variable collections |

`set_fills` and `set_strokes` also accept a `variableId` param — binds the token rather than hardcoding a color. Warns on raw hex; throws on unresolved token.

### Track C — Codegen serialization

`get_design_context` accepts `detail: "codegen"` to include per-node:
- `autoLayout` — flex semantics (direction, alignment, gap, sizing)
- `tokens` — names of bound design variables
- `componentRef` — `mainComponent` key + name for code mapping
- `codeConnect` — optional `codeConnectMap` param for local Code Connect resolution

### Track F — Library catalog and token discovery

| Operation | Purpose |
|------|---------|
| `fetch_library_catalog` | Fetch published components + thumbnails via REST (requires `FIGMA_TOKEN`) |
| `get_local_components` | Enumerate unpublished components through the plugin; omit `pageId` for whole-file local-master recovery or pass `pageId` for one-page bounded scans |
| `get_library_variables` | Enumerate variables from a subscribed library collection through the plugin/teamLibrary path |

### Batch — multi-op sequencing

In the default `core` profile, low-level write/read primitives are used as validated
`batch` op types rather than top-level MCP tools. Discover them with
`search_batch_ops`, inspect exact params with `get_batch_op_spec`, and dry-run a
generated plan with `validateOnly:true` before mutation.

```json
{
  "validateOnly": true,
  "ops": [
    { "type": "create_frame", "params": { "name": "Card", "width": 320, "height": 200 } },
    { "type": "set_fills",    "nodeIds": ["$0.id"], "params": { "color": "#FFFFFF" } },
    { "type": "get_node",     "nodeIds": ["$0.id"], "params": { "depth": 1 } }
  ]
}
```

`$0.id` is resolved at runtime — no round-trip back to the LLM between ops. Remove
`validateOnly` or set it to `false` to execute after validation passes. See
[ARCHITECTURE.md](ARCHITECTURE.md) for the full stop-policy and ref syntax.

---

## 7. Rebuild after code changes

| What changed | Rebuild command |
|---|---|
| Go server (`internal/*.go`, `cmd/`) | `make build-go` (→ `bin/figma-mcp-express`) + restart AI tool |
| Plugin (`plugin/src/*.ts`) | `cd plugin && bun run build` + close and re-run plugin in Figma |

---

## 8. Run tests

```bash
# Go tests
go test ./...

# Plugin tests (TypeScript via Bun)
cd plugin && bun test
```

---

## Known constraints

- **No arbitrary script eval.** The Figma plugin sandbox forbids `eval` / `new Function`, so the upstream `use_figma` arbitrary-script pattern cannot be ported here. The supported replacement is declarative `batch` / FigmaPlan JSON: validated op types, `$N.field` ref resolution, projection, and `map` for bounded per-item variation. Script-like fields such as `script`, `code`, `js`, `eval`, and `function` are rejected before plugin execution.
- **Desktop only.** The plugin WebSocket bridge requires Figma Desktop. The web app does not support local WebSocket connections.
- **`figma.teamLibrary` requires Team/Org plan.** `list_library_variable_collections` and related team-library calls fail on the free plan.
- **`enablePrivatePluginApi`.** The plugin uses a private API to expose `figma.fileKey`. This means it cannot be published to the public Figma Community — it is intended as a self-hosted developer plugin only.
