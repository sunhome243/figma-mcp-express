# Component Usage

How to correctly configure a placed component instance. Placing an instance is step one; configuring it with real content, variants, and dimensions is what makes it done.

---

## Configure-after-instantiate sequence

Use validated batch ops, not raw Plugin API scripts:

1. `import_component_by_key` with the variant component key.
2. `create_instance` with `componentId` from the import result and the intended `parentId`.
3. `set_instance_properties` with real variant, text, boolean, and swap-slot values.
4. `resize_nodes` when the instance must match a known width/height or FILL/HUG sizing.
5. Read back with `get_node` or `get_nodes_info`, then screenshot the wrapper.

Never skip steps 3-5. A default instance with placeholder content is always wrong.

## What Instance Properties Can Set

| Slot type | Value |
|---|---|
| TEXT | Real string content |
| INSTANCE_SWAP | Imported component id/reference expected by the server op |
| Variant property | A string matching the component's allowed variant value |
| Boolean layer visibility | `true` or `false` |

Property names are case-sensitive. Read `componentPropertyDefinitions` with `get_node` on the component before guessing names.

## Never Add Children to an Instance

Component instances are sealed. Their internal structure is owned by the component. Change visible content through `set_instance_properties` only.

Wrong model: create text and insert it into an instance.

Right model: set the instance's TEXT or INSTANCE_SWAP property to the intended content/component.

## Variants vs Separate Frames

Use variant properties for hover, focus, active, pressed, disabled, error, and selected states. Separate frames are only appropriate when the layout structure fundamentally changes, such as collapsed vs expanded navigation.

## Reset Behavior

Use the server's reset behavior through `set_instance_properties` with `resetOverrides:true` when you need component defaults restored before applying new properties. Do not clear fills or other visual properties manually to simulate a reset.

## Default Instance Means Not Done

Fresh instances commonly show placeholders:

| Placeholder | Meaning |
|---|---|
| `Heading`, `Title`, `Label` | TEXT slot needs real content |
| `Item 1`, `Item 1-5` | Repeating slot or legend needs data or hiding |
| `Swap it`, `Content` | INSTANCE_SWAP slot needs a real component |
| Blank area | Image/slot visibility needs explicit configuration |

After configuration, a viewer should immediately recognize the component's intended role and content.

## Dark Mode and Theming

Set the variable mode on the top-level wrapper so tokens cascade through instances. Manual fill overrides on instance internals break theme switching and should be treated as a failure.

## Common Mistakes

| Mistake | Consequence | Fix |
|---|---|---|
| Default instance left with placeholders | Viewer cannot recognize the component | `set_instance_properties` with real content + size verification |
| Raw child inserted into an instance | Instance can break or lose expected behavior | Use instance properties for slots |
| Visual reset by clearing fills | Instance looks broken | Use reset behavior in `set_instance_properties` |
| Manual child color override | Dark mode fails | Set wrapper mode and rely on bound variables |
| Wrong property name case | No-op or placeholder remains | Read `componentPropertyDefinitions` first |
| COMPONENT_SET key used as variant key | Import fails | Import the default variant's component key |
