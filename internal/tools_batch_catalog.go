package internal

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type BatchOpSpec struct {
	Name        string         `json:"name"`
	Category    string         `json:"category"`
	ReadOnly    bool           `json:"readOnly"`
	Mutates     bool           `json:"mutates"`
	Description string         `json:"description"`
	ParamKeys   []string       `json:"paramKeys,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Examples    []any          `json:"examples,omitempty"`
}

type batchOpSearchMatch struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	ReadOnly    bool   `json:"readOnly"`
	Mutates     bool   `json:"mutates"`
	Description string `json:"description"`
}

var pluginSupportedBatchOps = []string{
	"add_page",
	"add_dev_resource",
	"add_variable_mode",
	"apply_style_to_node",
	"batch_rename_nodes",
	"bind_variable_to_effect",
	"bind_variable_to_layout_grid",
	"bind_variable_to_node",
	"boolean_operation",
	"clone_node",
	"create_component",
	"create_effect_style",
	"create_ellipse",
	"create_frame",
	"create_gif",
	"create_grid_style",
	"create_instance",
	"create_line",
	"create_link_preview",
	"create_page_divider",
	"create_paint_style",
	"create_polygon",
	"create_rectangle",
	"create_section",
	"create_slice",
	"create_star",
	"create_table",
	"create_text",
	"create_text_path",
	"create_text_style",
	"create_vector",
	"create_video",
	"create_variable",
	"create_variable_alias",
	"create_variable_collection",
	"delete_dev_resource",
	"delete_nodes",
	"delete_page",
	"delete_style",
	"delete_variable",
	"detach_instance",
	"edit_dev_resource",
	"export_frames_to_pdf",
	"export_tokens",
	"find_replace_text",
	"get_annotations",
	"get_design_context",
	"get_dev_resources",
	"get_document",
	"get_file_thumbnail",
	"get_fonts",
	"get_image_by_hash",
	"get_library_variables",
	"get_local_components",
	"get_metadata",
	"get_node",
	"get_nodes_info",
	"get_pages",
	"get_prototype",
	"get_reactions",
	"get_remote_variable_collection",
	"get_screenshot",
	"get_selection",
	"get_selection_colors",
	"get_styles",
	"get_variable_defs",
	"get_viewport",
	"group_nodes",
	"import_component_by_key",
	"import_image",
	"import_style_by_key",
	"import_svg",
	"import_variable_by_key",
	"list_library_variable_collections",
	"lock_nodes",
	"move_nodes",
	"navigate_to_page",
	"pin_child",
	"remove_reactions",
	"rename_node",
	"rename_page",
	"reorder_nodes",
	"reorder_local_style",
	"reorder_local_style_folder",
	"reparent_nodes",
	"resize_nodes",
	"resolve_variable_for_consumer",
	"rotate_nodes",
	"scan_nodes_by_types",
	"scan_text_nodes",
	"search_nodes",
	"set_auto_layout",
	"set_blend_mode",
	"set_constraints",
	"set_corner_radius",
	"set_effects",
	"set_file_thumbnail",
	"set_fills",
	"set_fixed_children",
	"set_instance_properties",
	"set_opacity",
	"set_overflow",
	"set_prototype_background",
	"set_prototype_start",
	"set_reactions",
	"set_strokes",
	"set_text",
	"set_text_range",
	"set_variable_mode",
	"set_variable_value",
	"set_visible",
	"swap_component",
	"ungroup_nodes",
	"unlock_nodes",
	"update_paint_style",
	"update_variable",
	"update_variable_collection",
}

var demotedBatchOnlyInputSchemas = map[string]map[string]any{
	"boolean_operation": schemaObject([]string{"operation"}, map[string]any{
		"operation": enumProp("UNION", "SUBTRACT", "INTERSECT", "EXCLUDE", "FLATTEN"),
		"parentId":  stringProp(),
		"name":      stringProp(),
	}),
	"delete_page": schemaObject(nil, map[string]any{
		"pageId":   stringProp(),
		"pageName": stringProp(),
	}),
	"delete_style": schemaObject([]string{"styleId"}, map[string]any{
		"styleId": stringProp(),
	}),
	"delete_variable": schemaObject(nil, map[string]any{
		"variableId":   stringProp(),
		"collectionId": stringProp(),
	}),
	"detach_instance": schemaObject(nil, map[string]any{}),
	"lock_nodes":      schemaObject(nil, map[string]any{}),
	"remove_reactions": schemaObject(nil, map[string]any{
		"indices": arrayProp("number"),
	}),
	"rename_node": schemaObject([]string{"name"}, map[string]any{
		"name": stringProp(),
	}),
	"rename_page": schemaObject([]string{"newName"}, map[string]any{
		"pageId":   stringProp(),
		"pageName": stringProp(),
		"newName":  stringProp(),
	}),
	"reorder_nodes": schemaObject([]string{"order"}, map[string]any{
		"order": enumProp("bringToFront", "sendToBack", "bringForward", "sendBackward"),
	}),
	"rotate_nodes": schemaObject([]string{"rotation"}, map[string]any{
		"rotation": numberProp(),
	}),
	"set_blend_mode": schemaObject([]string{"blendMode"}, map[string]any{
		"blendMode": enumProp("NORMAL", "MULTIPLY", "SCREEN", "OVERLAY", "DARKEN", "LIGHTEN", "COLOR_DODGE", "COLOR_BURN", "LINEAR_DODGE", "LINEAR_BURN", "HARD_LIGHT", "SOFT_LIGHT", "DIFFERENCE", "EXCLUSION", "HUE", "SATURATION", "COLOR", "LUMINOSITY", "PASS_THROUGH"),
	}),
	"set_corner_radius": schemaObject(nil, map[string]any{
		"cornerRadius":      numberProp(),
		"topLeftRadius":     numberProp(),
		"topRightRadius":    numberProp(),
		"bottomLeftRadius":  numberProp(),
		"bottomRightRadius": numberProp(),
	}),
	"ungroup_nodes": schemaObject(nil, map[string]any{}),
	"unlock_nodes":  schemaObject(nil, map[string]any{}),
	"set_overflow": schemaObject([]string{"overflowDirection"}, map[string]any{
		"overflowDirection": enumProp("NONE", "HORIZONTAL", "VERTICAL", "BOTH"),
		"clipsContent":      boolProp(),
	}),
	"set_fixed_children": schemaObject([]string{"numberOfFixedChildren"}, map[string]any{
		"numberOfFixedChildren": numberProp(),
	}),
	"pin_child": schemaObject(nil, map[string]any{}),
	"set_prototype_background": schemaObject(nil, map[string]any{
		"color":   stringProp(),
		"opacity": numberProp(),
		"mode":    enumProp("set", "clear"),
	}),
}

var demotedBatchOpDescriptions = map[string]string{
	"boolean_operation":        "Combine or flatten vector shapes. Supports UNION, SUBTRACT, INTERSECT, EXCLUDE, and FLATTEN.",
	"delete_page":              "Delete a page by pageId or exact pageName.",
	"delete_variable":          "Delete a variable by variableId, or a collection by collectionId.",
	"rename_page":              "Rename a page by pageId or exact pageName.",
	"set_corner_radius":        "Set uniform or per-corner radius values on nodes.",
	"set_blend_mode":           "Set Figma blend mode on nodes.",
	"rename_node":              "Rename one node.",
	"reorder_nodes":            "Change node z-order: bringToFront, sendToBack, bringForward, or sendBackward.",
	"rotate_nodes":             "Set node rotation in degrees.",
	"lock_nodes":               "Lock nodes.",
	"unlock_nodes":             "Unlock nodes.",
	"ungroup_nodes":            "Ungroup groups.",
	"detach_instance":          "Detach component instances.",
	"remove_reactions":         "Remove prototype reactions by index, or clear all when indices is omitted.",
	"delete_style":             "Delete a local style by styleId.",
	"set_overflow":             "Set a frame's prototype scroll direction (NONE|HORIZONTAL|VERTICAL|BOTH). Optionally toggle clipsContent — a frame only scrolls when its content overflows and is clipped.",
	"set_fixed_children":       "Set how many leading children of a frame stay fixed while the rest scroll. Fixed children must be ordered first.",
	"pin_child":                "Pin a child so it stays fixed while its frame scrolls: sets it ABSOLUTE, moves it into the leading fixed band, and extends the parent's fixed-children count.",
	"set_prototype_background": "Set the page's prototype presentation background to one solid color (color, opacity?), or clear it with mode \"clear\".",
}

var batchOpCatalog = newBatchOpCatalog()

func newBatchOpCatalog() map[string]BatchOpSpec {
	out := map[string]BatchOpSpec{}
	for _, name := range pluginSupportedBatchOps {
		spec := defaultBatchOpSpec(name)
		if schema, ok := demotedBatchOnlyInputSchemas[name]; ok {
			spec.ParamKeys = schemaParamKeys(schema)
			spec.InputSchema = schema
		}
		if desc, ok := demotedBatchOpDescriptions[name]; ok {
			spec.Description = desc
		}
		out[name] = spec
	}
	mapSchema := schemaObject([]string{"over", "do"}, map[string]any{
		"over": map[string]any{
			"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "array"},
			},
		},
		"as": stringProp(),
		"do": map[string]any{"type": "object"},
	})
	out["map"] = BatchOpSpec{
		Name:        "map",
		Category:    "control",
		ReadOnly:    false,
		Mutates:     true,
		Description: "Batch control op. Iterates over an array and executes one validated op template with $item/$index bindings.",
		ParamKeys:   schemaParamKeys(mapSchema),
		InputSchema: mapSchema,
		Examples: []any{
			map[string]any{
				"type": "map",
				"over": "$0.matchingNodes[*]",
				"as":   "item",
				"do": map[string]any{
					"type":    "set_visible",
					"nodeIds": []any{"$item.id"},
					"params":  map[string]any{"visible": true},
				},
			},
		},
	}
	return out
}

func defaultBatchOpSpec(name string) BatchOpSpec {
	readOnly := isReadOnlyBatchOp(name)
	return BatchOpSpec{
		Name:        name,
		Category:    batchOpCategory(name),
		ReadOnly:    readOnly,
		Mutates:     !readOnly,
		Description: compactText(strings.ReplaceAll(name, "_", " "), 120),
	}
}

func isReadOnlyBatchOp(name string) bool {
	return strings.HasPrefix(name, "get_") ||
		strings.HasPrefix(name, "scan_") ||
		strings.HasPrefix(name, "search_") ||
		strings.HasPrefix(name, "list_") ||
		strings.HasPrefix(name, "export_")
}

func batchOpCategory(name string) string {
	switch {
	// Prototype ops are matched before the get_/set_ prefix rules so get_prototype,
	// set_prototype_start, and the scroll/fixed-children ops group under "prototype"
	// rather than "read"/"modify"/"write".
	case strings.Contains(name, "reaction"), strings.Contains(name, "prototype"),
		name == "set_overflow", name == "set_fixed_children", name == "pin_child":
		return "prototype"
	case strings.HasPrefix(name, "get_"), strings.HasPrefix(name, "scan_"), strings.HasPrefix(name, "search_"):
		return "read"
	case strings.HasPrefix(name, "create_"):
		return "create"
	case strings.HasPrefix(name, "set_"), strings.HasPrefix(name, "move_"), strings.HasPrefix(name, "resize_"), strings.HasPrefix(name, "rotate_"), strings.HasPrefix(name, "reorder_"), strings.HasPrefix(name, "rename_"), strings.HasPrefix(name, "clone_"), strings.HasPrefix(name, "delete_"):
		return "modify"
	case strings.Contains(name, "style"):
		return "styles"
	case strings.Contains(name, "variable"):
		return "variables"
	case strings.Contains(name, "component"), strings.Contains(name, "instance"):
		return "library"
	case strings.Contains(name, "page"):
		return "page"
	default:
		return "write"
	}
}

func syncBatchCatalogFromRegisteredTools(s *server.MCPServer) {
	for name, st := range s.ListTools() {
		spec, ok := batchOpCatalog[name]
		if !ok || st == nil {
			continue
		}
		keys := make([]string, 0, len(st.Tool.InputSchema.Properties))
		for k := range st.Tool.InputSchema.Properties {
			if isOuterToolParam(k) {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		props, _ := cloneCatalogValue(st.Tool.InputSchema.Properties).(map[string]any)
		delete(props, "channel")
		delete(props, "origin")
		spec.ParamKeys = keys
		spec.InputSchema = map[string]any{
			"type":       st.Tool.InputSchema.Type,
			"properties": props,
			"required":   batchSchemaRequiredWithoutOuterParams(st.Tool.InputSchema.Required),
		}
		if st.Tool.Description != "" {
			spec.Description = st.Tool.Description
		}
		batchOpCatalog[name] = spec
	}
}

func registerBatchCatalogTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("search_batch_ops",
		mcp.WithDescription("Search the validated batch/FigmaPlan operation catalog. Use this before get_batch_op_spec when you know the capability but not the exact op name."),
		mcp.WithString("query", mcp.Description("Optional capability/op/param words to search. Tolerates separators, camelCase, singular/plural, and filler words like op/tool.")),
		mcp.WithString("category", mcp.Description("Optional category filter: read, create, modify, styles, variables, library, page, prototype, control, write.")),
		mcp.WithBoolean("readOnly", mcp.Description("Optional. true returns read-only ops; false returns non-read-only ops.")),
		mcp.WithBoolean("mutates", mcp.Description("Optional. true returns mutating ops; false returns non-mutating ops.")),
		mcp.WithNumber("limit", mcp.Description("Maximum matches to return. Default 20, max 100.")),
		mcp.WithRawOutputSchema(json.RawMessage(`{"type":"object","properties":{"matches":{"type":"array","items":{"type":"object"}},"count":{"type":"number"},"total":{"type":"number"}}}`)),
	), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		query, _ := args["query"].(string)
		category, _ := args["category"].(string)
		limit := 20
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
			if limit > 100 {
				limit = 100
			}
		}
		readOnly, hasReadOnly := args["readOnly"].(bool)
		mutates, hasMutates := args["mutates"].(bool)

		matches := []batchOpSearchMatch{}
		for _, spec := range allBatchOpSpecs() {
			if query != "" && !matchesBatchOpQuery(batchOpHaystack(spec), query) {
				continue
			}
			if category != "" && spec.Category != category {
				continue
			}
			if hasReadOnly && spec.ReadOnly != readOnly {
				continue
			}
			if hasMutates && spec.Mutates != mutates {
				continue
			}
			matches = append(matches, batchOpSearchMatch{
				Name:        spec.Name,
				Category:    spec.Category,
				ReadOnly:    spec.ReadOnly,
				Mutates:     spec.Mutates,
				Description: compactText(spec.Description, 120),
			})
		}
		total := len(matches)
		if len(matches) > limit {
			matches = matches[:limit]
		}
		result := map[string]any{
			"matches": matches,
			"count":   len(matches),
			"total":   total,
		}
		// Zero exact matches is the moment an agent wrongly concludes "this
		// capability doesn't exist." Surface the closest ops (ranked) + a hint so
		// it verifies instead of giving up. Filtered-to-empty (category/readOnly)
		// is deliberate, so only offer this for a plain query miss.
		if total == 0 && query != "" && category == "" && !hasReadOnly && !hasMutates {
			if suggestions := suggestedBatchOps(query, 5); len(suggestions) > 0 {
				result["suggestions"] = suggestions
				result["hint"] = "no exact match — closest ops by relevance; confirm with get_batch_op_spec before concluding the capability is absent"
			}
		}
		return mcp.NewToolResultStructuredOnly(result), nil
	})

	s.AddTool(mcp.NewTool("get_batch_op_spec",
		mcp.WithDescription("Return the full validated batch/FigmaPlan spec for one operation. Use after search_batch_ops and before composing unfamiliar batch ops."),
		mcp.WithString("op", mcp.Required(), mcp.Description("Batch op name, e.g. create_frame, set_fills, rename_node, map.")),
		mcp.WithBoolean("includeExamples", mcp.Description("Include example op payloads when available. Default false.")),
		mcp.WithRawOutputSchema(json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"category":{"type":"string"},"readOnly":{"type":"boolean"},"mutates":{"type":"boolean"},"description":{"type":"string"},"paramKeys":{"type":"array","items":{"type":"string"}},"inputSchema":{"type":"object"}}}`)),
	), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		op, _ := req.GetArguments()["op"].(string)
		spec, ok := batchOpCatalog[op]
		if !ok {
			msg := "unknown batch op: " + op
			if suggestions := suggestedBatchOps(op, 5); len(suggestions) > 0 {
				msg += ". Did you mean: " + strings.Join(suggestions, ", ") + "?"
			}
			msg += " Use search_batch_ops before concluding an op is absent."
			return mcp.NewToolResultError(msg), nil
		}
		if include, _ := req.GetArguments()["includeExamples"].(bool); !include {
			spec.Examples = nil
		}
		return mcp.NewToolResultStructuredOnly(spec), nil
	})
}

func batchOpParamKeys(op string) (map[string]bool, bool) {
	spec, ok := batchOpCatalog[op]
	if !ok {
		return nil, false
	}
	out := make(map[string]bool, len(spec.ParamKeys))
	for _, k := range spec.ParamKeys {
		out[k] = true
	}
	return out, true
}

func allBatchOpSpecs() []BatchOpSpec {
	out := make([]BatchOpSpec, 0, len(batchOpCatalog))
	for _, spec := range batchOpCatalog {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func schemaParamKeys(schema map[string]any) []string {
	props, _ := schema["properties"].(map[string]any)
	out := make([]string, 0, len(props))
	for k := range props {
		if isOuterToolParam(k) {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func batchSchemaRequiredWithoutOuterParams(required []string) []string {
	out := make([]string, 0, len(required))
	for _, name := range required {
		if !isOuterToolParam(name) {
			out = append(out, name)
		}
	}
	return out
}

func isOuterToolParam(name string) bool {
	return name == "channel" || name == "origin"
}

func schemaObject(required []string, props map[string]any) map[string]any {
	out := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func stringProp() map[string]any {
	return map[string]any{"type": "string"}
}

func numberProp() map[string]any {
	return map[string]any{"type": "number"}
}

func boolProp() map[string]any {
	return map[string]any{"type": "boolean"}
}

func arrayProp(itemType string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": itemType}}
}

func enumProp(values ...string) map[string]any {
	enum := make([]any, 0, len(values))
	for _, v := range values {
		enum = append(enum, v)
	}
	return map[string]any{"type": "string", "enum": enum}
}

func cloneCatalogValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			out[k] = cloneCatalogValue(child)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, child := range x {
			out[i] = cloneCatalogValue(child)
		}
		return out
	default:
		return x
	}
}

// matchesBatchOpQuery reports whether every meaningful token in query appears
// in the catalog text — AND semantics. It deliberately accepts sloppy human
// search phrases: separators are tokenized, camelCase is split, singular/plural
// variants match each other, and filler words like "op" or "tool" are ignored.
// This keeps "delete_node op" and "reorder tool" from being false negatives.
func matchesBatchOpQuery(haystack, query string) bool {
	queryTerms := batchOpQueryTerms(query)
	if len(queryTerms) == 0 {
		return true
	}
	haystackTerms := batchOpHaystackTerms(haystack)
	for _, term := range queryTerms {
		found := false
		for _, variant := range batchOpTermVariants(term) {
			if batchOpHaystackContains(haystackTerms, variant) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// batchOpSearchAliases maps an op name to extra search-only synonyms. These are
// folded into the SEARCH text (matchesBatchOpQuery / suggestions) but are NEVER
// displayed — zero token cost on results/specs — so an agent searching "remove"
// finds delete_nodes even though the op name and description say only "delete".
// English only (the catalog is English; map foreign-language intent to English
// search terms upstream). Slim by design: only ops whose common search words
// diverge from the name. To extend, add the word here, not to the agent-facing
// description.
var batchOpSearchAliases = map[string][]string{
	"delete_nodes":    {"remove", "erase"},
	"clone_node":      {"duplicate", "copy"},
	"reparent_nodes":  {"move into"},
	"detach_instance": {"flatten"},
	"swap_component":  {"replace"},
	"group_nodes":     {"wrap"},
}

// batchOpHaystack builds the searchable text for one op: name + description +
// param keys + schema vocabulary + any search-only aliases. Single source so the
// search loop and the suggestion ranker stay in sync (and aliases apply to both).
func batchOpHaystack(spec BatchOpSpec) string {
	h := spec.Name + " " + spec.Description + " " + strings.Join(spec.ParamKeys, " ") + " " + catalogSchemaSearchText(spec.InputSchema)
	if aliases := batchOpSearchAliases[spec.Name]; len(aliases) > 0 {
		h += " " + strings.Join(aliases, " ")
	}
	return h
}

// batchOpNameHaystack is the NAME-only searchable text (name + aliases) used to
// rank suggestions: a query term hitting the op NAME is a stronger signal than
// one buried in a description, so it ranks higher.
func batchOpNameHaystack(spec BatchOpSpec) string {
	h := spec.Name
	if aliases := batchOpSearchAliases[spec.Name]; len(aliases) > 0 {
		h += " " + strings.Join(aliases, " ")
	}
	return h
}

func batchOpHaystackContains(haystackTerms map[string]bool, queryTerm string) bool {
	if haystackTerms[queryTerm] {
		return true
	}
	for haystackTerm := range haystackTerms {
		if strings.Contains(haystackTerm, queryTerm) {
			return true
		}
	}
	return false
}

// suggestedBatchOps returns the closest ops to a query, RANKED by relevance —
// for the "did you mean" on an unknown op and the zero-result search fallback.
// Unlike the main search (strict AND), this is a partial/OR ranker: any op whose
// text matches ≥1 query term is a candidate, ordered by how many query terms hit
// the op NAME (the strongest signal), then how many hit anywhere, then name. So a
// typo like "delete_node" surfaces "delete_nodes" first, and a query that the
// strict AND search misses still gets the nearest ops instead of a dead end. A
// query with no meaningful terms (empty/filler-only) returns nil — never a dump
// of arbitrary ops.
func suggestedBatchOps(query string, limit int) []string {
	queryTerms := batchOpQueryTerms(query)
	if limit <= 0 || len(queryTerms) == 0 {
		return nil
	}
	type scored struct {
		name              string
		nameHits, anyHits int
	}
	cands := []scored{}
	for _, spec := range allBatchOpSpecs() {
		nameTerms := batchOpHaystackTerms(batchOpNameHaystack(spec))
		allTerms := batchOpHaystackTerms(batchOpHaystack(spec))
		nameHits, anyHits := 0, 0
		for _, term := range queryTerms {
			hitName, hitAny := false, false
			for _, variant := range batchOpTermVariants(term) {
				if batchOpHaystackContains(nameTerms, variant) {
					hitName = true
				}
				if batchOpHaystackContains(allTerms, variant) {
					hitAny = true
				}
			}
			if hitName {
				nameHits++
			}
			if hitAny {
				anyHits++
			}
		}
		if anyHits == 0 {
			continue
		}
		cands = append(cands, scored{spec.Name, nameHits, anyHits})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].nameHits != cands[j].nameHits {
			return cands[i].nameHits > cands[j].nameHits
		}
		if cands[i].anyHits != cands[j].anyHits {
			return cands[i].anyHits > cands[j].anyHits
		}
		return cands[i].name < cands[j].name
	})
	out := make([]string, 0, limit)
	for _, c := range cands {
		out = append(out, c.name)
		if len(out) == limit {
			break
		}
	}
	return out
}

func batchOpQueryTerms(query string) []string {
	terms := batchOpBaseTerms(query)
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if isBatchOpQueryFiller(term) {
			continue
		}
		out = append(out, term)
	}
	return out
}

func batchOpHaystackTerms(haystack string) map[string]bool {
	out := map[string]bool{}
	for _, term := range batchOpBaseTerms(haystack) {
		for _, variant := range batchOpTermVariants(term) {
			out[variant] = true
		}
	}
	return out
}

func batchOpBaseTerms(s string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(term string) {
		term = strings.ToLower(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, chunk := range batchOpAlnumChunks(s) {
		add(chunk)
		for _, part := range splitBatchOpCamelChunk(chunk) {
			add(part)
		}
	}
	return out
}

func batchOpAlnumChunks(s string) []string {
	out := []string{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func splitBatchOpCamelChunk(chunk string) []string {
	out := []string{}
	var b strings.Builder
	var prev rune
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for i, r := range chunk {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			flush()
		}
		b.WriteRune(r)
		prev = r
	}
	flush()
	return out
}

func batchOpTermVariants(term string) []string {
	singular := singularBatchOpTerm(term)
	if singular == term {
		return []string{term}
	}
	return []string{term, singular}
}

func singularBatchOpTerm(term string) string {
	switch {
	case len(term) > 4 && strings.HasSuffix(term, "ies"):
		return strings.TrimSuffix(term, "ies") + "y"
	case len(term) > 3 && strings.HasSuffix(term, "s") && !strings.HasSuffix(term, "ss"):
		return strings.TrimSuffix(term, "s")
	default:
		return term
	}
}

func isBatchOpQueryFiller(term string) bool {
	switch singularBatchOpTerm(term) {
	case "op", "operation", "tool", "function", "method", "command", "api", "mcp", "figma", "batch", "call":
		return true
	default:
		return false
	}
}

func catalogSchemaSearchText(v any) string {
	switch x := v.(type) {
	case map[string]any:
		parts := make([]string, 0, len(x))
		for _, child := range x {
			parts = append(parts, catalogSchemaSearchText(child))
		}
		return strings.Join(parts, " ")
	case []any:
		parts := make([]string, 0, len(x))
		for _, child := range x {
			parts = append(parts, catalogSchemaSearchText(child))
		}
		return strings.Join(parts, " ")
	case string:
		return x
	default:
		return ""
	}
}
