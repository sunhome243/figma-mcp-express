# figma-mcp-express

![Figma MCP Express hero: the fast lane for AI agents into Figma](assets/figma-mcp-express-hero.png)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://go.dev)
[![npm](https://img.shields.io/npm/v/figma-mcp-express.svg)](https://www.npmjs.com/package/figma-mcp-express)
[![Works with Claude Code](https://img.shields.io/badge/Claude%20Code-compatible-8A2BE2?logo=anthropic)](https://claude.ai/code)
[![Works with Codex](https://img.shields.io/badge/Codex-compatible-000000)](https://github.com/openai/codex)

Enhanced fork of [vkhanhqui/figma-mcp-go](https://github.com/vkhanhqui/figma-mcp-go).

---

**Fast, quota-free, agent-ready Figma MCP.** Give AI agents direct read/write access to Figma through a local Desktop plugin, with batch execution, multi-file routing, and stable concurrent sessions that are not capped by Figma's official MCP server tool-call limits.

> **Claude Code, Codex, and other coding agents** that can use the local filesystem is **recommended.** Unlike cloud-only MCPs, figma-mcp-express uses the filesystem to optimize the performance and stability.

If you are building design migration, audit, or handoff agents, give it a try.

| Promise         | What it means in practice                                                                                                                                  |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Fast**        | Build with fewer LLM ↔ plugin round-trips by batching dependent operations into one call.                                                                  |
| **Quota-free**  | Plugin-side work is not capped by Figma's official MCP server limits, such as 6 calls/month for View/Collab seats or 200-600 calls/day for Dev/Full seats. |
| **Agent-ready** | Multiple agents can share a session safely through channel routing, reconnects, read dedup, and a hardened request queue.                                  |

### Why this fork exists

| Compared with        | What blocks real automation                                                                                                                                  | What figma-mcp-express adds                                                                                               |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| Official Figma MCP   | Seat-based MCP server limits: View/Collab seats get up to 6 calls/month, while Dev/Full seats get daily and per-minute caps.                                 | Local plugin-side read/write access for open files without those official MCP tool-call quotas.                           |
| Plain figma-mcp-go   | Single-connection assumptions, no batching, no parallel agents, weaker library automation, and reconnect flapping under multi-file or long-running sessions. | Multi-file channels, batch ops, library tooling, response spill-to-disk, reconnect safety, and concurrent agent handling. |
| Manual Figma cleanup | Repetitive token binding, component replacement, audits, and design-to-code extraction.                                                                      | Agent workflows that can scan, modify, verify, and report across large files.                                             |

---

## Who this is for

- **Coding agents (Claude Code, Codex)** — the primary target. Skills ship with the server and load from the local filesystem, so the agent has structured guidance without burning context on docs. Spill-to-disk keeps large Figma reads out of the context window entirely.
- Design systems teams migrating products to a new component library or token system
- Product designers cleaning up large production files without doing every replacement by hand
- Frontend engineers who need better design-to-code context than screenshots and comments
- Teams experimenting with multi-agent workflows for audits, migrations, and handoff generation

### Before / After

| Situation                                | Without figma-mcp-express                                             | With figma-mcp-express                                                                       |
| ---------------------------------------- | --------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Official Figma MCP on a restricted seat  | You hit seat-based call caps quickly and have to ration automation.   | Normal plugin-side work is not capped by the official MCP server limits.                     |
| Moving a product to a new design library | Designers swap components and fix spacing one screen at a time.       | An agent can inspect the old file, map the new library, and migrate frames in bulk.          |
| Large file audit                         | The model gets flooded with raw node data or times out on huge reads. | Large reads spill to disk and the agent can inspect only the relevant slice.                 |
| Parallel work                            | Multiple agents easily collide or queue uselessly on the same file.   | The bridge isolates channels, coordinates queueing, and supports safer multi-agent sessions. |

### Example prompts

```text
Migrate these 120 product frames to the new library. Keep the new library's UX patterns, spacing rules, and component variants consistent.
```

```text
Scan this file for detached buttons, hardcoded spacing, and off-system color usage. Group the findings by severity and suggest the cleanest replacement path.
```

```text
Turn this React settings page into a Figma review artifact using the correct library components, token bindings, and dark-mode variables.
```

```text
Read this design system file and generate a DESIGN.md with token scale, text styles, component inventory, and obvious consistency gaps.
```

```text
Open the product file and the source library at the same time. Compare their components, then replace outdated instances page by page without touching unaffected areas.
```

---

## Use cases

**For designers**

- Automate dull work — find detached components, rebind hardcoded values to tokens, fix deviations from the design system at scale
- Library swap — migrate a file from one design system to another: remap component keys, rebind tokens, update variants in bulk
- Frame and layout setup — scaffold auto layout, bind spacing variables, pin color modes — Claude handles the structural work while you focus on designing
- Design audit — scan for raw values, off-system components, token gaps, and repeated visuals that could become reusable local components

**For developers**

- Prompt → Figma — generate a Figma counterpart for an existing component or page for design review, using the correct library variants
- Code handoff — extract token names, auto layout spec, and component references per frame, ready to implement without guessing
- Learn a file — generate a DESIGN.md (token scale, text styles, color modes, component inventory) from any Figma file

**For creators**

- Prompt → Figma — describe a screen and have Claude build it end-to-end with real library components and bound tokens
- Stitch → Figma — take a [Stitch](https://stitch.withgoogle.com) wireframe draft and re-render it in Figma with the correct components, spacing tokens, and variable modes
- Pattern report — scan a file for what's there, what's reusable, and what's inconsistent before you start building

---

## Capabilities

| Track     | Capability                     | Why it matters                                                                                                           |
| --------- | ------------------------------ | ------------------------------------------------------------------------------------------------------------------------ |
| Speed     | Fewer back-and-forth steps     | The agent can do several related Figma actions in one go, so building or editing a screen feels much faster.             |
| Speed     | Compact default tool surface   | Compared with `vkhanhqui/figma-mcp-go@fe6cd768`, default `tools/list` drops from 73 tools / 12,214 `o200k_base` tokens to 21 tools / 3,283 tokens (73.1% smaller). |
| Speed     | Large reads stay manageable    | Big files do not dump huge walls of data into the model at once, so the agent can stay focused on the part that matters. |
| Free      | No official MCP quota limits   | You are not blocked by the official Figma MCP server's monthly or daily call caps for normal plugin-side work.           |
| Access    | Direct Figma editing           | The agent works on the open Figma file itself, not a disconnected copy or a limited export.                              |
| Access    | Uses your real design system   | It can work with your actual components, variables, and styles instead of rebuilding everything from raw shapes.         |
| Access    | Can inspect shared libraries   | It can still look up published library assets when the plugin cannot run inside that file.                               |
| Access    | Wire interactive prototypes    | The agent can read, audit, and wire prototype flows — clicks, navigation, overlays, transitions, and flow starting points — directly on the open file. |
| Stability | Multiple files stay separate   | Working on one file does not knock another file offline or mix their state together.                                     |
| Stability | Safe under parallel agent work | Multiple agents can share the same session without stepping on each other as easily.                                     |
| Stability | Better recovery from drops     | If the connection breaks or the MCP client restarts, the system is designed to recover without forcing a full restart.   |
| Scale     | Handles large production files | Big design files stay usable instead of freezing the plugin during long reads or scans.                                  |
| Handoff   | Better design-to-code context  | Developers get cleaner output about tokens, layout rules, and component references when turning designs into code.       |

### One short demo story

A single automated run migrated **100+ production design frames** to a new library while following that library's UX patterns, components, and guidelines. Instead of swapping visuals one by one, the agent could inspect the old file, map the new system, replace structures in bulk, and keep the migration consistent across the whole set.

### Architecture at a glance

```
                 typed MCP tools
AI client  ─────────────────────────▶  Go MCP server
Claude / Codex                         bridge + queue + gate
                                            │
                                            │ JSON over WebSocket
                                            ▼
                                      Figma Desktop plugin
                                      live file read/write

large payloads ─▶ .figma-mcp-cache/       library catalog ─▶ Figma REST only when needed
```

## Installation

Two paths depending on what you need.

### Option A — Plugin install (recommended)

Includes the MCP server + three skills (`/figma-mcp-express`, `/figma-design-patterns`, `/figma-design-md`) + a PreToolUse hook. No clone or build step required.

> Reviewing an unreleased branch? Use Option B and build from source. Marketplace/release installs use the latest published artifact, not draft branch code.

**Claude Code:**

```bash
claude plugin marketplace add sunhome243/figma-mcp-express
claude plugin install figma-mcp-express@figma-mcp-express
```

**Codex:**

```bash
codex plugin marketplace add sunhome243/figma-mcp-express
codex plugin add figma-mcp-express@figma-mcp-express
```

Codex installs from the configured marketplace snapshot. If you added this marketplace before a new release was published, refresh it first:

```bash
codex plugin marketplace upgrade figma-mcp-express
codex plugin add figma-mcp-express@figma-mcp-express
```

For project-scoped install (Claude Code only):

```bash
claude plugin install figma-mcp-express@figma-mcp-express --scope project
```

### Option B — Build from source

For integrating into other MCP clients, or if you want to modify the server.

```bash
git clone https://github.com/sunhome243/figma-mcp-express.git
cd figma-mcp-express
make build          # produces bin/figma-mcp-express + plugin/dist/
```

Add to your `.mcp.json` (or `claude_desktop_config.json` for Claude Desktop):

```json
{
  "mcpServers": {
    "figma-mcp-express": {
      "command": "/absolute/path/to/bin/figma-mcp-express",
      "args": ["--port", "1994"]
    }
  }
}
```

> The `command` path must match the Makefile output (`bin/figma-mcp-express`). Run `figma-mcp-express --version` to confirm the server reloaded your fresh build.

Restart Claude Code / Codex. Tools load on demand.

---

## Figma Desktop plugin setup

The plugin runs inside **Figma Desktop** (not the browser). It connects to the local MCP server over WebSocket and gives the AI agent direct access to the open file.

### Option A — Download from Releases (no clone required)

1. Go to the [Releases page](https://github.com/sunhome243/figma-mcp-express/releases/latest) and download **plugin.zip**
2. Unzip it anywhere — e.g. `~/figma-mcp-express-plugin/`
3. In Figma Desktop: **Plugins → Development → Import plugin from manifest...**
4. Navigate to the unzipped folder and select `manifest.json`

### Option B — From a cloned repo (build from source)

After running `make build` (see [DEV-SETUP.md](DEV-SETUP.md)):

1. In Figma Desktop: **Plugins → Development → Import plugin from manifest...**
2. Navigate to `plugin/` inside the cloned repo and select `manifest.json`

> **Where is "Import plugin from manifest..."?**
> Open any file in Figma Desktop → top menu bar → **Plugins** → hover **Development** → click **Import plugin from manifest...** in the submenu. If you don't see the Development submenu, make sure you are on Figma Desktop (not the web app).

### Running the plugin

1. Open a Figma file
2. **Plugins → Development → Figma MCP Express**
3. The plugin panel shows:
   - **Status** — `Connected` once the MCP server is running, `Waiting for server` otherwise
   - **WebSocket URL** — the address the plugin connected to (default `ws://127.0.0.1:1994`)
   - **Channel ID** — a unique ID for this file's session (pass this as `channel:` in multi-file workflows)
4. Minimize the panel with the **−** button — it collapses to a small pill and stays out of the way

**Multiple files:** open each file and run the plugin in each — every file gets its own channel ID and can be targeted independently.

---

## Environment variables

| Variable                 | Default | Description                                                                                                                                                                                                                                       |
| ------------------------ | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `FIGMA_TOKEN`            | —       | Figma Personal Access Token. Required only for `fetch_library_catalog`. Auto-loaded from `.env`.                                                                                                                                                  |
| `FIGMA_MCP_TOOL_PROFILE` | `core`  | Tool surface profile. `core` is the compact default; `full` restores the legacy top-level compatibility/debugging surface.                                                                                                                        |
| `FIGMA_MCP_TOOL_SCHEMA_MODE` | `compact` | Tool schema verbosity. Default `compact` trims `tools/list` descriptions to reduce MCP context tokens; set `verbose` to expose the full in-schema guidance.                                                                                       |
| `FIGMA_MCP_BATCH_MAX_OPS` | `200` | Maximum top-level `batch.ops` entries accepted before plugin execution. Split very large plans into logical sections; raise only for controlled local runs. |
| `FIGMA_MCP_BATCH_MAX_BYTES` | `2097152` | Maximum encoded `batch.ops` payload size in bytes before plugin execution. Prevents oversized generated plans from occupying the plugin bridge. |
| `FIGMA_MCP_SPILL_BYTES`  | `25000` | Response size threshold. Larger responses spill to `.figma-mcp-cache/`.                                                                                                                                                                           |
| `FIGMA_MCP_TIMEOUT`      | `120`   | Inactivity ceiling in seconds for lightweight ops (writes, metadata reads, styles). Resets on each progress heartbeat.                                                                                                                            |
| `FIGMA_MCP_READ_TIMEOUT` | `600`   | Inactivity ceiling in seconds for heavy reads (`get_node`, `get_nodes_info`, `get_design_context`, full-document reads, scan/search tools) and `batch`. Resets on each progress heartbeat. A firing timer means retry narrower, not raise the ceiling. |
| `FIGMA_MCP_STALL_THRESHOLD` | `45` | Seconds an op may hold a channel's serial slot with **no progress** before a NEW call on that channel is fast-rejected (`ErrChannelStalled`) instead of queueing behind the likely-hung op. Protects concurrent agents from a single wedged call; the stuck op itself still resolves at its own ceiling, and the guard self-heals when the slot frees. |

### FIGMA_TOKEN

Most tools work without a token — the plugin talks directly to Figma Desktop. You only need `FIGMA_TOKEN` for `fetch_library_catalog`, which hits the REST API to enumerate published components from a read-only shared library.

**Generate a token:** Figma → Account Settings → Personal access tokens → Generate new token. Read-only scope (`File content: Read`) is sufficient.

**Setting it — depends on your install path:**

**Option A (plugin):**

```bash
echo 'export FIGMA_TOKEN=your_token_here' >> ~/.${SHELL##*/}rc && source ~/.${SHELL##*/}rc
```

**Option B (build from source):** Add a `.env` file in the project root (already gitignored).

```bash
FIGMA_TOKEN=your_token_here
```

The binary loads `.env` from its working directory at startup. Shell env always takes precedence over `.env`.

> Treat it like a password — it grants read access to all Figma files visible to your account. Never commit it.

---

## See also

- [TOOLS.md](TOOLS.md) — full tool catalog with parameter tables
- [ARCHITECTURE.md](ARCHITECTURE.md) — batch ref resolution, response gating, singleflight, multi-channel routing
- [DEV-SETUP.md](DEV-SETUP.md) — build instructions, plugin rebuild, test commands

---

## Credits

Built on [vkhanhqui/figma-mcp-go](https://github.com/vkhanhqui/figma-mcp-go) (MIT). The original established the core insight: skip the REST API, talk directly to Figma Desktop over WebSocket. figma-mcp-express adds multi-channel routing, batch ops, response spill-to-disk, library automation, codegen context, library catalog discovery, cooperative yield, and depth-limited traversal.

---

## Known limitations

### Community UI kits are not importable by key unless published as a library

If a community kit has only been published to Community, its components are still **unpublished local components**. This is a Figma platform constraint, not a server bug.

Figma has two _unrelated_ meanings of "published" — community kits satisfy only the first:

| "Published"                | Means                                                         | Community kits |
| -------------------------- | ------------------------------------------------------------- | -------------- |
| Published **to Community** | the _file_ is shared so anyone can view / duplicate it        | ✅ yes         |
| Published **as a library** | the _components_ are importable via `import_component_by_key` | ❌ no          |

So even the kit's _own_ file cannot import its components by key, and the Plugin API has **no cross-file copy** — this server bridges several open files and moves _data_ between them, but it cannot fabricate a cross-document component _link_. (REST `components: 0` / `404` is **not** the arbiter, either — a kit published as a library after the fact imports fine even while REST still 404s. The live `import_component_by_key` probe decides.)

In that state:

- `import_component_by_key` fails with `Cannot import component ... since it is unpublished`
- `fetch_library_catalog` may return `components: 0`
- cross-file linked instances are not possible; only detached copies

> **Workaround**
>
> **Option A — publish the duplicated kit as a library**
>
> 1. Duplicate the community kit into your drafts or team workspace.
> 2. Open that duplicate in Figma and publish it from the **Assets** panel.
> 3. Re-run `import_component_by_key` and then `create_instance` in the target file.
> 4. This gives you real linked instances with normal variant behavior.
>
> **Option B — no publish**
>
> 1. Open both the kit file and the target file.
> 2. Copy the needed components from the kit and paste them into the target file once.
> 3. Use those pasted local components for future instances in the target file.
> 4. This works, but the instances are detached local copies, not links back to the kit.
