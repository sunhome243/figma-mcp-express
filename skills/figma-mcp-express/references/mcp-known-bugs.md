# MCP Known Bugs — figma-mcp-express server issues

Bugs confirmed in production builds with workarounds. Each has a GitHub issue for tracking.
Check issue status before relying on a workaround — it may be fixed in a newer version.

---

## #33 — `find_replace_text` ignores nodeId scope (PAGE-WIDE)

**Status:** OPEN  
**Issue:** https://github.com/sunhome243/figma-mcp-express/issues/33

When called with a `nodeId` to scope the replacement to one frame, `find_replace_text` traverses
the **entire page and component tree** — replacing the text in the target frame, its master
component, and every other instance on the page.

In 2 production builds this corrupted global tab bar labels, requiring a full batch revert.

**Workaround:** avoid `find_replace_text` entirely.
Use `scan_text_nodes(frameId)` to get specific node ids → `set_text` per node. Always verify
after with a follow-up `scan_text_nodes` to confirm only the intended nodes changed.

---

## #34 — `get_design_context` ignores `nodeId` parameter

**Status:** OPEN  
**Issue:** https://github.com/sunhome243/figma-mcp-express/issues/34

`get_design_context` called with a `nodeId` returns the design context for the **current Figma
selection** instead of the specified node. The nodeId parameter is silently ignored.

**Workaround:** use `get_node(nodeId, depth:N)` directly. It correctly targets the specified
node regardless of current selection. For codegen-level context, use
`get_design_context detail:"codegen"` after first selecting the node via `search_nodes`.

---

## #35 — `create_variable` batch op: validation rejects valid `type` field

**Status:** OPEN  
**Issue:** https://github.com/sunhome243/figma-mcp-express/issues/35

The `create_variable` batch op returns `"ops[0]: missing string 'type'"` even when the `type`
field is present in the payload. Root cause unknown — may be a validation layer checking a
different field path, or a serialization issue with the `type` discriminator conflicting with
the variable-type parameter.

**Workaround:** use existing variables from the design system instead of creating new ones.
`import_variable_by_key` or `get_variable_defs` to find existing tokens. Do not block a
build on variable creation.

---

## #36 — `get_batch_op_spec` returns 0 results for valid ops

**Status:** OPEN  
**Issue:** https://github.com/sunhome243/figma-mcp-express/issues/36

`get_batch_op_spec(op: "set_text")`, `get_batch_op_spec(op: "set_opacity")`, and others return
empty results even though those ops exist in the catalog. `search_batch_ops("text")` correctly
returns `set_text` — so the op exists, but spec lookup by exact name is unreliable.

**Workaround:** use `search_batch_ops` to discover op names, then rely on examples in
`references/batch-recipes.md` for correct parameter shapes. Do not block on a failed spec lookup —
use `batch(validateOnly:true)` with a best-guess payload to get real validation feedback.

---

## #28 — `save_screenshots` silently returns `{succeeded:0, total:0}`

**Status:** OPEN  
**Issue:** https://github.com/sunhome243/figma-mcp-express/issues/28

`save_screenshots` occasionally returns `{succeeded:0,total:0}` with no error and no indication
of which node failed. A transient write/contention issue — not a bad request.

**Workaround:** retry the same call once. If still 0, fall back to `get_screenshot` (in-memory
base64 export). Only escalate if BOTH the retry and `get_screenshot` return nothing — that
indicates a real node/selection problem.

---

## #27 — Fill/paint variable bindings are not readable

**Status:** OPEN  
**Issue:** https://github.com/sunhome243/figma-mcp-express/issues/27

`get_node` and `get_nodes_info` flatten every fill to a resolved hex. A fill bound to a variable
and a hand-typed raw hex of the same color value are byte-identical in the output. Only EFFECT
bindings (shadows) surface `boundVariables`; fills never do.

This means D3 token-binding review **cannot be confirmed by reading** — only by trusting the
write path (`set_fills variableId` / `bind_variable_to_node` both echo the bound variableId on
success).

**Workaround:** the only readable evidence of a raw fill violation is an **off-palette hex** — a
color value not in the project's token spine. Palette-matching values are presumed bound (trust
the write path). Do not fail D3 on palette-matching hex alone. See `references/gotchas.md` for
the full binding-proof caveat.
