# Prototype Inference and Audit

Use this when inferring flow from static frames or auditing an existing prototype.
Read `prototype-patterns.md` first for action, trigger, transition, and overlay rules.

**Quick navigation:** [Static-flow inference](#static-flow-inference) · [Audit workflow](#audit-workflow) · [Reaction flow map](#reaction-flow-map) · [Conservative fixes](#conservative-fixes)

---

## Static-flow inference

These are derived heuristics, not official Figma rules. Require at least two
corroborating signals before auto-wiring; flag single-signal guesses for review.

Signals:
- Screen order: numeric prefixes (`01 Login`, `Step 2`), left-to-right/top-to-bottom layout, entry words (`Splash`, `Welcome`, `Onboarding`), hub words (`Home`, `Dashboard`), terminal words (`Success`, `Done`, `Confirmation`).
- Button labels: Next/Continue/Submit/Sign in -> forward NAVIGATE; Back/Cancel -> BACK; Close/X/Done on modal -> CLOSE; Open/Add/Edit/Filter/Menu/kebab/hamburger -> OVERLAY; URL labels -> URL.
- Structure: consistent nav/tab bar across frames -> peer tabs; centered card with scrim -> overlay; small frame near a control -> manual overlay; full device-size -> screen; variants that differ only by state -> CHANGE_TO; near-identical frames with one changed region -> SMART_ANIMATE candidate.
- Start frame: no incoming inferred connection + lowest name index + top-left position.

Sources: [Introducing Overlays](https://www.figma.com/blog/introducing-overlays-taking-prototyping-to-the-next-layer/) · [5 ways to improve prototyping](https://www.figma.com/best-practices/five-ways-to-improve-your-prototyping-workflow/) · [Back-button UX](https://baymard.com/blog/back-button-expectations).

## Audit workflow

1. Read `get_prototype` for the page or scoped frames. It returns `edges[]`,
   `flowStartingPoints`, `overlays[]`, `reactionNodeCount`, and `edgeCount`. It walks
   reaction-bearing descendants, not just top frames.
2. Inventory by `actionType`, `navigation`, and `trigger.type`.
3. Flag issues using the table below.
4. Report source/destination IDs, issue, recommended fix, and whether it is safe to auto-apply.
5. Apply only unambiguous fixes, then re-run `get_prototype`.

| Check | Signal | Action |
|---|---|---|
| No entry point | `flowStartingPoints: []` / `prototypeStartNodeId: null` | Infer start and propose `set_prototype_start`. |
| Broken connection | NODE action with `destinationId:null` | Flag source; target was lost. |
| Dead end | Reachable frame appears only as destination | Propose forward or BACK affordance. |
| Missing back path | Forward NAVIGATE with no return edge | Propose BACK on destination. |
| Wrong direction | Forward `PUSH LEFT` or back `PUSH RIGHT` | Correct direction. |
| Heavy transition | `PUSH` between same-named/shared-layer frames | Recommend `SMART_ANIMATE` + `EASE_IN_AND_OUT`. |
| Misplaced overlay | `overlayPositionType:"CENTER"` on dropdown/sheet-shaped frame | Flag Figma UI config; do not auto-wire. |
| Mobile hover | `ON_HOVER`/`MOUSE_ENTER` on touch-sized frame | Replace with click/press. |
| Inconsistent trigger | Same element kind wired differently | Normalize. |

## Reaction flow map

Use this when the user wants a readable interaction map rather than a mutation.

```json
{
  "ops": [
    { "type": "get_design_context", "params": { "depth": 2, "detail": "minimal" } },
    { "type": "get_reactions",      "nodeIds": ["<screen-a-id>", "<screen-b-id>"] }
  ]
}
```

Process:
- Iterate each reaction's `actions[]` array.
- Keep destination-bearing actions: NODE actions with `destinationId`.
- Include `NAVIGATE`, `OVERLAY`, and `SWAP`; ignore `CHANGE_TO`, `CLOSE`, and entries without destinations.
- Resolve source/destination names with `get_nodes_info`.
- Emit `[ScreenA] --ON_CLICK/NAVIGATE--> [ScreenB]`.
- Use colon node IDs such as `4029:12345`, never hyphen IDs.

## Conservative fixes

Safe to auto-apply when the evidence is clear:
- set a missing start point
- correct flipped PUSH direction
- switch same-named/shared-layer frame pairs from heavy PUSH to SMART_ANIMATE

Do not auto-decide where dead ends should go, which overlay placement is correct, or
whether a single-signal inferred route is intended.

To remove starts, use `set_prototype_start` with `mode:"remove"` for specific frames
or `mode:"clear"` for all. Read first so the previous state can be restored.
