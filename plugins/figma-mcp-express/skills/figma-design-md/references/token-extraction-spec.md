# Token Extraction — What to Collect and How

Reference for Phase G1–G2. Defines what to extract from each tool, how to map raw data to DESIGN.md token groups, and how to handle gaps.

**Quick navigation:** [Tool inputs](#what-each-tool-provides) · [Gap handling](#gap-handling) · [Return shape](#return-shape-what-phase-g3-consumes)

---

## What each tool provides

### `export_tokens`

Primary source for all variable-based tokens. Run once; covers all collections and all modes.

| What to look for | How to identify | Maps to |
|---|---|---|
| Color variables | `resolvedType: "COLOR"` | `colors:` |
| Spacing/gap/padding | `resolvedType: "FLOAT"`, name contains spacing / gap / padding / margin / size | `spacing:` |
| Border radius | `resolvedType: "FLOAT"`, name contains radius / corner / rounded / round | `rounded:` |
| Border width | `resolvedType: "FLOAT"`, name contains border / stroke / weight | record separately |
| Duration / easing | `resolvedType: "FLOAT"` or `STRING`, name contains duration / easing / motion | record separately |

**Multi-mode variables:** if a collection has more than one mode (e.g., Light / Dark), record:
- The default mode value as the YAML token value
- All mode names and their values in a `modes[]` array alongside — referenced in prose, not YAML

**Never invent values.** If a variable has no resolved value (aliased to another variable), follow the alias chain until you reach a raw value or flag it as unresolved.

---

### `get_styles`

Fallback and supplement when variables are absent or sparse. Also the primary source for typography.

| Style type | What to record | Maps to |
|---|---|---|
| `PAINT` (solid) | name, `color` hex | `colors:` — merge, no duplicate keys |
| `TEXT` | name, fontFamily, fontSize, fontWeight, lineHeight, letterSpacing | `typography:` |
| `EFFECT` | name, effect type, blur/offset/spread/color | `## Elevation & Depth` prose |
| `GRID` | name, pattern, sectionSize | `## Layout` prose |

For `TEXT` styles: derive the scale role from the name where possible:
- Display / Hero / Title / H1–H6 → headings
- Body / Paragraph / Text → body copy
- Label / Caption / Helper / Overline → supporting text
- Code / Mono → monospace

---

### `get_local_components`

Source for the `components:` YAML section and the `## Components` prose.

For each component family (group variants by their base name before `/`):
- Record: family name, variant property names and allowed values, default variant key
- Map to a DESIGN.md `components:` entry using token references — `{colors.x}`, `{rounded.x}`, `{spacing.x}`
- Limit to **12 entries max**, prioritising interactive atoms:
  `button`, `input`, `select`, `checkbox`, `radio`, `badge`, `card`, `modal`, `tab`, `nav-item`, `toast`, `avatar`

---

### `get_design_context` (on representative frames)

Used in Phase G2 to verify how tokens are used in practice and to extract gestalt signals.

What to record from each frame:

| Signal | JSON path | What it tells you |
|---|---|---|
| Active color tokens | `fills[].boundVariables.color.id` | Which color vars are actually used vs. defined |
| Spacing in use | FRAME nodes → `paddingLeft/Top/Right/Bottom`, `itemSpacing` | Real rhythm vs. token names |
| Corner radius in use | `cornerRadius` on FRAME/RECT/INSTANCE | Real scale vs. token names |
| Active text styles | TEXT nodes → `style.textStyleId` | Which styles are applied vs. defined |
| Component families in use | INSTANCE nodes → `name` (before first `/`) | Which components dominate the UI |
| Variant states in use | INSTANCE → `variantProperties` | Which states are active (default/hover/error/disabled) |

Cross-check: if a value appears in `get_design_context` but has no matching token in `export_tokens` / `get_styles`, it is a hardcoded raw value — flag it in the `## Hardcoded Values` findings block (not in YAML, not in prose sections).

---

## Gap handling

If any of the following are missing after running all four tools, do NOT synthesize placeholders:

| Missing | What to do |
|---|---|
| `colors` empty | Note in delivery summary: "No color variables found — file may use styles only" |
| `typography` empty | Note: "No text styles found — typography section omitted" |
| `spacing` empty | Derive approximate scale from `get_design_context` frame padding values; mark as "inferred, not token-bound" |
| `rounded` empty | Same — infer from observed corner radii |
| `components` empty | Omit `components:` YAML block entirely |

Lint will surface missing required sections as errors — let the lint loop handle them rather than filling gaps with invented data.

---

## Return shape (what Phase G3 consumes)

```json
{
  "colors": [
    { "tokenName": "primary", "hex": "#...", "modes": [{ "name": "Light", "hex": "#..." }, { "name": "Dark", "hex": "#..." }] }
  ],
  "typography": [
    { "tokenName": "h1", "fontFamily": "...", "fontSize": 32, "fontWeight": 700, "lineHeight": "40px", "letterSpacing": "-0.02em" }
  ],
  "spacing": [
    { "tokenName": "sm", "px": 8 }
  ],
  "rounded": [
    { "tokenName": "md", "px": 8 }
  ],
  "effects": [
    { "tokenName": "shadow-md", "kind": "DROP_SHADOW", "blur": 12, "offsetY": 4, "color": "#00000026" }
  ],
  "components": [
    { "name": "button-primary", "key": "...", "variantProps": { "Variant": ["Primary", "Secondary"], "State": ["Default", "Hover", "Disabled"] }, "slots": ["label"] }
  ],
  "hardcodedValues": [
    { "nodeId": "...", "nodeName": "...", "property": "fills", "value": "#e5e5e5" }
  ]
}
```
