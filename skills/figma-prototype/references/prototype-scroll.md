# Prototype Scroll and Fixed Children

Use this when a prototype needs scrollable frames, sticky headers, pinned tab bars,
or fixed children.

**Quick navigation:** [Ops](#ops) · [Scroll rules](#scroll-rules) · [Fixed-child model](#fixed-child-model) · [Recipe](#recipe)

---

## Ops

Find exact schemas through `search_batch_ops` / `get_batch_op_spec`.

| Op | Sets | Use for |
|---|---|---|
| `set_overflow` | `overflowDirection` (`NONE|HORIZONTAL|VERTICAL|BOTH`) and optional `clipsContent` | Make a frame scroll in presentation |
| `set_fixed_children` | `numberOfFixedChildren` | Low-level fixed-child count when order/position are already correct |
| `pin_child` | child -> `ABSOLUTE`, moved into the fixed band, parent count bumped | Convenience path for one pinned child |

## Scroll rules

- A nested frame scrolls only when content exceeds bounds and clipping is enabled.
- Use `set_overflow` with `clipsContent:true` and ensure content is taller/wider than the frame.
- A frame directly under the canvas can auto-scroll when bigger than the device; nested frames need explicit overflow/clipping.

## Fixed-child model

Figma has no per-layer fixed boolean. A frame keeps the leading N children fixed, so a
fixed child must be out of auto-layout flow (`layoutPositioning:"ABSOLUTE"`) and
ordered into the leading fixed band. `pin_child` does this; `set_fixed_children` only
sets the count.

`pin_child` is only for prototype scroll-fixed children. It is not the generic way to
float a decorative/background layer or a glass tab bar inside an auto-layout screen:
it reorders the child into the fixed band and changes `numberOfFixedChildren`. For
non-scroll floating children, use `resize_nodes` with `layoutPositioning:"ABSOLUTE"`,
then position/reorder the node explicitly.

## Recipe

Scrollable mobile body with pinned bottom tab bar:

1. `set_overflow` on the screen frame with vertical overflow and clipping.
2. Make body content taller than the screen.
3. `pin_child` on `BottomTabBar`, and on a sticky `AppHeader` if present.
4. Keep safe-area padding separate from pinning; pinned bars still need top/bottom insets.

Anti-patterns: overflow without clipping, content that does not exceed bounds,
pinning a child while auto layout still controls it, or setting fixed-child count
greater than the child count.
