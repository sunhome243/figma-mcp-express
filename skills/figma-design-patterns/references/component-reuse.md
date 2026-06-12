# Component Reuse

Before writing a single `createFrame`, check whether the library already has a component for that element. Using a library instance is not optional — it is the rule. Raw frames are only for structural shells (wrappers, section containers) that have no library equivalent.

---

## Priority order

```
1. INSTANCE of a library component   ← always prefer this
2. Local COMPONENT (file-level)       ← when the library has no match and the element repeats
3. Structural raw FRAME               ← only for layout containers with no component equivalent
```

Never skip level 1 because importing feels slow or because the component "doesn't quite match." If the library has a Table component, use it. If you build a table from rectangles instead, you own every spacing, color, and hover-state bug forever.

---

## Whole-organism over atom-assembly

If the library has a DataTable component, use it — do not assemble a table from Row + Cell + Header atoms. If the library has a Sidebar or NavigationRail component, import it — do not construct a sidebar from a Frame + Icon + Text nodes.

Whole-organism wins because:
- It carries the correct structure, spacing, and state variants the library team designed
- Dark mode and theming apply automatically via the component's bound variables
- Instance overrides are documented; hand-built equivalents are not

The only exception: the library has atoms but genuinely no organism for this use case (confirmed by exhaustive search, not assumption). In that case, compose from the deepest available atoms, not from scratch.

---

## Search the library before building

Do not assume a component doesn't exist because your first search term didn't match.

Search strategy:
1. `get_local_components` — dump the full component list to disk, then `grep` for candidates
2. Try synonyms: a "dropdown" may be named "select", "combobox", or "picker"; a "sidebar" may be "nav-rail", "left-nav", or "drawer"
3. Search by function, not appearance: the icon you need may be named `customer-service` not `headset`
4. Visually scan icon sets — icon names are often functional, not descriptive

A search miss is not a GAP. A GAP is confirmed only after exhaustive search including synonyms and visual scan.

---

## When to use a variant vs. a separate component

Use **variant property** when:
- The element is the same component in a different state (hover, pressed, disabled, error, selected, active)
- The layout structure does not fundamentally change between states

Use a **separate component** (or separate library component import) when:
- The layout structure fundamentally changes (e.g., collapsed vs. expanded sidebar are different organisms)
- The two elements serve entirely different semantic roles (e.g., primary button vs. icon-only button)

State changes that never warrant a duplicate frame: hover, focus, active, disabled, error — these are always variant properties.

---

## Never clone by appearance

Do not duplicate (`clone_node`) a component instance that looks like what you need. Cloning copies the visual result but severs the library link — the clone becomes a detached frame. Changes to the library component no longer propagate.

Always:
1. `importComponentByKeyAsync(key)` to get the live component reference
2. `component.createInstance()` to place a linked instance

Exception: cloning an existing **in-file instance** to replicate it elsewhere in the same file is acceptable (the instance link is preserved on a clone). This is different from cloning to "fake" a component you haven't imported.

---

## Import mechanics

```
// COMPONENT (not a set — a single variant)
const comp = await figma.components.importComponentByKeyAsync(componentKey)
const inst = comp.createInstance()
parent.appendChild(inst)

// COMPONENT_SET (has multiple variants; import the default variant's key)
const defaultVariantComp = await figma.components.importComponentByKeyAsync(defaultVariantKey)
const inst = defaultVariantComp.createInstance()
parent.appendChild(inst)
// Then set desired variant via setProperties:
inst.setProperties({ "State": "Hover", "Size": "Medium" })
```

`importComponentByKeyAsync` rejects a COMPONENT_SET key — it only accepts a single COMPONENT key. Import the default variant's key, then use `setProperties` to switch variants.

---

## After placing an instance: it is not done

A freshly placed instance shows default content: "Heading", "Item 1", blank slots, wrong dimensions. This is never acceptable in a finished design.

Immediately after `createInstance()`:
1. Call `setProperties()` with real content (real labels, real data, correct variant)
2. Resize to match the design spec: `inst.resize(width, height)`
3. Ask: if someone screenshots this canvas right now, can they immediately recognise what this component is? If not, it is not done.

Full configure-after-instantiate guidance is in `references/component-usage.md`.

---

## Common mistakes

| Mistake | Consequence | Fix |
|---|---|---|
| Building a table from rectangles when a Table component exists | Manual spacing/color/state bugs; no theming | Import the Table component; use it |
| First search term didn't match → assumed GAP | Real component missed; built from scratch unnecessarily | Search synonyms + visual scan before concluding GAP |
| `clone_node` on a visual match | Library link severed; clone is a detached frame | `importComponentByKeyAsync` + `createInstance()` |
| Importing COMPONENT_SET key instead of variant key | `importComponentByKeyAsync` throws "not found" | Use the default variant's own key |
| Leaving default instance content ("Item 1") | Viewer cannot recognise the component; looks unfinished | `setProperties` + `resize` before marking done |
