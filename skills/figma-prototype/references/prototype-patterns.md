# Figma Prototype Patterns — Designer Conventions + API Ground Truth

Reference for wiring prototype interactions onto existing Figma designs through figma-mcp-express. Execution is declarative: the `set_reactions` batch op writes a node's reactions, `set_prototype_start` sets a page's flow starting points, and the `get_prototype` op reads the whole-page flow graph. Each reaction is `{ trigger, actions[] }` (the current plural `actions` array; a legacy singular `action` is still readable).

Two vocabularies are in play: the **designer-facing UI names** (help.figma.com) and the **API constants** you actually write. The bridge table in §0 is load-bearing.

Sources: [Action](https://developers.figma.com/docs/plugins/api/Action/) · [Trigger](https://developers.figma.com/docs/plugins/api/Trigger/) · [Transition](https://developers.figma.com/docs/plugins/api/Transition/) · [Overlay](https://developers.figma.com/docs/plugins/api/Overlay/) · [nodes-reactions](https://developers.figma.com/docs/plugins/api/properties/nodes-reactions).

---

## §0. Friendly-name ↔ API constant bridge

| Designer UI name | Action shape | Notes |
|---|---|---|
| **Navigate to** | `{ type:"NODE", navigation:"NAVIGATE", destinationId, transition }` | Full screen change |
| **Change to** | `{ type:"NODE", navigation:"CHANGE_TO", destinationId, transition }` | Swap to another **variant** in the same component set |
| **Open overlay** | `{ type:"NODE", navigation:"OVERLAY", destinationId, transition, overlayRelativePosition? }` | Position/scrim live on the destination frame, not the action (§4) |
| **Swap overlay** | `{ type:"NODE", navigation:"SWAP", destinationId, transition }` | Replaces the current overlay; keeps overlay settings |
| **Scroll to** | `{ type:"NODE", navigation:"SCROLL_TO", destinationId, transition }` | Anchor-scroll to a node in the same top frame |
| **Back** | `{ type:"BACK" }` | Returns to prior frame in nav history |
| **Close overlay** | `{ type:"CLOSE" }` | Dismisses the overlay |
| **Open link** | `{ type:"URL", url, openInNewTab? }` | External URL |
| **Set variable** | `{ type:"SET_VARIABLE", variableId, variableValue? }` | |
| **Set variable mode** | `{ type:"SET_VARIABLE_MODE", variableCollectionId, variableModeId }` | Theme / locale toggle |
| **Conditional (if/else)** | `{ type:"CONDITIONAL", conditionalBlocks[] }` | |

`Navigation` = `NAVIGATE | SWAP | OVERLAY | SCROLL_TO | CHANGE_TO`. NODE optional fields: `overlayRelativePosition` (only meaningful when the destination's `overlayPositionType === "MANUAL"`), `resetScrollPosition`, `resetVideoPosition`, `resetInteractiveComponents`. The `set_reactions` op requires `destinationId` for NODE and defaults `navigation` to `NAVIGATE`.

---

## §1. Flow / navigation conventions

- **NAVIGATE** — move between full screens; the default. Primary CTAs, links, list-item → detail. Builds nav history (so a later **BACK** works).
- **OVERLAY** — content on top of the current screen: modals/dialogs, dropdowns, menus, tooltips, toasts, bottom sheets.
- **SWAP** — replace the *currently open* overlay with another, preserving its settings (multi-step modal wizards). Not recorded in history.
- **SCROLL_TO** — jump within the same top frame (anchor nav, back-to-top).
- **CHANGE_TO** — switch to another *variant* of the same component set (state changes). Belongs on the component, not the instance (§5).

Standard element → wiring:

| Element | Trigger | Action | Transition |
|---|---|---|---|
| Primary CTA (Next/Submit/Continue) | `ON_CLICK` | `NAVIGATE` to next screen | `PUSH` `RIGHT` (forward) or `SMART_ANIMATE` if frames share layers |
| Back button | `ON_CLICK` | `BACK` (preferred) | `PUSH` `LEFT` |
| Tab bar / bottom-nav item | `ON_CLICK` | `NAVIGATE` to that tab | `DISSOLVE` or instant (peers, no directional push) |
| Modal / dialog open | `ON_CLICK` | `OVERLAY` (centered) | `DISSOLVE` or `MOVE_IN` `BOTTOM` |
| Modal close (X / Cancel / scrim) | `ON_CLICK` | `CLOSE` | reverse of open |
| Dropdown / menu open | `ON_CLICK` | `OVERLAY` (manual, anchored) | short `DISSOLVE` |
| Bottom sheet | `ON_CLICK` / `ON_DRAG` | `OVERLAY` (bottom) | `MOVE_IN` `BOTTOM` |
| Toast / snackbar | `AFTER_TIMEOUT` show, then `AFTER_TIMEOUT` dismiss | `OVERLAY` open → `CLOSE` | `MOVE_IN`/`MOVE_OUT` `TOP`/`BOTTOM` |
| List item → detail | `ON_CLICK` | `NAVIGATE` | `PUSH` `RIGHT` (mobile) / `DISSOLVE` (desktop) |

Sources: [Prototype actions](https://help.figma.com/hc/en-us/articles/360040035874-Prototype-actions) · [Connect your prototype](https://help.figma.com/hc/en-us/articles/360040315773-Connect-your-prototype).

---

## §2. Trigger conventions

`Trigger` values: `ON_CLICK`, `ON_HOVER`, `ON_PRESS`, `ON_DRAG`, `AFTER_TIMEOUT` (`timeout` ms), `MOUSE_UP`/`MOUSE_DOWN`/`MOUSE_ENTER`/`MOUSE_LEAVE` (`delay`), `ON_KEY_DOWN` (`device`, `keyCodes[]`), `ON_MEDIA_HIT` (`mediaHitTime`), `ON_MEDIA_END`.

- **`ON_CLICK`** — the workhorse; maps to click on desktop AND tap on mobile. Buttons, links, nav.
- **`ON_DRAG`** — swipe/drag (carousels, swipe-to-delete, sheet pull). Mobile-centric.
- **`ON_HOVER` / `MOUSE_ENTER` / `MOUSE_LEAVE`** — **desktop only** (no hover on touch). `ON_HOVER` auto-reverses on cursor-off; `MOUSE_ENTER`/`LEAVE` fire once, asymmetric.
- **`ON_PRESS` / `MOUSE_DOWN` / `MOUSE_UP`** — press-and-hold; `ON_PRESS` auto-reverses on release.
- **`AFTER_TIMEOUT`** — splash→home, loading→loaded, toast auto-dismiss, carousel rotate.
- **`ON_KEY_DOWN`** — keyboard/gamepad (Enter to submit, Esc to close).

Mobile → `ON_CLICK`/`ON_DRAG`/`ON_PRESS`, never hover. Desktop → may add hover + `ON_KEY_DOWN`. Source: [Prototype triggers](https://help.figma.com/hc/en-us/articles/360040035834-Prototype-triggers).

---

## §3. Transition conventions

`Transition.type`: simple `DISSOLVE`/`SMART_ANIMATE`/`SCROLL_ANIMATE`; directional `MOVE_IN`/`MOVE_OUT`/`PUSH`/`SLIDE_IN`/`SLIDE_OUT` (`direction: LEFT|RIGHT|TOP|BOTTOM`). Both carry `duration` (seconds, e.g. `0.3`) and `easing`. `Easing.type`: `EASE_IN`/`EASE_OUT`/`EASE_IN_AND_OUT`/`LINEAR`/`*_BACK`/`GENTLE`/`QUICK`/`BOUNCY`/`SLOW`/`CUSTOM_CUBIC_BEZIER`/`CUSTOM_SPRING`.

| Intent | Transition | Direction | Easing |
|---|---|---|---|
| Forward navigation | `PUSH` | `RIGHT` | `EASE_OUT` |
| Backward navigation | `PUSH` | `LEFT` | `EASE_OUT` |
| Modal / sheet appear | `MOVE_IN` | `BOTTOM` | `EASE_OUT` |
| Modal / sheet dismiss | `MOVE_OUT` | `BOTTOM` | `EASE_IN` |
| Tab switch / peers | `DISSOLVE` | — | `LINEAR`/`EASE_IN_AND_OUT` |
| State change w/ shared layers | `SMART_ANIMATE` | — | `EASE_OUT`/`GENTLE` |
| Drawer slide-over | `SLIDE_IN`/`SLIDE_OUT` | `LEFT`/`RIGHT` | `EASE_OUT` |

`PUSH` = new screen pushes old off (both move); `MOVE_IN`/`MOVE_OUT` = new slides over old (old stays); `SLIDE` = move with overlap. Use PUSH for page nav, MOVE for overlays/sheets.

**Pick the transition by how similar source and destination are — this detail is what separates a polished prototype from a janky one:**

| Source vs destination | Right transition | Why |
|---|---|---|
| **Near-identical** — same chrome (GNB/sidebar/tab bar), only the main content / a status differs | `SMART_ANIMATE` + `EASE_IN_AND_OUT` | Shared layers (matched by name) are held in place; **only the changed section morphs**. A `PUSH` here slides identical chrome off-screen and back for no reason — reads as heavier and more disorienting than the change actually is. |
| **Distinct screens** — different layouts, a real context change | `PUSH` (forward `RIGHT` / back `LEFT`), `EASE_OUT` | The whole-screen slide correctly signals "you moved somewhere new". |
| **Same surface, element state change** (toggle, selected row, expand) | `SMART_ANIMATE`, `EASE_OUT`/`GENTLE` | Morphs the one element; built into the component when possible (§5). |
| **Layered surface** (modal/sheet/dropdown over the current screen) | `MOVE_IN`/`MOVE_OUT` `BOTTOM` (overlay, §4) | Old screen stays put underneath; only the overlay moves. |

The litmus test: *what should the viewer's eye actually track?* If only the inner content changes, only the inner content should move (`SMART_ANIMATE`). If they're going somewhere new, move the whole screen (`PUSH`). Verify with the `get_prototype` op — same-named source/dest frames wired with `PUSH` are usually a `SMART_ANIMATE` waiting to happen.

**Duration/easing — community convention, NOT a documented Figma default:** UI transitions ~**150–600ms** (~300ms common), **ease-out** for nav (decelerates into place), **ease-in-and-out** for in-place content morphs (settles symmetrically). Sources: [LogRocket smart-animate](https://blog.logrocket.com/ux-design/using-figma-smart-animate-prototype-animations/). Mistakes: wrong direction (forward push LEFT); durations >600ms; over-animation; `SMART_ANIMATE` between frames whose layers don't share names (silently degrades to a fade).

---

## §4. Overlay best practices

Overlay position/scrim/dismiss are **properties on the destination frame**, NOT in the action — and they are **read-only via the API** (configured in the Figma UI). The skill can set `navigation:"OVERLAY"` + `overlayRelativePosition` but must **read and respect** the frame's existing settings via the `get_prototype` op rather than assume it can write them. Source: [forum: read-only overlayPositionType](https://forum.figma.com/ask-the-community-7/how-to-setup-overlay-interaction-when-overlaypositiontype-is-a-readonly-property-in-framenode-but-essential-for-target-reaction-object-29613), [Create overlays](https://help.figma.com/hc/en-us/articles/360039818254-Create-overlays-in-your-prototypes).

- `overlayPositionType`: `CENTER | TOP_* | BOTTOM_* | MANUAL`.
- `overlayBackground` (scrim): `{type:"NONE"}` or `{type:"SOLID_COLOR", color:RGBA}`.
- `overlayBackgroundInteraction`: `NONE | CLOSE_ON_CLICK_OUTSIDE`.

Position-by-intent: **dialog/alert** → CENTER + scrim + close-on-click-outside; **dropdown/menu/tooltip** → MANUAL (anchored) + no scrim + close-on-click-outside; **bottom sheet** → BOTTOM + scrim + close-on-click-outside; **toast** → TOP/BOTTOM, no scrim, auto-dismiss. **If a frame looks like a dropdown/sheet but its `overlayPositionType` is still `CENTER`, flag it for UI configuration — do not wire a misplaced overlay.**

---

## §5. Interactive components & variants

Build state interactions INTO the component (between variants); build navigation on the instance/frame. Wire `CHANGE_TO` between variants inside the component set so every instance inherits it. Hover state: `ON_HOVER` → `CHANGE_TO` hover variant; pressed: `ON_PRESS` → `CHANGE_TO`; toggle: `ON_CLICK` → opposite variant. Use `SMART_ANIMATE` when variants share named layers. Anti-pattern: wiring a *library* instance directly (broken inheritance) — wrap in a local master first. Source: [Interactive components with variants](https://help.figma.com/hc/en-us/articles/360061175334-Create-interactive-components-with-variants).

---

## §6. Starting points / flows

Set a **flow starting point** with the `set_prototype_start` op (assigns the page's `flowStartingPoints`). A page can hold many flows; a top-level frame can be in multiple flows but has one starting point. Name flows meaningfully (e.g. "Onboarding", "Checkout"). The natural start candidate is the frame with no incoming connection, lowest name-index, top-left position. Source: [Guide to prototyping](https://help.figma.com/hc/en-us/articles/360040314193-Guide-to-prototyping-in-Figma).

---

## §7. Anti-patterns

- **Dead-end screens** — a reachable non-terminal frame with no outgoing interaction and no back path.
- **Missing back affordance** — forward nav with no `BACK`/return.
- **Wrong transition direction** — forward `PUSH LEFT` / back `PUSH RIGHT` reads as broken.
- **Over-animation** — long durations, gratuitous spring/bounce.
- **Inconsistent triggers** — same element type wired differently across screens.
- **Smart Animate without matched layers** — silently degrades to a crossfade.
- **Overlay with no dismiss** — modal/dropdown lacking CLOSE / scrim-click / Esc.
- **Hover triggers on a mobile prototype** — never fire on touch.

---

## §8. Inferring flow from a static design (derived heuristics)

Convention-derived reasoning, not from official docs. Treat each as a weighted signal; **require ≥2 corroborating signals before auto-wiring**, flag single-signal guesses for review.

**Screen ordering:** numeric/ordinal name prefixes (`01 Login`, `Step 2`) imply sequence; designers lay flows left→right, top→bottom (leftmost/topmost ≈ start); keywords — `Splash`/`Onboarding`/`Welcome` = entry, `Home`/`Dashboard` = hub, `Success`/`Done`/`Confirmation` = terminal.

**Button-label → action:** `Next`/`Continue`/`Submit`/`Sign in`/`→` → NAVIGATE forward (PUSH RIGHT); `Back`/`Cancel`/`←` → BACK (PUSH LEFT); `Close`/`X`/`Done` (on a modal) → CLOSE; `Open`/`Add`/`Edit`/`Filter`/`Menu`/kebab/hamburger → OVERLAY; external labels/URLs → URL; a repeated row/card → NAVIGATE to a detail frame.

**Structural:** a consistent nav/tab bar across N frames → those are peer tabs (wire each tab → its frame, DISSOLVE); a smaller-than-viewport centered frame with scrim/rounded card + Cancel/Confirm → OVERLAY target, not a NAVIGATE screen; a small frame anchored near a control → OVERLAY MANUAL; full device-size → screen, partial/floating → overlay; variant sets differing only by state → CHANGE_TO candidates built into the component (§5); two near-identical frames differing in one element → strong `SMART_ANIMATE` candidate.

**Start frame:** no incoming inferred connection + lowest name-index + top-left → set the flow starting point there.

Sources: [Introducing Overlays](https://www.figma.com/blog/introducing-overlays-taking-prototyping-to-the-next-layer/) · [5 ways to improve prototyping](https://www.figma.com/best-practices/five-ways-to-improve-your-prototyping-workflow/) · [Baymard back-button UX](https://baymard.com/blog/back-button-expectations).

---

## §9. Analysis & audit workflow (existing prototype)

Use this when a prototype already exists and the job is to **understand, audit, or refine** it — not wire from scratch. Reading is free and non-destructive; do it before proposing any change.

**1. Read the graph.** Run the `get_prototype` op on the page (or scope to frames). It returns `edges[]` (every `source → destination` with trigger/navigation/transition), `flowStartingPoints`, `overlays[]`, plus `reactionNodeCount`/`edgeCount`. It walks reaction-bearing descendants (buttons, instances), not just top frames. Large pages spill to disk — query the sidecar with `jq` (group by `actionType`, `trigger.type`, `navigation`).

**2. Inventory.** Summarize: edges by `actionType` (NODE/URL/BACK/CLOSE…), by `navigation` (NAVIGATE/CHANGE_TO/OVERLAY…), by `trigger.type`. This alone reveals the prototype's character (e.g. mostly `AFTER_TIMEOUT`/`ON_MEDIA_END` = a linear video/animation walkthrough; mostly `ON_CLICK` = interactive).

**3. Audit — flag against §7 + the checks below:**

| Check | Signal in `get_prototype` output | Action |
|---|---|---|
| No entry point | `flowStartingPoints: []` / `prototypeStartNodeId: null` | Infer the start frame (§6/§8) → propose `set_prototype_start`. |
| Broken connection | `actionType:"NODE"` with `destinationId: null` (destination unset or deleted) | Flag with source id; the target was lost. Cross-check raw data with the `get_reactions` op — `destinationId:null` there too means it's real, not a read artifact. |
| Dead-end screen | a reachable frame appears only as a destination, never a source | Propose a forward or BACK affordance. |
| Missing back path | forward NAVIGATE with no return edge between the pair | Propose `BACK` on the destination. |
| Wrong transition direction | forward edge with `PUSH`/`LEFT` (or back with `RIGHT`) | Correct the direction. |
| **Heavy transition on near-identical screens** | `PUSH` between two frames with the **same name / shared layer names** | Recommend `SMART_ANIMATE` + `EASE_IN_AND_OUT` (§3) — only the changed section should move. |
| Misplaced overlay | `overlays[]` entry with `overlayPositionType:"CENTER"` on a dropdown/sheet-shaped frame | Flag for Figma-UI config (read-only via API, §4); do not auto-wire. |
| Mobile hover | `ON_HOVER`/`MOUSE_ENTER` trigger on a touch-sized frame | Replace with `ON_CLICK`/`ON_PRESS`. |
| Inconsistent triggers | same element kind wired differently across screens | Normalize. |

**4. Report, then fix conservatively.** Present findings as `{ source/destination ids, issue, recommended fix }`. **Auto-apply only the unambiguous, reversible fixes** (set a missing start point; correct a flipped direction; PUSH→SMART_ANIMATE on same-named pairs). Everything requiring judgment (which dead-end goes where, overlay placement) → flag for the user. Re-run the `get_prototype` op after writing to confirm.

> To take a start point away, use the `set_prototype_start` op: `mode:"remove"` drops the given frame(s) and keeps the rest (remove one without re-listing the others); `mode:"clear"` empties them all (e.g. resetting a flow during iteration). These are the only paths that remove a start point — `replace`/`append` can only add. Both are `commitUndo`-backed (one Figma undo restores them), and §9 step 1's read-first rule means you always have the prior state to re-apply.
