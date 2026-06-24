package internal

import (
	"fmt"
	"strings"
)

// batchOpIntent maps every plugin-supported batch op to ONE intent category.
// This is the SSOT for search_batch_ops(category) browsing and for the tier-0
// capability seed (server instructions). It replaces the old name-prefix
// heuristic in batchOpCategory, because intent is not derivable from the op
// name (set_effects is "effects", not "modify"; get_variable_defs is "tokens",
// not "read").
//
// Categories — 9 featured (carry a categoryTeaser, surfaced in the seed) and 3
// baseline (browse-only; an AI already reaches for these):
//
//	featured: effects layout tokens vector components prototype media styles handoff
//	baseline: content arrange read
//
// Invariant (enforced by TestCapabilitySeedMetadata): every op in
// pluginSupportedBatchOps appears here, and every op flagged novel sits in a
// category that has a categoryTeaser.
var batchOpIntent = map[string]string{
	// read — generic inspection (domain reads live with their domain below)
	"get_node": "read", "get_nodes_info": "read", "get_metadata": "read",
	"get_design_context": "read", "get_document": "read", "get_selection": "read",
	"get_pages": "read", "get_viewport": "read", "get_fonts": "read",
	"scan_nodes_by_types": "read", "scan_text_nodes": "read", "search_nodes": "read",
	"get_screenshot": "read",

	// content — text + basic paint (reflexive; baseline)
	"create_text": "content", "set_text": "content", "set_text_range": "content",
	"find_replace_text": "content", "set_fills": "content", "set_strokes": "content",
	"set_blend_mode": "content", "set_opacity": "content",

	// arrange — structural create/transform/order + page management (baseline)
	"create_frame": "arrange", "create_rectangle": "arrange", "create_section": "arrange",
	"create_table": "arrange", "create_page_divider": "arrange", "group_nodes": "arrange",
	"ungroup_nodes": "arrange", "reparent_nodes": "arrange", "move_nodes": "arrange",
	"reorder_nodes": "arrange", "rotate_nodes": "arrange", "resize_nodes": "arrange",
	"clone_node": "arrange", "delete_nodes": "arrange", "rename_node": "arrange",
	"batch_rename_nodes": "arrange", "lock_nodes": "arrange", "unlock_nodes": "arrange",
	"set_visible": "arrange", "add_page": "arrange", "delete_page": "arrange",
	"rename_page": "arrange", "navigate_to_page": "arrange",

	// effects — visual depth (featured)
	"set_effects": "effects", "create_effect_style": "effects",

	// layout — responsive auto-layout (featured)
	"set_auto_layout": "layout", "set_constraints": "layout",

	// tokens — variables, modes, theming (featured)
	"create_variable": "tokens", "create_variable_collection": "tokens",
	"create_variable_alias": "tokens", "add_variable_mode": "tokens",
	"set_variable_value": "tokens", "set_variable_mode": "tokens",
	"update_variable": "tokens", "update_variable_collection": "tokens",
	"delete_variable": "tokens", "bind_variable_to_node": "tokens",
	"bind_variable_to_effect": "tokens", "bind_variable_to_layout_grid": "tokens",
	"resolve_variable_for_consumer": "tokens", "get_variable_defs": "tokens",
	"get_library_variables": "tokens", "get_remote_variable_collection": "tokens",
	"list_library_variable_collections": "tokens", "import_variable_by_key": "tokens",

	// vector — custom shapes & paths (featured)
	"boolean_operation": "vector", "create_vector": "vector", "create_text_path": "vector",
	"create_line": "vector", "create_ellipse": "vector", "create_polygon": "vector",
	"create_star": "vector", "set_corner_radius": "vector",

	// components — components, instances, variants (featured)
	"create_component": "components", "create_instance": "components",
	"set_instance_properties": "components", "swap_component": "components",
	"detach_instance": "components", "import_component_by_key": "components",
	"get_local_components": "components",

	// prototype — interactions & scroll-fixed chrome (featured)
	"set_reactions": "prototype", "remove_reactions": "prototype", "get_reactions": "prototype",
	"get_prototype": "prototype", "set_prototype_start": "prototype",
	"set_prototype_background": "prototype", "pin_child": "prototype",
	"set_fixed_children": "prototype", "set_overflow": "prototype",

	// media — real images, svg, video (featured)
	"import_image": "media", "import_svg": "media", "create_video": "media",
	"create_gif": "media", "create_link_preview": "media", "get_image_by_hash": "media",
	"get_file_thumbnail": "media", "set_file_thumbnail": "media",

	// styles — shared paint/text/effect/grid styles (featured)
	"create_paint_style": "styles", "create_text_style": "styles", "create_grid_style": "styles",
	"apply_style_to_node": "styles", "update_paint_style": "styles",
	"reorder_local_style": "styles", "reorder_local_style_folder": "styles",
	"import_style_by_key": "styles", "get_styles": "styles", "delete_style": "styles",

	// handoff — dev inspection & export (featured)
	"get_annotations": "handoff", "get_dev_resources": "handoff", "add_dev_resource": "handoff",
	"edit_dev_resource": "handoff", "delete_dev_resource": "handoff",
	"get_selection_colors": "handoff", "export_tokens": "handoff",
	"export_frames_to_pdf": "handoff", "create_slice": "handoff",
}

// novelBatchOps is the underused subset: (an AI won't spontaneously reach for it)
// AND (using it improves the design). Reflexive ops (create_frame, set_fills) and
// craft-irrelevant utilities (lock_nodes) are excluded. This flag is the
// data-driven link that makes a category "featured" — TestCapabilitySeedMetadata
// requires every novel op's category to carry a categoryTeaser.
var novelBatchOps = map[string]bool{
	"set_effects": true, "create_effect_style": true,
	"set_auto_layout": true, "set_constraints": true, "set_overflow": true,
	"set_variable_mode": true, "add_variable_mode": true, "create_variable_alias": true,
	"bind_variable_to_node": true, "bind_variable_to_effect": true,
	"bind_variable_to_layout_grid": true, "resolve_variable_for_consumer": true,
	"get_remote_variable_collection": true,
	"boolean_operation":              true, "create_text_path": true, "create_vector": true,
	"create_polygon": true, "create_star": true, "set_corner_radius": true,
	"set_instance_properties": true, "swap_component": true,
	"set_reactions": true, "set_prototype_start": true, "pin_child": true,
	"set_fixed_children": true,
	"import_image":       true, "import_svg": true, "create_video": true, "create_gif": true,
	"create_link_preview": true,
	"reorder_local_style": true, "reorder_local_style_folder": true,
	"get_annotations": true, "add_dev_resource": true, "get_selection_colors": true,
	"create_slice": true,
	// set_blend_mode / set_text_range are underused too, but they live in the
	// baseline "content" category (reflexive text/paint) and are reached by
	// browsing it; their craft (blend compositing, per-range type) is taught in
	// figma-design-patterns/visual-craft.md, not the category seed.
}

// featuredCategoryOrder is the display order of featured categories in the seed.
var featuredCategoryOrder = []string{
	"effects", "layout", "tokens", "vector", "components",
	"prototype", "media", "styles", "handoff",
}

// categoryIntentLabel is the human "what you're trying to do" label per featured
// category (ASCII only — this string is injected verbatim into client context).
var categoryIntentLabel = map[string]string{
	"effects":    "depth/realism",
	"layout":     "responsive layout",
	"tokens":     "theming/tokens",
	"vector":     "custom shapes",
	"components": "components/variants",
	"prototype":  "interactions",
	"media":      "real media",
	"styles":     "shared styles",
	"handoff":    "dev handoff",
}

// categoryTeaser is the novelty keyword line per featured category — the "you'll
// forget these exist" trigger. Inclusion of a category in the seed is data-driven
// (a category is featured iff it holds a novel op); only the wording is authored.
var categoryTeaser = map[string]string{
	"effects":    "glass, noise, texture, progressive-blur, shadows",
	"layout":     "grid, min/max bounds, wrap, fill/hug, constraints",
	"tokens":     "variable binding, per-subtree dark-mode, aliases",
	"vector":     "boolean ops, text-on-path, per-corner radius, custom paths",
	"components": "variant + exposed props, swap, instance-from-key",
	"prototype":  "reactions, fixed chrome over scroll",
	"media":      "real images + filters, svg, video, gif, link preview",
	"styles":     "shared paint/text/effect/grid styles, reorder",
	"handoff":    "annotations, dev resources, selection colors, codegen, slices",
}

// BuildCapabilitySeed renders the tier-0 awareness block injected once per
// session via server.WithInstructions. It teaches the discovery flow and lists,
// by intent, the advanced capabilities an AI would otherwise never think to use.
// Schemas stay on-demand (get_batch_op_spec) — this is awareness only (~250 tokens).
func BuildCapabilitySeed() string {
	var b strings.Builder
	b.WriteString("figma-mcp-express - to build: browse the capability, then drill.\n")
	b.WriteString("  search_batch_ops(category) -> get_batch_op_spec(op) -> batch(ops:[...])\n")
	b.WriteString("  How / when / craft: load the figma-design-patterns skill.\n\n")
	b.WriteString("Don't default to flat fills + basic shadow. Capabilities by intent\n")
	b.WriteString("(browse the category that fits, then drill for params):\n")
	for _, cat := range featuredCategoryOrder {
		fmt.Fprintf(&b, "  %-20s -> category:%-11s (incl. %s)\n",
			categoryIntentLabel[cat], cat, categoryTeaser[cat])
	}
	return b.String()
}
