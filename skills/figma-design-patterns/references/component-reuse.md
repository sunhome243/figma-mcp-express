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

Do not use a detached visual clone to fake a library component. Use batch ops:

1. `import_component_by_key` with a concrete variant component key.
2. `create_instance` with the imported component id.
3. `set_instance_properties` to select the intended variant/state/content.
4. `resize_nodes` if the instance requires a specific size.

Cloning an existing in-file instance to repeat it elsewhere can preserve the instance link, but it is not a substitute for importing a missing library component.

## After Placing an Instance

A fresh instance is not finished. Immediately configure real content, variants, visibility, and dimensions, then verify the node and screenshot the wrapper. Default content such as `Heading`, `Item 1`, or blank slots is a failure.

## Common Mistakes

| Mistake | Consequence | Fix |
|---|---|---|
| Building a table from rectangles when a Table exists | Manual spacing/color/state bugs | Import the Table component |
| First search term missed, then declared gap | Real component missed | Search synonyms and catalog pages |
| Detached visual clone | Library updates no longer propagate | Import and place a real instance |
| COMPONENT_SET key used as variant key | Import fails | Use a concrete default variant component key |
| Default content left visible | Looks unfinished | Configure properties and verify |
