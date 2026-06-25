# Workflow Recipes

Use this for higher-level Figma maintenance and analysis workflows. For base batch
syntax, refs, `map`, projections, and validation rules, read `batch-recipes.md` first.

**Quick navigation:** [Bulk rename](#bulk-rename) · [Chunked text replacement](#chunked-text-replacement) · [Annotation analysis](#annotation-analysis) · [Transfer overrides](#transfer-overrides)

---

## Bulk rename

Scope first, then scan and rename flagged nodes. Show a preview table before applying.

```json
{
  "ops": [
    { "type": "scan_nodes_by_types", "params": { "nodeId": "<frame-id>", "types": ["FRAME","GROUP","INSTANCE","TEXT","RECTANGLE","ELLIPSE","VECTOR"] } },
    { "type": "batch_rename_nodes", "nodeIds": ["$0.matchingNodes[*].id"], "params": { "pattern": "<Prefix>$n" } }
  ],
  "continueOnError": true
}
```

For per-node names, use `map` with a `rename_node` do-op.

Rules:
- Never rename COMPONENT master nodes; rename instances/frames only.
- Preserve "/" hierarchy separators.
- When unsure, leave the node and flag it.
- Screens use PascalCase; sections use `Section/Name`; text nodes use role names such as `Label`, `Title`, `Body`, `Caption`.

## Chunked text replacement

Do not use `find_replace_text`; bug #33 can traverse the whole page and component
masters. Use `scan_text_nodes` plus per-node `set_text`.

Flow:
1. Scan text nodes in the scoped root.
2. Clone a safe copy of the root.
3. Replace copy per logical chunk with validated `set_text` ops.
4. Verify each chunk with `save_screenshots`.
5. Export a final screenshot at low scale for full-design review.

For varying text per node, use the `map` op instead of a single-node loop.

## Annotation analysis

Read-only. The server reads native annotations with `get_annotations`; no
annotation-write op exists.

```json
{
  "ops": [
    { "type": "get_annotations",     "nodeIds": ["<frame-id>"] },
    { "type": "scan_text_nodes",     "params": { "nodeId": "<frame-id>" } },
    { "type": "scan_nodes_by_types", "params": { "nodeId": "<frame-id>", "types": ["COMPONENT","INSTANCE","FRAME"] } }
  ]
}
```

Match annotations by path, then name terms, then proximity. Report: marker,
description, target ID/name, confidence, evidence. Do not claim annotations were
written unless a future annotation-write op exists and succeeds.

## Transfer overrides

Copy content and property overrides from one instance to one or more target instances.

```json
{
  "ops": [
    { "type": "scan_nodes_by_types", "params": { "nodeId": "<parent-id>", "types": ["INSTANCE"] } },
    { "type": "get_node",            "nodeIds": ["<source-instance-id>"], "params": { "depth": 4 } },
    { "type": "set_text",            "nodeIds": ["<target-text-id>"], "params": { "text": "<content>" } }
  ]
}
```

Flow: identify source/targets, read the source deeply, capture text/property
overrides, apply via batch setters, then verify with `get_node`, design context, or
screenshots. Use `map` when the override pattern is consistent across all targets.
