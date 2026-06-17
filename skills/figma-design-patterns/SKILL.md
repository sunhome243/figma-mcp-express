---
name: figma-design-patterns
description: Use when building, editing, or reviewing Figma UI layouts, auto layout, components, spacing, typography, states, dark mode, or developer handoff.
---

# Figma Design Patterns

Use this skill for design judgment. It should not duplicate MCP operation schemas or teach raw Plugin API scripts; execution goes through figma-mcp-express tools and validated `batch` ops.

> **SCOPE (skill ownership).** This skill owns **DESIGN CRAFT / PRODUCTION QUALITY** — tool-agnostic design rules: no-clip (container ≥ content), no-cram, BUTTON CANON, shared-chrome robustness ("inherited flaw ≠ consistency"), structured-text line-breaks, status-color vocabulary, DRY/componentization, accent rationing, component-first, empty/loading patterns, cross-screen consistency. A new **craft** rule belongs here. MCP tool-mechanics → `figma-mcp-express`; workflow/gate → `figma-product-build`; per-project keys + digest → `kit-keys.md`. See memory `figma-skill-ecosystem-map` (layered duplication is by design).

## Core Rules

1. Structural frames use dynamic auto layout, not fixed-position piles.
2. Spacing, padding, radius, fills, strokes, effects, and text styles come from library tokens/styles.
3. Component priority is library `INSTANCE` first, then local component, then raw structural frame only when no component exists.
4. Configure every placed instance with real content, variants, and dimensions before calling it done.
5. Layer names must be semantic; generated names such as `Frame 47` or `Rectangle 3` are unfinished.
6. Repeating raw structures become a local component and reused instances.
7. Chrome that recurs across states or screens — app bar, header, bottom nav/tab bar — is ONE component instanced everywhere, identical in alignment, height, padding, and contents. Rebuilding it per state is how a header silently drifts (left-aligned here, centered there; an icon present on one screen, missing on the next). Same element ⇒ same instance.
8. Every child fits its parent's padded box: `FILL`, or a width ≤ the parent's inner width. Anything past the frame edge — an overflowing skeleton bar, text clipped at the margin — is a layout bug, not a rendering quirk. Fix it with FILL or a width that respects padding, never a wider frame.
9. Empty and loading states are designed, not afterthoughts. Center icon + heading + body in a padded container; give body text auto-height so it wraps instead of clipping. A loading skeleton mirrors the shape of the content it stands in for (same row/card structure) so the page doesn't lurch when real data arrives — a generic avatar-plus-two-lines list is a tell that the skeleton was dropped in blind.
10. An icon carries a semantic color like any other element — `muted` when inactive/secondary, `foreground` by default, `accent` when active/selected, the status color when it marks status — and never disagrees with its adjacent label (a muted label beside a full-contrast icon reads as a bug). An icon left at an arbitrary or default fill is unfinished.
11. **Every element earns its place.** Every type style, line, divider, box, card, icon, and copy string must serve a purpose — carry information, establish hierarchy, enable an action, or separate regions. If an element earns its place nowhere, remove it: decorative noise and redundant chrome dilute the screen and bury what actually matters. (This is the opposite of "fill the space" — restraint IS the craft.)
12. **The screen is legible at a glance — the squint test.** A viewer should grasp WHAT screen this is, what the PRIMARY action is, and what STATE it's in *without reading the copy* — through visual hierarchy, layout, iconography, status encoding, and affordances. Mentally blur the text: if the meaning collapses (everything the same size/weight, the primary action not visually dominant, statuses unmarked, no clear focal point), the hierarchy is doing too little. A screen that only makes sense once you read every label is not done — fix the hierarchy until the structure alone tells the story.
13. **Design for the target device — sizing follows the input method.** On TOUCH platforms (mobile / tablet) every interactive element — button, input, list row, tappable icon, checkbox, tab, segmented control — meets the platform minimum touch target (**≥44×44pt iOS / 48dp Android**) and gets generous spacing so a finger can't mis-tap; pointer-sized controls crammed onto a phone (32–36px inputs, tight rows, 8px gaps between tap targets) are a defect, not a density choice. Touch screens want **bigger controls + more breathing room (시원시원)** — inputs ~48–56px tall, primary buttons ~48px, a comfortable 16–24px rhythm, real padding around tappable areas. On POINTER platforms (desktop / web) denser layouts are fine. Read `targetPlatform`: the same form is laid out *differently* on a phone than on a desktop — match the device, never ship a desktop-density screen on mobile. The reviewer applies this too: on a touch screen, flag any interactive element under the touch minimum or any cramped tap zone.
14. **A control and its label share one vertical center — always.** In ANY `[control + text]` row — checkbox, radio, toggle, switch, leading icon + label, avatar + name — the control and the label are vertically center-aligned to each other via the row's auto-layout `counterAxisAlignItems = CENTER`. The recurring defect this kills: a small (16px) checkbox/icon pinned to the TOP of a taller (44px) row while the label is vertically centered, so the box floats above the text and reads as broken. Never place the control by a hand-set y inside a tall row — let the row's cross-axis centering align it to the text's middle. (For a control beside a multi-line label, center the control on the first line, not the whole block, unless the design clearly wants block-center.) Check it whenever a row height changes (e.g. after touch-resizing): the control must re-center with its label, not strand at the top.

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
- **Copy** placeholders such as `Heading`, `Item 1`, `Slot`, or `Lorem ipsum` — copy is always real strings. (This is different from an **asset** placeholder: a photo/cover/map/logo you can't source may use a neutral placeholder fill — that's expected, and omitting the visual entirely is worse than a placeholder. Forbid placeholder *words*, allow placeholder *images*.)
- Manual substitutes for icons, buttons, inputs, navigation, pagination, badges, or dialogs when the library has a component.
- Separate frames for interactive states that should be component variants.
- A child whose bounds spill past the parent's padded box (overflow / clipping).
- Chrome (header, nav, tab bar) rebuilt differently between states instead of a shared instance.
- Status conveyed by color alone — pair it with text and/or an icon so it survives color-blindness and grayscale.
- A saturated accent used as a large solid background fill for an active/selected state — prefer an accent tint or a slim indicator (underline, dot, left-bar) unless the design system explicitly specifies the solid fill.
