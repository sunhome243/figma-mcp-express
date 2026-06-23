# Component Reuse

Before creating raw structure, check whether the library already has a component for that element. Library instances are the default; raw frames are only for structural shells with no component equivalent.

---

## Priority Order

```
1. Library INSTANCE
2. Local COMPONENT
3. Structural raw FRAME
```

Never skip the library because importing feels slow or because the first search term misses. If the library has a Table, Sidebar, NavigationRail, Dialog, Button, Input, Icon, or Badge component, use it.

## Whole Organism Over Atom Assembly

Prefer the deepest available component. A DataTable organism beats hand-assembled Row and Cell atoms. A Sidebar organism beats a frame with icon/text rows.

Whole organisms carry:
- Library-approved spacing and state structure.
- Bound variables for theming.
- Supported instance properties instead of improvised overrides.

Only assemble from atoms after exhaustive search confirms there is no organism for the use case.

## Search Before Building

1. Use `get_local_components` or `fetch_library_catalog`, then search the saved catalog.
2. Try synonyms: dropdown/select/combobox, sidebar/nav-rail/drawer, badge/chip/status.
3. Search by role, not only visual appearance.
4. For icons, scope to the icon page and visually scan names when necessary.

A search miss is not a gap. A gap is confirmed only after synonyms and visual scan.

## Variant vs Separate Component

Use a variant property when the same component changes state: hover, pressed, disabled, error, selected, active.

Use a separate component when structure or semantic role changes: collapsed vs expanded navigation, primary button vs icon-only button, dialog vs sheet.

## Never Clone By Appearance

Do not use a detached visual clone to fake a library component. Use these **`batch` op types** (chained in one `batch(ops:[…])` call via `$N.id` refs — they are not top-level tools in `core`):

1. `import_component_by_key` with a concrete variant component key.
2. `create_instance` with the imported component id (`$0.id`).
3. `set_instance_properties` to select the intended variant/state/content.
4. `resize_nodes` if the instance requires a specific size.

Cloning an existing in-file instance to repeat it elsewhere can preserve the instance link, but it is not a substitute for importing a missing library component.

## Reusing a File-Local Component (no published key)

A component you `create_component` in the current file has **no published library key** — so you cannot re-instance it with `create_instance {componentKey}` (that errors: `componentKey` alone does not satisfy the required `componentId`). But `create_instance`'s required `componentId` accepts **any COMPONENT node id**, including a file-local master's. So a local organism IS reusable — you just have to carry its master node id.

**Ledger discipline (do this every time you create a shared local component):**
1. When `create_component` returns, it gives you the new master's **node id**. Record it immediately in the build contract/ledger under a stable name, e.g. `localComponents: { "AppHeaderBack": "19:17384", "SkeletonCard": "19:15652" }`.
2. Any later screen that reuses that organism reads the ledger and calls `create_instance {componentId: "<recorded-master-id>"}` — never rebuilds a look-alike, never copy-pastes the master's frames.
3. The orchestrator passes the shared master ids into each builder's brief so partitioned builders don't double-create.

Why this matters: the best path is still capture-at-creation, because it avoids a broad recovery scan and gives every builder the same stable master id up front. If the ledger is missing, recover in this order:

1. Read the existing instance with `get_node` or `get_nodes_info`; if its master resolves, use the top-level `mainComponentId`.
2. If the instance is gone or detached, call unscoped `get_local_components` and search the whole-file component catalog for the local master id. Do not pass `pageId` for this recovery pass; `pageId` is the bounded one-page scan for large libraries.
3. Once recovered, add the id back to the build contract/ledger before creating more instances.

## After Placing an Instance

A fresh instance is not finished. Immediately configure real content, variants, visibility, and dimensions, then verify the node and screenshot the wrapper. Default content such as `Heading`, `Item 1`, or blank slots is a failure.

## Common Mistakes

| Mistake | Consequence | Fix |
|---|---|---|
| Building a table from rectangles when a Table exists | Manual spacing/color/state bugs | Import the Table component |
| First search term missed, then declared gap | Real component missed | Search synonyms and catalog pages |
| Detached visual clone | Library updates no longer propagate | Import and place a real instance |
| COMPONENT_SET key used without type handling | Import may probe the wrong route first | Pass `assetType:"COMPONENT_SET"` or use a concrete default variant component key |
| Default content left visible | Looks unfinished | Configure properties and verify |

## Keep the component page tidy

Every component you register lives on the dedicated component section/page, organized and kept tidy — grouped by kind, laid out in a clean non-overlapping grid, semantically named. Registering a component means **moving the master INTO that section and positioning it cleanly** — never leaving a master loose on a screen, orphaned, or scattered on the canvas.

The component page is the library's source of truth: other builders read it to discover and reuse. A messy or scattered component page rots the system and makes reuse-vs-rebuild decisions unreliable — a master buried on a random screen won't be found, so it gets rebuilt as a look-alike (the exact failure "whole organism over atom" is meant to prevent).
