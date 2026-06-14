package internal

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

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
	"add_variable_mode",
	"apply_style_to_node",
	"batch_rename_nodes",
	"bind_variable_to_node",
	"boolean_operation",
	"clone_node",
	"create_component",
	"create_effect_style",
	"create_ellipse",
	"create_frame",
	"create_grid_style",
	"create_instance",
	"create_paint_style",
	"create_rectangle",
	"create_section",
	"create_text",
	"create_text_style",
	"create_variable",
	"create_variable_collection",
	"delete_nodes",
	"delete_page",
	"delete_style",
	"delete_variable",
	"detach_instance",
	"export_frames_to_pdf",
	"export_tokens",
	"find_replace_text",
	"get_annotations",
	"get_design_context",
	"get_document",
	"get_fonts",
	"get_library_variables",
	"get_local_components",
	"get_metadata",
	"get_node",
	"get_nodes_info",
	"get_pages",
	"get_reactions",
	"get_remote_variable_collection",
	"get_screenshot",
	"get_selection",
	"get_styles",
	"get_variable_defs",
	"get_viewport",
	"group_nodes",
	"import_component_by_key",
	"import_image",
	"import_style_by_key",
	"import_variable_by_key",
	"list_library_variable_collections",
	"lock_nodes",
	"move_nodes",
	"navigate_to_page",
	"remove_reactions",
	"rename_node",
	"rename_page",
	"reorder_nodes",
	"reparent_nodes",
	"resize_nodes",
	"rotate_nodes",
	"scan_nodes_by_types",
	"scan_text_nodes",
	"search_nodes",
	"set_auto_layout",
	"set_blend_mode",
	"set_constraints",
	"set_corner_radius",
	"set_effects",
	"set_fills",
	"set_instance_properties",
	"set_opacity",
	"set_reactions",
	"set_strokes",
	"set_text",
	"set_variable_mode",
	"set_variable_value",
	"set_visible",
	"swap_component",
	"ungroup_nodes",
	"unlock_nodes",
	"update_paint_style",
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
		"blendMode": enumProp("NORMAL", "MULTIPLY", "SCREEN", "OVERLAY", "DARKEN", "LIGHTEN", "COLOR_DODGE", "COLOR_BURN", "HARD_LIGHT", "SOFT_LIGHT", "DIFFERENCE", "EXCLUSION", "HUE", "SATURATION", "COLOR", "LUMINOSITY", "PASS_THROUGH"),
	}),
	"set_constraints": schemaObject(nil, map[string]any{
		"horizontal": enumProp("MIN", "MAX", "CENTER", "STRETCH", "SCALE"),
		"vertical":   enumProp("MIN", "MAX", "CENTER", "STRETCH", "SCALE"),
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
}

var demotedBatchOpDescriptions = map[string]string{
	"boolean_operation": "Combine or flatten vector shapes. Supports UNION, SUBTRACT, INTERSECT, EXCLUDE, and FLATTEN.",
	"delete_page":       "Delete a page by pageId or exact pageName.",
	"delete_variable":   "Delete a variable by variableId, or a collection by collectionId.",
	"rename_page":       "Rename a page by pageId or exact pageName.",
	"set_constraints":   "Set horizontal and/or vertical constraints on nodes.",
	"set_corner_radius": "Set uniform or per-corner radius values on nodes.",
	"set_blend_mode":    "Set Figma blend mode on nodes.",
	"rename_node":       "Rename one node.",
	"reorder_nodes":     "Change node z-order: bringToFront, sendToBack, bringForward, or sendBackward.",
	"rotate_nodes":      "Set node rotation in degrees.",
	"lock_nodes":        "Lock nodes.",
	"unlock_nodes":      "Unlock nodes.",
	"ungroup_nodes":     "Ungroup groups.",
	"detach_instance":   "Detach component instances.",
	"remove_reactions":  "Remove prototype reactions by index, or clear all when indices is omitted.",
	"delete_style":      "Delete a local style by styleId.",
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
	case strings.Contains(name, "reaction"):
		return "prototype"
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
			if k == "channel" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		props, _ := cloneCatalogValue(st.Tool.InputSchema.Properties).(map[string]any)
		delete(props, "channel")
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
		mcp.WithString("query", mcp.Description("Optional name/description substring to search.")),
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
			haystack := strings.ToLower(spec.Name + " " + spec.Description + " " + strings.Join(spec.ParamKeys, " ") + " " + catalogSchemaSearchText(spec.InputSchema))
			if query != "" && !strings.Contains(haystack, strings.ToLower(query)) {
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
		return mcp.NewToolResultStructuredOnly(map[string]any{
			"matches": matches,
			"count":   len(matches),
			"total":   total,
		}), nil
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
			return mcp.NewToolResultError("unknown batch op: " + op), nil
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
		if k == "channel" {
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
		if name != "channel" {
			out = append(out, name)
		}
	}
	return out
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
