---
name: figma-prototype
description: Use when wiring prototype interactions (clicks, navigation, overlays, transitions, flow starting points) onto existing Figma frames, or auditing or inferring a prototype flow from a static design.
---

# Figma Prototype Wiring

Two modes: **wire** prototype interactions onto existing designs, or **analyze/audit/refine** an existing flow. Execution goes through figma-mcp-express batch ops; this skill is the judgment layer. Read `references/prototype-patterns.md` for navigation/trigger/transition/overlay conventions, the flow-inference heuristics (§8), and the analysis & audit workflow (§9).

## First Checks

1. Figma Desktop open with the plugin running; load the `figma-mcp-express` skill for tool/batch discipline.
2. Prototype reactions live on ANY node (buttons, instances), not just frames — and prototypes are per-page.
3. Overlay appearance (position/scrim/dismiss) is **read-only** via the API. You can open an overlay (`navigation:"OVERLAY"`) but not place or style it — that is configured in the Figma UI on the destination frame.

## Workflow

1. **Scope** — given `nodeId`s, wire those frames; otherwise the current page.
2. **Read current state** — run the `get_prototype` op for the existing flow graph (starting points, edges, overlay config). It walks reaction-bearing descendants, not just top-level frames.
3. **Analyze the frames** — `get_design_context` for structure/components, `scan_text_nodes` for button labels. Identify screens, CTAs, nav bars, and modal-shaped frames.
4. **Infer the flow** (references §8). Combine signals:
   - frame name order (`01 Login`, `Step 2`) and left→right / top→bottom layout
   - CTA labels: Next/Continue/Submit → forward NAVIGATE; Back/Cancel → BACK; Close/X → CLOSE; Open/Menu/Filter → OVERLAY
   - a consistent nav/tab bar across N frames → peer tabs
   - **≥2 corroborating signals required** before auto-wiring; flag single-signal guesses for review rather than guessing.
5. **Wire** — build one `batch` of `set_reactions` ops, plus a `set_prototype_start` op for the entry frame. Standard mapping (references §1/§3): primary CTA → NAVIGATE + PUSH RIGHT; Back → BACK + PUSH LEFT; tab → NAVIGATE + DISSOLVE; modal open → OVERLAY + MOVE_IN BOTTOM (~300ms ease-out). **Pick the transition by source/dest similarity (§3): near-identical screens (shared chrome, only content differs) → SMART_ANIMATE + EASE_IN_AND_OUT so only the changed section morphs; distinct screens → directional PUSH — this detail makes or breaks the feel.**
6. **Overlay flag** — for a frame that looks like a dropdown/sheet but whose `overlayPositionType` is the default `CENTER`, do NOT wire a misplaced overlay. Tell the user to set MANUAL/BOTTOM + scrim + close-on-click-outside in the Figma UI first, then re-run.
7. **Verify** — re-run the `get_prototype` op. Flag: dead-end screens, forward navigation with no back path, hover triggers on mobile frames, unwired CTAs, heavy PUSH between same-named frames (→ SMART_ANIMATE). To audit/refine an existing prototype rather than wire one, follow references §9 (read → inventory → audit table → conservative fixes).

## Cannot Do

- Set overlay position/scrim/dismiss (read-only API) — only open the overlay.
- State interactions belong on the component variants (CHANGE_TO), not stamped per instance from outside.
- Create files, FigJam, or Slides.
