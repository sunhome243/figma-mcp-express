---
name: figma-prototype
description: Use when wiring or auditing Figma prototypes, including clicks, navigation, overlays, transitions, flow starts, scroll-fixed chrome, reaction maps, or inferred flows from static frames. Companion domain skill for figma-mcp-express prototype work.
---

# Figma Prototype Wiring

Companion domain layer for prototype judgment. It does not redefine MCP execution
rules: use `figma-mcp-express` for tool discovery, batch syntax, origin/channel,
and validation; use this skill for interaction intent, transition choice, overlay
limits, flow audits, and scroll-fixed behavior.

## First Checks

1. Figma Desktop open with the plugin running; load the `figma-mcp-express` skill first for tool/batch discipline.
2. Prototype reactions live on ANY node (buttons, instances), not just frames — and prototypes are per-page.
3. Overlay appearance (position/scrim/dismiss) is **read-only** via the API. You can open an overlay (`navigation:"OVERLAY"`) but not place or style it — that is configured in the Figma UI on the destination frame.

## Workflow

1. **Scope** — given `nodeId`s, wire those frames; otherwise use the current page.
2. **Read current state** — run `get_prototype` for starts, edges, overlays, and existing reactions.
3. **Analyze frames** — use `get_design_context` for structure and `scan_text_nodes` for CTA labels.
4. **Infer only with evidence** — require at least two corroborating signals before auto-wiring; flag single-signal guesses.
5. **Wire** — batch `set_reactions` ops plus `set_prototype_start` for the entry frame.
6. **Choose transitions by similarity** — near-identical screens use `SMART_ANIMATE`; distinct screens use directional `PUSH`; peers use `DISSOLVE`; overlays use `MOVE_IN`/`MOVE_OUT`.
7. **Verify** — re-run `get_prototype`; flag dead ends, missing back paths, mobile hover triggers, unwired CTAs, and heavy PUSH between same-named frames.

## Reference Router

| Need | Read |
|---|---|
| Actions, triggers, transitions, overlays, component variants, flow starts | `references/prototype-patterns.md` |
| Infer flow from static frames, audit existing prototypes, reaction maps, conservative fixes | `references/prototype-audit.md` |
| Scrollable frames, sticky headers, pinned tab bars, fixed children | `references/prototype-scroll.md` |

## Cannot Do

- Set overlay position/scrim/dismiss (read-only API) — only open the overlay.
- State interactions belong on the component variants (CHANGE_TO), not stamped per instance from outside.
- Create files, FigJam, or Slides.
