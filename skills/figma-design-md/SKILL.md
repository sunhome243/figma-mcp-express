---
description: >-
  Extract a DESIGN.md from a Figma file — tokens, text styles, component
  inventory, and design rationale. Required argument: Figma file URL or fileKey.
  Outputs DESIGN.md + optional Tailwind/DTCG exports. Read-only from Figma.
---

## REQUIRED ARGUMENT

```
/figma-design-md <figma-file-url-or-filekey>
```

Extract the fileKey from a URL: `figma.com/design/<fileKey>/...`

**Output:** `DESIGN.md` in the current working directory (or `--output <path>` to override).
**Optional exports:** `tailwind-theme.json` + `tokens.json` via `@google/design.md` CLI.

---

## Phase G0 — Bootstrap

Run all three in one message turn (parallel):

1. **Channel check** — `list_channels`. If multiple channels, identify which maps to the target fileKey. Record the `channel` value for all subsequent calls.
2. **CLI probe** — `npx --yes @google/design.md spec --format json > /tmp/dmd-spec.json 2>/tmp/dmd-spec.err`. Non-zero exit → hard stop: show stderr, tell user to check Node/npm.
3. **Output path** — default to `./DESIGN.md`. Overridden by `--output`.

---

## Phase G1 — Data Harvest

Run all four in one message turn (parallel):

```
export_tokens format:"json"
get_styles
get_local_components
get_pages
```

- `export_tokens` — all variable collections with modes and values; primary token source
- `get_styles` — local paint, text, effect, grid styles
- `get_local_components` — component inventory for this file
- `get_pages` — page list needed for frame discovery in G2

---

## Phase G2 — Visual Analysis

**Source priority rule:** tool call results are always authoritative. Screenshots are used to write prose (mood, brand character, visual harmony) — never to override or fill in values that tool calls returned. If a screenshot suggests a color that differs from `export_tokens`, trust the token. If a layout looks tighter than the padding values, trust the numbers.

Both structured data and screenshots are used. Numbers give exact values; screenshots give gestalt and design character that token tables can't capture.

**G2a — find representative frames** (from `get_pages` results):

Exclude pages whose name contains: Archive / Scratch / WIP / Draft / _old / Template (case-insensitive).
For each remaining page call `search_nodes types:["FRAME"] limit:5`. Score by child count. Pick up to **3 frames**:
- 1 densest screen (highest child count)
- 1 components/styles guide frame if a page is named Components / Styles / Library / Design System
- 1 alternate screen (different layout or density)

**G2b — read all targets** (one message, all parallel):

For each selected frame, issue both simultaneously:
```
get_design_context  nodeId:<frameId>  detail:"full"  depth:4
save_screenshots    nodeIds:[<frameId>]  (maxDimension: 1024)
```

**G2c — signals to extract:**

From `get_design_context`:

| Signal | Where | What to record |
|---|---|---|
| Color usage | `fills[].boundVariables` | Which token names appear most; primary vs. accent vs. background roles |
| Typography | TEXT nodes → `style.*` | Full size range, weight distribution, font families |
| Spacing rhythm | FRAME nodes → `padding*`, `itemSpacing` | Dominant values; regular scale or ad-hoc |
| Elevation | `effects[]` | Shadow blur/offset/spread — sharp vs. soft, tinted vs. neutral |
| Shape language | `cornerRadius` | Full range; consistent or component-specific |
| Component usage | INSTANCE nodes → `name`, `variantProperties` | Which families; which variants are active |

From screenshots (prose only — never use to override tool call data):
- Overall mood: minimal / expressive / enterprise / playful / premium
- Color harmony: cohesive or disparate; warm / cool / neutral
- Visual weight: light and airy vs. dense
- Brand personality: adjectives a designer would use
- Anything surprising not explained by the numbers

---

## Phase G3 — Compose DESIGN.md

### YAML front matter

| Key | Source | Rule |
|---|---|---|
| `colors` | `export_tokens` COLOR variables | Default-mode value as `"#RRGGBB"`. Multi-mode: record default; describe other modes in prose. |
| `colors` (supplement) | `get_styles` PAINT styles | Merge when variables are absent or sparse. No duplicate keys. |
| `typography` | `get_styles` TEXT styles | One entry per named style: fontFamily / fontSize / fontWeight / lineHeight / letterSpacing. |
| `spacing` | `export_tokens` FLOAT variables | Names containing spacing/gap/padding/margin/size. Value as `"<n>px"`. |
| `rounded` | `export_tokens` FLOAT variables | Names containing radius/corner/rounded/round. Value as `"<n>px"`. |
| `components` | `get_local_components` | Group variants by family. One entry per family. Use `{colors.x}` / `{rounded.x}` / `{spacing.x}` references — never literal values. Max 12 entries; prioritise interactive atoms. |

Never invent values not present in G1 data.

### Prose sections

Write each section using G2c structured observations as the factual backbone and screenshots as the aesthetic anchor.

**`## Overview`** — always write. 3–5 sentences. Design system character: what product it serves, visual tone, key aesthetic decisions. A developer reading this should immediately picture the product.

**`## Colors`** — palette's emotional register, semantic roles, multi-mode behavior if present.

**`## Typography`** — typeface personality first (from screenshots), then scale and weight distribution from data.

**`## Layout`** — spatial philosophy from screenshots, spacing rhythm from data (dominant padding/gap values, modular scale or ad-hoc).

**`## Elevation & Depth`** — flat+tonal vs. shadow-based, using actual blur/offset/spread values. Omit if no effects found.

**`## Shapes`** — corner radius philosophy: sharp / softly rounded / approachable / pill / mixed. Note component-specific deviations.

**`## Components`** — for each family in YAML: visual style, sizing philosophy, state communication.

Write draft to `<outputPath>.new` for atomic promotion after lint.

---

## Phase G4 — Lint

```bash
npx --yes @google/design.md lint <outputPath>.new --format json
```

Both errors and warnings trigger regeneration. Info findings are recorded only.

| Iteration | Outcome |
|---|---|
| 1st | Clean → continue. Findings → fix → re-lint. |
| 2nd | Clean → continue. Still failing → one more attempt. |
| 3rd | Still failing → ask user: Accept-with-findings / Cancel / Edit manually |

---

## Phase G5 — Write + Exports

**Promote:**
```bash
mv <outputPath>.new <outputPath>
```

**Exports** (run in parallel, both optional — skip if `--no-exports`):
```bash
npx --yes @google/design.md export --format tailwind <outputPath> > tailwind-theme.json
npx --yes @google/design.md export --format dtcg    <outputPath> > tokens.json
```

Export errors are non-fatal — record and continue.

---

## Delivery

```
✓ DESIGN.md: <output path>
  Source frames : [N] ([page / name list])
  Tokens        : [C] colors · [T] typography · [S] spacing · [R] rounded · [K] components
  Lint          : [n errors] [n warnings]  (iterations: [n])
  Exports       : tailwind-theme.json · tokens.json
```
