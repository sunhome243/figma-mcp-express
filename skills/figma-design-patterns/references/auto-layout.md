# Auto Layout

Every structural frame needs real auto layout, not fixed coordinates that only
look right at one wrapper width.

**Quick navigation:** [Sizing](#sizing) · [Resize test](#resize-test) · [Grid and wrap](#grid-and-wrap) · [Floating children](#floating-children-inside-auto-layout) · [Mistakes](#mistakes)

---

## Sizing

| Pattern | Child sizing | Container |
|---|---|---|
| Equal-width tabs, stats, toolbar items | children `FILL` | `HORIZONTAL`, small gap |
| Sidebar + main | sidebar `FIXED`, main `FILL` | `HORIZONTAL` |
| Header logo left, actions right | groups `HUG` | `SPACE_BETWEEN` |
| Section stack | children `FILL` width | `VERTICAL` |
| Separator in row | `FIXED` width, `FILL` height | row stays stable |

`itemSpacing` is visual rhythm only: usually 8 / 12 / 16 / 24 / 32. If you need
`itemSpacing` above 48 to spread items, set children to `FILL` instead.

Set FILL sizing after placement. A parentless node cannot meaningfully fill its
future parent: create/place it first, then use `resize_nodes` with
`layoutSizingHorizontal:"FILL"` or `layoutSizingVertical:"FILL"`.

Responsive bounds: `minWidth` / `maxWidth` / `minHeight` / `maxHeight` are
settable on frames (`set_auto_layout`) and on auto-layout children
(`resize_nodes`). Use `maxWidth` to cap a FILL content column; pass `null` to
clear a bound.

## Resize test

Before DONE, resize mentally or actually to **1200px** and **1920px**.

| Failure | Meaning | Fix |
|---|---|---|
| Items clip or overflow | rigid layout | add auto layout/FILL/bounds |
| Gaps explode | fake distribution | FILL children, small gap |
| Cards stack unexpectedly | WRAP+FILL trap | fixed card width or GRID |
| Sidebar changes width | wrong sizing | sidebar stays FIXED |

## Grid and wrap

For fixed-count card grids, prefer native GRID when available:
`layoutMode:"GRID"`, row/column counts, row/column gaps, and gap variable IDs.
Verify the result because older Figma hosts may no-op GRID.

For WRAP grids, do not use FILL children. Compute fixed card width:

```text
cardWidth = (containerInnerWidth - (columns - 1) * gap) / columns
```

## Floating children inside auto-layout

Use `layoutPositioning:"ABSOLUTE"` for decorative or floating children that must
sit outside the flow inside an auto-layout parent: aurora blobs, floating glass
tab bars, absolute badges, or overlays inside a composed screen.

Pattern:

1. Create/place the child under the auto-layout parent.
2. Use `resize_nodes` on the child with `layoutPositioning:"ABSOLUTE"`.
3. Position it with move/constraints.
4. Fix z-order if needed.

Do **not** use `pin_child` for this. `pin_child` is a prototype-scroll helper:
it sets ABSOLUTE, reorders into the fixed-children band, and increments fixed
child count. That is right for sticky scroll chrome, not for decoration or a
floating tab bar that must stay visually on top.

Figma limitation: x/y cannot bind to variables. Safe-area and spacing tokens can
drive padding/size/gaps, but a floating child's absolute coordinates remain
numeric. For floating chrome, document the safe-area constants and use
constraints to prevent drift; do not promise token-bound x/y.

## Mistakes

| Mistake | Fix |
|---|---|
| HUG children + giant `itemSpacing` | FILL children + small gap |
| WRAP + FILL cards | GRID or computed FIXED card width |
| FILL before placement | Place first, then `resize_nodes` |
| Sidebar set to FILL | Keep sidebar FIXED |
| Wrapper has no auto layout | Add direction, padding, gap, child sizing |
| `SPACE_BETWEEN` with many siblings | Use FILL children + small gap |
| `pin_child` for decoration/chrome | Use `layoutPositioning:"ABSOLUTE"` via `resize_nodes` |
