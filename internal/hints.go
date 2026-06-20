package internal

import (
	"regexp"
	"strings"
)

// unknownBatchOpRe extracts the op name from an "unknown op type \"X\"" error
// (emitted both by batch validation and ValidateRPC, both via %q).
var unknownBatchOpRe = regexp.MustCompile(`unknown op type "([^"]+)"`)

// hintFor returns an actionable, self-correction hint to append to a failed
// request's error, or "" when no hint applies. requestType is the plugin op
// (e.g. "get_node"); errText is the transport error or plugin-side message.
//
// The goal is that the LLM can recover WITHOUT human help — "retry smaller",
// "use REST instead", "the id is stale" — turning a dead-end error into a
// next step.
func hintFor(requestType, errText string) string {
	// A "unknown op type X" for an op THIS binary's catalog knows means the request
	// was forwarded to a leader running a DIFFERENT figma-mcp-express version (the
	// follower→leader RPC path executes at the leader, which validates against its
	// own, older catalog). The raw error tells the LLM to "call get_batch_op_spec" —
	// misleading, since the op is perfectly valid here. Name the real cause.
	if m := unknownBatchOpRe.FindStringSubmatch(errText); m != nil {
		if _, known := batchOpCatalog[m[1]]; known {
			return "op \"" + m[1] + "\" exists in THIS binary but the process holding port 1994 " +
				"(the leader that executes ops) rejected it as unknown — the leader is a different, " +
				"likely older figma-mcp-express version. Restart so the current binary leads, or align " +
				"all running instances to one version (compare `figma-mcp-express --version`)."
		}
	}

	e := strings.ToLower(errText)

	timedOut := strings.Contains(e, "timed out") || strings.Contains(e, "timeout") ||
		strings.Contains(e, "busy")
	notConnected := strings.Contains(e, "not connected")

	switch {
	case timedOut && requestType == "get_local_components":
		return "For a quick bounded result, pass get_local_components(pageId=…) to scan " +
			"one page. Omit pageId only when you need whole-file local-master recovery; " +
			"that path loads all pages and can be heavy."
	case timedOut && isHeavyRead(requestType):
		return "Request too large/slow. Retry narrower: target a specific nodeId " +
			"(not a page), add a `types` filter and a `limit`, and reduce depth. " +
			"Do not traverse a whole page."
	case notConnected && isCatalogRead(requestType):
		return "Open the Figma file and run the plugin (check list_channels). OR read " +
			"WITHOUT the plugin via REST: components/component_sets/styles are " +
			"available through fetch_library_catalog (needs fileKey + FIGMA_TOKEN). " +
			"Note: variables (design tokens) require a Figma Enterprise plan."
	case notConnected:
		return "Open the Figma file and run the plugin (check list_channels). If several " +
			"files are connected, pass the correct `channel`. (This op needs the live " +
			"canvas — no REST fallback.)"
	case strings.Contains(e, "send:"):
		return "Plugin socket dropped mid-request. Re-run the plugin in Figma, then retry."
	case strings.Contains(e, "not found"):
		return "The nodeId may be stale (cache miss). Re-confirm the real id with a " +
			"bounded get_metadata/search_nodes before mutating."
	default:
		return ""
	}
}

// isHeavyRead reports whether a read op can produce a large/slow payload that
// benefits from the "retry narrower" hint on timeout.
func isHeavyRead(requestType string) bool {
	switch requestType {
	case "get_node", "get_nodes_info", "get_document", "get_design_context":
		return true
	}
	return strings.HasPrefix(requestType, "scan_")
}

// isCatalogRead reports whether a read's data is library-catalog shaped, so the
// "fetch_library_catalog via REST" alternative is genuinely applicable.
func isCatalogRead(requestType string) bool {
	switch requestType {
	case "get_local_components", "get_styles", "fetch_library_catalog",
		"get_library_variables", "list_library_variable_collections":
		return true
	}
	return false
}

// hintForDesignContextDetail returns a just-in-time guidance hint for a
// successful get_design_context response when the requested detail level omits
// typography, color, and autoLayout data. Returns "" when no hint is needed
// (full, codegen, or unspecified detail levels already include that data).
func hintForDesignContextDetail(detail string) string {
	switch detail {
	case "minimal", "compact":
		return "typography/color/autoLayout omitted at this detail — " +
			"for code/style fidelity use detail:full or detail:codegen."
	}
	return ""
}
