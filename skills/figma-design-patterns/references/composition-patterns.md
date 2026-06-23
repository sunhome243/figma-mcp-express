# Composition Patterns

Structural patterns for common UI regions. Use `states-and-feedback.md` for color,
status, empty, and loading behavior.

**Quick navigation:** [Reuse](#reuse) · [Typography](#typography) · [Navigation](#navigation) · [Forms](#forms) · [Buttons](#buttons) · [Data display](#data-display) · [Modals](#modals) · [Fixed-height frames](#fixed-height-frames)

---

## Reuse

- Repeated composite units become components plus instances: cards, rows, stat cells,
  message bubbles, bracket nodes, and repeated skeletons.
- Creating the component is not enough; every occurrence must be an instance.
- Shared chrome is one component instance across screens and states.
- State differences belong in variants/properties or the body region, not redrawn
  headers/nav.

## Typography

- Use library text styles; no hand-set font/size/weight.
- Multiline copy uses `textAutoResize:"HEIGHT"`.
- Single-line labels that should hug use `WIDTH_AND_HEIGHT`.
- Declare `fontFamily`, `fontStyle`, and `fontSize` in text ops before changing text.
- Structured facts use rows/line breaks, not one overflowing run.

## Navigation

Shared chrome:

- Header height, side slots, padding, and alignment remain identical across states.
- Centered headers use three zones: fixed left slot, FILL centered title, fixed right
  slot.
- Hide unused side actions with `opacity:0` or an empty space-reserving slot; do not
  remove the slot from flow.
- Fix flawed chrome in the master, not per-screen copies.

Top nav/app bar:

| Property | Rule |
|---|---|
| Width/height | `FILL` width, library `FIXED` height |
| Layout | horizontal auto layout |
| Left/right groups | `HUG` |
| Active state | variant/property |

Sidebar: `FIXED` width, `FILL` height, vertical auto layout, FILL items, variant-backed
active/collapsed states.

Bottom tab bar: `FILL` width, fixed height, horizontal layout, FILL tabs, centered
icon+label, active variant, safe-area token.

## Forms

Every field is a molecule: label row, control, helper/error text.

```
FieldGroup
├── LabelRow
├── InputControl
└── HelperText / ErrorText
```

| Cue | Component |
|---|---|
| Text entry | Text input |
| Chevron/list | Select/dropdown |
| Calendar | Date picker |
| On/off | Switch |
| Checkbox/radio | Checkbox or radio group |
| Multiline | Textarea |

Rules:
- Replace factory placeholders with real localized copy.
- Do not fake controls with raw frames or wrong components.
- Focus/filled/error/disabled are real variants/properties.

## Buttons

- Pick canonical variants/sizes once and reuse them.
- Prefer native HUG height; if touch target is too small, use one uniform touch-safe
  height.
- Set label via component property, never overlaid text.
- Full-width buttons center content on both axes.
- Do not exceed the parent's padded width.

## Data display

Tables:
- Use a table component when available.
- Real data only.
- Fixed widths for predictable fields; FILL for variable text.
- Header component/variant, visible row dividers, aligned columns, real pagination/sort
  controls.
- Empty state required.

Lists:
- Use list/row components.
- Consistent row height unless the component supports multiline rows.
- Token separators.
- Empty state required.

Stats:
- Horizontal row with equal FILL cells.
- Value uses display style; label uses muted secondary style.
- Separators are token-colored 1px frames.

## Modals

| Property | Rule |
|---|---|
| Overlay | design token |
| Shell | Dialog/Modal/Sheet instance |
| Size | library-defined |
| Footer | button instances |
| Close | built-in or icon-button |
| Overflow | body scrolls; header/footer fixed |

Do not build a modal from raw frames, raw shadows, and raw buttons.

## Fixed-height frames

- Required content must fit inside the frame and above pinned bars.
- Keep hero/cover placeholders modest, often around 180-220px on mobile.
- Reserve bottom padding at least equal to pinned CTA/chrome height.
- Scrollable content should be auto-height/scrollable, not fixed shorter than content.
- Containers must fit content plus outside strokes/effects. HUG where possible; fixed
  size only when it includes all visible extents.
- If `clipsContent` is on, verify every edge by screenshot.

## Scroll-ready, prototype-convertible mobile screens

Build tall mobile screens so they convert to a scrolling prototype with **no restructure** later. The rule: the scrolling content lives in ONE vertical content frame; the chrome (top header, bottom tab/CTA dock) is **pinned / scroll-fixed** via `pin_child`.

A screen built this way converts to a 390×844 prototype directly:

- the outer viewport frame **clips content** (`clipsContent`),
- the content frame is set to **vertical overflow scrolling**,
- the pinned chrome is **"fixed when scrolling."**

If instead the regions are scattered at the screen root, or the chrome isn't pinned, "make it scrollable later" becomes a rebuild. Keep content-in-one-frame + chrome-pinned from the start, and reserve `paddingBottom ≥ pinned-CTA height` on the content column so the last region never renders behind the dock (see the pinned-bar note above). Prototype reaction wiring itself → the `figma-prototype` skill.
