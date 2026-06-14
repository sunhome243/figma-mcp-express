---
name: figma-mcp-express
description: Use when calling figma-mcp-express MCP tools for Figma reads, validated writes, library imports, audits, token binding, screenshots, or multi-file/channel work.
---

# Figma MCP Express

Use the compact core surface first. In default `core` profile, low-level write primitives are not top-level tools; full plugin capability is available through validated `batch` ops.

## First Checks

1. Figma Desktop must be open with **Plugins -> Development -> Figma MCP Express** running.
2. Start with `get_metadata`; if multiple files are open, call `list_channels` and pass `channel` on every file-specific tool call.
3. If the plugin is not connected, ask the user to open the file and run the plugin. Do not retry in a loop.

## Tool Surface

- `core` profile exposes read/high-value tools plus `batch`, `search_batch_ops`, and `get_batch_op_spec`.
- `full` profile is compatibility/debug mode for the legacy top-level surface.
- For unfamiliar writes: `search_batch_ops` -> `get_batch_op_spec` -> `batch(validateOnly:true)` -> `batch`.
- Do not write raw Plugin API JS, `use_figma`-style scripts, `eval`, or code strings. Use declarative `FigmaPlan` batch ops only.

## Working Rules

- Read wide-shallow, then targeted-deep: `get_pages`, `search_nodes` / `scan_nodes_by_types`, then `get_node` / `get_nodes_info`.
- Use `get_design_context detail:"codegen"` for codegen-grade context.
- Use `save_screenshots`, not base64 screenshot payloads, for visual review.
- After every write, validate structurally with a trailing read op or a follow-up `get_node`; screenshots are the final visual pass, not proof of mutation.
- Build one logical section per batch. Avoid one huge batch that blocks the plugin queue.
- Use `continueOnError:true` for scanned lists; inspect per-item errors.
- For multi-file work, channel is mandatory. A channel maps to one open Figma file.
- Library import keys are not node IDs. Component/style keys must be full 40-char lowercase hex; for component sets pass `assetType:"COMPONENT_SET"` or fetch the catalog first so the server can route it.

## Reference Router

| Need | Read |
|---|---|
| Read-tool choice, validation matrix, common errors | `references/tool-selection.md` |
| Batch refs, projection, `map`, validation-first recipes | `references/batch-recipes.md` |
| Connection drops, cache spill files, text/image gotchas | `references/gotchas.md` |
| Parallel agents and channel partitioning | `references/multi-agent.md` |

Exact operation names and params live in `BatchOpCatalog`; never copy op schemas into skill docs. Use `get_batch_op_spec` for the current schema.

## Cannot Do

- Create new Figma files.
- Work on unopened files through the plugin, except REST-backed `fetch_library_catalog` with `FIGMA_TOKEN`.
- FigJam or Slides.
- Publish libraries.
- Arbitrary script execution.
