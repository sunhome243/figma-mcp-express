package internal

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	batchRefPattern        = regexp.MustCompile(`^\$(\d+)\.[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*$`)
	batchProjectionPattern = regexp.MustCompile(`^\$(\d+)\.[A-Za-z0-9_.]+\[\*\](?:\.[A-Za-z0-9_.]+)?$`)
	batchNumericRefHead    = regexp.MustCompile(`^\$(\d+)\.`)
	batchNamedRefPattern   = regexp.MustCompile(`^\$([A-Za-z_]\w*)(?:\.[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*)?$`)
	batchNamedRefHead      = regexp.MustCompile(`^\$([A-Za-z_]\w*)(?:\.|$)`)
	batchBindingName       = regexp.MustCompile(`^[A-Za-z_]\w*$`)
)

var scriptLikeBatchKeys = map[string]bool{
	"script":   true,
	"code":     true,
	"js":       true,
	"eval":     true,
	"function": true,
}

const (
	defaultBatchMaxOps          = 200
	defaultBatchMaxPayloadBytes = 2 * 1024 * 1024
)

func validateBatchOps(rawOps []interface{}) error {
	normalizeBatchNodeIDs(rawOps)
	for i, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("ops[%d] must be an object {type, nodeIds?, params?}", i)
		}
		if err := validateBatchOp(i, op, nil); err != nil {
			return err
		}
	}
	return nil
}

func batchOpsFromParams(params map[string]interface{}) ([]interface{}, error) {
	for k := range params {
		switch k {
		case "ops", "continueOnError", "validateOnly", "channel":
			// structural batch params
		default:
			// Presence params (origin/status/sessionId/task) ride params; sessionId is
			// injected by Node.Send and declared in no schema. Allow them through the
			// follower /rpc re-validation (they're stripped/forwarded downstream, not ops).
			if !isPresenceParam(k) {
				return nil, fmt.Errorf("batch: unknown top-level param %q", k)
			}
		}
	}
	rawOps, ok := params["ops"].([]interface{})
	if !ok || len(rawOps) == 0 {
		return nil, fmt.Errorf("batch requires a non-empty `ops` array")
	}
	if err := validateBatchEnvelopeLimits(rawOps); err != nil {
		return nil, err
	}
	return rawOps, nil
}

func validateBatchEnvelopeLimits(rawOps []interface{}) error {
	maxOps := envInt("FIGMA_MCP_BATCH_MAX_OPS", defaultBatchMaxOps)
	if len(rawOps) > maxOps {
		return fmt.Errorf("batch has %d ops, exceeding the cap of %d; split into smaller logical sections or raise FIGMA_MCP_BATCH_MAX_OPS", len(rawOps), maxOps)
	}

	raw, err := json.Marshal(rawOps)
	if err != nil {
		return fmt.Errorf("batch ops must be JSON-serializable: %w", err)
	}
	maxBytes := envInt("FIGMA_MCP_BATCH_MAX_BYTES", defaultBatchMaxPayloadBytes)
	if len(raw) > maxBytes {
		return fmt.Errorf("batch encoded ops payload is %d bytes, exceeding the cap of %d; split the batch or raise FIGMA_MCP_BATCH_MAX_BYTES", len(raw), maxBytes)
	}
	return nil
}

func validateAndPrepareBatchParams(params map[string]interface{}) error {
	rawOps, err := batchOpsFromParams(params)
	if err != nil {
		return err
	}
	if err := validateBatchOps(rawOps); err != nil {
		return err
	}
	normalizeBatchNodeIDs(rawOps)
	prepareBatchImportParams(rawOps)
	// Sanitize the presence label on the follower /rpc path (which is NOT MCP-
	// schema-validated): keep `origin` only when it is a known roster member,
	// drop it otherwise so a stray label never reaches the plugin. The leader-
	// local path applies the same filter via pickOrigin in the batch handler.
	if o, ok := pickOrigin(params); ok {
		params["origin"] = o
	} else {
		delete(params, "origin")
	}
	// Manual `status` no longer flows through batch — it moved to the dedicated
	// set_presence tool, so presence is one consistent path. Strip any stray status
	// (e.g. from a not-yet-migrated caller) so the leader and follower paths agree
	// and batch carries identity only.
	delete(params, "status")
	return nil
}

func normalizeBatchNodeIDs(rawOps []interface{}) {
	for _, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if op["type"] == "map" {
			if over, ok := op["over"].([]interface{}); ok {
				normalizeBatchRefIDValues(over)
			}
			if do, ok := op["do"].(map[string]interface{}); ok {
				normalizeBatchNodeIDs([]interface{}{do})
			}
			continue
		}
		hoistNodeIDsFromParams(op)
		if nids, ok := op["nodeIds"].([]interface{}); ok {
			for j, v := range nids {
				if s, ok := v.(string); ok {
					nids[j] = NormalizeNodeID(s)
				}
			}
		}
		if params, ok := op["params"].(map[string]interface{}); ok {
			normalizeBatchRefIDValues(params)
		}
	}
}

func normalizeBatchRefIDValues(v interface{}) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, child := range x {
			if s, ok := child.(string); ok && isBatchNodeIDParam(k) && !isBatchRefLike(s) {
				x[k] = NormalizeNodeID(s)
				continue
			}
			normalizeBatchRefIDValues(child)
		}
	case []interface{}:
		for _, child := range x {
			normalizeBatchRefIDValues(child)
		}
	}
}

func isBatchNodeIDParam(name string) bool {
	switch name {
	case "nodeId", "parentId", "pageId", "componentId":
		return true
	default:
		return false
	}
}

// hoistNodeIDsFromParams makes node-target ops forgiving of a common composition
// mistake: get_batch_op_spec lists `nodeIds` under paramKeys (it is the op's input
// schema), so callers composing straight from the spec nest the target in `params` —
// but a batch op takes its mutate-target as the OP-LEVEL `nodeIds` field. When the
// op-level field is absent/empty and a plural `nodeIds` was nested in params, hoist it.
//
// ONLY the plural `nodeIds` is hoisted: no op uses a plural `nodeIds` param as anything
// other than its targets. The SINGULAR `nodeId` is deliberately left in params — read
// /scan ops (scan_nodes_by_types, scan_text_nodes, get_design_context, …) legitimately
// take `nodeId` as a subtree ROOT param, so hoisting it would break them.
func hoistNodeIDsFromParams(op map[string]interface{}) {
	if existing, ok := op["nodeIds"].([]interface{}); ok && len(existing) > 0 {
		return // op-level target already provided — never override it
	}
	params, ok := op["params"].(map[string]interface{})
	if !ok {
		return
	}
	if pn, ok := params["nodeIds"]; ok {
		op["nodeIds"] = pn
		delete(params, "nodeIds")
	}
}

func validateBatchOp(i int, op map[string]interface{}, allowedNamedRefs map[string]bool) error {
	t, _ := op["type"].(string)
	if t == "" {
		return fmt.Errorf("ops[%d] missing string `type`", i)
	}
	if t == "batch" {
		return fmt.Errorf("ops[%d]: batch cannot be nested", i)
	}
	if _, ok := batchOpCatalog[t]; !ok {
		return fmt.Errorf("ops[%d]: unknown op type %q; call search_batch_ops/get_batch_op_spec first", i, t)
	}

	allowedKeys := map[string]bool{"type": true, "nodeIds": true, "params": true}
	if t == "map" {
		allowedKeys = map[string]bool{"type": true, "over": true, "as": true, "do": true}
	}
	for k := range op {
		if scriptLikeBatchKeys[strings.ToLower(k)] {
			return fmt.Errorf("ops[%d]: script-like field %q is not allowed; use declarative batch/FigmaPlan ops", i, k)
		}
		if !allowedKeys[k] {
			return fmt.Errorf("ops[%d]: unknown op field %q", i, k)
		}
	}
	if err := rejectScriptLikeKeys(op, fmt.Sprintf("ops[%d]", i)); err != nil {
		return err
	}

	if t == "map" {
		if allowedNamedRefs != nil {
			return fmt.Errorf("ops[%d]: map cannot be nested inside map.do", i)
		}
		over, ok := op["over"]
		if !ok {
			return fmt.Errorf("ops[%d]: map.over must be a non-empty ref string or literal array", i)
		}
		switch v := over.(type) {
		case string:
			if v == "" {
				return fmt.Errorf("ops[%d]: map.over must be a non-empty ref string or literal array", i)
			}
		case []interface{}:
			// Literal arrays are supported by the plugin runtime. Refs inside the
			// array are still validated below.
		default:
			return fmt.Errorf("ops[%d]: map.over must be a non-empty ref string or literal array", i)
		}
		if err := validateBatchRefs(over, i, nil); err != nil {
			return err
		}
		bindingName := "item"
		if as, ok := op["as"]; ok {
			s, ok := as.(string)
			if !ok || s == "" {
				return fmt.Errorf("ops[%d]: map.as must be a non-empty string when provided", i)
			}
			bindingName = s
		}
		if !batchBindingName.MatchString(bindingName) {
			return fmt.Errorf("ops[%d]: map.as must be an identifier matching [A-Za-z_]\\w*", i)
		}
		if bindingName == "index" {
			return fmt.Errorf("ops[%d]: map.as %q is reserved for the iteration index", i, bindingName)
		}
		doRaw, ok := op["do"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("ops[%d]: map.do must be an op object", i)
		}
		bindings := map[string]bool{bindingName: true, "index": true}
		if err := validateBatchOp(i, doRaw, bindings); err != nil {
			return fmt.Errorf("ops[%d].do: %s", i, err)
		}
		return nil
	}

	if err := validateBatchRefs(op, i, allowedNamedRefs); err != nil {
		return err
	}

	if rawNodeIDs, exists := op["nodeIds"]; exists {
		nids, ok := rawNodeIDs.([]interface{})
		if !ok {
			return fmt.Errorf("ops[%d].nodeIds must be an array of strings", i)
		}
		for j, v := range nids {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("ops[%d].nodeIds[%d] must be a string", i, j)
			}
			if isBatchRefLike(s) || isAllowedNamedBindingRef(s, allowedNamedRefs) {
				continue
			}
			if !ValidNodeID(NormalizeNodeID(s)) {
				return fmt.Errorf("ops[%d].nodeIds[%d] must use colon format e.g. 4029:12345 or a valid ref, got: %s", i, j, s)
			}
		}
	}

	if rawParams, exists := op["params"]; exists {
		opParams, ok := rawParams.(map[string]interface{})
		if !ok {
			return fmt.Errorf("ops[%d].params must be an object", i)
		}
		if msg := rejectUnknownBatchOpParams(t, opParams); msg != "" {
			return fmt.Errorf("ops[%d]: %s", i, msg)
		}
		if err := validateBatchParamsAgainstSchema(t, opParams, allowedNamedRefs); err != nil {
			return fmt.Errorf("ops[%d]: %s", i, err)
		}
		if msg := validateBatchImportKey(t, opParams, allowedNamedRefs); msg != "" {
			return fmt.Errorf("ops[%d]: %s", i, msg)
		}
	} else if err := validateBatchParamsAgainstSchema(t, nil, allowedNamedRefs); err != nil {
		return fmt.Errorf("ops[%d]: %s", i, err)
	}
	if msg := validateBatchOpSemantics(t, op, allowedNamedRefs); msg != "" {
		return fmt.Errorf("ops[%d]: %s", i, msg)
	}
	return nil
}

func validateBatchImportKey(tool string, params map[string]interface{}, allowedNamedRefs map[string]bool) string {
	switch tool {
	case "import_component_by_key", "import_style_by_key", "import_variable_by_key":
	default:
		return ""
	}
	key, _ := params["key"].(string)
	if isBatchRefLike(key) || isAllowedNamedBindingRef(key, allowedNamedRefs) {
		return ""
	}
	switch tool {
	case "import_component_by_key":
		if msg := validatePublishedImportKey("component", key); msg != "" {
			return msg
		}
		assetType, _ := params["assetType"].(string)
		if isBatchRefLike(assetType) || isAllowedNamedBindingRef(assetType, allowedNamedRefs) {
			return ""
		}
		return validateImportComponentAssetType(params["assetType"])
	case "import_style_by_key":
		return validatePublishedImportKey("style", key)
	case "import_variable_by_key":
		return validateVariableImportKey(key)
	}
	return ""
}

func rejectUnknownBatchOpParams(tool string, params map[string]interface{}) string {
	allowed, ok := batchOpParamKeys(tool)
	if !ok {
		return fmt.Sprintf("unknown op type %q", tool)
	}
	return rejectUnknownParams(tool, params, allowed, toolParamHints[tool])
}

func rejectScriptLikeKeys(v interface{}, path string) error {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, child := range x {
			if scriptLikeBatchKeys[strings.ToLower(k)] {
				return fmt.Errorf("%s.%s: script-like field is not allowed; use declarative batch/FigmaPlan ops", path, k)
			}
			if isFreeformBatchMapPath(path, k) {
				continue
			}
			if err := rejectScriptLikeKeys(child, path+"."+k); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, child := range x {
			if err := rejectScriptLikeKeys(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func isFreeformBatchMapPath(path, key string) bool {
	if path == "" || key == "" {
		return false
	}
	lower := strings.ToLower(key)
	if lower != "properties" && lower != "variantproperties" {
		return false
	}
	return strings.HasSuffix(path, ".params") || strings.Contains(path, ".do.params")
}

func validateBatchRefs(v interface{}, currentIndex int, allowedNamedRefs map[string]bool) error {
	switch x := v.(type) {
	case string:
		return validateBatchRefString(x, currentIndex, allowedNamedRefs)
	case []interface{}:
		for _, child := range x {
			if err := validateBatchRefs(child, currentIndex, allowedNamedRefs); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for _, child := range x {
			if err := validateBatchRefs(child, currentIndex, allowedNamedRefs); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBatchRefString(s string, currentIndex int, allowedNamedRefs map[string]bool) error {
	m := batchNumericRefHead.FindStringSubmatch(s)
	if m != nil {
		if strings.Count(s, "[*]") > 1 {
			return fmt.Errorf("ref %s: exactly one [*] wildcard is allowed", s)
		}
		if !batchRefPattern.MatchString(s) && !batchProjectionPattern.MatchString(s) {
			return fmt.Errorf("ref %s: malformed ref; use $N.field or $N.array[*].field", s)
		}
		var idx int
		if _, err := fmt.Sscanf(m[1], "%d", &idx); err != nil {
			return fmt.Errorf("ref %s: invalid op index", s)
		}
		if idx >= currentIndex {
			return fmt.Errorf("ref %s points to op #%d, which has not run yet; refs may target earlier ops only", s, idx)
		}
		return nil
	}
	return validateNamedBindingRefString(s, allowedNamedRefs)
}

func isBatchRefLike(s string) bool {
	return batchRefPattern.MatchString(s) || batchProjectionPattern.MatchString(s)
}

func isAllowedNamedBindingRef(s string, allowedNamedRefs map[string]bool) bool {
	if allowedNamedRefs == nil {
		return false
	}
	m := batchNamedRefPattern.FindStringSubmatch(s)
	return len(m) > 1 && allowedNamedRefs[m[1]]
}

func validateNamedBindingRefString(s string, allowedNamedRefs map[string]bool) error {
	head := batchNamedRefHead.FindStringSubmatch(s)
	if len(head) < 2 {
		return nil
	}
	if allowedNamedRefs == nil {
		if strings.Contains(s, ".") || strings.Contains(s, "[*]") {
			return fmt.Errorf("ref %s: named binding refs are only allowed inside map.do", s)
		}
		return nil
	}
	if strings.Contains(s, "[*]") {
		return fmt.Errorf("ref %s: named binding projections are not supported; map.do supports $%s.path and $index only", s, allowedBindingPrimary(allowedNamedRefs))
	}
	m := batchNamedRefPattern.FindStringSubmatch(s)
	if len(m) < 2 {
		return fmt.Errorf("ref %s: malformed named binding ref; use $%s.field or $index", s, allowedBindingPrimary(allowedNamedRefs))
	}
	if !allowedNamedRefs[m[1]] {
		return fmt.Errorf("ref %s: unknown map binding $%s (allowed: %s)", s, m[1], formatAllowedBindings(allowedNamedRefs))
	}
	return nil
}

func allowedBindingPrimary(allowed map[string]bool) string {
	for name := range allowed {
		if name != "index" {
			return name
		}
	}
	return "item"
}

func formatAllowedBindings(allowed map[string]bool) string {
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, "$"+name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func validateBatchParamsAgainstSchema(op string, params map[string]interface{}, allowedNamedRefs map[string]bool) error {
	spec, ok := batchOpCatalog[op]
	if !ok || spec.InputSchema == nil {
		return nil
	}
	for _, name := range schemaRequired(spec.InputSchema) {
		// nodeId(s) are supplied out-of-band; origin/status are TOP-LEVEL batch params
		// (presence labels) that never appear on an inner op, so an op catalog spec that
		// inherited their required-ness (when presence requires origin) must not enforce
		// them per-op — the batch carries them once at the top level.
		if name == "nodeId" || name == "nodeIds" || name == "origin" || name == "status" {
			continue
		}
		if params == nil || params[name] == nil {
			return fmt.Errorf("%s: missing required param %q", op, name)
		}
	}
	props, _ := spec.InputSchema["properties"].(map[string]any)
	for name, value := range params {
		prop, _ := props[name].(map[string]any)
		if len(prop) == 0 {
			continue
		}
		if err := validateBatchSchemaValue(op, name, value, prop, allowedNamedRefs); err != nil {
			return err
		}
	}
	return nil
}

func validateBatchOpSemantics(tool string, op map[string]interface{}, allowedNamedRefs map[string]bool) string {
	nodeIDs := semanticNodeIDsForBatch(op["nodeIds"], allowedNamedRefs)
	params, _ := op["params"].(map[string]interface{})
	return ValidateRPC(tool, nodeIDs, semanticParamsForBatch(tool, params, allowedNamedRefs))
}

func semanticNodeIDsForBatch(raw interface{}, allowedNamedRefs map[string]bool) []string {
	values, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		s, _ := v.(string)
		if isBatchRefLike(s) || isAllowedNamedBindingRef(s, allowedNamedRefs) {
			out = append(out, "1:1")
			continue
		}
		out = append(out, NormalizeNodeID(s))
	}
	return out
}

func semanticParamsForBatch(tool string, params map[string]interface{}, allowedNamedRefs map[string]bool) map[string]interface{} {
	if params == nil {
		return nil
	}
	out := make(map[string]interface{}, len(params))
	props := map[string]any{}
	if spec, ok := batchOpCatalog[tool]; ok && spec.InputSchema != nil {
		props, _ = spec.InputSchema["properties"].(map[string]any)
	}
	for k, v := range params {
		prop, _ := props[k].(map[string]any)
		out[k] = semanticParamValueForBatch(tool, k, v, prop, allowedNamedRefs)
	}
	return out
}

func semanticParamValueForBatch(tool, key string, value interface{}, prop map[string]any, allowedNamedRefs map[string]bool) interface{} {
	if s, ok := value.(string); ok && (isBatchRefLike(s) || isAllowedNamedBindingRef(s, allowedNamedRefs)) {
		return semanticPlaceholderForBatch(tool, key, prop)
	}
	if s, ok := value.(string); ok {
		switch key {
		case "nodeId", "parentId", "pageId", "componentId":
			return NormalizeNodeID(s)
		}
	}
	if key == "effects" {
		return semanticEffectsForBatch(value, allowedNamedRefs)
	}
	switch x := value.(type) {
	case []interface{}:
		items, _ := prop["items"].(map[string]any)
		out := make([]interface{}, 0, len(x))
		for _, child := range x {
			out = append(out, semanticParamValueForBatch(tool, key, child, items, allowedNamedRefs))
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for childKey, child := range x {
			out[childKey] = semanticParamValueForBatch(tool, childKey, child, nil, allowedNamedRefs)
		}
		return out
	default:
		return value
	}
}

func semanticEffectsForBatch(value interface{}, allowedNamedRefs map[string]bool) interface{} {
	effects, ok := value.([]interface{})
	if !ok {
		return value
	}
	out := make([]interface{}, 0, len(effects))
	for _, raw := range effects {
		effect, ok := raw.(map[string]interface{})
		if !ok {
			out = append(out, raw)
			continue
		}
		cloned := make(map[string]interface{}, len(effect))
		for k, child := range effect {
			if k == "type" {
				if s, ok := child.(string); ok && (isBatchRefLike(s) || isAllowedNamedBindingRef(s, allowedNamedRefs)) {
					cloned[k] = "DROP_SHADOW"
					continue
				}
			}
			cloned[k] = semanticParamValueForBatch("", k, child, nil, allowedNamedRefs)
		}
		out = append(out, cloned)
	}
	return out
}

func semanticPlaceholderForBatch(tool, key string, prop map[string]any) interface{} {
	switch key {
	case "nodeId", "parentId", "pageId", "componentId":
		return "1:1"
	case "key":
		return strings.Repeat("a", 40)
	case "assetType":
		return "COMPONENT"
	}
	if enum, ok := prop["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	switch tool + "." + key {
	case "create_variable.type":
		return "COLOR"
	case "set_fills.mode", "set_strokes.mode", "set_reactions.mode":
		return "replace"
	case "apply_style_to_node.target":
		return "fill"
	case "set_effects.effects":
		return []interface{}{}
	}
	typ, _ := prop["type"].(string)
	switch typ {
	case "number", "integer":
		return float64(1)
	case "boolean":
		return true
	case "array":
		return []interface{}{}
	case "object":
		return map[string]interface{}{}
	default:
		return "ref"
	}
}

func schemaRequired(schema map[string]any) []string {
	switch x := schema["required"].(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, v := range x {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func validateBatchSchemaValue(op, name string, value interface{}, prop map[string]any, allowedNamedRefs map[string]bool) error {
	// JSON null is a deliberate "clear / restore default" signal for optional params
	// (e.g. maxLines, minWidth/maxWidth, hyperlink) — the plugin handlers treat it as
	// such. Allow it through so the batch path matches top-level forwarding, which
	// forwards nil via presence checks rather than rejecting it.
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		if isBatchRefLike(s) || isAllowedNamedBindingRef(s, allowedNamedRefs) {
			return nil
		}
	}
	if alternatives := schemaAnyOf(prop); len(alternatives) > 0 {
		for _, alt := range alternatives {
			if err := validateBatchSchemaValue(op, name, value, alt, allowedNamedRefs); err == nil {
				return nil
			}
		}
		return fmt.Errorf("%s.%s must match one of the allowed schema shapes", op, name)
	}
	if enum, ok := prop["enum"].([]any); ok && len(enum) > 0 {
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s.%s must be a string enum value", op, name)
		}
		for _, allowed := range enum {
			if s == allowed {
				return nil
			}
		}
		return fmt.Errorf("%s.%s must be one of %s", op, name, formatEnum(enum))
	}

	typ, _ := prop["type"].(string)
	switch typ {
	case "", "any":
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s.%s must be a string", op, name)
		}
	case "number", "integer":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s.%s must be a number", op, name)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s.%s must be a boolean", op, name)
		}
	case "array":
		arr, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("%s.%s must be an array", op, name)
		}
		if items, _ := prop["items"].(map[string]any); len(items) > 0 {
			for i, item := range arr {
				if err := validateBatchSchemaValue(op, fmt.Sprintf("%s[%d]", name, i), item, items, allowedNamedRefs); err != nil {
					return err
				}
			}
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("%s.%s must be an object", op, name)
		}
	}
	return nil
}

func schemaAnyOf(prop map[string]any) []map[string]any {
	switch xs := prop["anyOf"].(type) {
	case []map[string]any:
		return append([]map[string]any(nil), xs...)
	case []any:
		out := make([]map[string]any, 0, len(xs))
		for _, x := range xs {
			if schema, ok := x.(map[string]any); ok {
				out = append(out, schema)
			}
		}
		return out
	default:
		return nil
	}
}

func formatEnum(values []any) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprint(v))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
