# States and Feedback Patterns

Use this for color, icon, status, empty, and loading rules. Pair it with
`composition-patterns.md` for structural layout.

**Quick navigation:** [Color and fills](#color-and-fills) · [Status encoding](#status-encoding) · [Icons](#icons) · [Empty states](#empty-states) · [Loading states](#loading-states)

---

## Color and fills

- Bind fills through library variables with `set_fills` using `variableId`, or `bind_variable_to_node`. Matching a token's hex by hand is not binding.
- Set variable mode on the top-level wrapper for dark/light mode. Token bindings cascade; do not manually rebind children.
- Use library effect styles for shadows/blurs unless the task specifically needs native effect mechanics from the MCP reference.
- Bind stroke color and stroke width through catalog-backed batch ops.
- Active states usually read better as tint, underline, dot, slim side bar, or colored icon+label. Avoid large saturated accent blocks unless the design system specifies them.

## Status encoding

- A status color is a fixed vocabulary across the product. The same status keeps the same color everywhere.
- Status is never color alone. Pair color with text and/or icon.
- Status is not bare text either. Mark status words such as `bye`, `TBD`, `awaiting`, `advancing`, `eliminated`, `live`, or `final` with a context-appropriate visual treatment.
- Match treatment weight to signal weight: muted/italic for quiet placeholders, winner-bold + loser-muted for finished matches, dot or slim accent for live, row tint or cut-line for advancing.
- Use loud pills only when the status is a primary signal. A dense table full of badges becomes noise.
- Place the marker on the specific item, not just the section. A `LIVE` badge floating at section level does not identify the live row/card.

## Icons

- Icons take semantic color: `muted`, `foreground`, `accent`, or a status token.
- Icon color must agree with the label next to it; a muted label plus full-contrast icon reads broken.
- Color the glyph, not the icon frame. A fill on the frame becomes a solid tile; a stroke on the frame becomes a box.
- Prefer library icon/button components. Do not hand-draw substitutes when a component exists.

## Empty states

Every table, list, grid, or feed needs a designed empty state.

Minimum anatomy:

```
EmptyState (centered vertical auto layout, padded)
├── Icon or illustration
├── Heading
├── Optional body text
└── Optional primary CTA
```

Rules:
- Build it as an explicit frame/component variant, not a hidden layer toggled by prototype logic.
- Keep shared chrome identical to the populated state.
- Center and contain the message. Body text uses `textAutoResize = "HEIGHT"` and a width that respects padding.
- Tint the icon glyph with a muted token; leave the icon frame unfilled/unstroked.
- Preserve hierarchy: heading larger/heavier than body text.

## Loading states

Loading mirrors the content about to arrive.

- Build skeletons from this screen's actual layout: same regions, count, heights, gaps, and padding.
- Prefer cloning the real content container and swapping text/images for neutral blocks. This preserves metrics and avoids load lurch.
- Skeleton rows obey the same width discipline as real content: FILL or within the parent's inner width.
- Keep shared chrome identical to the loaded state; only the body swaps.
- A loading label is optional. If present, place it as a padded body element, not jammed under chrome.
- Repeated skeleton cards/rows become one component plus instances.
- Use the library skeleton/shimmer component when one exists; otherwise use neutral token-filled blocks.
