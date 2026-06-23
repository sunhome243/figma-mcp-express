# Gotchas

Remaining failure modes not covered by structured references. For permanent Figma
API constraints, read `platform-constraints.md`; for tracked server bugs, read
`mcp-known-bugs.md`.

**Quick navigation:** [Writes](#top-level-writes) · [Discovery](#discover-before-concluding) · [Live ids](#live-ids-and-coordinates) · [Text/font](#text-and-fonts) · [Connection](#connection-drops) · [Bindings](#bindings-and-resets) · [Assets](#external-assets)

---

## Top-level writes

In the default `core` profile, low-level writes such as `create_frame`, `set_fills`,
and `import_component_by_key` are batch op types, not top-level MCP tools. Put them
inside `batch(ops:[...])`. Use `FIGMA_MCP_TOOL_PROFILE=full` only for legacy clients
that need the old top-level surface.

## Discover before concluding

Never declare "there is no op for X" from memory. Call
`search_batch_ops("<intent words>")`, then `get_batch_op_spec`, then
`batch(validateOnly:true)`. Common wrong guesses: `delete_node` instead of
`delete_nodes`, `reorder` instead of `reorder_nodes`, or missing `clone_node` /
`reparent_nodes`.

If `get_batch_op_spec` misses a valid op, use `search_batch_ops` plus
`batch(validateOnly:true)`; see bug #36 in `mcp-known-bugs.md`.

## Live ids and coordinates

Before any edit or placement, resolve live data:

1. `search_nodes` by name/scope
2. `get_node` on the returned id
3. read real `absoluteBoundingBox`
4. mutate using live ids and parent-relative coordinates

Do not trust ids or x/y from summaries, briefs, or old screenshots. URL ids use
hyphens; API ids use colons. After rebuilds, delete superseded frames and confirm one
remaining frame by name.

## Spilled responses

`.json` holds nested payloads; `.ndjson` holds flat records. Use `jq` for the nested
tree and `grep`/`jq` over `.ndjson` for field lookup.

## Text and fonts

- `set_text` can restyle without changing text; `text` is optional.
- Use `textAlignHorizontal/Vertical`, `textAutoResize`, line/letter params, and font
  params; never fake alignment with spaces.
- For locked/brand fonts, write with a fallback family, then apply the existing style.
  Start long sessions with `get_fonts`.
- Discrete tool params use server names: `text`, not `characters`; `lineHeightValue`
  + `lineHeightUnit`, not raw `lineHeight`.

## Connection drops

A dropped WebSocket returns an error; it should not hang. Confirm the plugin is open,
wait for reconnect, then retry once. In long sessions, call `list_channels` every
15-20 writes.

## Bindings and resets

`get_node` can flatten paint variables to resolved hex, so palette-matching bound and
raw fills may look identical; see bug #27. Trust validated write paths and catch
off-palette raw values.

Reset instances with `set_instance_properties` + `resetOverrides:true`. Clearing
fills manually adds another override instead of restoring defaults.

Use `get_remote_variable_collection` when you need mode ids for a subscribed remote
collection.

## External assets

The orchestrator should pre-fetch assets and pass local paths or raw markup:

| Asset | Path |
|---|---|
| PNG/JPEG | `batch` op `import_image` with `imagePath` |
| SVG | `batch` op `import_svg` with raw SVG markup |
| Lottie JSON | import a poster PNG; keep `.json` path for handoff |

Use `get_batch_op_spec(op:"import_svg")` before large SVG imports.
