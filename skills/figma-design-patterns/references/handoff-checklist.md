# Handoff Checklist

Run before marking a section or screen DONE. Production-level Figma design means
native features, not visual lookalikes.

**Quick navigation:** [Gates](#gates) · [Pass summary](#pass-summary)

---

## Gates

### 1. Responsive layout

Resize to the target extremes, usually 1200px and 1920px for desktop or the device
range for mobile.

FAIL if any item clips, overflows, stacks unexpectedly, explodes its gap, changes a
fixed sidebar width, or truncates body text that should wrap. Fix with auto layout,
FILL/HUG/FIXED, GRID/WRAP discipline, and `textAutoResize:"HEIGHT"` before moving on.

### 2. Figma-native production features

Use the strongest native Figma feature for the job:

| Need | Production path |
|---|---|
| Reusable UI | library instance, or local component + instances |
| State | component variant/property, not duplicate frames |
| Theme/spacing/radius/color | variables, modes, text/paint/effect styles |
| Responsive structure | auto layout, GRID/WRAP, constraints, min/max sizing |
| Floating chrome/decoration | `resize_nodes` with `layoutPositioning:"ABSOLUTE"` + constraints/z-order |
| Prototype behavior | `set_reactions`, `set_prototype_start`, scroll/fixed-child ops |
| Handoff notes | annotations/dev resources where useful |

Raw frame approximations fail when a component, variant, style, variable, prototype,
or annotation/dev-resource feature exists and fits the intent.

### 3. No placeholder copy

Visible text must be real product copy. FAIL strings include `Title`, `Heading`,
`Label`, `Item 1`, `Slot`, `Content`, `Button`, default `Submit`, `Lorem ipsum`, or
blank required slots. Fix instance slots with `set_instance_properties`.

Image/asset placeholders are different: a neutral thumbnail/map/logo placeholder is
acceptable when the source asset is missing, as long as the layout reserves the real
asset's space.

### 4. No hardcoded design values

Spacing, color, stroke, radius, effects, and typography come from variables/styles.
Check `boundVariables`, style ids, and mode inheritance.

Allowed exception: if the library truly has no spacing variables, documented scale
integers such as 4/8/12/16/24/32 may be used and noted. This does not allow raw
colors, arbitrary radii, manual shadows, or inline font systems.

### 5. Library/component coverage

Interactive and semantic UI should be instances: nav/app bars, sidebars, buttons,
inputs, selects, icon buttons, icons, modals, pagination, badges, and status chips.
Use local components only after library search/synonyms confirm no suitable asset.

### 6. State completeness

Buttons, inputs, dropdowns, rows, tabs, checkboxes/radios, toggles, and async surfaces
need applicable default/hover/pressed/focus/filled/error/disabled/loading/empty states.
States are component variants/properties or explicit screen states, not ad hoc visual
overrides.

### 7. Theme and accessibility

Apply the dark/light variable mode on the wrapper and screenshot it. FAIL if a light
fill bleeds through, text loses contrast, border disappears, or icon/status color does
not match its label/meaning. Status must read by text/icon plus color, never color
alone.

### 8. Layer names, containment, chrome

- Semantic layer names only: no `Frame 47`, `Group 12`, `Rectangle 3`, generic `Text`.
- Children stay inside the parent's padded box; no clipped strokes, shadows, glyphs,
  skeleton bars, cards, or fixed bars.
- Shared chrome is the same component instance across default/loading/empty/error and
  across matching screens. Body state changes do not redraw the header/tab bar.
- Empty/loading states are centered, padded, and shaped like the real content.
- Element necessity — no redundant restatement: every element adds info its neighbours
  don't already carry. A badge/pill/label that restates the headline, an adjacent value,
  the icon, or the screen's own context is a FAIL. Decide whether it *belongs* before
  judging how it looks (rule 10 / PC13).

## Pass summary

```
[ ] Responsive at target extremes
[ ] Native Figma features used, no visual lookalikes
[ ] Real copy, asset placeholders only when source asset is missing
[ ] Variables/styles/modes for design values
[ ] Library/local components and instances cover reusable UI
[ ] States complete and variant/property-backed
[ ] Dark/light mode and status/icon accessibility pass
[ ] Semantic names, containment, and shared chrome pass
```

Anything unchecked means the section is not done.
