---
name: figma-design-patterns
description: Use when designing, editing, or reviewing Figma UI layout craft for auto layout, components, spacing, typography, states, dark mode, or handoff.
---

# Figma Design Patterns

Design-judgment skill — execution goes through figma-mcp-express `batch` ops, not raw Plugin API.

> **SCOPE.** Owns **design craft** (detail in `references/`). Tool mechanics belong to `figma-mcp-express`.

## Core Rules

1. Structural frames use dynamic auto layout, not fixed-position piles.
2. Spacing, padding, radius, fills, strokes, effects, and text styles come from library tokens/styles.
3. Actively use Figma-native production features: variables/modes, styles, components, variants/properties, auto layout/grid, constraints, prototypes, annotations/dev resources when the task calls for them.
4. Component priority: library `INSTANCE` → local component → raw structural frame only when no component exists.
5. Configure every placed instance with real content, variants, and dimensions before calling it done.
6. Layer names must be semantic; generated names such as `Frame 47` or `Rectangle 3` are unfinished.
7. Repeating raw structures become components and reused instances; shared chrome is one instanceable component across states/screens.
8. Children fit the parent's padded box (`FILL`, or width ≤ inner width); overflow/clip is a layout bug.
9. Empty/loading/error states are designed, not omitted; icons/status use semantic color plus text/icon cues.
10. Every element earns its place. Use the squint test: screen, primary action, and state should read without copy.
11. Size for the input method: touch meets platform minimums; pointer can be denser. Controls and labels share one vertical center.

## Reference Router

| Topic | File |
|---|---|
| FILL/HUG/FIXED, WRAP grids, resize test | `references/auto-layout.md` |
| Padding ownership, gap vs padding, spacing tokens | `references/padding-strategy.md` |
| Library-first selection and component search | `references/component-reuse.md` |
| Instance properties, slots, variants, reset behavior | `references/component-usage.md` |
| Navigation, forms, buttons, data display, modals, fixed-height frames | `references/composition-patterns.md` |
| Color/status encoding, icons, empty/loading states | `references/states-and-feedback.md` |
| Final PASS/FAIL gates before handoff | `references/handoff-checklist.md` |

## Stop Flags

- Raw hex/rgb, raw spacing, raw radius, manual shadows, or inline font values.
- Visual lookalikes built from raw frames when a Figma component/variant/style/prototype feature exists.
- Large `itemSpacing` to distribute children; WRAP container with FILL children.
- **Copy** placeholders (`Heading`, `Item 1`, `Slot`, `Lorem ipsum`) — copy is real strings (a missing-asset *image* placeholder is fine).
- Manual substitutes for icons, buttons, inputs, nav, pagination, badges, or dialogs when the library has a component; separate frames for states that should be variants.
- A child spilling past the parent's padded box (overflow / clip); chrome rebuilt differently per state instead of a shared instance.
- Status by color alone (pair with text/icon); a saturated accent flooded as a large active-state background (prefer a tint or slim indicator).
