# Composition Patterns

Structural and semantic patterns for the most common UI regions. Use these as the starting model before consulting the library for exact component keys and dimensions.

---

## Typography rules (applies everywhere)

- Use **text style references** from the design library — never set font/size/weight manually.
- `textAutoResize = "HEIGHT"` for all body copy, descriptions, and multi-line labels. `"NONE"` is almost always wrong — it clips text silently.
- `textAutoResize = "WIDTH_AND_HEIGHT"` for single-line display text that should shrink the frame to its content.
- **`loadFontAsync()` before any text mutation.** Load all expected font families and styles at the top of the script — before creating any text node. Missing this causes silent failures or crashes mid-script.

```
// Load all fonts used in the script before any text work
await Promise.all([
  figma.loadFontAsync({ family: "YourFont", style: "Regular" }),
  figma.loadFontAsync({ family: "YourFont", style: "Medium" }),
  figma.loadFontAsync({ family: "YourFont", style: "SemiBold" }),
])
```

---

## Navigation patterns

### Top nav / app bar

| Property | Rule |
|---|---|
| Width | `FILL` — spans full screen width |
| Height | `FIXED` — use the library component's defined height |
| Layout | Horizontal auto layout, `SPACE_BETWEEN` or FILL children |
| Left group | Logo/wordmark: `HUG` |
| Right group | Actions/avatar: `HUG` |
| Background | Design variable token — never raw hex |
| Active state | Variant property on the nav component, not a manual fill override |

If the library has a top-nav or app-bar component: import it. Do not build from a raw frame + text + icon.

### Sidebar / left nav

| Property | Rule |
|---|---|
| Width | `FIXED` — use the library component's defined width |
| Height | `FILL` — takes full available screen height |
| Layout | Vertical auto layout |
| Items | `FILL` width inside the sidebar |
| Active state | Variant property, not manual fill override |
| Collapse/expand | Separate component variant or separate component, not a hidden-layer trick |

### Bottom tab bar (mobile)

| Property | Rule |
|---|---|
| Width | `FILL` |
| Height | `FIXED` |
| Layout | Horizontal auto layout, `SPACE_BETWEEN` |
| Each tab | `FILL`, centered icon + label (vertical stack) |
| Active state | Variant property |
| Safe area | Use the library's bottom-safe inset token for `paddingBottom` |

---

## Form patterns

Every form field is a **molecule**: label row + input control + optional helper/error text, stacked vertically in an auto-layout frame. Never collapse this to a bare input with no label.

### Anatomy of a single field

```
FieldGroup (vertical auto layout, gap = field-gap token)
├── LabelRow (horizontal auto layout)
│   ├── FieldNameText  (TEXT node, text style = label style)
│   └── RequiredMark   (TEXT node "*", color = required-color token) ← only when required
├── InputControl       (library component instance — see input-type map below)
└── HelperText         (TEXT node, text style = helper style, hidden by default)
    // OR ErrorText    (same position, error variant visible on error state)
```

### Input type → component mapping

| Visual cue in design | Component to use |
|---|---|
| Plain text entry | Text input / Input field component |
| Chevron / arrow → opens list | Select / Dropdown component |
| Calendar icon → date picker | DatePicker component |
| On/off toggle | Switch / Toggle component |
| Checkbox list | Checkbox group component |
| Radio group | Radio group component |
| Multi-line text | Textarea component |

Never use a text input to fake a dropdown. Never use a raw frame to fake a toggle. Use the component whose semantic role matches the design intent.

### Form states

All states are variant properties — never duplicate frames:

| State | Variant property value |
|---|---|
| Normal / resting | `"Default"` |
| Focused (cursor in field) | `"Focus"` |
| Filled (has value) | `"Filled"` |
| Error | `"Error"` — show ErrorText below the control |
| Disabled | `"Disabled"` — entire FieldGroup, not just the input |

---

## Data display patterns

### Tables

- If the library has a Table component: import it. Do not assemble from Rectangle + Frame rows.
- Use real data in every row — no "Data 1", "Lorem ipsum", or placeholder text.
- Column widths: fixed for known-length content (IDs, dates, status badges); FILL for variable-length text (names, descriptions).
- Every table needs an **empty state** — a design for zero rows. Do not skip it.
- Header: use the library's table-header component or variant; never a plain Frame + bold text.
- Pagination, sorting controls: use library components; never hand-draw chevrons.

### Lists

- Use library list-item or row components where available.
- Consistent row height: FIXED within a list; never let individual rows vary unless the component explicitly supports multi-line rows.
- Dividers between rows: use the library's separator component or a 1px FIXED-height frame with a border-color token fill.
- Empty state: required. Design it.

### Stat / metric displays

- Use a horizontal auto-layout row with FILL children for equal-width stat cells.
- Each cell: vertical auto layout, center-aligned, `HUG` height.
- Metric value: display text style (large, bold). Label: secondary text style (small, muted).
- Separator between cells: 1px FIXED width, FILL height, border-color token — not a raw color.
- Do not use one Card component per stat and line them up manually — this creates rigid, unequal widths.

---

## Modal and overlay patterns

| Property | Rule |
|---|---|
| Overlay background | Semi-transparent fill using a design variable token — never raw `rgba(0,0,0,0.5)` |
| Modal width/height | Follow the library's modal component defined sizes — never invent arbitrary dimensions |
| Modal shell | Library component instance (Dialog, Modal, Sheet) |
| Footer buttons | Library button instances inside the modal's footer slot |
| Close action | Library icon-button instance or the modal's built-in close slot |
| Scroll | If content can overflow: the modal body scrolls, the header/footer stay fixed |

Do not build a modal from a Frame + drop shadow + raw button. Use the library's modal component and configure its slots.

---

## Color and fills (all patterns)

- All fills use design variable tokens: `setBoundVariableForPaint("fills", colorToken)`.
- Dark/light mode: one call on the top-level wrapper sets `setExplicitVariableModeForCollection(collectionId, modeId)`. Variable tokens cascade to every child automatically. Never manually rebind children for dark mode.
- Effects (shadows, blurs): use effect style references from the library — never set raw `boxShadow` values.
- Stroke: bind `strokeWeight` to the library's border-width token; bind stroke color via `setBoundVariableForPaint("strokes", borderToken)`.

---

## Empty states (required for every data surface)

Every table, list, grid, or feed needs an empty state design:
- Illustration or icon (from library)
- Heading: "No [items] yet" or equivalent
- Optional sub-text explaining next action
- Optional primary CTA button

The empty state must be a separate frame or component variant — not a hidden layer toggled by prototype logic. Design it explicitly.
