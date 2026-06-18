package internal

import (
	"fmt"
	"regexp"
	"strings"
)

// nodeIDPattern matches Figma node IDs:
//
//	simple:   "4029:12345"
//	compound: "I2167:9091;186:1579;186:1745" (instances/variants)
var nodeIDPattern = regexp.MustCompile(`^I?\d+:\d+(;\d+:\d+)*$`)

// fileKeyPattern matches safe Figma file keys: alphanumeric, hyphens, underscores.
// This prevents URL injection via fileKey in fetch_library_catalog.
var fileKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// publishedKeyPattern matches published Figma component/style keys.
// These keys are 40-char lowercase hex SHA-1 strings. Node IDs like "410:49695"
// and truncated 16-char IDs should be rejected before they reach the plugin.
var publishedKeyPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

var bareNodeIDPattern = regexp.MustCompile(`^[0-9]+:[0-9]+$`)

// NormalizeNodeID converts hyphen-format node IDs (LLM output artifact) to colon format.
// "4029-12345" → "4029:12345". No-ops for already-valid or unrecognized strings.
func NormalizeNodeID(s string) string {
	if strings.Contains(s, "-") && !strings.Contains(s, ":") {
		normalized := strings.ReplaceAll(s, "-", ":")
		if nodeIDPattern.MatchString(normalized) {
			return normalized
		}
	}
	return s
}

// ValidNodeID reports whether s is a valid Figma node ID.
func ValidNodeID(s string) bool {
	return nodeIDPattern.MatchString(s)
}

func validatePublishedImportKey(kind, key string) string {
	if key == "" {
		return "key is required"
	}
	if publishedKeyPattern.MatchString(key) {
		return ""
	}
	if bareNodeIDPattern.MatchString(key) {
		switch kind {
		case "component":
			return "that's a node id, not a published component key — use the component's published key (40-char hex), or import_component_by_key on the default variant's key"
		case "style":
			return "that's a node id, not a published style key — use the style's published key (40-char hex)"
		}
		return fmt.Sprintf("that's a node id, not a published %s key", kind)
	}
	if len(key) < 40 {
		return fmt.Sprintf("%s key looks truncated (got %d chars, expected 40) — pass the full published key", kind, len(key))
	}
	return fmt.Sprintf("malformed %s key; expected 40-char hex", kind)
}

func validateVariableImportKey(key string) string {
	if key == "" {
		return "key is required"
	}
	if bareNodeIDPattern.MatchString(key) {
		return "that's a node id, not a published variable key — use the variable key from the library catalog"
	}
	return ""
}

func validateImportComponentAssetType(assetType interface{}) string {
	if assetType == nil {
		return ""
	}
	s, ok := assetType.(string)
	if !ok {
		return "assetType must be COMPONENT or COMPONENT_SET"
	}
	if s == "" || s == "COMPONENT" || s == "COMPONENT_SET" {
		return ""
	}
	return "assetType must be COMPONENT or COMPONENT_SET"
}

// ValidateRPC validates an incoming RPC request against the tool's expected
// input shape. Returns an error string on failure, empty string if valid.
// rejectUnknownParams returns a loud error if params contains any key not in the
// allowed set. The plugin silently ignores unrecognized params, so a typo or a
// Plugin-API field name (characters/fills/lineHeight) would otherwise produce an
// empty/default node with no signal. hints maps a known-mistaken key to the
// correct param so the message is actionable.
func rejectUnknownParams(tool string, params map[string]interface{}, allowed map[string]bool, hints map[string]string) string {
	for k := range params {
		if allowed[k] {
			continue
		}
		// Presence params (origin/status/sessionId/task) ride params but are NOT
		// declared in every tool's schema — `sessionId` is injected by Node.Send and
		// declared nowhere. The leader re-validates proxied follower /rpc calls, so they
		// must pass here or every 2nd+-session call 400s. They never reach the plugin op.
		if isPresenceParam(k) {
			continue
		}
		if correct, ok := hints[k]; ok {
			return fmt.Sprintf("%s: unknown param %q — use %s", tool, k, correct)
		}
		return fmt.Sprintf("%s: unknown param %q (silently ignored by the plugin) — check the tool schema for the correct name", tool, k)
	}
	return ""
}

func ValidateRPC(tool string, nodeIDs []string, params map[string]interface{}) string {
	switch tool {
	case "get_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if raw, present := params["depth"]; present {
			d, ok := raw.(float64)
			if !ok {
				return "depth must be a number"
			}
			if d < 0 {
				return "depth must be a non-negative number"
			}
		}

	case "get_nodes_info":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "export_frames_to_pdf":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "get_screenshot":
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		if format, ok := params["format"].(string); ok {
			if !validExportFormat(format) {
				return fmt.Sprintf("format must be PNG, SVG, JPG, or PDF, got: %s", format)
			}
		}

	case "save_screenshots":
		items, ok := params["items"]
		if !ok {
			return "items is required"
		}
		itemList, ok := items.([]interface{})
		if !ok || len(itemList) == 0 {
			return "items must be a non-empty array"
		}
		for i, item := range itemList {
			m, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Sprintf("items[%d] must be an object", i)
			}
			nodeID, _ := m["nodeId"].(string)
			if !ValidNodeID(nodeID) {
				return fmt.Sprintf("items[%d].nodeId must use colon format e.g. 4029:12345", i)
			}
			outputPath, _ := m["outputPath"].(string)
			if outputPath == "" {
				return fmt.Sprintf("items[%d].outputPath is required", i)
			}
		}

	case "get_design_context":
		if depth, ok := params["depth"].(float64); ok {
			if depth < 0 {
				return "depth must be a non-negative number"
			}
		}
		if detail, ok := params["detail"].(string); ok && detail != "" {
			switch detail {
			case "minimal", "compact", "full", "codegen":
			default:
				return fmt.Sprintf("detail must be minimal, compact, full, or codegen, got: %s", detail)
			}
		}

	case "search_nodes":
		query, _ := params["query"].(string)
		if query == "" {
			return "query is required"
		}
		if nodeID, ok := params["nodeId"].(string); ok && nodeID != "" {
			if !ValidNodeID(nodeID) {
				return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
			}
		}
		if limit, ok := params["limit"].(float64); ok && limit <= 0 {
			return "limit must be a positive number"
		}

	case "get_reactions":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}

	case "get_prototype":
		// Scope is optional: with no nodeIds the whole current page is read; with
		// nodeIds, each must be a valid node id (subtree scope).
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", id)
			}
		}

	case "scan_text_nodes", "scan_nodes_by_types":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if tool == "scan_nodes_by_types" {
			types, ok := params["types"].([]interface{})
			if !ok || len(types) == 0 {
				return "types must be a non-empty array"
			}
		}

	// ── Write tools ──────────────────────────────────────────────────────────

	case "set_opacity":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		op, ok := params["opacity"].(float64)
		if !ok {
			return "opacity is required"
		}
		if op < 0 || op > 1 {
			return "opacity must be between 0 and 1"
		}

	case "set_corner_radius":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasUniform := params["cornerRadius"]
		_, hasTL := params["topLeftRadius"]
		_, hasTR := params["topRightRadius"]
		_, hasBL := params["bottomLeftRadius"]
		_, hasBR := params["bottomRightRadius"]
		if !hasUniform && !hasTL && !hasTR && !hasBL && !hasBR {
			return "at least one of cornerRadius, topLeftRadius, topRightRadius, bottomLeftRadius, or bottomRightRadius is required"
		}

	case "group_nodes":
		if len(nodeIDs) < 2 {
			return "nodeIds must contain at least 2 nodes to group"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "ungroup_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "navigate_to_page":
		pageID, _ := params["pageId"].(string)
		pageName, _ := params["pageName"].(string)
		if pageID == "" && pageName == "" {
			return "pageId or pageName is required"
		}

	case "create_component":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}

	case "export_tokens":
		if format, ok := params["format"].(string); ok && format != "" {
			switch format {
			case "json", "css":
			default:
				return fmt.Sprintf("format must be json or css, got: %s", format)
			}
		}

	case "create_frame":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}
		if msg := validateAutoLayoutParams(params); msg != "" {
			return msg
		}
		if msg := validateLayoutSizingParams(params); msg != "" {
			return msg
		}

	case "set_auto_layout":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if msg := validateAutoLayoutParams(params); msg != "" {
			return msg
		}

	case "create_rectangle", "create_ellipse":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_text":
		if text, _ := params["text"].(string); text == "" {
			return "text is required"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}
		// Fail loud on Plugin-API field names that the discrete tool silently drops
		// (characters→text, fills→fillColor, lineHeight→lineHeightValue/Unit, width→none).
		if msg := rejectUnknownParams("create_text", params, createTextKeys, createTextHints); msg != "" {
			return msg
		}
		if msg := validateTextStyleParams(params); msg != "" {
			return msg
		}

	case "set_text":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if msg := validateTextStyleParams(params); msg != "" {
			return msg
		}
		_, hasText := params["text"]
		hasStyle := false
		for _, k := range textStyleKeys {
			if _, ok := params[k]; ok {
				hasStyle = true
				break
			}
		}
		if !hasText && !hasStyle {
			return "set_text requires `text` or at least one styling param (e.g. textAlignHorizontal, textAutoResize)"
		}

	case "set_fills":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if _, hasPaints := params["paints"]; !hasPaints {
			if color, _ := params["color"].(string); color == "" {
				return "color or paints is required (a hex string e.g. #FF5733, or a paints[] array for gradients/images)"
			}
		}
		if mode, ok := params["mode"].(string); ok && mode != "replace" && mode != "append" {
			return "mode must be 'replace' or 'append'"
		}

	case "set_strokes":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if _, hasPaints := params["paints"]; !hasPaints {
			if color, _ := params["color"].(string); color == "" {
				return "color or paints is required (a hex string e.g. #FF5733, or a paints[] array for gradients/images)"
			}
		}
		if mode, ok := params["mode"].(string); ok && mode != "replace" && mode != "append" {
			return "mode must be 'replace' or 'append'"
		}

	case "move_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasX := params["x"]
		_, hasY := params["y"]
		if !hasX && !hasY {
			return "at least one of x or y is required"
		}

	case "resize_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		if msg := validateLayoutSizingParams(params); msg != "" {
			return msg
		}
		_, hasW := params["width"]
		_, hasH := params["height"]
		hasSizing := false
		for _, k := range layoutSizingKeys {
			if _, ok := params[k]; ok {
				hasSizing = true
				break
			}
		}
		if !hasW && !hasH && !hasSizing {
			return "resize_nodes requires width, height, or a layout-sizing param (e.g. layoutSizingHorizontal)"
		}

	case "boolean_operation":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		op, _ := params["operation"].(string)
		switch op {
		case "UNION", "SUBTRACT", "INTERSECT", "EXCLUDE", "FLATTEN":
		default:
			return fmt.Sprintf("operation must be UNION, SUBTRACT, INTERSECT, EXCLUDE, or FLATTEN, got: %q", op)
		}
		if op != "FLATTEN" && len(nodeIDs) < 2 {
			return fmt.Sprintf("%s needs at least 2 nodes, got %d", op, len(nodeIDs))
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "delete_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "rename_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}

	case "clone_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "import_image":
		imageData, _ := params["imageData"].(string)
		imagePath, _ := params["imagePath"].(string)
		if imageData == "" && imagePath == "" {
			return "provide either imagePath (a local file) or imageData (base64)"
		}
		if sm, ok := params["scaleMode"].(string); ok && sm != "" {
			switch sm {
			case "FILL", "FIT", "CROP", "TILE":
			default:
				return fmt.Sprintf("scaleMode must be FILL, FIT, CROP, or TILE, got: %s", sm)
			}
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	// ── Style tools ──────────────────────────────────────────────────────────

	case "create_paint_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if _, hasPaints := params["paints"]; !hasPaints {
			if color, _ := params["color"].(string); color == "" {
				return "color or paints is required (a hex string e.g. #FF5733, or a paints[] array for gradients/images)"
			}
		}

	case "create_text_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if td, ok := params["textDecoration"].(string); ok && td != "" {
			switch td {
			case "NONE", "UNDERLINE", "STRIKETHROUGH":
			default:
				return fmt.Sprintf("textDecoration must be NONE, UNDERLINE, or STRIKETHROUGH, got: %s", td)
			}
		}
		if unit, ok := params["lineHeightUnit"].(string); ok && unit != "" {
			switch unit {
			case "PIXELS", "PERCENT":
			default:
				return fmt.Sprintf("lineHeightUnit must be PIXELS or PERCENT, got: %s", unit)
			}
		}
		if unit, ok := params["letterSpacingUnit"].(string); ok && unit != "" {
			switch unit {
			case "PIXELS", "PERCENT":
			default:
				return fmt.Sprintf("letterSpacingUnit must be PIXELS or PERCENT, got: %s", unit)
			}
		}

	case "create_effect_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if t, ok := params["type"].(string); ok && t != "" {
			switch t {
			case "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR":
			default:
				return fmt.Sprintf("type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR, got: %s", t)
			}
		}

	case "create_grid_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if p, ok := params["pattern"].(string); ok && p != "" {
			switch p {
			case "GRID", "COLUMNS", "ROWS":
			default:
				return fmt.Sprintf("pattern must be GRID, COLUMNS, or ROWS, got: %s", p)
			}
		}
		if a, ok := params["alignment"].(string); ok && a != "" {
			switch a {
			case "STRETCH", "CENTER", "MIN", "MAX":
			default:
				return fmt.Sprintf("alignment must be STRETCH, CENTER, MIN, or MAX, got: %s", a)
			}
		}

	case "update_paint_style":
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}
		_, hasName := params["name"]
		_, hasColor := params["color"]
		_, hasPaints := params["paints"]
		_, hasDesc := params["description"]
		if !hasName && !hasColor && !hasPaints && !hasDesc {
			return "at least one of name, color, paints, or description is required"
		}

	case "delete_style":
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}

	// ── Variable tools ───────────────────────────────────────────────────────

	case "create_variable_collection":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}

	case "add_variable_mode":
		if collectionId, _ := params["collectionId"].(string); collectionId == "" {
			return "collectionId is required"
		}
		if modeName, _ := params["modeName"].(string); modeName == "" {
			return "modeName is required"
		}

	case "create_variable":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if collectionId, _ := params["collectionId"].(string); collectionId == "" {
			return "collectionId is required"
		}
		varType, _ := params["type"].(string)
		switch varType {
		case "COLOR", "FLOAT", "STRING", "BOOLEAN":
		default:
			return fmt.Sprintf("type must be COLOR, FLOAT, STRING, or BOOLEAN, got: %s", varType)
		}

	case "set_variable_value":
		if variableId, _ := params["variableId"].(string); variableId == "" {
			return "variableId is required"
		}
		if modeId, _ := params["modeId"].(string); modeId == "" {
			return "modeId is required"
		}
		if _, ok := params["value"]; !ok {
			return "value is required"
		}

	case "delete_variable":
		vid, _ := params["variableId"].(string)
		cid, _ := params["collectionId"].(string)
		if vid == "" && cid == "" {
			return "variableId or collectionId is required"
		}

	// ── Linked tools ─────────────────────────────────────────────────────────

	case "apply_style_to_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}
		if target, ok := params["target"].(string); ok && target != "" {
			switch target {
			case "fill", "stroke":
			default:
				return fmt.Sprintf("target must be fill or stroke, got: %s", target)
			}
		}

	case "bind_variable_to_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if variableId, _ := params["variableId"].(string); variableId == "" {
			return "variableId is required"
		}
		if field, _ := params["field"].(string); field == "" {
			return "field is required"
		}

	case "swap_component":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if componentId, _ := params["componentId"].(string); componentId == "" {
			return "componentId is required"
		}
		if cid, _ := params["componentId"].(string); cid != "" && !ValidNodeID(cid) {
			return fmt.Sprintf("componentId must use colon format e.g. 4029:12345, got: %s", cid)
		}

	case "detach_instance":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	// ── Library tools ────────────────────────────────────────────────────────

	case "import_component_by_key":
		key, _ := params["key"].(string)
		if msg := validatePublishedImportKey("component", key); msg != "" {
			return msg
		}
		if msg := validateImportComponentAssetType(params["assetType"]); msg != "" {
			return msg
		}

	case "import_style_by_key":
		key, _ := params["key"].(string)
		if msg := validatePublishedImportKey("style", key); msg != "" {
			return msg
		}

	case "import_variable_by_key":
		key, _ := params["key"].(string)
		if msg := validateVariableImportKey(key); msg != "" {
			return msg
		}

	case "create_instance":
		componentID, _ := params["componentId"].(string)
		if componentID == "" {
			return "componentId is required"
		}
		if !ValidNodeID(componentID) {
			return fmt.Sprintf("componentId must use colon format e.g. 4029:12345, got: %s", componentID)
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "set_instance_properties":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		props, ok := params["properties"].(map[string]interface{})
		if !ok || len(props) == 0 {
			return "properties is required and must not be empty"
		}

	case "set_variable_mode":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if collectionID, _ := params["collectionId"].(string); collectionID == "" {
			return "collectionId is required"
		}
		if modeID, _ := params["modeId"].(string); modeID == "" {
			return "modeId is required"
		}

	case "get_remote_variable_collection":
		if collectionID, _ := params["collectionId"].(string); collectionID == "" {
			return "collectionId is required"
		}

	case "get_local_components":
		// pageId is optional; validate only when present and non-empty.
		if pageID, ok := params["pageId"].(string); ok && pageID != "" {
			if !ValidNodeID(pageID) {
				return fmt.Sprintf("pageId must use colon format e.g. 0:1, got: %s", pageID)
			}
		}

	case "fetch_library_catalog":
		fileKey, _ := params["fileKey"].(string)
		if fileKey == "" {
			return "fileKey is required"
		}
		if !fileKeyPattern.MatchString(fileKey) {
			return "fileKey must be alphanumeric (got invalid characters)"
		}
		if outPath, _ := params["outPath"].(string); outPath == "" {
			return "outPath is required"
		}

	// ── Prototype tools ──────────────────────────────────────────────────────

	case "set_reactions":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		rawReactions, ok := params["reactions"]
		if !ok {
			return "reactions is required"
		}
		reactions, ok := rawReactions.([]any)
		if !ok {
			return "reactions must be an array"
		}
		if mode, ok := params["mode"].(string); ok && mode != "" {
			if mode != "replace" && mode != "append" {
				return fmt.Sprintf("mode must be 'replace' or 'append', got: %s", mode)
			}
		}
		for i, raw := range reactions {
			r, ok := raw.(map[string]any)
			if !ok {
				return fmt.Sprintf("reactions[%d] must be an object", i)
			}
			if msg := validateReaction(i, r); msg != "" {
				return msg
			}
		}

	case "remove_reactions":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if raw, ok := params["indices"].([]any); ok {
			for i, v := range raw {
				if _, ok := v.(float64); !ok {
					return fmt.Sprintf("indices[%d] must be a number", i)
				}
			}
		}

	case "set_prototype_start":
		// One or more frame nodeIds become the page's flow starting points.
		// Exception: mode "clear" empties the page's start points and needs no nodeId.
		mode, _ := params["mode"].(string)
		if mode != "clear" && len(nodeIDs) == 0 {
			return "at least one nodeId is required (unless mode is 'clear')"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", id)
			}
		}
		if names, ok := params["names"].([]any); ok {
			for i, v := range names {
				if _, ok := v.(string); !ok {
					return fmt.Sprintf("names[%d] must be a string", i)
				}
			}
		}
		if mode != "" && mode != "replace" && mode != "append" && mode != "remove" && mode != "clear" {
			return fmt.Sprintf("mode must be 'replace', 'append', 'remove', or 'clear', got: %s", mode)
		}

	// ── Node Control ────────────────────────────────────────────────

	case "set_visible":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		if _, ok := params["visible"].(bool); !ok {
			return "visible (boolean) is required"
		}

	case "lock_nodes", "unlock_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "rotate_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		if _, ok := params["rotation"].(float64); !ok {
			return "rotation (degrees) is required"
		}

	case "reorder_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		order, _ := params["order"].(string)
		switch order {
		case "bringToFront", "sendToBack", "bringForward", "sendBackward":
		default:
			return fmt.Sprintf("order must be bringToFront, sendToBack, bringForward, or sendBackward, got: %s", order)
		}

	case "set_blend_mode":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		blendMode, _ := params["blendMode"].(string)
		if blendMode == "" {
			return "blendMode is required"
		}
		validBlendModes := map[string]bool{
			"NORMAL": true, "MULTIPLY": true, "SCREEN": true, "OVERLAY": true,
			"DARKEN": true, "LIGHTEN": true, "COLOR_DODGE": true, "COLOR_BURN": true,
			"HARD_LIGHT": true, "SOFT_LIGHT": true, "DIFFERENCE": true, "EXCLUSION": true,
			"HUE": true, "SATURATION": true, "COLOR": true, "LUMINOSITY": true,
			"PASS_THROUGH": true,
		}
		if !validBlendModes[blendMode] {
			return fmt.Sprintf("blendMode %q is not a valid Figma blend mode", blendMode)
		}

	case "set_constraints":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasH := params["horizontal"]
		_, hasV := params["vertical"]
		if !hasH && !hasV {
			return "at least one of horizontal or vertical is required"
		}
		if h, ok := params["horizontal"].(string); ok && h != "" {
			switch h {
			case "MIN", "MAX", "CENTER", "STRETCH", "SCALE":
			default:
				return fmt.Sprintf("horizontal must be MIN, MAX, CENTER, STRETCH, or SCALE, got: %s", h)
			}
		}
		if v, ok := params["vertical"].(string); ok && v != "" {
			switch v {
			case "MIN", "MAX", "CENTER", "STRETCH", "SCALE":
			default:
				return fmt.Sprintf("vertical must be MIN, MAX, CENTER, STRETCH, or SCALE, got: %s", v)
			}
		}

	case "reparent_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		parentID, _ := params["parentId"].(string)
		if parentID == "" {
			return "parentId is required"
		}
		if !ValidNodeID(parentID) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", parentID)
		}

	case "batch_rename_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasFind := params["find"]
		_, hasReplace := params["replace"]
		_, hasPrefix := params["prefix"]
		_, hasSuffix := params["suffix"]
		if !hasFind && !hasReplace && !hasPrefix && !hasSuffix {
			return "at least one of find/replace, prefix, or suffix is required"
		}
		if hasFind && !hasReplace {
			return "replace is required when find is provided"
		}

	case "find_replace_text":
		find, _ := params["find"].(string)
		if find == "" {
			return "find is required"
		}
		if _, ok := params["replace"]; !ok {
			return "replace is required"
		}
		if nodeID, ok := params["nodeId"].(string); ok && nodeID != "" && !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if len(nodeIDs) > 0 && nodeIDs[0] != "" && !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}

	// ── Page management ─────────────────────────────────────────────

	case "add_page":
		if idx, ok := params["index"].(float64); ok && idx < 0 {
			return "index must be non-negative"
		}

	case "delete_page", "rename_page":
		pageID, _ := params["pageId"].(string)
		pageName, _ := params["pageName"].(string)
		if pageID == "" && pageName == "" {
			return "pageId or pageName is required"
		}
		if tool == "rename_page" {
			if newName, _ := params["newName"].(string); newName == "" {
				return "newName is required"
			}
		}

	case "set_effects":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		effects, ok := params["effects"]
		if !ok {
			return "effects array is required"
		}
		effectList, ok := effects.([]interface{})
		if !ok {
			return "effects must be an array"
		}
		for i, e := range effectList {
			em, ok := e.(map[string]interface{})
			if !ok {
				return fmt.Sprintf("effects[%d] must be an object", i)
			}
			t, _ := em["type"].(string)
			switch t {
			case "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR":
			default:
				return fmt.Sprintf("effects[%d].type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR, got: %s", i, t)
			}
		}

	case "create_section":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
	}

	// Generic unknown-param guard for EVERY registered tool — the schema-derived
	// allowlist catches a Plugin-API-name typo the plugin would otherwise silently
	// drop (create_text is also checked in-case above with its richer hints). The
	// registry is populated by RegisterTools; in pure unit tests it's empty, so this
	// is a no-op there. Batch/FigmaPlan ops use the BatchOpCatalog validator.
	if msg := rejectUnknownToolParams(tool, params); msg != "" {
		return msg
	}

	return ""
}

var validTriggerTypes = map[string]bool{
	"ON_CLICK": true, "ON_HOVER": true, "ON_PRESS": true, "ON_DRAG": true,
	"AFTER_TIMEOUT": true, "MOUSE_ENTER": true, "MOUSE_LEAVE": true,
	"MOUSE_UP": true, "MOUSE_DOWN": true,
	"ON_KEY_DOWN": true, "ON_MEDIA_HIT": true, "ON_MEDIA_END": true,
}

func validateReaction(idx int, r map[string]any) string {
	if trigger, ok := r["trigger"].(map[string]any); ok {
		if msg := validateTriggerType(idx, trigger); msg != "" {
			return msg
		}
	}
	// The current API uses an `actions` array (plural); `action` (singular) is the
	// deprecated form. Validate whichever is present, mirroring the plugin's
	// buildReaction (`actions ?? [action]`) so the real path is actually checked.
	if actions, ok := r["actions"].([]any); ok {
		for _, raw := range actions {
			if action, ok := raw.(map[string]any); ok {
				if msg := validateActionType(idx, action); msg != "" {
					return msg
				}
			}
		}
	} else if action, ok := r["action"].(map[string]any); ok {
		if msg := validateActionType(idx, action); msg != "" {
			return msg
		}
	}
	return ""
}

func validateTriggerType(idx int, trigger map[string]any) string {
	t, _ := trigger["type"].(string)
	if t != "" && !validTriggerTypes[t] {
		return fmt.Sprintf("reactions[%d].trigger.type is invalid: %s", idx, t)
	}
	if t == "AFTER_TIMEOUT" {
		if _, ok := trigger["timeout"].(float64); !ok {
			return fmt.Sprintf("reactions[%d].trigger.timeout is required for AFTER_TIMEOUT and must be a number (milliseconds)", idx)
		}
	}
	return ""
}

// validateActionType checks the required field(s) of each well-known action type,
// mirroring the plugin's normalizeAction. NODE requires destinationId (navigation is
// optional — the plugin defaults it to NAVIGATE). BACK/CLOSE need no fields. Unknown
// or future action types pass through: the plugin forwards them to setReactionsAsync
// for forward-compatibility, so the pre-flight check stays lenient to match.
func validateActionType(idx int, action map[string]any) string {
	switch t, _ := action["type"].(string); t {
	case "NODE":
		if action["destinationId"] == nil {
			return fmt.Sprintf("reactions[%d] NODE action requires destinationId", idx)
		}
	case "URL":
		if url, _ := action["url"].(string); url == "" {
			return fmt.Sprintf("reactions[%d] URL action requires url", idx)
		}
	case "SET_VARIABLE":
		if action["variableId"] == nil {
			return fmt.Sprintf("reactions[%d] SET_VARIABLE action requires variableId", idx)
		}
	case "SET_VARIABLE_MODE":
		if action["variableCollectionId"] == nil || action["variableModeId"] == nil {
			return fmt.Sprintf("reactions[%d] SET_VARIABLE_MODE action requires variableCollectionId and variableModeId", idx)
		}
	case "CONDITIONAL":
		if _, ok := action["conditionalBlocks"].([]any); !ok {
			return fmt.Sprintf("reactions[%d] CONDITIONAL action requires a conditionalBlocks array", idx)
		}
	case "UPDATE_MEDIA_RUNTIME":
		if action["mediaAction"] == nil {
			return fmt.Sprintf("reactions[%d] UPDATE_MEDIA_RUNTIME action requires mediaAction", idx)
		}
	}
	return ""
}

// validateTextStyleParams checks the optional text-styling enums (set_text, create_text).
func validateTextStyleParams(params map[string]interface{}) string {
	if v, ok := params["textAlignHorizontal"].(string); ok && v != "" {
		switch v {
		case "LEFT", "CENTER", "RIGHT", "JUSTIFIED":
		default:
			return fmt.Sprintf("textAlignHorizontal must be LEFT, CENTER, RIGHT, or JUSTIFIED, got: %s", v)
		}
	}
	if v, ok := params["textAlignVertical"].(string); ok && v != "" {
		switch v {
		case "TOP", "CENTER", "BOTTOM":
		default:
			return fmt.Sprintf("textAlignVertical must be TOP, CENTER, or BOTTOM, got: %s", v)
		}
	}
	if v, ok := params["textAutoResize"].(string); ok && v != "" {
		switch v {
		case "NONE", "HEIGHT", "WIDTH_AND_HEIGHT", "TRUNCATE":
		default:
			return fmt.Sprintf("textAutoResize must be NONE, HEIGHT, WIDTH_AND_HEIGHT, or TRUNCATE, got: %s", v)
		}
	}
	if v, ok := params["textCase"].(string); ok && v != "" {
		switch v {
		case "ORIGINAL", "UPPER", "LOWER", "TITLE", "SMALL_CAPS", "SMALL_CAPS_FORCED":
		default:
			return fmt.Sprintf("textCase must be ORIGINAL, UPPER, LOWER, TITLE, SMALL_CAPS, or SMALL_CAPS_FORCED, got: %s", v)
		}
	}
	if v, ok := params["textDecoration"].(string); ok && v != "" {
		switch v {
		case "NONE", "UNDERLINE", "STRIKETHROUGH":
		default:
			return fmt.Sprintf("textDecoration must be NONE, UNDERLINE, or STRIKETHROUGH, got: %s", v)
		}
	}
	if v, ok := params["lineHeightUnit"].(string); ok && v != "" {
		switch v {
		case "PIXELS", "PERCENT", "AUTO":
		default:
			return fmt.Sprintf("lineHeightUnit must be PIXELS, PERCENT, or AUTO, got: %s", v)
		}
	}
	if v, ok := params["letterSpacingUnit"].(string); ok && v != "" {
		switch v {
		case "PIXELS", "PERCENT":
		default:
			return fmt.Sprintf("letterSpacingUnit must be PIXELS or PERCENT, got: %s", v)
		}
	}
	return ""
}

// validateLayoutSizingParams checks the optional sizing-within-parent enums
// (resize_nodes, create_frame).
func validateLayoutSizingParams(params map[string]interface{}) string {
	for _, k := range []string{"layoutSizingHorizontal", "layoutSizingVertical"} {
		if v, ok := params[k].(string); ok && v != "" {
			switch v {
			case "FIXED", "HUG", "FILL":
			default:
				return fmt.Sprintf("%s must be FIXED, HUG, or FILL, got: %s", k, v)
			}
		}
	}
	if v, ok := params["layoutAlign"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "STRETCH", "INHERIT":
		default:
			return fmt.Sprintf("layoutAlign must be MIN, CENTER, MAX, STRETCH, or INHERIT, got: %s", v)
		}
	}
	if v, ok := params["layoutPositioning"].(string); ok && v != "" {
		switch v {
		case "AUTO", "ABSOLUTE":
		default:
			return fmt.Sprintf("layoutPositioning must be AUTO or ABSOLUTE, got: %s", v)
		}
	}
	return ""
}

func validateAutoLayoutParams(params map[string]interface{}) string {
	if lm, ok := params["layoutMode"].(string); ok && lm != "" {
		switch lm {
		case "HORIZONTAL", "VERTICAL", "NONE":
		default:
			return fmt.Sprintf("layoutMode must be HORIZONTAL, VERTICAL, or NONE, got: %s", lm)
		}
	}
	if v, ok := params["primaryAxisAlignItems"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "SPACE_BETWEEN":
		default:
			return fmt.Sprintf("primaryAxisAlignItems must be MIN, CENTER, MAX, or SPACE_BETWEEN, got: %s", v)
		}
	}
	if v, ok := params["counterAxisAlignItems"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "BASELINE":
		default:
			return fmt.Sprintf("counterAxisAlignItems must be MIN, CENTER, MAX, or BASELINE, got: %s", v)
		}
	}
	if v, ok := params["primaryAxisSizingMode"].(string); ok && v != "" {
		switch v {
		case "FIXED", "AUTO":
		default:
			return fmt.Sprintf("primaryAxisSizingMode must be FIXED or AUTO, got: %s", v)
		}
	}
	if v, ok := params["counterAxisSizingMode"].(string); ok && v != "" {
		switch v {
		case "FIXED", "AUTO":
		default:
			return fmt.Sprintf("counterAxisSizingMode must be FIXED or AUTO, got: %s", v)
		}
	}
	if v, ok := params["layoutWrap"].(string); ok && v != "" {
		switch v {
		case "NO_WRAP", "WRAP":
		default:
			return fmt.Sprintf("layoutWrap must be NO_WRAP or WRAP, got: %s", v)
		}
	}
	return ""
}

func validExportFormat(f string) bool {
	switch f {
	case "PNG", "SVG", "JPG", "PDF":
		return true
	}
	return false
}
