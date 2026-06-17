# Platform Constraints — Figma Plugin API permanent limits

These are Figma Plugin API design decisions, not figma-mcp-express bugs. They will not be "fixed" —
work with them. Each entry: **what you can't do → why → what to do instead**.

---

## Instance children: geometry is locked

You cannot reposition, resize, or reparent a child node that lives inside a COMPONENT INSTANCE.
`move_nodes` on an instance child returns an error; `set_auto_layout` on it is also blocked.
This is a Plugin API hard limit — instances are "sealed" copies of their master.

**What to do:** edit the MASTER component (find it via `mainComponentId` on the instance, then
`get_node` to confirm it's the master). Changes propagate to all instances. If you need
ONE instance to differ, use `set_instance_properties` (text, visible, instance-swap) — those
are the approved per-instance override paths. Geometry overrides are not supported.

---

## Auto-layout children: x/y is parent-controlled, move_nodes silently no-ops

When a FRAME has `layoutMode: VERTICAL | HORIZONTAL`, its children's positions are calculated
by the parent. Calling `move_nodes` on a child inside such a frame succeeds (returns ok) but
the position never changes — the auto-layout recalculates it on the next layout pass.

This is the same reason safe area cannot be applied by moving an AppHeader to y:59 — if the
screen frame is auto-layout, the header's y is owned by the parent.

**What to do:** adjust the **parent's** padding or gap instead:
```
set_auto_layout(screenFrameId, paddingTop: 59, paddingBottom: 34)
```
The children reflow automatically. To place something at an absolute position, it must either
be in a non-auto-layout parent, or use `position: "ABSOLUTE"` within an auto-layout container
(only supported in certain Figma versions).

---

## clone_node: all child IDs change

After `clone_node`, the cloned frame and every node inside it receive NEW ids. The source frame's
child ids are NOT re-used. Any id you read before the clone is stale the moment the clone exists.

**What to do:** after `clone_node`, immediately call `get_node(cloneId, depth:N)` and rebuild
your id map from the live response. Never feed pre-clone ids into post-clone batch ops.

---

## Component variant visibility: auto-layout recalculates, causes title drift

When a component variant has auto-layout and one of its children is hidden (via a property like
`"Share Icon=false"`), the auto-layout recalculates positions for the remaining visible children.
Fresh instances of that variant inherit the recalculated layout.

Real example: AppHeaderBack with `share-2` icon hidden → title shifts from x:48 to x:80 because
the icon's space collapses and the auto-layout redistributes.

**What to do:** fix the MASTER component's auto-layout rules so the title stays centered
regardless of icon visibility (use `primaryAxisAlignItems: SPACE_BETWEEN` or a fixed-width
spacer). Re-instance after fixing the master.

---

## Editing text inside an INSTANCE: compound ids

Instance children have compound ids in the form `I{instanceId};{innerId}`
(e.g. `I2167:9091;186:1579`). These are the ids you need to call `set_text` on the right node.

**What to do:** prefer `set_instance_properties` when the text is an exposed property
(`Label#…`, `Title#…` etc.) — it survives component swaps. Otherwise, `get_node(instanceId, depth:6)`
to find the compound id, then `set_text` on it. Always load the font before `set_text`.

---

## COMPONENT nodes fully support auto-layout

A common false belief: "`set_auto_layout` doesn't work on COMPONENT nodes." Wrong. COMPONENT
and COMPONENT_SET nodes support `layoutMode` exactly like FRAME. Set auto-layout on the master
so instances reflow correctly. If a layout op seems to fail on a component, check the params
(tool uses its OWN names, not Plugin API names) — the node TYPE is not the blocker.

---

## Community UI kit components are unpublished

"Published to Community" (the file is duplicatable) ≠ "published as a library" (components
importable by key). Community kit files are only the former — their components are unpublished
local nodes. `import_component_by_key` fails with "Cannot import … since it is unpublished."

Proven: REST `fetch_library_catalog` returning 404/0 is NOT the arbiter. After a user publishes
a file as a library, REST may still return 404, yet live `import_component_by_key` into a
subscribed target succeeds. Always do one live probe before concluding a key is unimportable.

**What to do:** `get_local_components(pageId)` → `import_component_by_key` probe into the
target channel. If `remote:true` → `create_instance` works. If unpublished → either publish
the file as a library (Assets panel, paid plan) or one-time copy-paste (detached local copy).

---

## Per-instance icon variation: nested-swap is the only path

You cannot add, remove, or reparent a child INSIDE an INSTANCE (or a frame nested in one).
The only per-instance override paths are: text content, fill/opacity, visibility, instance-swap.

To give N instances of one master different nested icons:
1. Edit the MASTER once: place a default icon instance in the target sub-frame.
2. Per instance: `swap_component` the nested slot (compound id `I<instance>;<masterIconId>`).
3. Recolor LAST: `scan_nodes_by_types(container, ["VECTOR"])` → bulk `set_strokes`.
   Lucide/shadcn icons are STROKED, not filled.

Note: `"Removing this node is not allowed"` means the node backs a component property —
remove the property binding first, or use `set_visible:false` as a suppression override.

---

## Dark mode: set_variable_mode cascades DOWN from the node it's set on

`set_variable_mode` applies the token mode to all descendants of the target node. If you call it
on a child frame, only that subtree switches mode — siblings and ancestors stay in their current
mode. Setting it on a mid-level frame while the screen wrapper remains in light mode = bleed-through.

**What to do:** set `set_variable_mode` once on the outermost wrapper frame of the screen.
Everything underneath inherits the mode.

---

## Slow import: bounded queue, self-clears — not a permanent jam

A valid-looking but blocked key (unpublished, wrong library, permission issue) can reach the
Plugin API and wait for its import timeout. Calls queued behind it also wait. This is NOT a
permanent channel jam — the queue drains on its own once the timeout fires.

**What to do:** validate the key with `get_local_components` or `fetch_library_catalog` BEFORE
importing. Do not loop-retry on a stuck import — each retry queues another timeout window.
Wait for the queue to drain, then retry once with a confirmed key.
