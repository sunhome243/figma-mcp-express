# Padding Strategy

Padding and gap are the two mechanisms that control internal spacing in auto layout. They are **not interchangeable** — using the wrong one at the wrong level is the most common cause of layouts that look right once and break on resize.

---

## Container padding vs. item gap — what each one does

| Concept | API property | Controls | Level |
|---|---|---|---|
| **Container padding** | `paddingLeft/Right/Top/Bottom` | Space between the frame edge and its children | The frame that owns the children |
| **Item gap** | `itemSpacing` | Space between adjacent siblings inside an auto-layout frame | Same frame that has `layoutMode` set |
| **Counter-axis spacing** | `counterAxisSpacing` | Gap in the cross-axis direction (WRAP grids) | Same frame, only when `layoutWrap = "WRAP"` |

They live on the **same frame** but serve completely different purposes. Do not use `itemSpacing` to create breathing room inside a card — that is the card's own `paddingLeft/Right/Top/Bottom`.

---

## Which level owns spacing

Every spacing value belongs to exactly one frame. Ownership follows a single rule: **the frame that directly parents the spaced content owns that space.**

```
Page
└── Screen (wrapper)          → paddingLeft/Right = screen-edge inset token
    └── ContentArea           → itemSpacing = section-gap token
        ├── Header            → paddingLeft/Right/Top/Bottom = component internal padding
        │   └── ...children
        └── CardGrid          → itemSpacing = card-gap token, paddingLeft/Right = grid-inset token
            ├── Card          → paddingLeft/Right/Top/Bottom = card internal padding
            └── Card
```

Violations occur when a child tries to create its own external margin (Figma has no margin), or when a parent sets enormous `itemSpacing` to push children apart instead of setting children to `FILL`.

---

## Binding every padding/gap to a token (mandatory)

Raw integers for spacing are never correct. Every padding and gap property must be bound to a spacing variable from the design library.

```
// WRONG — raw px hardcoded
frame.paddingLeft   = 24
frame.paddingRight  = 24
frame.paddingTop    = 16
frame.paddingBottom = 16
frame.itemSpacing   = 12

// CORRECT — every value bound to a token
const sp6  = await figma.variables.importVariableByKeyAsync("your-library/spacing/large")
const sp4  = await figma.variables.importVariableByKeyAsync("your-library/spacing/medium")
const sp3  = await figma.variables.importVariableByKeyAsync("your-library/spacing/small")

frame.setBoundVariable("paddingLeft",   sp6)
frame.setBoundVariable("paddingRight",  sp6)
frame.setBoundVariable("paddingTop",    sp4)
frame.setBoundVariable("paddingBottom", sp4)
frame.setBoundVariable("itemSpacing",   sp3)
```

Import the spacing variables **once at the top of the script**, before creating any frame that will use them. Waiting until you need them means you'll be tempted to hardcode a fallback.

---

## Spacing scale discipline

Use only the token values your design library provides. Round to the nearest available token — never invent a value between steps.

Generic scale pattern (adapt to your library's actual tokens):

| Semantic name | Typical value | Use for |
|---|---|---|
| `spacing/xs` | 4 px | Tight inline gaps (icon + text label) |
| `spacing/sm` | 8 px | Chip + text, dense list rows |
| `spacing/md` | 12–16 px | Standard row item gap, card internal padding top/bottom |
| `spacing/lg` | 24 px | Section padding, panel-to-panel gap, card-to-card gap |
| `spacing/xl` | 32–40 px | Page-level section breaks |

If your library has no token near the value you need, go to the next available token up or down. If that still looks wrong, the problem is usually the layout model (FILL vs HUG) not the spacing value.

There is no 20px, 28px, or other between-step value in most scales — do not invent one.

---

## How consistent padding ownership makes resize survive

When every frame owns only its own internal padding and gap, resizing the wrapper only changes the frame that explicitly uses `FILL`. Nothing else shifts. The predictability comes from ownership clarity:

```
// Resize-safe structure: wrapper changes width → only ContentArea FILL adapts
Screen (1440px → 1200px)
  paddingLeft = sp6 token    ← stays 24px regardless of wrapper width
  paddingRight = sp6 token   ← stays 24px
  └── ContentArea (FILL)     ← absorbs the width change
      itemSpacing = sp4 token ← stays fixed gap
      ├── LeftPanel (FIXED)   ← stays at its fixed width
      └── MainArea (FILL)     ← absorbs remaining space

// Resize-broken structure: multiple frames with hardcoded padding fight over space
Screen (1440px)
  └── ContentArea
      paddingLeft  = 120   ← arbitrary; was "centered at 1440" only
      paddingRight = 120   ← same
```

Fixed padding tokens stay constant. FILL children absorb the width. This is why tokens + FILL children is the only combination that is truly resize-safe.

---

## Worked examples: good vs. bad padding placement

### Example 1 — Card internal padding

```
BAD: Card frame has no padding, child text node has paddingLeft
  Card (frame, auto layout vertical, NO padding)
  └── TitleText (paddingLeft = 16)   ← text nodes have no padding property; this is a layout hack

GOOD: Card owns its padding; children have none
  Card (frame, auto layout vertical, paddingLeft/Right/Top/Bottom bound to card-padding token)
  └── TitleText (no padding — inherits visual inset from Card)
  └── DescText  (no padding)
```

### Example 2 — Page-level section gap

```
BAD: Each section has marginTop = 40 (Figma has no margin; this is actually paddingTop on the section)
  Page
  └── Section1 (paddingTop = 40)   ← the first section shouldn't have top padding
  └── Section2 (paddingTop = 40)   ← now spacing is uneven at top vs between

GOOD: The page wrapper owns the gap between sections
  Page (auto layout vertical, itemSpacing bound to section-gap token)
  └── Section1 (no padding for external spacing)
  └── Section2 (no padding for external spacing)
  // Each section still has its own internal paddingLeft/Right for content inset
```

### Example 3 — Grid inset vs. card gap (two separate tokens)

```
CORRECT — two distinct concerns, two distinct bindings:
  CardGrid
    paddingLeft  = gridInsetToken   // space between grid edge and first card
    paddingRight = gridInsetToken
    itemSpacing  = cardGapToken     // space between cards

  Do NOT set paddingLeft on individual Card children to fake the inset.
  Do NOT set itemSpacing on CardGrid to a massive value to fake distribution.
```

---

## Common mistakes

| Mistake | Symptom | Fix |
|---|---|---|
| `frame.paddingLeft = 24` | Hardcoded spacing breaks token theming + dark mode | `setBoundVariable("paddingLeft", token)` |
| `itemSpacing` used for card-internal breathing room | Content crowds the card edge | Use `paddingLeft/Right/Top/Bottom` on the card frame |
| Container padding on the child instead of the parent | Spacing disappears or doubles on resize | Move padding to the direct parent |
| `itemSpacing = 200` on a HUG-child row | Looks spread at one width, broken at all others | Children to FILL + small gap token |
| Different padding values at same semantic level | Inconsistent spacing across the design | Create and reuse one token per semantic role |

---

## Cross-reference

Gap values (the `itemSpacing` part of this reference) are also covered from the auto-layout angle in `references/auto-layout.md`. That file covers FILL/HUG decisions and the resize test. This file covers ownership, level discipline, and token binding — read both for a complete picture.
