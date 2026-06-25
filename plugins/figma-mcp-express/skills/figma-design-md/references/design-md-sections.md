# DESIGN.md Sections

Load this when composing the markdown body of a `DESIGN.md`. For YAML token
front matter, read `design-md-schema.md`.

**Quick navigation:** [Order](#order) · [Section guidance](#section-guidance) · [Component tokens](#component-tokens)

---

## Order

Sections may be omitted when irrelevant, but those present should follow this order.
Use `##` headings. An optional document `#` title may appear but is not parsed as a
section.

1. Overview, also "Brand & Style"
2. Colors
3. Typography
4. Layout, also "Layout & Spacing"
5. Elevation & Depth, also "Elevation"
6. Shapes
7. Components
8. Do's and Don'ts

## Section guidance

**Overview** describes product personality, audience, density, tone, and intended
emotional response. Use it for high-level style judgment when no token gives a direct
answer.

**Colors** explains palette roles. At least `primary` should be defined; additional
palettes may be semantic (`secondary`, `tertiary`, `neutral`, `surface`, `error`) or
domain-specific. The front matter's `colors:` tokens carry the actual values.

**Typography** explains type roles. Most systems use 9-15 levels across categories
such as `headline`, `display`, `body`, `label`, and `caption`. The front matter's
`typography:` tokens carry exact font fields.

**Layout** explains spacing/grid strategy: fixed or fluid grid, margins, gutters,
safe areas, density, and containment. The front matter's `spacing:` tokens carry
exact scale values.

**Elevation & Depth** explains hierarchy through shadows, tonal layers, borders,
blur, or flat contrast. If shadows are used, document spread/blur/color intent.

**Shapes** explains corner-radius language across buttons, inputs, cards, and other
rectangular controls. The front matter's `rounded:` tokens carry exact values.

**Components** gives style guidance for common atoms/molecules: buttons, chips,
lists, tooltips, checkboxes, radios, inputs, and domain-specific components.

**Do's and Don'ts** lists practical guardrails, for example contrast, accent use,
corner-radius consistency, and typography limits.

## Component tokens

The front matter `components:` map is intentionally flexible while the spec evolves.
Values may be literals or token references.

```yaml
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
```

Common component token properties:

- `backgroundColor`
- `textColor`
- `typography`
- `rounded`
- `padding`
- `size`
- `height`
- `width`

Variants may be separate related keys such as `button-primary`,
`button-primary-hover`, and `button-primary-active`.
