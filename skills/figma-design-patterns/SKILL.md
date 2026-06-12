---
name: figma-design-patterns
description: >-
  Use when building, editing, or reviewing any Figma UI — layouts, auto layout,
  components, spacing, typography, states, dark mode, or developer handoff.
  Load proactively before any design build task.
---

# Figma Design Patterns

## CORE RULES

Five rules that must hold on every frame, every build:

1. **Auto layout is mandatory on every structural frame** — presence is not enough; it must be dynamic. A layout that breaks when you resize the wrapper is wrong. → `references/auto-layout.md`

2. **Spacing and padding must be tokens, never raw px** — `frame.paddingLeft = 24` is always wrong. Container padding vs. gap are different concepts; each is owned at a specific level. → `references/padding-strategy.md`

3. **Component-first, always** — priority order: INSTANCE of library component > local COMPONENT > structural raw FRAME. Raw frames are only for layout shells with no library equivalent. → `references/component-reuse.md`

4. **Fills, strokes, and colors = design variables** — no hex, no rgb. `setBoundVariableForPaint("fills", token)`. Variable-driven color is how dark mode works correctly.

5. **Name layers semantically** — `Button/Primary/Default`, `Icon/chevron-right`, `Card/Product`. Never `Frame 47`, `Group 12`, `Rectangle 3`.

6. **Create local component** — using the same structure over and over again? Create a new local component with library or existing components to create a new component and reuse them instead of manual raw copy and paste

---

## REFERENCE ROUTER

| Topic                                                                     | File                                 | Read when...                                                      |
| ------------------------------------------------------------------------- | ------------------------------------ | ----------------------------------------------------------------- |
| FILL vs HUG, WRAP+FILL, `itemSpacing>48`, resize test                     | `references/auto-layout.md`          | Building or reviewing any auto-layout frame                       |
| Padding ownership, gap vs padding, level discipline, token binding        | `references/padding-strategy.md`     | Placing padding or gap on any frame                               |
| Instance priority, whole-organism rule, when to import                    | `references/component-reuse.md`      | Choosing whether to use a library component or build from scratch |
| `setProperties`, variants, configure-after-instantiate, slots             | `references/component-usage.md`      | Configuring or customising a placed instance                      |
| Nav/sidebar/forms/data-display/modals patterns                            | `references/composition-patterns.md` | Building a complete screen or UI section                          |
| Resize · no placeholder · token coverage · state completeness · dark mode | `references/handoff-checklist.md`    | Before marking any section or screen DONE                         |

> **Typography and color mechanics** live in `references/composition-patterns.md` (typography in form/data sections) and `references/handoff-checklist.md` (dark-mode gate). The principles — text style refs, `textAutoResize="HEIGHT"`, `loadFontAsync()` before mutations, `setBoundVariableForPaint` for all fills — are part of the core rules above and expanded in context in those references.

---

## QUICK ANTI-PATTERN FLAGS

Stop immediately if you see any of these — they are always wrong:

| Anti-pattern                                       | Fix                                                                   |
| -------------------------------------------------- | --------------------------------------------------------------------- |
| `frame.paddingLeft = N` (raw px)                   | `frame.setBoundVariable("paddingLeft", spacingToken)`                 |
| `itemSpacing > 48`                                 | Children should be `FILL`; large gap ≠ distribution                   |
| WRAP layout + FILL children                        | Children must be `FIXED` width: `(W − 2P − (N−1)G) / N`               |
| `instance.appendChild(child)`                      | Use `instance.setProperties({...})` for TEXT/INSTANCE_SWAP slots      |
| `instance.fills = []` to "reset"                   | Use `instance.resetOverrides()`                                       |
| Hex or rgb fill on any node                        | `setBoundVariableForPaint("fills", colorToken)`                       |
| Default instance with "Heading" / "Item 1" visible | Call `setProperties()` with real content + resize before marking done |
| Separate frame for hover/disabled/error state      | Variant property on the instance, not a duplicate frame               |
