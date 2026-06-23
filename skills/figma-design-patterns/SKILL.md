---
name: figma-design-patterns
description: Use when building, editing, or reviewing Figma UI layouts, auto layout, components, spacing, typography, states, dark mode, or developer handoff.
---

# Figma Design Patterns

Design-judgment skill — execution goes through figma-mcp-express `batch` ops, not raw Plugin API.

> **SCOPE.** Owns **design craft** (detail in `references/`). Tool-mechanics → `figma-mcp-express`; workflow → `figma-product-build`.

## Core Rules

1. Structural frames use dynamic auto layout, not fixed-position piles.
2. Spacing, padding, radius, fills, strokes, effects, and text come from library tokens/styles.
3. Component priority: library `INSTANCE` → local component → raw frame only if none exists.
4. Configure every instance with real content, variants, and dimensions before done.
5. Layer names must be semantic; auto-generated (`Frame 47`) and cryptic shorthand (`t`, `lbl`) are unfinished — name as you create.
6. Repeating raw structures become a local component and reused instances.
7. Shared chrome (app bar, header, nav) is ONE component instanced everywhere — identical across states.
8. Every child fits its parent's padded box (`FILL`, or ≤ inner width); bounds past the edge are a bug.
9. Empty/loading states are designed (icon + heading + body; skeleton mirrors real content).
10. An icon carries a semantic color matching its label; never a default fill.
11. Every element earns its place — cut decorative noise. Restraint is the craft, not "fill the space."
12. **Squint test:** screen, PRIMARY action, and STATE read without copy; if that collapses, hierarchy is weak.
13. Size for input: TOUCH meets **≥44×44pt iOS / 48dp**; POINTER (desktop) may be denser.
14. A control and its label share one vertical center (`counterAxisAlignItems = CENTER`).
15. Registered components live tidy on the component page — grouped, non-overlapping, named; never loose on a screen (`references/component-reuse.md`).
16. Build mobile screens scroll-ready: content in ONE frame, chrome pinned (`pin_child`) → scroll prototype, no restructure (`references/composition-patterns.md`).

## Reference Router

| Topic | File |
|---|---|
| FILL/HUG/FIXED, WRAP grids, resize test | `references/auto-layout.md` |
| Padding ownership, gap vs padding, spacing tokens | `references/padding-strategy.md` |
| Library-first selection, component search, tidy component page | `references/component-reuse.md` |
| Instance properties, slots, variants, reset behavior | `references/component-usage.md` |
| Navigation, forms, data, modals, states, scroll-ready, effect-safe elevation | `references/composition-patterns.md` |
| Final gates, redundant-element check | `references/handoff-checklist.md` |

## Stop Flags

- Raw hex/rgb, raw spacing, raw radius, manual shadows, or inline font values.
- Large `itemSpacing` to distribute children; WRAP container with FILL children.
- **Copy** placeholders (`Heading`, `Item 1`, `Lorem ipsum`) — copy is real strings (a missing *image* is fine).
- Manual substitutes for components the library provides; separate frames for variants.
- A child spilling past the padded box; chrome rebuilt per state instead of a shared instance.
- Status by color alone (pair text/icon); a saturated accent as a large active background.
- An element whose info its neighbours already carry — decide if it *belongs* (rule 11 + PC13).
- An elevation effect (shadow/glow/stroke) clipped for lack of room — inset content, don't flush-fit (`references/composition-patterns.md`).
