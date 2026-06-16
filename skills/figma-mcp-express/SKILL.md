---
name: figma-mcp-express
description: Use when calling figma-mcp-express MCP tools for Figma reads, validated writes, library imports, audits, token binding, screenshots, or multi-file/channel work.
---

# Figma MCP Express

Use the compact core surface first. In default `core` profile, low-level write primitives are not top-level tools; full plugin capability is available through validated `batch` ops.

## First Checks

1. Figma Desktop must be open with **Plugins -> Development -> Figma MCP Express** running.
2. Start with `get_metadata`; if multiple files are open, call `list_channels` and pass `channel` on every file-specific top-level tool call. For `batch`, put `channel` on the outer call, not inside `ops[*].params`.
3. If the plugin is not connected, ask the user to open the file and run the plugin. Do not retry in a loop.
4. **Check for enabled (subscribed) libraries before concluding a file has no design system.** `get_variable_defs` returns only the file's **local** variables — it comes back empty in a file that subscribes to an external library, even though that library's tokens are fully available. To see what a file can actually bind, call `list_library_variable_collections` (subscribed collections + keys), then `get_library_variables` per collection key for the variable keys, and `import_variable_by_key` / `import_component_by_key` to bring tokens and components in. Treat an empty `get_variable_defs` as "no *local* tokens," not "no tokens" — confirm with `list_library_variable_collections` first.

## Tool Surface

- `core` (default) profile exposes read/high-value tools plus `batch`, `search_batch_ops`, and `get_batch_op_spec`. Low-level write primitives (`create_frame`, `set_fills`, `import_component_by_key`, …) are NOT top-level tools here — they are `batch` op types.
- `full` profile is compatibility/debug mode for the legacy top-level surface. Set `FIGMA_MCP_TOOL_PROFILE=full` to restore the legacy top-level write tools; set `FIGMA_MCP_TOOL_SCHEMA_MODE=verbose` to restore full in-schema guidance (default is compact).
- For unfamiliar writes: `search_batch_ops` -> `get_batch_op_spec` -> `batch(validateOnly:true)` -> `batch`.
- Do not write raw Plugin API JS, `use_figma`-style scripts, `eval`, or code strings. Use declarative `FigmaPlan` batch ops only.

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
| Batch refs, projection, `map`, validation-first recipes, write-workflow recipes (rename/text-replace/annotations/reactions/swap-overrides) | `references/batch-recipes.md` |
| Connection drops, cache spill files, text/image gotchas | `references/gotchas.md` |
| Parallel agents and channel partitioning | `references/multi-agent.md` |
| Parameterized generators (type scale, color palette, component variants, design tokens) | MCP prompts: `generate_type_scale`, `generate_color_palette`, `generate_component_variants`, `design_token_generation_strategy` — invoke via the MCP prompts list |

Exact operation names and params live in `BatchOpCatalog`; never copy op schemas into skill docs. Use `get_batch_op_spec` for the current schema.

## Cannot Do

- Create new Figma files.
- Work on unopened files through the plugin, except REST-backed `fetch_library_catalog` with `FIGMA_TOKEN`.
- FigJam or Slides.
- Publish libraries.
- Arbitrary script execution.
