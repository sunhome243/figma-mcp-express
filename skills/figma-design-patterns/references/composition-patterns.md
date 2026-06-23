# Composition Patterns

Structural and semantic patterns for the most common UI regions. Use these as the starting model before consulting the library for exact component keys and dimensions.

**Quick navigation:** [Reuse/DRY](#reuse) · [Typography](#typography) · [Navigation (nav/sidebar/tabs)](#navigation) · [Forms](#forms) · [Buttons](#buttons) · [Data display (tables/lists/stats)](#data-display) · [Modals](#modals) · [Fixed-height frames](#fixed-height) · [Color & fills / status encoding](#color) · [Empty states](#empty-states) · [Loading/skeletons](#loading)

---

## Reuse & componentization (applies everywhere)

When the same composite unit appears more than once — a card, a list/table row, a stat cell, a payment-method row, a message bubble, a bracket node, even a **skeleton** placeholder card — build it ONCE as a (file-local) component and place **instances**, never copy-paste duplicate frames. Three hand-duplicated card frames that drift independently is the classic maintainability failure: a later edit has to be redone N times and the copies diverge. The tell: if you catch yourself selecting a frame → copy → paste → nudge, stop and make it a component. This holds for **authored** structures too, not only library kinds — a loading/skeleton placeholder that repeats is still a repeated unit and gets componentized (the populated row and its skeleton are simply two components, each instanced). Trivial primitives — a 1px divider, a lone icon — don't need their own component; the rule is about repeated **composite** units. Reuse the SAME master across screens and across states so the whole product edits in one place. **Building the component is not the finish line — you must INSTANCE it at every occurrence.** A recurring failure is to create the row/card component and then hand-build (copy-paste) the actual rows anyway, leaving the component sitting UNUSED on the page. Having a component you didn't instance is the same defect as no component at all.

## Typography rules (applies everywhere)

- Use **text style references** from the design library — never set font/size/weight manually.
- `textAutoResize = "HEIGHT"` for all body copy, descriptions, and multi-line labels. `"NONE"` is almost always wrong — it clips text silently.
- `textAutoResize = "WIDTH_AND_HEIGHT"` for single-line display text that should shrink the frame to its content.
- **Declare font intent before text mutation.** Use the `create_text` / `set_text` **batch op** params for `fontFamily`, `fontStyle`, and `fontSize` so the server can load fonts before applying text changes. Missing font intent causes fallback fonts or failed text styling.

```
Batch text pattern:
  op 0: create_text with text, fontFamily, fontStyle, fontSize, textAutoResize
  op 1: set_text for later content/style changes
  op 2: get_node to verify style fields and textAutoResize
```

---

## Navigation patterns

**Chrome is shared, not re-drawn.** An app bar, header, or bottom nav appears on many screens and in every state (default / loading / empty / error) of a single screen. It must be the **same component instanced** in each place — identical alignment, height, padding, and contents. The failure mode to avoid: building the header fresh per state, so it ends up left-aligned with an icon on the default state but centered and icon-less on the loading and empty states. If you catch the same chrome diverging across frames, make it a component once and instance it; differences between states should live in the *body*, never in the chrome.

**Shared chrome must survive OVERRIDES — and an inherited flaw is NOT "consistency."** Build the master so a screen that retitles or reconfigures it can't break it: a header title that instances override must FILL the space between the fixed side slots (back-chevron, right-action) with `textAlignHorizontal = CENTER` (or LEFT+RIGHT stretch constraints), NEVER a fixed-x / HUG title sized for one specific string. A title hard-positioned for "Detail" strands off-center the instant another screen overrides it to a longer or shorter title — and every screen inherits that defect. The trap to avoid: a reviewer (or builder) seeing the off-center title and excusing it as "consistent with the canon master." **A defect inherited consistently across every screen is still a defect.** If the shared master itself is mis-built (off-center title, wrong height, a dropped affordance), fix the MASTER once so all instances inherit the correction — do not replicate the flaw everywhere and defend it as canon. The same applies to dropped chrome in non-default states: an empty/error state must carry the SAME pinned bars/CTAs as the default (the action matters *more* when the list is empty), not silently omit them. To HIDE a side slot in a centered auto-layout header (e.g. the right-action icon on a screen that has none), set its **`opacity: 0`, NOT `visible: false`** — `visible: false` removes it from the flex flow, so the auto-layout recomputes and the centered title shifts off-center (and on an instance that de-centered position bakes in as a hard-to-undo override). `opacity: 0` keeps the slot in the flow so the title stays symmetrically centered while the icon is invisible. **Root cause of the whole "title drifts off-center on screens with no right-action" class:** a title that is FILL-centered *between two present side slots* derives its center from both slots existing — so the moment one slot is removed from the flex flow (a screen with no right-action hides it via `visible:false`), the title loses its counterweight and drifts toward the remaining slot. Build the centered header so its centering does NOT depend on both slots being present: use a 3-zone layout `[left-slot FIXED-width | title FILL-center | right-slot FIXED-width]` where each side zone RESERVES its width even when empty, OR center the title relative to the FRAME (constraints) rather than between siblings. Then hiding/omitting a side action (via `opacity:0`, or an empty-but-space-reserving slot) can never shift the title. A header master built this way is override-proof; one that isn't will de-center on every screen that doesn't use the right-action.

**Structured / multi-line text gets real line breaks.** A run of label-value facts (bank / account / holder / deadline; date / time / court; address blocks) or any long sentence must wrap and break cleanly — give prose `textAutoResize = "HEIGHT"` so it wraps within the container padding, and lay structured fields as **separate lines or rows, one fact per line**, never a single non-wrapping run-on string that overflows or clips. Cramming `은행: … · 계좌: … · 예금주: … · 기한: …` onto one line is a craft FAIL; break it into readable rows. Attending to these wrap/line-break details is part of production craft, not optional polish.

### Top nav / app bar

| Property | Rule |
|---|---|
| Width | `FILL` — spans full screen width |
| Height | `FIXED` — use the library component's defined height |
| Layout | Horizontal auto layout, `SPACE_BETWEEN` or FILL children |
| Left group | Logo/wordmark: `HUG` |
| Right group | Actions/avatar: `HUG` |
| Background | Design variable token — never raw hex |
| Active state | Variant property on the nav component, not a manual fill override |

If the library has a top-nav or app-bar component: import it. Do not build from a raw frame + text + icon.

### Sidebar / left nav

| Property | Rule |
|---|---|
| Width | `FIXED` — use the library component's defined width |
| Height | `FILL` — takes full available screen height |
| Layout | Vertical auto layout |
| Items | `FILL` width inside the sidebar |
| Active state | Variant property, not manual fill override |
| Collapse/expand | Separate component variant or separate component, not a hidden-layer trick |

### Bottom tab bar (mobile)

| Property | Rule |
|---|---|
| Width | `FILL` |
| Height | `FIXED` |
| Layout | Horizontal auto layout, `SPACE_BETWEEN` |
| Each tab | `FILL`, centered icon + label (vertical stack) |
| Active state | Variant property |
| Safe area | Use the library's bottom-safe inset token for `paddingBottom` |

---

## Form patterns

Every form field is a **molecule**: label row + input control + optional helper/error text, stacked vertically in an auto-layout frame. Never collapse this to a bare input with no label.

### Anatomy of a single field

```
FieldGroup (vertical auto layout, gap = field-gap token)
├── LabelRow (horizontal auto layout)
│   ├── FieldNameText  (TEXT node, text style = label style)
│   └── RequiredMark   (TEXT node "*", color = required-color token) ← only when required
├── InputControl       (library component instance — see input-type map below)
└── HelperText         (TEXT node, text style = helper style, hidden by default)
    // OR ErrorText    (same position, error variant visible on error state)
```

### Input type → component mapping

| Visual cue in design | Component to use |
|---|---|
| Plain text entry | Text input / Input field component |
| Chevron / arrow → opens list | Select / Dropdown component |
| Calendar icon → date picker | DatePicker component |
| On/off toggle | Switch / Toggle component |
| Checkbox list | Checkbox group component |
| Radio group | Radio group component |
| Multi-line text | Textarea component |

**Never ship a component's factory/English default placeholder** ("Type here", "Enter text", "Placeholder", "Label") on a localized form — set field-appropriate copy IN THE PRODUCT LANGUAGE via the input's placeholder property (e.g. an empty Korean form needs Korean placeholders). Spec silence on placeholder copy licenses designing it well, not shipping the untouched factory default — a leftover English default in a primary field is an instant not-production-ready tell. Never use a text input to fake a dropdown. Never use a raw frame to fake a toggle. Use the component whose semantic role matches the design intent. And the reverse trap: never use a Select/Dropdown (anything with a chevron) to fake a plain text field — a chevron-down on an email/name/text input is a clear bug. If the library lacks an error-state text input, use the default text input with error styling (red border + error text), never substitute a Select.

### Form states

All states are variant properties — never duplicate frames:

**Select the real state VARIANT — don't FAKE a state with overrides.** A disabled/error/focus state must be the component's actual `state=` variant (`setProperties state=disabled`), not the enabled variant with a hand-applied grey fill. A faked state looks right but the instance still REPORTS `enabled` — it desyncs from the library's true state and breaks code/handoff fidelity.


| State | Variant property value |
|---|---|
| Normal / resting | `"Default"` |
| Focused (cursor in field) | `"Focus"` |
| Filled (has value) | `"Filled"` |
| Error | `"Error"` — show ErrorText below the control |
| Disabled | `"Disabled"` — entire FieldGroup, not just the input |

---

## Buttons (labels + width)

**One button spec, reused everywhere (visual consistency).** Pick the canonical variants ONCE and use the SAME ones on every screen: primary CTA = the kit's Solid Button at one chosen size (e.g. `size=lg`), secondary = Outline at the same size, destructive = its destructive variant. **Never set an explicit button height.** The variant already carries a native height (e.g. `size=lg` resolves to ~40px from its own padding + line-height); let the button keep it with `layoutSizingVertical=HUG`. Hand-setting a height — `36` on one screen, `48` on another — is the #1 source of button-thickness drift, and it's the failure a user spots immediately ('the buttons are different sizes'). Two buttons of the same variant sized HUG are guaranteed identical; two with hand-set heights are guaranteed to diverge. So: lock the variant in the project cheat-sheet, size every button `HUG` (never a pixel number), and audit every screen to the same variant.

**Exception — the touch floor overrides HUG.** HUG keeps buttons consistent, but a control must never render below the platform's touch-target floor (≈44pt iOS / 48dp Android; pointer/desktop has no such floor). Web-origin kits (shadcn-class and most pointer-first libraries) size their controls for a mouse — their *largest* button is often only ~36–40px, **under** the mobile floor — so on a touch platform, HUG-native alone ships a sub-floor target. When the chosen variant's native height is below the floor: size that button up to ≥ the floor (FILL its CTA bar, or ONE deliberate height applied uniformly to every same-role button) **and** re-center the label with the verified label-node method below. Do it at the **build layer** — never edit the shared kit master just to gain height or centering (the instance method handles it). Sizing *up to a single uniform touch-safe height* is NOT the thickness-drift the HUG rule warns against; mixing hand-set heights screen-to-screen is. This is a modality mismatch (web kit on a touch product), not a kit bug — expect it and adapt at the application layer.


A button's label belongs to the **button's own text/label property** — set it via the component's label property (`set_instance_properties`), NEVER by dropping a separate text node inside or on top of the button. An overlaid label floats by its own coordinates (so it misaligns), and it is lost when the variant is swapped.

**Full-width buttons must CENTER their label.** Kit buttons HUG their content and center it by default — but the moment you stretch one to FILL (a full-width CTA), its content stays pinned to the LEFT padding edge unless you re-center it, leaving the label stuck left with dead space on the right (the classic "alignment is messed up" tell). When a button is FILL / full-width, set its `primaryAxisAlignItems = "CENTER"` and `counterAxisAlignItems = "CENTER"`. **Vertical centering (VERIFIED method):** kit buttons often bottom-align their label (master `counterAxisAlignItems=MAX`), so the text sits a few px low. Do NOT try `set_auto_layout` on the button (it rejects instances) — you don't need it. Center the label ON THE LABEL TEXT NODE: `resize_nodes {layoutSizingVertical:"FILL"}` (label grows to the button's content height) + `set_text {textAlignVertical:"CENTER"}` (glyph centers), plus horizontal FILL + `textAlignHorizontal=CENTER`. Verified to center the label both axes. (This is a skill technique, not an MCP limitation — before declaring "can't", check for the op on the right node.) And never give a button a FIXED width wider than its container — size it FILL within the container's padding so it can't run off the screen edge.

## Data display patterns

### Tables

- If the library has a Table component: import it. Do not assemble from Rectangle + Frame rows.
- Use real data in every row — no "Data 1", "Lorem ipsum", or placeholder text.
- Column widths: fixed for known-length content (IDs, dates, status badges); FILL for variable-length text (names, descriptions).
- Every table needs an **empty state** — a design for zero rows. Do not skip it.
- Header: use the library's table-header component or variant; never a plain Frame + bold text.
- Pagination, sorting controls: use library components; never hand-draw chevrons.
- **A table must READ as a table — visible structure, not floating text.** Verify the rendered result has all three: (1) a header clearly delineated from the body — a distinct background AND a 1px bottom rule (a pale `#f5f5f5` band alone on white is invisible); (2) **1px dividers between rows** (border-color token) so rows don't blur together; (3) columns whose data sits directly under its header with comfortable spacing — numeric columns right-aligned with breathing room, never two narrow numbers crammed a few px apart at the edge. Optionally contain the whole table in a bordered/rounded card so it reads as one unit. Borderless white rows on a white surface with no header rule are loose text, not a table — a craft FAIL the reviewer MUST actively check (read the row frames: no divider / no header rule / crammed columns = fail).

### Lists

- Use library list-item or row components where available.
- Consistent row height: FIXED within a list; never let individual rows vary unless the component explicitly supports multi-line rows.
- Dividers between rows: use the library's separator component or a 1px FIXED-height frame with a border-color token fill.
- Empty state: required. Design it.

### Stat / metric displays

- Use a horizontal auto-layout row with FILL children for equal-width stat cells.
- Each cell: vertical auto layout, center-aligned, `HUG` height.
- Metric value: display text style (large, bold). Label: secondary text style (small, muted).
- Separator between cells: 1px FIXED width, FILL height, border-color token — not a raw color.
- Do not use one Card component per stat and line them up manually — this creates rigid, unequal widths.

---

## Modal and overlay patterns

| Property | Rule |
|---|---|
| Overlay background | Semi-transparent fill using a design variable token — never raw `rgba(0,0,0,0.5)` |
| Modal width/height | Follow the library's modal component defined sizes — never invent arbitrary dimensions |
| Modal shell | Library component instance (Dialog, Modal, Sheet) |
| Footer buttons | Library button instances inside the modal's footer slot |
| Close action | Library icon-button instance or the modal's built-in close slot |
| Scroll | If content can overflow: the modal body scrolls, the header/footer stay fixed |

Do not build a modal from a Frame + drop shadow + raw button. Use the library's modal component and configure its slots.

---

## Fixed-height frames (mobile screens)

A mobile screen is a **fixed-height frame** (e.g. 844px) with, often, a **pinned bottom bar/CTA**. Every region the spec requires must fit *inside* the frame and *above* the pinned bar. Two failure modes, both = a region silently disappears:
- **Oversized placeholders.** A cover/hero/image sized to half the screen pushes the real content (badges, description, secondary actions) below the fold or under the pinned CTA. Size placeholders modestly (a cover is ~180–220px, not ~500px).
- **No bottom budget for the pinned bar.** If the scrollable content column has no bottom padding equal to the pinned CTA height, its last region renders *behind* the opaque CTA and spills past the frame edge. Reserve `paddingBottom ≥ CTA height` on the content column (the canon pattern), and make the column auto-height/scrollable — don't fix it to a height that's shorter than its content.
Never rationalize clipped/occluded content as "it scrolls" or "acceptable mobile behavior" — if a required region isn't visible, it's a defect. Fix by shrinking placeholders or tightening spacing.

### A container must never be smaller than its content (the most common clipping bug)

A frame whose fixed size is smaller than its children — or one hugged to *exactly* a child that carries an **outside stroke or a drop-shadow** — clips whatever extends past the frame bounds. Outside strokes and effects render *beyond* a node's box, so a wrapper sized to the child's exact bounds shaves the child's stroke/shadow at the edges, and the element reads as "cut off." Text behaves the same way: a frame shorter than the text's line-height clips descenders.

The fix is structural, not cosmetic:
- **Default to HUG.** Structural frames should size to their content (`layoutSizing` HUG on the relevant axis) so the container grows to fit — it can never be too small for what's inside.
- **If a frame must be a fixed size, set it ≥ content + any outside-stroke/effect extent**, never a hand-picked number that happens to be smaller. Don't pin a child into a frame thinner or narrower than the child.
- **If `clipsContent` is on, verify nothing visible crosses the edge** — stroke, shadow, last row, text descenders. When in doubt, either turn clipping off for that wrapper or add padding so content doesn't kiss the boundary.
- **Verify by screenshot, per edge.** Scan every element's four edges for a shaved stroke/shadow/glyph; any clip means the container is undersized — enlarge it, don't ship it.

Pair this with breathing room: elements need air (section padding 16–24, item gaps 8/12/16), never zero-gap touching or a cluster jammed against an edge. A cramped or clipped composition reads as broken even when every required element is technically present — sizing and spacing are the builder's craft to get right, not the spec's to dictate.

## Color and fills (all patterns)

- All fills use design variable tokens through the `set_fills` (with `variableId`) or `bind_variable_to_node` **batch op types**. **Matching a token's hex value by hand is NOT binding.** Writing `#c2410c` because that happens to be the accent value leaves a raw-hex fill with no `VariableID` — it won't mode-switch, won't update when the token changes, and fails the zero-raw-hex check. You must actually bind the variable so the fill carries a `VariableID`. (Imported library instances keep their own paints — that's fine; the rule is for fills YOU author.)
- Dark/light mode: set variable mode on the top-level wrapper. Variable tokens cascade to every child automatically. Never manually rebind children for dark mode.
- Effects (shadows, blurs): use effect style references from the library — never set raw `boxShadow` values.
- Stroke: bind stroke width and stroke color to library tokens through the matching catalog-backed batch ops.
- **Icons are colored, not left default.** An icon takes a semantic color the same way text does: `muted` when it's inactive or secondary, `foreground` by default, `accent` when its item is active/selected, the status color when it marks status. Bind it — don't ship an icon at the kit's arbitrary default fill. Most telling failure: an icon sitting next to a `muted` label while the icon itself is full-contrast `foreground` — the pair must agree, or the row reads broken. **Color the GLYPH, never the icon FRAME.** An icon = a container frame + an inner glyph vector. Put color ONLY on the glyph's stroke/fill; NEVER put a fill on the icon frame (it renders as a solid TILE behind the glyph) and NEVER put a stroke on the icon frame (it renders as a 1px BOX around the glyph). Both read as broken/placeholder. Kit icon instances carry their glyph stroke internally — bind/override THAT; leave the frame's fill and stroke empty. (A clean icon instance with no frame stroke is the reference.)
- **A status’s color is a fixed vocabulary across the whole product.** The SAME status uses the SAME color on every screen (a list’s “open” pill and a detail’s “open” pill must match). Reusing one status’s color for a DIFFERENT status on another screen (e.g. “open” shown in the color another screen uses for “live”) is a cross-screen semantic collision a user notices when navigating between screens — a craft FAIL. Lock status→color once (from the canon screen) and reuse it everywhere.
- **Status is never color alone.** A status (open / live / closed, success / error) is encoded by color **and** text and/or an icon, so it survives color-blindness, grayscale, and sunlight. Two different statuses must also stay visually distinguishable from each other — don't render "open" and "closed" as the same neutral chip.
- **A status is never BARE TEXT, either — encode every status placeholder.** This is the one that slips through dense layouts (brackets, tables, lists): a `bye` / `TBD` / `awaiting` / `advancing` / `eliminated` / `live` / `final` marker must be **visually DISTINGUISHED from the surrounding data — but with a context-appropriate, usually SUBTLE treatment**, not a plain text run in the same style as the data. Match the treatment's weight to the context: a muted/italic placeholder for a bye/TBD slot, a winner-bold + loser-mute for a finished match, a small status-color accent or a 6px dot for "live", a thin row tint or left-rule for "advancing". Two failure modes, equal and opposite: (1) **bare text identical to the data** (a "bye" that looks like a team name, an "advancing" rank with no marker) — invisible status; and (2) **blanketing every status with a loud badge/pill/chip** — visual noise that screams where a quiet cue would do. The goal is the quietest treatment that still reads at a glance; reach for a loud badge only when the status is genuinely a primary signal (e.g. a single "LIVE" match), not for every cell. The failures to catch: a "bye" (부전승) or "winner-TBD" (승자 가림) rendered as a team-name-styled text so it reads like an actual team; a standings table that never marks which rows **advance** (진출) vs are out (no badge, no highlight, no cut-line); a "LIVE" badge floating at the *section* level instead of sitting **on the specific live item**. Each of those reads as unstyled data, not status. **Reviewers MUST actively scan for bare-text statuses** — read the text nodes and ask "is this word a status? then does it have a badge/icon/color, and is it attached to the right item?" A status word indistinguishable from adjacent content, or a state (live/done/upcoming, advancing/out) with no per-item visual marker, is a craft FAIL — do not pass it.
- **Active states: tint, don't flood.** Prefer an accent *tint* or a slim indicator (underline, dot, left-bar, colored icon+label) to mark the active nav/tab item. A large saturated-accent solid background block is heavy and de-rations the accent; use it only when the design system explicitly specifies that fill.

---

## Empty states (required for every data surface)

Every table, list, grid, or feed needs an empty state design:
- Illustration or icon (from library)
- Heading: "No [items] yet" or equivalent
- Optional sub-text explaining next action
- Optional primary CTA button

The empty state must be a separate frame or component variant — not a hidden layer toggled by prototype logic. Design it explicitly.

**Layout — center and contain.** Put the icon, heading, and body in a **center-aligned, padded** container (horizontally centered, vertically centered or comfortably below the chrome). Give the body text `textAutoResize = "HEIGHT"` and a width that respects the container padding so it **wraps** instead of running off an edge. The recurring bug: an empty/loading message left-aligned and positioned by hand, so its first characters are clipped at the left margin and the line spills past the right. Centered + auto-height + padded prevents it. Keep the shared chrome (header, tab bar) identical to the populated state.

**Empty-state icon + hierarchy.** Tint the icon's GLYPH STROKE to a muted token — never put a fill on the icon FRAME. A stroke-glyph icon with a frame fill renders as a solid dark/grey TILE behind the glyph and reads as broken (the single focal element of the empty screen, looking like an error). And keep the heading larger/heavier than the body line (heading ≥18 semibold, body ~14 muted): an empty state whose explanatory body is bigger than its heading is an inverted hierarchy.

## Loading states

A loading state stands in for content that's about to arrive, so it should **mirror the shape of that content** — if the loaded screen is a list of cards, the skeleton is a stack of card-shaped skeletons with blocks where the title/meta/figure go; if it's a stat row, skeleton stat cells. A generic "circle + two lines" repeated down the page is a tell that the skeleton was dropped in without looking at the real layout, and it makes the page visibly lurch when data replaces it. Concretely: walk THIS screen's actual regions and give EACH its own skeleton in the same place at the same size — a hero stat-strip → a 3-cell strip skeleton, a cover → a full-width block skeleton, list rows → row-shaped skeletons, a badge row → short pill skeletons, a paragraph → 2–3 lines. Skeletoning only the easy regions (and dropping the badges/description) leaves a height mismatch that lurches on load. A lone avatar-circle + two bars standing in for a rich hero is the tell you skeletoned a generic page, not THIS one. Every default content region must have a matching skeleton, and the skeleton column height must match the loaded height. **ROOT RULE — reproduce the loaded layout's EXACT metrics, don't hand-build a looser/denser stack.** Same element COUNT, same heights, same vertical gaps/itemSpacing, same padding as the real content. The reliable method: build the skeleton by cloning the real content's container and swapping each text/image node for a neutral block of identical size — so spacing is INHERITED, not re-guessed. A hand-built skeleton with rows jammed tighter (or MORE rows) than the real content reads CRAMPED/busy and lurches on load. Overlay-test: every skeleton block must land where the real element sits.

- Skeleton rows obey the same width discipline as real content: `FILL` or width ≤ the container's inner box — never let a skeleton bar overflow the frame edge.
- Keep the shared chrome (header, tab bar) identical to the loaded state — only the body swaps to skeletons.
- A short status line ("Loading…") is fine, but place it within the padded, centered/aligned body like any other text — and give it the SAME breathing room from the chrome as any body content. A status line jammed directly against the header (or half-clipped under the skeletons) with no gap reads as broken, not as a designed state. It belongs in the body region with real separation from the fixed chrome — never butted against the app bar.
- Prefer carrying the loading message INSIDE the skeleton body (or omitting it) over a lone line of text under the header. If you keep it, it must be a deliberately placed, padded element — not an orphaned string filling the gap between header and skeletons.
- Skeletons are real structure, not throwaway: when the skeleton repeats (a stack of card skeletons), make ONE skeleton component and **instance** it — never copy-paste N identical skeleton frames (see Reuse & componentization).
- Use the library's skeleton/shimmer component when one exists; otherwise neutral token-filled placeholder blocks.

## Scroll-ready, prototype-convertible mobile screens

Build tall mobile screens so they convert to a scrolling prototype with **no restructure** later. The rule: the scrolling content lives in ONE vertical content frame; the chrome (top header, bottom tab/CTA dock) is **pinned / scroll-fixed** via `pin_child`.

A screen built this way converts to a 390×844 prototype directly:

- the outer viewport frame **clips content** (`clipsContent`),
- the content frame is set to **vertical overflow scrolling**,
- the pinned chrome is **"fixed when scrolling."**

If instead the regions are scattered at the screen root, or the chrome isn't pinned, "make it scrollable later" becomes a rebuild. Keep content-in-one-frame + chrome-pinned from the start, and reserve `paddingBottom ≥ pinned-CTA height` on the content column so the last region never renders behind the dock (see the pinned-bar note above). Prototype reaction wiring itself → the `figma-prototype` skill.
