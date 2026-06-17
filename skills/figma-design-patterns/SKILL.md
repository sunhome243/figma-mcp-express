---
name: figma-design-patterns
description: Use when building, editing, or reviewing Figma UI layouts, auto layout, components, spacing, typography, states, dark mode, or developer handoff.
---

# Figma Design Patterns

Design-judgment skill — execution goes through figma-mcp-express `batch` ops, not raw Plugin API.

> **SCOPE.** Owns **design craft** (detail in `references/`). Tool-mechanics → `figma-mcp-express`; workflow → `figma-product-build`; keys → `kit-keys.md`.

## Core Rules

1. Structural frames use dynamic auto layout, not fixed-position piles.
2. Spacing, padding, radius, fills, strokes, effects, and text styles come from library tokens/styles.
3. Component priority: library `INSTANCE` → local component → raw structural frame only when no component exists.
4. Configure every placed instance with real content, variants, and dimensions before calling it done.
5. Layer names must be semantic; generated names such as `Frame 47` or `Rectangle 3` are unfinished.
6. Repeating raw structures become a local component and reused instances.
7. Shared chrome (app bar, header, bottom nav) is ONE component instanced everywhere — identical across states/screens.
8. Every child fits its parent's padded box (`FILL`, or width ≤ inner width); bounds past the frame edge are a layout bug.
9. Empty and loading states are designed: centered icon + heading + auto-height body; a skeleton mirrors the real content's shape.
10. An icon carries a semantic color (`muted`/`foreground`/`accent`/status) agreeing with its label; never a default fill.
11. Every element earns its place — cut decorative noise. Restraint is the craft, not "fill the space."
12. **Squint test:** the screen, its PRIMARY action, and its STATE read without the copy; if that collapses when text blurs, the hierarchy is too weak.
13. Size for the input method: TOUCH (mobile/tablet) meets the platform minimum (**≥44×44pt iOS / 48dp**) with generous spacing; POINTER (desktop) may be denser.
14. A control and its label share one vertical center (`counterAxisAlignItems = CENTER`) — never strand a checkbox/icon atop a taller row.

## Reference Router

| Topic | File |
|---|---|
| FILL/HUG/FIXED, WRAP grids, resize test | `references/auto-layout.md` |
| Padding ownership, gap vs padding, spacing tokens | `references/padding-strategy.md` |
| Library-first selection and component search | `references/component-reuse.md` |
| Instance properties, slots, variants, reset behavior | `references/component-usage.md` |
| Navigation, forms, data display, modals, empty states | `references/composition-patterns.md` |
| Final PASS/FAIL gates before handoff | `references/handoff-checklist.md` |

## Stop Flags

- Raw hex/rgb, raw spacing, raw radius, manual shadows, or inline font values.
- Large `itemSpacing` to distribute children; WRAP container with FILL children.
- **Copy** placeholders (`Heading`, `Item 1`, `Slot`, `Lorem ipsum`) — copy is real strings (a missing-asset *image* placeholder is fine).
- Manual substitutes for icons, buttons, inputs, nav, pagination, badges, or dialogs when the library has a component; separate frames for states that should be variants.
- A child spilling past the parent's padded box (overflow / clip); chrome rebuilt differently per state instead of a shared instance.
- Status by color alone (pair with text/icon); a saturated accent flooded as a large active-state background (prefer a tint or slim indicator).
