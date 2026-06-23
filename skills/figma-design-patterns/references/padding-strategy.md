# Padding Strategy

Padding and gap are distinct auto-layout controls. Use them deliberately; wrong
ownership is the fastest way to make a screen look right once and break on resize.

**Quick navigation:** [Concepts](#concepts) · [Ownership](#ownership) · [Token binding](#token-binding) · [Scale](#scale) · [Examples](#examples) · [Mistakes](#mistakes)

---

## Concepts

| Concept | API property | Controls |
|---|---|---|
| Container padding | `paddingLeft/Right/Top/Bottom` | space between frame edge and children |
| Item gap | `itemSpacing` | space between siblings in the same auto-layout frame |
| Wrap cross-gap | `counterAxisSpacing` | cross-axis gap for WRAP grids |

Do not use `itemSpacing` to create card breathing room; that belongs to the card's
own padding. Do not fake external margin on children; Figma has no margin.

## Ownership

The direct parent owns the spacing between its own children.

```
Screen -> padding = screen inset
  ContentArea -> itemSpacing = section gap
    CardGrid -> padding = grid inset, itemSpacing = card gap
      Card -> padding = card internal padding
```

If spacing disappears, doubles, or behaves differently at another width, the value is
probably on the wrong frame.

## Token binding

Every padding/gap should bind to a spacing variable. Import variables once up front,
then create/update frames with token-bound padding/gap fields and verify
`boundVariables`.

Allowed exception: when the library has no spacing variables, use documented scale
integers such as 4/8/12/16/24/32 and record the exception. This does not allow raw
colors, radii, shadows, or typography.

## Scale

Use the library's actual scale. Generic mapping:

| Token | Typical use |
|---|---|
| `xs` / 4 | icon-label micro gaps |
| `sm` / 8 | chips, dense rows |
| `md` / 12-16 | normal row gaps, card vertical padding |
| `lg` / 24 | section padding, card gaps |
| `xl` / 32-40 | page-level breaks |

Avoid invented between-step values such as 20 or 28 unless the design system defines
them. If the nearest token looks wrong, revisit layout sizing before inventing spacing.

## Examples

Card:

```
BAD: text node tries to create inset
GOOD: Card frame owns padding; text children have none
```

Sections:

```
BAD: each section uses top padding as fake margin
GOOD: page wrapper owns itemSpacing between sections
```

Grid:

```
CardGrid padding = grid inset
CardGrid itemSpacing = card gap
Cards do not fake inset with child padding
```

## Mistakes

| Mistake | Fix |
|---|---|
| Raw `paddingLeft:24` | bind padding to spacing variable |
| `itemSpacing` used for internal card padding | put padding on the card frame |
| Padding on child instead of direct parent | move it to the parent |
| Huge `itemSpacing` to spread HUG children | use FILL children + small token gap |
| Different values for same semantic role | define/reuse one token |

Cross-reference: `auto-layout.md` covers FILL/HUG/GRID and resize testing.
