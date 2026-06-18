---
name: figma-mcp-express
description: Use when calling figma-mcp-express MCP tools for Figma reads, validated writes, library imports, audits, token binding, screenshots, or multi-file/channel work.
---

! touch "/tmp/fme-skill-loaded-${CLAUDE_SESSION_ID:-$PPID}" 2>/dev/null || true

# Figma MCP Express

Use the compact core surface first. In default `core` profile, low-level write primitives are not top-level tools; full plugin capability is available through validated `batch` ops.

## First Checks

1. Figma Desktop must be open with **Plugins -> Development -> Figma MCP Express** running.
2. Start with `get_metadata`; if multiple files are open, call `list_channels` and pass `channel` on every file-specific top-level tool call. For `batch`, put `channel` on the outer call, not inside `ops[*].params`.
3. If the plugin is not connected, ask the user to open the file and run the plugin. Do not retry in a loop.
4. **Empty `get_variable_defs` ≠ no design system.** It returns only *local* variables; a file subscribing to an external library shows empty here. Confirm with `list_library_variable_collections` before concluding there are no tokens. See `references/tool-selection.md`.

## Tool Surface

- Default `core` profile = read/high-value tools + `batch`, `search_batch_ops`, `get_batch_op_spec`. (Legacy top-level write tools: `FIGMA_MCP_TOOL_PROFILE=full`.)
- Unfamiliar writes: `search_batch_ops` (intent words; e.g. `delete_node op` -> `delete_nodes`) -> `get_batch_op_spec` -> `batch(validateOnly:true)` -> `batch`.
- Do not write raw Plugin API JS, `use_figma` scripts, `eval`, or code strings — declarative `batch` ops only.

## Working Rules

- Read wide-shallow, then targeted-deep. **Two-phase bounded read** for a large node: list shallow (`get_design_context detail:"minimal" depth:1`), then deep per frame (`scan_text_nodes` + `get_node depth:2-3`). `depth` bounds the work.
- After every write, validate structurally with a trailing/follow-up read; `save_screenshots` (not base64) is the final visual pass, not mutation proof.
- Build one logical section per batch; use `continueOnError:true` for scanned lists.
- For multi-file work, channel is mandatory — one `channel` per open file, on every file-specific call.

Read-tool choice, validation matrix, batch refs, import-key format, and gotchas live in the references below.

## Reference Router

| Need | Read |
|---|---|
| Read-tool choice, detail levels, style audit, common errors | `references/tool-selection.md` |
| Batch refs, projection, `map`, validation-first recipes, write-workflow recipes | `references/batch-recipes.md` |
| Permanent Figma Plugin API constraints (instance children, clone IDs, auto-layout children, etc.) | `references/platform-constraints.md` |
| Server bugs + workarounds with issue tracking (#33, #34) | `references/mcp-known-bugs.md` |
| Remaining failure modes: stale IDs, node format, spilled cache, text/font/image | `references/gotchas.md` |
| Parallel agents, channel partitioning, Watch-agent presence (`origin` + `set_presence` for status/task; per-session no-clobber) | `references/multi-agent.md` |
| Parameterized generators (type scale, color palette, component variants, design tokens) | MCP prompts: `generate_type_scale`, `generate_color_palette`, `generate_component_variants`, `design_token_generation_strategy` — invoke via the MCP prompts list |

Exact operation names and params live in `BatchOpCatalog`; never copy op schemas into skill docs. Use `get_batch_op_spec` for the current schema.

## Cannot Do

- Create new Figma files.
- Work on unopened files through the plugin, except REST-backed `fetch_library_catalog` with `FIGMA_TOKEN`.
- FigJam or Slides.
- Publish libraries.
- Arbitrary script execution.
