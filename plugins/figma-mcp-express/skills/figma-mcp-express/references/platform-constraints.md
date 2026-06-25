# Platform Constraints

Figma Plugin API limits that affect agent behavior. Keep exact schemas in
`BatchOpCatalog` / `get_batch_op_spec`; this file only says what not to try and the
right escape path.

**Quick navigation:** [Constraints](#constraints) · [Decision table](#decision-table)

---

## Constraints

| Constraint | Do not try | Use instead |
|---|---|---|
| Instance children are sealed | move/resize/reparent a child inside an INSTANCE | edit the master, or use exposed `set_instance_properties` slots |
| Instance-child ids are compound | guess inner ids | `get_node(instanceId, depth:6)`, then use `I<instanceId>;<innerId>` only when no exposed property exists |
| Auto-layout owns in-flow child x/y | `move_nodes` on an in-flow child | parent padding/gap, or `resize_nodes` with `layoutPositioning:"ABSOLUTE"` for a floating child |
| Existing floating child needs ABSOLUTE | `set_auto_layout` with `layoutPositioning` | batch/FigmaPlan `resize_nodes` op on the child with `layoutPositioning:"ABSOLUTE"` |
| Scroll-fixed child | generic ABSOLUTE only | `pin_child` or `set_fixed_children`; this changes fixed-child count/order |
| Clone IDs change | reuse source child ids after `clone_node` | read the cloned root and rebuild the id map |
| Hidden variant child collapses layout | hide a side slot with `visible:false` in centered chrome | fix master layout; reserve slot space or use opacity/space-preserving variants |
| COMPONENT nodes need layout | assume components cannot use auto layout | `set_auto_layout` on COMPONENT/COMPONENT_SET masters |
| Community kit components are local | import by key from an unpublished Community duplicate | publish as a library, or use local component ids/copy once |
| Nested icon variation in instances | add/remove/reparent children inside an instance | put a swappable icon slot in the master, then `swap_component` per instance |
| Dark mode scope | set mode on a child and expect siblings to change | `set_variable_mode` on the outer screen/wrapper |
| Slow import | loop-retry a maybe-bad component key | validate key/source first, wait for queue to drain, retry once |

## Decision table

| Need | Correct first move |
|---|---|
| Pin safe-area chrome in normal auto layout | parent padding/gap tokens |
| Float glass tab bar or decorative blob | `resize_nodes` `layoutPositioning:"ABSOLUTE"`, then position/reorder |
| Sticky prototype header/tab while scrolling | `pin_child` / `set_fixed_children` |
| Make one instance's label/icon/content differ | exposed component property or nested instance swap |
| Make all instances reflow differently | edit the master component |
| Need subscribed library variables | `list_library_variable_collections` -> `get_library_variables` -> `import_variable_by_key` |
