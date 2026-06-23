package internal

import (
	"fmt"
	"math"
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

func normalizeRPCNodeReferences(nodeIDs []string, params map[string]interface{}) {
	for i, id := range nodeIDs {
		nodeIDs[i] = NormalizeNodeID(id)
	}
	if params == nil {
		return
	}
	for _, key := range []string{"nodeId", "parentId", "pageId", "componentId"} {
		if v, ok := params[key].(string); ok {
			params[key] = NormalizeNodeID(v)
		}
	}
	if hyperlink, ok := params["hyperlink"].(map[string]interface{}); ok {
		if v, ok := hyperlink["nodeId"].(string); ok {
			hyperlink["nodeId"] = NormalizeNodeID(v)
		}
	}
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

func validateMediaScaleMode(scaleMode string) string {
	if scaleMode == "" {
		return ""
	}
	switch scaleMode {
	case "FILL", "FIT", "CROP", "TILE":
		return ""
	default:
		return fmt.Sprintf("scaleMode must be FILL, FIT, CROP, or TILE, got: %s", scaleMode)
	}
}

func validateMediaFilters(params map[string]interface{}) string {
	for _, k := range []string{"exposure", "contrast", "saturation", "temperature", "tint", "highlights", "shadows"} {
		if msg := validateOptionalNumberRange(params, k, "", -1, 1); msg != "" {
			return msg
		}
	}
	return ""
}

func validateMediaTransform(params map[string]interface{}, key string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	rows, ok := v.([]interface{})
	if !ok || len(rows) != 2 {
		return fmt.Sprintf("%s must be a 2x3 numeric matrix", key)
	}
	for i, rowRaw := range rows {
		row, ok := rowRaw.([]interface{})
		if !ok || len(row) != 3 {
			return fmt.Sprintf("%s must be a 2x3 numeric matrix", key)
		}
		for j, cell := range row {
			n, ok := cell.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				return fmt.Sprintf("%s[%d][%d] must be a finite number", key, i, j)
			}
		}
	}
	return ""
}

func validateOptionalPositiveNumber(params map[string]interface{}, key, prefix string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Sprintf("%s%s must be a number", prefix, key)
	}
	if n <= 0 {
		return fmt.Sprintf("%s%s must be positive", prefix, key)
	}
	return ""
}

func validateOptionalNumberRange(params map[string]interface{}, key, prefix string, min, max float64) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Sprintf("%s%s must be a number", prefix, key)
	}
	if n < min || n > max {
		return fmt.Sprintf("%s%s must be between %v and %v", prefix, key, min, max)
	}
	return ""
}

func validateOptionalNumber(params map[string]interface{}, key, prefix string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Sprintf("%s%s must be a number", prefix, key)
	}
	return ""
}

func validateOptionalNumberMin(params map[string]interface{}, key, prefix string, min float64) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Sprintf("%s%s must be a number", prefix, key)
	}
	if n < min {
		return fmt.Sprintf("%s%s must be >= %v", prefix, key, min)
	}
	return ""
}

func validateOptionalBool(params map[string]interface{}, key, prefix string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	if _, ok := v.(bool); !ok {
		return fmt.Sprintf("%s%s must be a boolean", prefix, key)
	}
	return ""
}

func validateEffectVector(params map[string]interface{}, key, prefix string, min, max float64, bounded bool) string {
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	vec, ok := v.(map[string]interface{})
	if !ok {
		return fmt.Sprintf("%s%s must be an object with numeric x and y", prefix, key)
	}
	for _, axis := range []string{"x", "y"} {
		raw, ok := vec[axis]
		if !ok {
			return fmt.Sprintf("%s%s.%s is required", prefix, key, axis)
		}
		n, ok := raw.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Sprintf("%s%s.%s must be a number", prefix, key, axis)
		}
		if bounded {
			if n < min || n > max {
				return fmt.Sprintf("%s%s.%s must be between %v and %v", prefix, key, axis, min, max)
			}
		} else if n <= min {
			return fmt.Sprintf("%s%s.%s must be > %v", prefix, key, axis, min)
		}
	}
	return ""
}

func validateEffectObject(prefix string, effect map[string]interface{}, allowDefaultType bool) string {
	label := ""
	if prefix != "" {
		label = prefix + "."
	}
	t, hasType := effect["type"]
	effectType, ok := t.(string)
	if hasType && !ok {
		return label + "type must be a string"
	}
	if effectType == "" {
		if allowDefaultType {
			effectType = "DROP_SHADOW"
		} else {
			return fmt.Sprintf("%stype must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, BACKGROUND_BLUR, GLASS, NOISE, or TEXTURE, got: %s", label, effectType)
		}
	}
	switch effectType {
	case "DROP_SHADOW", "INNER_SHADOW":
		if msg := validateOptionalNumberRange(effect, "opacity", label, 0, 1); msg != "" {
			return msg
		}
		for _, key := range []string{"radius", "spread"} {
			if msg := validateOptionalNumberMin(effect, key, label, 0); msg != "" {
				return msg
			}
		}
		for _, key := range []string{"offsetX", "offsetY"} {
			if msg := validateOptionalNumber(effect, key, label); msg != "" {
				return msg
			}
		}
	case "LAYER_BLUR", "BACKGROUND_BLUR":
		if blurType, ok := effect["blurType"].(string); ok && blurType != "" {
			switch blurType {
			case "NORMAL", "PROGRESSIVE":
			default:
				return fmt.Sprintf("%sblurType must be NORMAL or PROGRESSIVE, got: %s", label, blurType)
			}
		} else if raw, ok := effect["blurType"]; ok && raw != nil {
			return label + "blurType must be a string"
		}
		for _, key := range []string{"radius", "startRadius"} {
			if msg := validateOptionalNumberMin(effect, key, label, 0); msg != "" {
				return msg
			}
		}
		for _, key := range []string{"startOffset", "endOffset"} {
			if msg := validateEffectVector(effect, key, label, 0, 1, true); msg != "" {
				return msg
			}
		}
	case "GLASS":
		for _, key := range []string{"lightIntensity", "refraction", "dispersion"} {
			if msg := validateOptionalNumberRange(effect, key, label, 0, 1); msg != "" {
				return msg
			}
		}
		if msg := validateOptionalNumberMin(effect, "depth", label, 1); msg != "" {
			return msg
		}
		if msg := validateOptionalNumberMin(effect, "radius", label, 0); msg != "" {
			return msg
		}
		if msg := validateOptionalNumber(effect, "lightAngle", label); msg != "" {
			return msg
		}
	case "NOISE":
		if noiseType, ok := effect["noiseType"].(string); ok && noiseType != "" {
			switch noiseType {
			case "MONOTONE", "DUOTONE", "MULTITONE":
			default:
				return fmt.Sprintf("%snoiseType must be MONOTONE, DUOTONE, or MULTITONE, got: %s", label, noiseType)
			}
		} else if raw, ok := effect["noiseType"]; ok && raw != nil {
			return label + "noiseType must be a string"
		}
		for _, key := range []string{"opacity", "density"} {
			if msg := validateOptionalNumberRange(effect, key, label, 0, 1); msg != "" {
				return msg
			}
		}
		if msg := validateOptionalNumberMin(effect, "noiseSize", label, 0); msg != "" {
			return msg
		}
		if msg := validateEffectVector(effect, "noiseSizeVector", label, 0, 0, false); msg != "" {
			return msg
		}
	case "TEXTURE":
		if msg := validateOptionalNumberMin(effect, "noiseSize", label, 0); msg != "" {
			return msg
		}
		if msg := validateOptionalNumberMin(effect, "radius", label, 0); msg != "" {
			return msg
		}
		if msg := validateEffectVector(effect, "noiseSizeVector", label, 0, 0, false); msg != "" {
			return msg
		}
		if msg := validateOptionalBool(effect, "clipToShape", label); msg != "" {
			return msg
		}
	default:
		return fmt.Sprintf("%stype must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, BACKGROUND_BLUR, GLASS, NOISE, or TEXTURE, got: %s", label, effectType)
	}
	if msg := validateOptionalBool(effect, "visible", label); msg != "" {
		return msg
	}
	return ""
}

func validateEffectsArray(params map[string]interface{}, key, prefix string, allowDefaultType bool) string {
	raw, ok := params[key]
	if !ok {
		return ""
	}
	effects, ok := raw.([]interface{})
	if !ok {
		return fmt.Sprintf("%s%s must be an array", prefix, key)
	}
	for i, rawEffect := range effects {
		effect, ok := rawEffect.(map[string]interface{})
		if !ok {
			return fmt.Sprintf("%s%s[%d] must be an object", prefix, key, i)
		}
		if msg := validateEffectObject(fmt.Sprintf("%s%s[%d]", prefix, key, i), effect, allowDefaultType); msg != "" {
			return msg
		}
	}
	return ""
}

func validateCodeSyntaxPlatform(platform string) string {
	switch platform {
	case "WEB", "ANDROID", "iOS":
		return ""
	default:
		return fmt.Sprintf("codeSyntax platform must be WEB, ANDROID, or iOS, got: %s", platform)
	}
}

func validateStyleType(styleType string) string {
	switch styleType {
	case "PAINT", "TEXT", "EFFECT", "GRID":
		return ""
	default:
		return fmt.Sprintf("styleType must be PAINT, TEXT, EFFECT, or GRID, got: %s", styleType)
	}
}

func isPageDividerName(name string) bool {
	if name == "" {
		return false
	}
	runes := []rune(name)
	first := runes[0]
	switch first {
	case '*', '-', ' ', '\u2013', '\u2014':
	default:
		return false
	}
	for _, r := range runes[1:] {
		if r != first {
			return false
		}
	}
	return true
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

	case "get_image_by_hash":
		if hash, _ := params["hash"].(string); hash == "" {
			return "hash is required"
		}

	case "get_dev_resources":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}

	case "resolve_variable_for_consumer":
		if variableID, _ := params["variableId"].(string); variableID == "" {
			return "variableId is required"
		}
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
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

	case "create_vector", "create_slice":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_page_divider":
		if name, ok := params["name"].(string); ok && name != "" && !isPageDividerName(name) {
			return "name must be all asterisks, hyphens, spaces, en dashes, or em dashes"
		}

	case "create_text_path":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if startSegment, ok := params["startSegment"].(float64); ok && (startSegment < 0 || math.IsNaN(startSegment) || math.IsInf(startSegment, 0) || startSegment != math.Trunc(startSegment)) {
			return "startSegment must be a non-negative integer"
		}
		if startPosition, ok := params["startPosition"].(float64); ok && (startPosition < 0 || startPosition > 1 || math.IsNaN(startPosition) || math.IsInf(startPosition, 0)) {
			return "startPosition must be between 0 and 1"
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
		if msg := rejectChildLayoutSizingParamsForAutoLayout(params); msg != "" {
			return msg
		}
		if msg := validateMinMaxParams(params); msg != "" {
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

	case "create_polygon", "create_star":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pc, ok := params["pointCount"].(float64); ok && pc < 3 {
			return "pointCount must be at least 3"
		}
		if tool == "create_star" {
			if ir, ok := params["innerRadius"].(float64); ok && (ir < 0 || ir > 1) {
				return "innerRadius must be between 0 and 1"
			}
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_line":
		if l, ok := params["length"].(float64); ok && l <= 0 {
			return "length must be positive"
		}
		if sc, ok := params["strokeCap"].(string); ok && sc != "" {
			switch sc {
			case "NONE", "ROUND", "SQUARE", "ARROW_LINES", "ARROW_EQUILATERAL", "DIAMOND_FILLED", "TRIANGLE_FILLED", "CIRCLE_FILLED":
			default:
				return fmt.Sprintf("strokeCap %q is not a valid Figma stroke cap", sc)
			}
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "import_svg":
		if svg, _ := params["svg"].(string); svg == "" {
			return "svg (raw SVG markup string) is required"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_table":
		nr, hasNR := params["numRows"].(float64)
		if !hasNR || nr < 1 {
			return "numRows is required and must be at least 1"
		}
		nc, hasNC := params["numColumns"].(float64)
		if !hasNC || nc < 1 {
			return "numColumns is required and must be at least 1"
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

	case "set_text_range":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		start, hasStart := params["startOffset"].(float64)
		if !hasStart {
			return "startOffset is required"
		}
		end, hasEnd := params["endOffset"].(float64)
		if !hasEnd {
			return "endOffset is required"
		}
		if start < 0 || end <= start {
			return fmt.Sprintf("invalid range: need 0 <= startOffset < endOffset, got [%v, %v)", start, end)
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
		if lo, ok := params["listOptions"].(map[string]interface{}); ok {
			if t, ok := lo["type"].(string); ok && t != "" {
				switch t {
				case "ORDERED", "UNORDERED", "NONE":
				default:
					return fmt.Sprintf("listOptions.type must be ORDERED, UNORDERED, or NONE, got: %s", t)
				}
			}
		}
		if link, ok := params["hyperlink"].(map[string]interface{}); ok {
			nodeID, hasNodeID := link["nodeId"].(string)
			_, hasURL := link["url"]
			if hasNodeID && hasURL {
				return "hyperlink must provide url or nodeId, not both"
			}
			if hasNodeID && nodeID != "" && !ValidNodeID(nodeID) {
				return fmt.Sprintf("hyperlink.nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
			}
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

	case "set_file_thumbnail":
		if nodeID, ok := params["nodeId"].(string); ok && nodeID != "" && !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}

	case "add_dev_resource":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if url, _ := params["url"].(string); url == "" {
			return "url is required"
		}

	case "edit_dev_resource":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if currentURL, _ := params["currentUrl"].(string); currentURL == "" {
			return "currentUrl is required"
		}
		_, hasURL := params["url"]
		_, hasName := params["name"]
		if !hasURL && !hasName {
			return "edit_dev_resource requires url or name"
		}

	case "delete_dev_resource":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if url, _ := params["url"].(string); url == "" {
			return "url is required"
		}

	case "import_image":
		imageData, _ := params["imageData"].(string)
		imagePath, _ := params["imagePath"].(string)
		imageURL, _ := params["imageUrl"].(string)
		if imageData == "" && imagePath == "" && imageURL == "" {
			return "provide imagePath (a local file), imageData (base64), or imageUrl (remote URL)"
		}
		if sm, ok := params["scaleMode"].(string); ok {
			if msg := validateMediaScaleMode(sm); msg != "" {
				return msg
			}
		}
		if msg := validateMediaFilters(params); msg != "" {
			return msg
		}
		if msg := validateOptionalNumber(params, "rotation", ""); msg != "" {
			return msg
		}
		if msg := validateOptionalPositiveNumber(params, "scalingFactor", ""); msg != "" {
			return msg
		}
		if msg := validateMediaTransform(params, "imageTransform"); msg != "" {
			return msg
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_video":
		videoData, _ := params["videoData"].(string)
		videoPath, _ := params["videoPath"].(string)
		if videoData == "" && videoPath == "" {
			return "provide either videoPath (a local file) or videoData (base64)"
		}
		if sm, ok := params["scaleMode"].(string); ok {
			if msg := validateMediaScaleMode(sm); msg != "" {
				return msg
			}
		}
		if msg := validateMediaFilters(params); msg != "" {
			return msg
		}
		if msg := validateOptionalNumber(params, "rotation", ""); msg != "" {
			return msg
		}
		if msg := validateOptionalPositiveNumber(params, "scalingFactor", ""); msg != "" {
			return msg
		}
		if msg := validateMediaTransform(params, "videoTransform"); msg != "" {
			return msg
		}
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_gif":
		if imageHash, _ := params["imageHash"].(string); imageHash == "" {
			return "imageHash is required"
		}
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_link_preview":
		if url, _ := params["url"].(string); url == "" {
			return "url is required"
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
		if msg := validateEffectsArray(params, "effects", "", true); msg != "" {
			return msg
		}
		if _, hasEffects := params["effects"]; !hasEffects {
			if msg := validateEffectObject("", params, true); msg != "" {
				return msg
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

	case "reorder_local_style":
		styleType, _ := params["styleType"].(string)
		if styleType == "" {
			return "styleType is required"
		}
		if msg := validateStyleType(styleType); msg != "" {
			return msg
		}
		if styleID, _ := params["styleId"].(string); styleID == "" {
			return "styleId is required"
		}

	case "reorder_local_style_folder":
		styleType, _ := params["styleType"].(string)
		if styleType == "" {
			return "styleType is required"
		}
		if msg := validateStyleType(styleType); msg != "" {
			return msg
		}
		if folder, _ := params["folder"].(string); folder == "" {
			return "folder is required"
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

	case "create_variable_alias":
		if variableID, _ := params["variableId"].(string); variableID == "" {
			return "variableId is required"
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

	case "update_variable":
		if vid, _ := params["variableId"].(string); vid == "" {
			return "variableId is required"
		}
		if raw, ok := params["scopes"].([]interface{}); ok {
			for _, s := range raw {
				sv, _ := s.(string)
				if !validVariableScopes[sv] {
					return fmt.Sprintf("invalid variable scope: %q", sv)
				}
			}
		}
		if cs, ok := params["codeSyntax"].(map[string]interface{}); ok {
			for k := range cs {
				if msg := validateCodeSyntaxPlatform(k); msg != "" {
					return msg
				}
				if _, ok := cs[k].(string); !ok {
					return fmt.Sprintf("codeSyntax.%s must be a string", k)
				}
			}
		} else if raw, ok := params["codeSyntax"]; ok && raw != nil {
			return "codeSyntax must be an object"
		}
		if raw, ok := params["removeCodeSyntax"].([]interface{}); ok {
			for _, p := range raw {
				platform, _ := p.(string)
				if msg := validateCodeSyntaxPlatform(platform); msg != "" {
					return msg
				}
			}
		}

	case "update_variable_collection":
		if cid, _ := params["collectionId"].(string); cid == "" {
			return "collectionId is required"
		}
		if rm, ok := params["renameMode"].(map[string]interface{}); ok {
			if mid, ok := rm["modeId"].(string); !ok || mid == "" {
				return "renameMode requires a modeId"
			}
			if newName, ok := rm["newName"].(string); !ok || newName == "" {
				return "renameMode requires a newName"
			}
		} else if raw, ok := params["renameMode"]; ok && raw != nil {
			return "renameMode must be an object"
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

	case "bind_variable_to_effect":
		if _, ok := params["effect"].(map[string]interface{}); !ok {
			return "effect is required and must be an object"
		}
		if field, _ := params["field"].(string); field == "" {
			return "field is required"
		}
		if variableID, _ := params["variableId"].(string); variableID == "" {
			return "variableId is required"
		}

	case "bind_variable_to_layout_grid":
		if _, ok := params["layoutGrid"].(map[string]interface{}); !ok {
			return "layoutGrid is required and must be an object"
		}
		if field, _ := params["field"].(string); field == "" {
			return "field is required"
		}
		if variableID, _ := params["variableId"].(string); variableID == "" {
			return "variableId is required"
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
			"LINEAR_DODGE": true, "LINEAR_BURN": true,
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
			effect, ok := e.(map[string]interface{})
			if !ok {
				return fmt.Sprintf("effects[%d] must be an object", i)
			}
			if msg := validateEffectObject(fmt.Sprintf("effects[%d]", i), effect, false); msg != "" {
				return msg
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

// validVariableScopes is the Figma VariableScope union (publishing scopes that
// restrict where a variable is surfaced in the UI). WEB/ANDROID code-syntax scopes
// are configured via codeSyntax, not here.
var validVariableScopes = map[string]bool{
	"ALL_SCOPES": true, "TEXT_CONTENT": true, "CORNER_RADIUS": true,
	"WIDTH_HEIGHT": true, "GAP": true, "ALL_FILLS": true, "FRAME_FILL": true,
	"SHAPE_FILL": true, "TEXT_FILL": true, "STROKE_COLOR": true, "STROKE_FLOAT": true,
	"EFFECT_FLOAT": true, "EFFECT_COLOR": true, "OPACITY": true, "FONT_FAMILY": true,
	"FONT_STYLE": true, "FONT_WEIGHT": true, "FONT_SIZE": true, "LINE_HEIGHT": true,
	"LETTER_SPACING": true, "PARAGRAPH_SPACING": true, "PARAGRAPH_INDENT": true,
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
	if v, ok := params["textTruncation"].(string); ok && v != "" {
		switch v {
		case "DISABLED", "ENDING":
		default:
			return fmt.Sprintf("textTruncation must be DISABLED or ENDING, got: %s", v)
		}
	}
	if v, ok := params["leadingTrim"].(string); ok && v != "" {
		switch v {
		case "CAP_HEIGHT", "NONE":
		default:
			return fmt.Sprintf("leadingTrim must be CAP_HEIGHT or NONE, got: %s", v)
		}
	}
	return ""
}

// validateLayoutSizingParams checks the optional sizing-within-parent enums
// (resize_nodes, create_frame).
func validateLayoutSizingParams(params map[string]interface{}) string {
	if msg := validateMinMaxParams(params); msg != "" {
		return msg
	}
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

func validateMinMaxParams(params map[string]interface{}) string {
	for _, k := range []string{"minWidth", "maxWidth", "minHeight", "maxHeight"} {
		if v, ok := params[k]; ok && v != nil {
			if n, ok := v.(float64); ok && n <= 0 {
				return fmt.Sprintf("%s must be positive", k)
			}
		}
	}
	return ""
}

func rejectChildLayoutSizingParamsForAutoLayout(params map[string]interface{}) string {
	for _, k := range []string{"layoutSizingHorizontal", "layoutSizingVertical", "layoutGrow", "layoutAlign", "layoutPositioning"} {
		if _, ok := params[k]; ok {
			return fmt.Sprintf("%s belongs to resize_nodes/create_frame, not set_auto_layout", k)
		}
	}
	return ""
}

func validateAutoLayoutParams(params map[string]interface{}) string {
	if lm, ok := params["layoutMode"].(string); ok && lm != "" {
		switch lm {
		case "HORIZONTAL", "VERTICAL", "GRID", "NONE":
		default:
			return fmt.Sprintf("layoutMode must be HORIZONTAL, VERTICAL, GRID, or NONE, got: %s", lm)
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
	if v, ok := params["counterAxisAlignContent"].(string); ok && v != "" {
		switch v {
		case "AUTO", "SPACE_BETWEEN":
		default:
			return fmt.Sprintf("counterAxisAlignContent must be AUTO or SPACE_BETWEEN, got: %s", v)
		}
	}
	if v, ok := params["overflowDirection"].(string); ok && v != "" {
		switch v {
		case "NONE", "HORIZONTAL", "VERTICAL", "BOTH":
		default:
			return fmt.Sprintf("overflowDirection must be NONE, HORIZONTAL, VERTICAL, or BOTH, got: %s", v)
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
