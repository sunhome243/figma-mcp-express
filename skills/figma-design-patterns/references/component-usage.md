# Component Usage

How to correctly configure a placed component instance. Placing an instance is step one — configuring it with real content and the right variant is what makes it done.

---

## Configure-after-instantiate: the full sequence

```
// Step 1 — import and place
const comp = await figma.components.importComponentByKeyAsync(variantKey)
const inst = comp.createInstance()
parent.appendChild(inst)

// Step 2 — set variant properties
inst.setProperties({
  "Size":     "Medium",
  "State":    "Default",
  "HasIcon":  true,
})

// Step 3 — set text and slot content
inst.setProperties({
  "Label":       "Save changes",
  "Icon":        iconComponentRef,   // INSTANCE_SWAP slot
})

// Step 4 — resize to spec dimensions
inst.resize(targetWidth, targetHeight)

// Step 5 — verify: would a viewer recognise what this is?
//   If the screenshot still shows "Button" / "Heading" / "Item 1" → not done
```

Never skip steps 2–5. A default instance with placeholder content is always wrong.

---

## `setProperties` — what it can set

`setProperties` handles three slot types:

| Slot type | What to pass | Example |
|---|---|---|
| `TEXT` | A string | `{ "Label": "Submit" }` |
| `INSTANCE_SWAP` | A component reference (from `importComponentByKeyAsync`) | `{ "Icon": iconComp }` |
| Variant property | A string matching a valid variant value | `{ "State": "Error", "Size": "Large" }` |
| Boolean layer visibility | `true` / `false` | `{ "Show Label#1234": false }` |

Property names are case-sensitive and must exactly match the component's defined property names. Use `get_node` on the component to read the exact property names before calling `setProperties`.

---

## Never `appendChild` on a component instance

```
// WRONG — Figma will either throw or silently corrupt the instance
const badge = badgeComp.createInstance()
const label = figma.createText()
badge.appendChild(label)        // ← cannot add children to an instance

// CORRECT — use setProperties for text and swap slots
const badge = badgeComp.createInstance()
badge.setProperties({ "Label": "Active", "Variant": "Success" })
```

Component instances are sealed. Their internal structure is owned by the component. The only way to change content is through the component's defined properties. Appending raw nodes breaks the instance.

---

## Variants vs. separate frames for state

Use **variant property** for state changes:

```
// CORRECT — state as variant
inst.setProperties({ "State": "Hover" })
inst.setProperties({ "State": "Disabled" })
inst.setProperties({ "State": "Error" })

// WRONG — duplicate frames to represent state
const hoverFrame = figma.createFrame()   // ← manual clone; loses all component benefits
```

Every interactive state (hover, focus, active, pressed, disabled, error, selected) is a variant. Only use separate frames when the layout structure fundamentally changes between states (e.g., a collapsed sidebar is structurally different from an expanded one).

---

## `resetOverrides()` — restoring defaults

When you want to reset an instance back to its component defaults:

```
// CORRECT
instance.resetOverrides()

// WRONG — sets fills to empty array; wipes fills entirely; breaks the instance visually
instance.fills = []
```

`fills = []` removes the fill property — it does not restore the component default. Use `resetOverrides()` to restore all overrides at once, or re-call `setProperties` with the original default values to reset selectively.

---

## Slots: what "default instance = not done" means

Every component has slots — text, icon, image, or nested instance positions. When placed fresh, slots show the component's placeholder content:

| Common placeholder text | Meaning |
|---|---|
| "Heading", "Title", "Label" | TEXT slot — needs real content |
| "Item 1", "Item 1-5" | Repeating slot or legend — set real data or hide |
| "Swap it", "Content" | INSTANCE_SWAP slot — import and assign the real component |
| Blank / empty box | Either an image slot or a hidden element that should be shown/hidden explicitly |

After `setProperties` with real content, take a mental screenshot: can a viewer immediately recognise what this organism represents (DataTable, NavigationBar, ProductCard)? If the answer is "not really," the content is still placeholder — iterate.

---

## Reading component property names before `setProperties`

```
// Use get_node on the component (not the instance) to read defined properties
// Look for "componentPropertyDefinitions" in the response
// Each key is the exact property name to pass to setProperties

// Example response excerpt:
{
  "componentPropertyDefinitions": {
    "Label": { "type": "TEXT", "defaultValue": "Button" },
    "State": { "type": "VARIANT", "variantOptions": ["Default","Hover","Disabled","Error"] },
    "HasIcon": { "type": "BOOLEAN", "defaultValue": false }
  }
}
// → setProperties({ "Label": "...", "State": "Hover", "HasIcon": true })
```

Property names are exact-match. "label", "LABEL", and "Label" are different. Always read the component definition before guessing.

---

## Dark mode and theming: no manual fill overrides

```
// WRONG — manually overriding fills on an instance child
child.fills = [{ type: "SOLID", color: { r: 0.1, g: 0.1, b: 0.1 } }]

// CORRECT — set the mode on the wrapper once; tokens cascade automatically
wrapper.setExplicitVariableModeForCollection(colorCollectionId, darkModeId)
```

Manual fill overrides on instance children break variable-driven theming. When the mode switches, the manually overridden fills stay behind (they are no longer bound to the token). Set the mode at the wrapper level; every component instance inside it updates correctly.

---

## Common mistakes

| Mistake | Consequence | Fix |
|---|---|---|
| Default instance left with "Heading"/"Item 1" | Viewer cannot recognise the component | `setProperties` with real content + `resize` |
| `appendChild` on an instance | Throws or silently corrupts | Use `setProperties` for all slot content |
| `fills = []` to "reset" | Wipes fills; instance looks broken | `instance.resetOverrides()` |
| Manual color override on instance child | Theming breaks when mode switches | Set mode on wrapper; never override fills manually |
| Wrong property name case in `setProperties` | Silent no-op; default placeholder stays | Read `componentPropertyDefinitions` first |
| `COMPONENT_SET` key passed to `importComponentByKeyAsync` | "not found" error | Use the default variant's own COMPONENT key |
