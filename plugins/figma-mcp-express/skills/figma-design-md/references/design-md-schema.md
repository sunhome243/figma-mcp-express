# DESIGN.md Token Schema

Load this when composing or validating the YAML front matter of a `DESIGN.md`.
For markdown body sections and prose requirements, read `design-md-sections.md`.

**Quick navigation:** [Format](#format) · [Schema](#schema) · [Value types](#value-types) · [Token names](#token-names) · [Unknown content](#unknown-content)

---

## Format

`DESIGN.md` is a plain-text design-system source of truth with two parts:

1. Optional YAML front matter containing machine-readable design tokens.
2. Markdown body containing human-readable design rationale and usage guidance.

The front matter starts and ends with a line containing exactly `---`.

```yaml
---
version: alpha
name: Daylight Prestige
colors:
  primary: "#1A1C1E"
typography:
  body-md:
    fontFamily: Public Sans
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.6
---
```

Tokens are normative. Prose may use descriptive names, but token values remain the
machine-readable source for export to Figma variables, `tokens.json`, or Tailwind.

## Schema

```yaml
version: <string> # optional, current version: "alpha"
name: <string>
description: <string> # optional
colors:
  <token-name>: <Color>
typography:
  <token-name>: <Typography>
rounded:
  <scale-level>: <Dimension>
spacing:
  <scale-level>: <Dimension | number>
components:
  <component-name>:
    <token-name>: <string | token reference>
```

Any descriptive token key is valid. Common scale names include `xs`, `sm`, `md`,
`lg`, `xl`, and `full`.

## Value types

**Color** may be any valid CSS color string:

- hex: `#RGB`, `#RGBA`, `#RRGGBB`, `#RRGGBBAA`
- named colors, `transparent`
- `rgb()`, `rgba()`, `hsl()`, `hsla()`, `hwb()`
- wide-gamut forms such as `oklch()`, `oklab()`, `lch()`, `lab()`
- `color-mix(in srgb, ...)`

Internally, colors are converted to sRGB for WCAG checks while preserving the
original format for display/export. Hex `#RRGGBB` is the recommended default.

**Typography** fields:

- `fontFamily` string
- `fontSize` Dimension
- `fontWeight` number or quoted number
- `lineHeight` Dimension or unitless multiplier
- `letterSpacing` Dimension
- `fontFeature` string for `font-feature-settings`
- `fontVariation` string for `font-variation-settings`

**Dimension** is a string with `px`, `em`, or `rem`.

**Token references** use `{path.to.token}`. Most groups reference primitive values,
for example `{colors.primary}`. Component tokens may reference composite values such
as `{typography.label-md}`.

## Token names

Recommended, non-normative names:

- Colors: `primary`, `secondary`, `tertiary`, `neutral`, `surface`, `on-surface`, `error`
- Typography: `headline-display`, `headline-lg`, `headline-md`, `body-lg`, `body-md`, `body-sm`, `label-lg`, `label-md`, `label-sm`
- Rounded: `none`, `sm`, `md`, `lg`, `xl`, `full`

## Unknown content

| Scenario | Behavior |
|---|---|
| Unknown section heading | Preserve; do not error |
| Unknown color token name | Accept if value is valid |
| Unknown typography token name | Accept as valid typography |
| Unknown spacing value | Accept; store as string if not a valid dimension |
| Unknown component property | Accept with warning |
| Duplicate section heading | Error; reject the file |
