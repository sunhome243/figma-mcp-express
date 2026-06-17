# Handoff Checklist

Run every check before marking any section or screen DONE. All eight must pass. A section that passes seven of eight is not done.

---

## The eight gates

### 1. Resize test

Set the wrapper width to **1200px**, then **1920px** (or the two extremes of your target breakpoint range).

At each width, ask:
- Do items clip or overflow the container? → **FAIL — layout is rigid**
- Do gaps explode disproportionately? → **FAIL — children should be FILL, not HUG**
- Does any column unexpectedly stack? → **FAIL — WRAP+FILL problem, switch to FIXED card width**
- Does the sidebar change width? → **FAIL — sidebar must be FIXED**
- Does any text node truncate that shouldn't? → **FAIL — check `textAutoResize = "HEIGHT"`**

If any answer is yes, fix the layout before continuing. Do not declare PASS based on a single width.

---

### 2. No placeholder *copy* (asset placeholders are fine)

Scan every visible text node. Any of these strings is a FAIL:

| Placeholder | What it means |
|---|---|
| "Title", "Heading", "Label" | TEXT slot not set |
| "Item 1", "Item 1-5" | Repeating slot or chart legend not configured |
| "Slot", "Content", "swap it" | INSTANCE_SWAP slot not assigned |
| "Button", "Submit" (default) | Button label not set to real content |
| Lorem ipsum | Body copy placeholder not replaced |
| Empty / blank where content is expected | Slot hidden or not wired |

Fix: use `set_instance_properties` for each slot hit. For chart legends showing `Item 1`, set the relevant visibility/property field to hide or replace the placeholder.

**Copy vs. asset — different rules.** This gate is about *words*. An **image asset** you can't source (a photo, cover, map tile, brand logo) MAY use a neutral placeholder fill — that is expected and correct, and omitting the visual entirely is worse (it changes the layout). So: forbid placeholder *text*, allow placeholder *images*. A card that should have a thumbnail and simply has none is a gap; a card with a neutral placeholder image is fine.

---

### 3. No hardcoded values

Every spacing, color, and stroke value must be bound to a design token. Run a mental audit:

| Property | Check |
|---|---|
| `paddingLeft/Right/Top/Bottom` | Must appear in `boundVariables`, not as a raw integer |
| `itemSpacing` | Must appear in `boundVariables` |
| `fills` (any node) | Must be a variable-bound paint, not a hex/rgb literal |
| `strokes` | Must be variable-bound |
| `strokeWeight` | Must be variable-bound (border-width token) |
| `cornerRadius` | Must be variable-bound (border-radius token) |
| Font size, line height | Must reference a text style, not inline values |

Any raw integer or hex that is not in `boundVariables` is a hardcode violation. Bind it before PASS.

**Exception:** If the project's design library has no spacing variables (confirmed by `export_tokens` returning no spacing entries), discrete-scale integers matching the library's documented scale (4/8/12/16/24/32) are acceptable. Document this exception in the project's memory/notes — it is not a general dispensation.

---

### 4. Library component coverage

For every interactive or semantic element, confirm it is an `INSTANCE` pointing to a library component — not a raw frame.

**Must be library instances (never raw frames):**
- Navigation bar / app bar
- Sidebar / left nav
- Buttons (primary, secondary, icon-only)
- Input fields, dropdowns, selects
- Icons (never unicode substitutes or colored circles)
- Modals / dialogs
- Pagination controls
- Badges / status chips

**How to spot a violation:** A raw Frame with manual fills, manual text, and no component link where any of the above should appear. Use `get_node` and check `type` — it must be `INSTANCE`, not `FRAME` or `GROUP`.

---

### 5. State completeness

Every interactive element must have all applicable states designed. Missing states are incomplete handoff.

| Element | Required states |
|---|---|
| Button | Default, Hover, Active/Pressed, Disabled, Loading (if applicable) |
| Input field | Default, Focus, Filled, Error, Disabled |
| Dropdown / Select | Default, Open, Selected, Disabled |
| List item / row | Default, Hover, Selected, Disabled |
| Tab | Default, Active, Disabled |
| Checkbox / Radio | Unchecked, Checked, Indeterminate (checkbox), Disabled |
| Toggle / Switch | Off, On, Disabled |

States are variant properties on the component instance — not duplicate frames. If a state is genuinely not applicable for this design, note it explicitly rather than leaving it undesigned.

---

### 6. Dark mode

Take a screenshot of the frame with dark mode applied (set the wrapper's variable collection mode to the dark mode ID). Inspect at full resolution.

Check for:
- Any element with a light fill that didn't update → manual fill override; bind to a token instead
- Any text that became unreadable (contrast failure) → wrong token binding; use the correct semantic token
- Any border or stroke that disappeared → stroke not bound to a token
- Background/surface colors that look identical in dark mode → semantic tokens may be wrong (check `surface` vs `background` vs `overlay` tokens)

A single manual fill anywhere in the frame will cause a dark-mode bleed. Find and bind it.

---

### 7. Layer naming

Every layer must have a semantic name. Auto-generated names are never acceptable in a finished frame.

| Bad | Good |
|---|---|
| `Frame 47` | `CardGrid` |
| `Group 12` | `HeaderActions` |
| `Rectangle 3` | `Divider` |
| `Text` | `PageTitle` |
| `Ellipse 1` | `Avatar/User` |
| `Component 1` | `Button/Primary/Default` |

Naming convention: `ComponentType/Variant/State` for instances; `SemanticRole` for structural frames; `Icon/name` for icon instances. Every layer name must be readable to a developer who has never seen this file.

---

### 8. Containment, shared chrome & encoding

The craft checks that separate "structurally correct" from "actually shippable." Each is a FAIL on its own.

- **Containment / no overflow** — no node's bounds spill past its parent's padded box. Walk the frame edges: a skeleton bar running off the right, a line of text clipped at the left margin, a card wider than its column. Fix with `FILL` or a width that respects padding — never by widening the frame.
- **Shared chrome is identical across states/screens** — the header/app bar and bottom nav/tab bar are the *same component instance* in the default, loading, empty, and error frames: same alignment, height, padding, and contents (same title position, same icons present). Chrome that's left-aligned with a bell on one state and centered without it on another = FAIL. Differences belong in the body, not the chrome.
- **Empty & loading states centered and contained** — icon + heading + body centered in a padded container; body text auto-height and wrapping, not clipped at an edge. A loading skeleton mirrors the real content's shape (card-shaped for a card list), not a generic avatar+lines list.
- **Icon color is semantic and agrees with its label** — `muted` inactive, `foreground` default, `accent` active, status color for status; an icon at the kit's default fill, or full-contrast beside a muted label, is a FAIL.
- **Status not by color alone** — every status reads via color **and** text/icon, and distinct statuses are visually distinct from each other (not two identical neutral chips).
- **Active state uses a tint/indicator, not a flooded accent block** — unless the design system explicitly specifies a solid-accent fill.

---

## Quick PASS/FAIL summary

```
[ ] 1. Resize test — 1200px and 1920px both hold
[ ] 2. No placeholder copy — real strings everywhere (asset/image placeholders OK)
[ ] 3. No hardcoded values — all spacing/color/stroke in boundVariables
[ ] 4. Library coverage — nav/button/input/icon/modal are all INSTANCE nodes
[ ] 5. State completeness — all interactive variants designed
[ ] 6. Dark mode — screenshot shows no light-mode bleed
[ ] 7. Layer naming — every layer has a semantic name
[ ] 8. Containment, chrome & encoding — no overflow; chrome identical across states; empty/loading centered+contained; icons semantic; status not color-alone; active = tint not flood
```

All eight checked = PASS. Anything unchecked = continue the fix loop. Do not write a completion note or move to the next section until all eight are checked.
