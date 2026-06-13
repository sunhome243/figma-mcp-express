---
name: figma-design-patterns
description: Use when building, editing, or reviewing Figma UI layouts, auto layout, components, spacing, typography, states, dark mode, or developer handoff.
---

# Figma Design Patterns

Use this skill for design judgment. It should not duplicate MCP operation schemas or teach raw Plugin API scripts; execution goes through figma-mcp-express tools and validated `batch` ops.

## Core Rules

1. Structural frames use dynamic auto layout, not fixed-position piles.
2. Spacing, padding, radius, fills, strokes, effects, and text styles come from library tokens/styles.
3. Component priority is library `INSTANCE` first, then local component, then raw structural frame only when no component exists.
4. Configure every placed instance with real content, variants, and dimensions before calling it done.
5. Layer names must be semantic; generated names such as `Frame 47` or `Rectangle 3` are unfinished.
6. Repeating raw structures become a local component and reused instances.

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
- Large `itemSpacing` used to distribute children.
- WRAP container with FILL children.
- Placeholder text such as `Heading`, `Item 1`, `Slot`, or `Lorem ipsum`.
- Manual substitutes for icons, buttons, inputs, navigation, pagination, badges, or dialogs when the library has a component.
- Separate frames for interactive states that should be component variants.
