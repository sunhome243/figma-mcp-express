---
name: figma-mcp-express
description: Use when any figma-mcp-express MCP tool is needed for Figma reads, writes, batch ops, screenshots, libraries, tokens, prototypes, or multi-file/channel work.
---

! sid="${CLAUDE_CODE_SESSION_ID:-${CLAUDE_SESSION_ID:-${CODEX_SESSION_ID:-$PPID}}}"; sid="$(printf "%s" "$sid" | tr -cd 'A-Za-z0-9_-')"; touch "/tmp/fme-skill-loaded-${sid:-default}" 2>/dev/null || true

# Figma MCP Express

Use the compact `core` tool surface first. Full plugin capability lives behind validated `batch` ops; legacy top-level tools appear only with `FIGMA_MCP_TOOL_PROFILE=full`.

## First Checks

1. Figma Desktop must be open with the Figma MCP Express plugin running.
2. Start with `get_metadata`; if multiple files are open, call `list_channels`.
3. For multi-file work, `channel is mandatory`: pass it on every file-specific tool. For `batch`, put `channel` only on the outer call.
4. If the plugin is not connected, ask the user to open the file and run the plugin. Do not retry in a loop.
5. Empty `get_variable_defs` means no local variables, not no design system. Check subscribed libraries with `list_library_variable_collections`.

## Tool Surface

- Unfamiliar ops: `search_batch_ops` -> `get_batch_op_spec` -> `batch(validateOnly:true)` -> `batch`.
- Exact operation names and params live in `BatchOpCatalog`; do not copy op schemas into skill docs.
- Do not write raw Plugin API JS, `use_figma` scripts, `eval`, or code strings. Use declarative `batch` ops only.

## Working Rules

- Read wide-shallow, then targeted-deep. For large nodes: `get_design_context detail:"minimal" depth:1`, then `scan_text_nodes` or `get_node depth:2-3`.
- After every write, validate structurally with a follow-up read; use `save_screenshots` only as the final visual pass.
- Build one logical section per batch; use `continueOnError:true` for scanned lists.

## Reference Router

| Need | Read |
|---|---|
| Read-tool choice, detail levels, style audit, common errors | `references/tool-selection.md` |
| Batch refs, projection, `map`, validation-first recipes, write-workflow recipes | `references/batch-recipes.md` |
| Workflow recipes: rename, text replacement, annotations, overrides | `references/workflow-recipes.md` |
| Prototype wiring/audit/reaction maps/scroll-fixed chrome | Load companion skill `figma-prototype`; keep MCP execution discipline here |
| Native effects (`GLASS`/`NOISE`/`TEXTURE`), `set_effects`, `create_effect_style` | `references/effects.md` |
| Permanent Figma Plugin API constraints (instance children, clone IDs, auto-layout children, etc.) | `references/platform-constraints.md` |
| Server bugs + workarounds with issue tracking (#33, #34) | `references/mcp-known-bugs.md` |
| Remaining failure modes: stale IDs, node format, spilled cache, text/font/image | `references/gotchas.md` |
| Parallel agents, channel partitioning, shared-resource handoff | `references/multi-agent.md` |
| Watch-agent presence (`origin`, `set_presence`, status/task, per-session no-clobber) | `references/presence.md` |
| Parameterized generators (type scale, color palette, component variants, design tokens) | MCP prompts: `generate_type_scale`, `generate_color_palette`, `generate_component_variants`, `design_token_generation_strategy` — invoke via the MCP prompts list |

## Cannot Do

- Create new Figma files.
- Work on unopened files through the plugin, except REST-backed `fetch_library_catalog` with `FIGMA_TOKEN`.
- FigJam or Slides.
- Publish libraries.
- Arbitrary script execution.
