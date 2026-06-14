# Auto Layout

Auto layout is mandatory on every structural frame. Presence alone is not enough — the layout must respond correctly when the wrapper width changes. A rigid layout that looks right at one size but breaks at another is wrong.

---

## FILL vs HUG — decision table

| Pattern | Child sizing | Container axis | Notes |
|---|---|---|---|
| N equal-width items in a row (tab bar, stat strip, toolbar) | Each child `FILL` | `HORIZONTAL` | Never HUG children + giant gap to fake spread |
| Sidebar + main content | Sidebar `FIXED`, main `FILL` | `HORIZONTAL` | Sidebar width from library component spec |
| Card grid (N columns) | Each card `FIXED` = formula | `HORIZONTAL` + `WRAP` | See WRAP+FILL formula below |
| Header: logo left, actions right | Both groups `HUG` | `HORIZONTAL` | `primaryAxisAlignItems = "SPACE_BETWEEN"` |
| Section with title + stacked content | Children `FILL` width | `VERTICAL` | Width inherits from parent |
| Full-screen page shell | Content area `FILL` both axes | — | After `appendChild`, before FILL |
| Separator in a flex row | Separator `FIXED` 1px wide, `FILL` height | — | Surrounding items stay FILL |

---

## The resize test (mandatory before DONE)

Mentally (or actually) resize the wrapper to **1200px**, then **1920px**.

Ask at each size:
- Do items clip or overflow? → **layout is rigid**
- Do gaps explode to fill all available space? → **wrong tool — switch children to FILL**
- Does any column stack unexpectedly? → **WRAP+FILL problem — switch to FIXED**
- Does the sidebar change width? → **sidebar must stay FIXED**

If any answer is yes, fix the layout. Do not mark the section DONE first.

---

## `itemSpacing > 48` = fake distribution (antipattern)

```
BAD — faking spread with a huge gap:
  row.layoutMode = "HORIZONTAL"
  row.itemSpacing = 200          // ← looks right at 1440px, breaks at every other width
  // children are HUG

GOOD — real distribution:
  row.layoutMode = "HORIZONTAL"
  row.itemSpacing = 16           // visual rhythm gap only
  childA.layoutSizingHorizontal = "FILL"
  childB.layoutSizingHorizontal = "FILL"
  childC.layoutSizingHorizontal = "FILL"
```

`itemSpacing` sets **visual rhythm** (8 / 12 / 16 / 24 / 32 px) between sibling items. It never distributes space. If you're writing `itemSpacing = 100+` to make items "spread out," you're using the wrong tool — set children to `FILL` instead.

---

## WRAP + FILL collapses to one row (antipattern)

When `layoutWrap = "WRAP"` and children are `FILL`, Figma gives each child the full container width. Every card goes on its own row. Result: a single-column stack that looks like a grid only when 1 card fits per row.

**Fix — compute FIXED card width:**

```
cardWidth = (containerInnerWidth - (columns - 1) * gap) / columns

// Example: 1200px container, 24px padding each side, 16px gap, 3 columns
innerWidth = 1200 - 24 - 24           = 1152
cardWidth  = (1152 - (3-1)*16) / 3    = (1152 - 32) / 3  = 373.3 → round to 373
```

Set each card to `layoutSizingHorizontal = "FIXED"` and `resize(373, cardHeight)`.

---

## `layoutSizingHorizontal/Vertical = "FILL"` must come AFTER `appendChild`

```
// WRONG — sizing set on a parentless node; Figma ignores it silently
const card = figma.createFrame()
card.layoutSizingHorizontal = "FILL"     // ← no-op, card has no parent yet
row.appendChild(card)

// CORRECT
const card = figma.createFrame()
row.appendChild(card)                     // parent established first
card.layoutSizingHorizontal = "FILL"     // ← now takes effect
```

---

## Common mistakes

| Mistake | Symptom | Fix |
|---|---|---|
| Children HUG + `itemSpacing = 150` | Items bunch left at narrow widths | Set children to `FILL`, gap to ≤32 |
| WRAP + FILL children | All cards collapse to single column | Compute FIXED card width with the formula |
| FILL set before `appendChild` | Sizing silently ignored | Always `appendChild` first |
| Sidebar set to FILL | Sidebar grows with page, pushes content | Sidebar always `FIXED` width |
| No auto layout on a wrapper frame | Frame is rigid, doesn't adapt | Add auto layout; set direction + gap |
| `primaryAxisAlignItems = "SPACE_BETWEEN"` with 3+ items | Outer two pinned, middle items float | Use FILL children + small gap instead |
