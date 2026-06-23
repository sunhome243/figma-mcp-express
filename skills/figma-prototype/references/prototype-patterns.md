# Prototype Patterns

Reference for wiring prototype interactions onto existing Figma designs through
figma-mcp-express. Execution is declarative: `set_reactions` writes reactions,
`set_prototype_start` sets flow starts, and `get_prototype` reads the graph.

Each reaction is `{ trigger, actions[] }`. The current shape is plural `actions`;
legacy singular `action` may still appear in old data.

Sources: [Action](https://developers.figma.com/docs/plugins/api/Action/) · [Trigger](https://developers.figma.com/docs/plugins/api/Trigger/) · [Transition](https://developers.figma.com/docs/plugins/api/Transition/) · [Overlay](https://developers.figma.com/docs/plugins/api/Overlay/) · [nodes-reactions](https://developers.figma.com/docs/plugins/api/properties/nodes-reactions).

**Quick navigation:** [API bridge](#api-bridge) · [Flow conventions](#flow-conventions) · [Triggers](#triggers) · [Transitions](#transitions) · [Overlays](#overlays) · [Interactive components](#interactive-components) · [Flow starts](#flow-starts) · [Anti-patterns](#anti-patterns)

---

## API bridge

| Designer UI name | Action shape | Notes |
|---|---|---|
| Navigate to | `{ type:"NODE", navigation:"NAVIGATE", destinationId, transition }` | Full screen change |
| Change to | `{ type:"NODE", navigation:"CHANGE_TO", destinationId, transition }` | Swap to another variant in the same component set |
| Open overlay | `{ type:"NODE", navigation:"OVERLAY", destinationId, transition, overlayRelativePosition? }` | Position/scrim live on the destination frame |
| Swap overlay | `{ type:"NODE", navigation:"SWAP", destinationId, transition }` | Replaces current overlay |
| Scroll to | `{ type:"NODE", navigation:"SCROLL_TO", destinationId, transition }` | Anchor-scroll inside the same top frame |
| Back | `{ type:"BACK" }` | Returns to prior frame |
| Close overlay | `{ type:"CLOSE" }` | Dismisses overlay |
| Open link | `{ type:"URL", url, openInNewTab? }` | External URL |
| Set variable | `{ type:"SET_VARIABLE", variableId, variableValue? }` | Prototype variable |
| Set variable mode | `{ type:"SET_VARIABLE_MODE", variableCollectionId, variableModeId }` | Theme/locale toggle |
| Conditional | `{ type:"CONDITIONAL", conditionalBlocks[] }` | If/else |

`Navigation` = `NAVIGATE | SWAP | OVERLAY | SCROLL_TO | CHANGE_TO`. NODE fields
include `overlayRelativePosition` (effective only when the destination overlay is
MANUAL), `resetScrollPosition`, `resetVideoPosition`, and
`resetInteractiveComponents`. `set_reactions` requires `destinationId` for NODE and
defaults `navigation` to `NAVIGATE`.

## Flow conventions

| Element | Trigger | Action | Transition |
|---|---|---|---|
| Primary CTA | `ON_CLICK` | `NAVIGATE` to next screen | `PUSH RIGHT`, or `SMART_ANIMATE` when frames share layers |
| Back button | `ON_CLICK` | `BACK` | `PUSH LEFT` |
| Tab/nav peer | `ON_CLICK` | `NAVIGATE` to peer | `DISSOLVE` or instant |
| Modal/dialog open | `ON_CLICK` | `OVERLAY` | `DISSOLVE` or `MOVE_IN BOTTOM` |
| Modal close | `ON_CLICK` | `CLOSE` | Reverse of open |
| Dropdown/menu | `ON_CLICK` | `OVERLAY` manual anchored | Short `DISSOLVE` |
| Bottom sheet | `ON_CLICK` / `ON_DRAG` | `OVERLAY` bottom | `MOVE_IN BOTTOM` |
| Toast/snackbar | `AFTER_TIMEOUT` open then close | `OVERLAY` -> `CLOSE` | `MOVE_IN`/`MOVE_OUT` |
| List item -> detail | `ON_CLICK` | `NAVIGATE` | `PUSH RIGHT` mobile, `DISSOLVE` desktop |

Sources: [Prototype actions](https://help.figma.com/hc/en-us/articles/360040035874-Prototype-actions) · [Connect your prototype](https://help.figma.com/hc/en-us/articles/360040315773-Connect-your-prototype).

## Triggers

`Trigger` values: `ON_CLICK`, `ON_HOVER`, `ON_PRESS`, `ON_DRAG`,
`AFTER_TIMEOUT`, `MOUSE_UP`, `MOUSE_DOWN`, `MOUSE_ENTER`, `MOUSE_LEAVE`,
`ON_KEY_DOWN`, `ON_MEDIA_HIT`, `ON_MEDIA_END`.

- Mobile uses `ON_CLICK`, `ON_DRAG`, and `ON_PRESS`; never hover.
- Desktop may add hover and keyboard triggers.
- `AFTER_TIMEOUT` fits splash, loading, carousel, and toast flows.
- `ON_CLICK` maps to click on desktop and tap on mobile.

Source: [Prototype triggers](https://help.figma.com/hc/en-us/articles/360040035834-Prototype-triggers).

## Transitions

`Transition.type`: `DISSOLVE`, `SMART_ANIMATE`, `SCROLL_ANIMATE`, `MOVE_IN`,
`MOVE_OUT`, `PUSH`, `SLIDE_IN`, `SLIDE_OUT`. Directional transitions use
`LEFT | RIGHT | TOP | BOTTOM`; durations are seconds.

| Intent | Transition | Direction | Easing |
|---|---|---|---|
| Forward navigation | `PUSH` | `RIGHT` | `EASE_OUT` |
| Backward navigation | `PUSH` | `LEFT` | `EASE_OUT` |
| Modal/sheet appear | `MOVE_IN` | `BOTTOM` | `EASE_OUT` |
| Modal/sheet dismiss | `MOVE_OUT` | `BOTTOM` | `EASE_IN` |
| Tab switch / peers | `DISSOLVE` | - | `LINEAR` or `EASE_IN_AND_OUT` |
| State change with shared layers | `SMART_ANIMATE` | - | `EASE_OUT` or `GENTLE` |

Pick by similarity:
- Near-identical screens with shared chrome and changed content -> `SMART_ANIMATE` + `EASE_IN_AND_OUT`.
- Distinct screens -> directional `PUSH`.
- Element state changes -> `SMART_ANIMATE`.
- Layered surfaces -> overlay `MOVE_IN`/`MOVE_OUT`.

Community convention: UI transitions usually sit around 150-600ms, with about
300ms common. Avoid wrong directions, durations over 600ms, and Smart Animate
between unmatched layer names.

## Overlays

Overlay position, scrim, and dismiss behavior are destination-frame properties and
are read-only via the API. The skill can write `navigation:"OVERLAY"` and
`overlayRelativePosition`; it cannot set the destination's `overlayPositionType`,
`overlayBackground`, or `overlayBackgroundInteraction`.

- Dialog/alert -> CENTER + scrim + close-on-click-outside.
- Dropdown/menu/tooltip -> MANUAL anchored + no scrim.
- Bottom sheet -> BOTTOM + scrim.
- Toast -> TOP/BOTTOM + no scrim + auto-dismiss.

If a dropdown/sheet-looking frame still has `overlayPositionType:"CENTER"`, flag it
for Figma UI configuration before wiring.

Dropdown recipe: write open reaction on the trigger with `OVERLAY` and
`overlayRelativePosition`; write `CLOSE` on explicit dismiss controls. Click-outside
dismiss must be configured in Figma UI.

## Interactive components

Build state interactions into the component set, not on every instance. Wire
`CHANGE_TO` between variants for hover, pressed, selected, expanded, and toggled
states. Use `SMART_ANIMATE` when variants share named layers. For a library instance,
wrap or localize a master before adding inherited state interactions.

Source: [Interactive components](https://help.figma.com/hc/en-us/articles/360061175334-Create-interactive-components-with-variants).

## Flow starts

Use `set_prototype_start` for page flow starting points. A page can hold many flows;
a top-level frame can be in multiple flows but has one starting point. Good flow names
are product-level tasks such as "Onboarding" or "Checkout".

Natural start candidate: no incoming connection, lowest name index, top-left
position. Source: [Guide to prototyping](https://help.figma.com/hc/en-us/articles/360040314193-Guide-to-prototyping-in-Figma).

## Anti-patterns

- Reachable non-terminal frame with no outgoing interaction.
- Forward navigation with no `BACK`/return path.
- Forward `PUSH LEFT` or back `PUSH RIGHT`.
- Long, springy, or gratuitous transitions.
- Same element type wired differently across screens.
- Smart Animate between layers that do not share names.
- Overlay with no close/dismiss path.
- Hover trigger on a mobile prototype.
