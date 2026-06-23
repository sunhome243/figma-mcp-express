---
name: figma-design-patterns
description: Use when designing, editing, or reviewing Figma UI layout craft for auto layout, components, spacing, typography, states, dark mode, or handoff.
---

# Figma Design Patterns

Design-judgment skill — execution goes through figma-mcp-express `batch` ops, not raw Plugin API.

> **SCOPE.** Owns **design craft** (detail in `references/`). Tool-mechanics → `figma-mcp-express`; workflow → `figma-product-build`.

## Core Rules

1. Structural frames use dynamic auto layout, not fixed-position piles.
2. Spacing, padding, radius, fills, strokes, effects, and text come from library tokens/styles.
3. Use Figma-native production features: variables/modes, styles, components, variants/properties, auto layout/grid, constraints, prototypes, and annotations/dev resources when relevant.
4. Component priority: library `INSTANCE` → local component → raw structural frame only when none exists.
5. Configure every placed instance with real content, variants, and dimensions before done.
6. Layer names must be semantic; generated names (`Frame 47`) and cryptic shorthand (`t`, `lbl`) are unfinished — name as you create.
7. Repeating raw structures become components and reused instances; shared chrome is ONE component instanced identically across states.
8. Children fit the parent's padded box (`FILL`, or width ≤ inner width); overflow/clip is a layout bug.
9. Empty/loading/error states are designed (icon + heading + body; skeleton mirrors real content); icons/status use semantic color plus a text/icon cue.
10. Every element earns its place; cut decorative noise. **Squint test:** screen, PRIMARY action, and STATE read without copy.
11. Size for input: TOUCH meets **≥44×44pt / 48dp**; POINTER may be denser. Control and label share one vertical center (`counterAxisAlignItems = CENTER`).
12. Registered components live tidy on the component page — grouped, named, never loose on a screen (`references/component-reuse.md`).
13. Build mobile screens scroll-ready: content in ONE frame, chrome pinned via `pin_child`, scroll prototype (`references/composition-patterns.md`).

## Reference Router

| Topic | File |
|---|---|
| FILL/HUG/FIXED, WRAP grids, resize test | `references/auto-layout.md` |
| Padding ownership, gap vs padding, spacing tokens | `references/padding-strategy.md` |
| Library-first selection, component search, tidy component page | `references/component-reuse.md` |
| Instance properties, slots, variants, reset behavior | `references/component-usage.md` |
| Navigation, forms, data display, modals, scroll-ready frames | `references/composition-patterns.md` |
| Color/status encoding, icons, empty/loading states | `references/states-and-feedback.md` |
| Final PASS/FAIL gates, redundant-element check | `references/handoff-checklist.md` |

## Stop Flags

- Raw hex/rgb, raw spacing, raw radius, manual shadows, or inline font values.
- Visual lookalikes built from raw frames when a Figma component/variant/style/prototype feature exists.
- Large `itemSpacing` to distribute children; WRAP container with FILL children.
- **Copy** placeholders (`Heading`, `Item 1`, `Lorem ipsum`) — copy is real strings (a missing *image* is fine).
- Manual substitutes for components the library provides; separate frames for variants.
- A child spilling past the padded box; chrome rebuilt per state instead of a shared instance.
- Status by color alone (pair text/icon); a saturated accent as a large active background.
- An element whose info its neighbours already carry — decide if it *belongs* (rule 10 + PC13).
- An elevation effect (shadow/glow) clipped for lack of room — inset content, don't flush-fit.
