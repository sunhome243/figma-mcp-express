# Visual Craft — making it production-grade, not flat

How real designers decide **when** to reach past flat fills and basic shadows. This is judgment (WHEN/WHY); the op fields and HOW live in figma-mcp-express `references/effects.md` and the live catalog (`search_batch_ops`, `get_batch_op_spec`). Bind every value to a token/style — these moves are about *intent*, never raw values.

**Quick navigation:** [Elevation](#elevation--depth) · [Effects](#effects) · [Type ladder](#type-ladder) · [Imagery](#imagery) · [Custom shapes](#custom-shapes) · [Radius & blend](#radius--blend)

Status color, icons, and empty/loading states live in `states-and-feedback.md`; hierarchical spacing lives in `padding-strategy.md`. This file is the net-new aesthetic layer — cross-link, don't restate.

## Elevation & depth

Elevation encodes **z-order meaning**, not taste. Pick a shadow tier per layer and stay consistent:
- resting surface (page) → flat
- raised card / list row that can be grabbed → soft low shadow
- menu / popover / sticky bar / FAB → medium
- modal / dialog → strong, over a scrim
- toast / snackbar → strong, transient

**When to elevate:** a surface that floats above content, or that the user can move. **When NOT:** a card flush to the page background — flat-on-flat reads unfinished. Separate it by *one* of elevation **or** a token border, not both heavily.

**Depth must vary by hierarchy — uniform-tier is its own failure.** Applying one elevation tier (or one flat fill + one hairline) identically to every surface is not restraint, it's monotone: when the hero card and a footnote chip carry the *same* shadow, you've thrown away your strongest z-order lever. Let the one focal element per view sit visibly forward (a step up in elevation, a richer surface) while supporting elements stay quiet. Even a deliberately flat aesthetic needs this — flat means restrained materials, not zero hierarchy.

**How (judgment):** use bound effect styles via `set_effects` / `create_effect_style` — manual per-node shadows are a Stop-Flag. Higher float = larger, softer, lower-opacity blur with a small downward offset. Never let an elevation effect clip: inset the content, don't flush-fit the frame.

**Receding depth:** push background elements back with a slight `LAYER_BLUR` plus reduced scale/opacity so the foreground advances — modal scrims, hero backdrops, a focused card over a dimmed list.

## Effects

The single home for **when each effect reads well** (fields/params → figma-mcp-express `references/effects.md`). One hero effect per surface; stacking glass + noise + shadow on one node is noise. All bound via effect styles.

- **Drop / inner shadow** — elevation (above) and pressed/inset wells (inputs, toggles, recessed panels).
- **`GLASS`** — floating chrome over rich content: a translucent nav or tab bar over a photo or gradient. *Only works over a transparent / semi-transparent fill with content behind it* — over a solid fill it does nothing. Pair with a hairline border for edge definition.
- **`NOISE` / `TEXTURE`** — kill flat-gradient banding on large dark or vivid surfaces, or add tactile grain to a hero panel. Large fills only — on a small control it reads as dirt.
- **Background / progressive blur** — frosted overlays; progressive (ramped) blur fades a scrolling list under a sticky header so text doesn't collide at the edge. Use for legibility, not decoration.

## Type ladder

Hierarchy is a **ladder across three axes used together** — size, weight, color. A flat screen uses one size at one weight; the fix is contrast, not more elements.

- **Hero metrics / KPIs:** oversized numerals (a display style, 2–4× body), tight to a small muted label. The number is the hero; the label recedes. Dashboards, stat cells, pricing, balance screens.
- **The ladder:** display → title → body → caption/label, each a real text style (never an ad-hoc size). Jump *two* steps for a true focal moment, *one* for ordinary grouping.
- **Color ladder:** the **foreground (bright) token is the DEFAULT** for every piece of primary content the reader must actually read — body, list items, table cells, values, node labels. A muted/dim token is for *secondary support only* (a caption under a value, an eyebrow, a footnote) — never the body default, and never a multi-word sentence in the dimmest token. Accent is reserved for **one** emphasis per view. **Never encode emphasis or de-emphasis by lowering text contrast** — a "fade" just makes content unreadable; show rank with size, position, the accent, or a chip/border, keeping every label legible. If a whole column or card body reads gray, that's the bug, not a style. (Doubly true on dark surfaces, where a muted token sits close to the background.)

## Imagery

**When real photography beats a flat fill:** marketing heroes, avatars/profiles, product cards, onboarding — anything selling emotion, realism, or a real thing. A flat color block where a photo belongs reads as a wireframe.

- **Legibility:** when text sits over a photo, put a token-bound scrim or gradient between them — never raw text on a busy image.
- **Reserve the box:** keep the real aspect-ratio frame even when the source asset is missing (a missing *image* is acceptable; placeholder *copy* is not — see `handoff-checklist.md`).
- **Dark surface + one vivid accent:** a dark neutral surface carrying a single saturated accent (one CTA, one data highlight) is the highest-craft "not-flat" move that still stays restrained. Accent is scarce by design.

## Custom shapes

**When to drop to vector** — a glyph, mark, or diagram the library has no component for: a logo lockup, a chart segment, a ticket-stub notch, a speech-bubble tail. Try a library component or icon first (`component-reuse.md`); hand-craft only when none fits.

- `boolean_operation` — union / subtract / intersect / exclude to cut and combine shapes (a notch is a rectangle minus a circle).
- `import_svg` — ingest an existing vector asset rather than rebuilding it.
- `create_vector` — bespoke paths when nothing else fits.

Never hand-draw a shape a component or icon already provides.

## Radius & blend

- **Corner radius is a token ladder** (sharp → subtle → rounded → pill), consistent per surface tier. Mixing arbitrary radii across sibling cards reads broken. Nested corners must nest: child radius < parent radius. Set via `set_corner_radius`, bound to a radius token — per-corner values for one-sided shapes (a top-rounded sheet).
- **Blend modes** (`set_blend_mode`) are for compositing only: `MULTIPLY` for tints and scrims that darken realistically over imagery, `OVERLAY` / `SCREEN` for glow on dark surfaces. Never use a blend mode to fake a color a token should provide.
