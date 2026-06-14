package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// buildCatalogNDJSON projects the byKey map into a slim newline-delimited JSON
// index — one {key,name,type,nodeId} record per line, key-sorted for stable diffs.
// This is the line-oriented search surface that lets rg/grep/duckdb skip the
// full-document parse the pretty catalog used to force on every query.
func buildCatalogNDJSON(byKey map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		m, _ := byKey[k].(map[string]any)
		name, _ := m["name"].(string)
		typ, _ := m["type"].(string)
		nodeID, _ := m["nodeId"].(string)
		line, err := json.Marshal(map[string]any{"key": k, "name": name, "type": typ, "nodeId": nodeID})
		if err != nil {
			return nil, err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// catalogFetcher is a function type that fetches catalog data from a source.
// The returned map must have keys "components", "component_sets", "styles"
// (each holding []any of catalog items with "key", "name", "node_id" fields)
// and optionally "variables" / "variableCollections" (map[string]any, Enterprise only).
// Injecting this allows unit tests to avoid real HTTP calls.
type catalogFetcher func(ctx context.Context, fileKey, scope string) (map[string]any, error)

// httpCatalogFetcher is the production catalogFetcher implementation.
// It reads FIGMA_TOKEN from env and calls the Figma REST API.
func httpCatalogFetcher(ctx context.Context, fileKey, scope string) (map[string]any, error) {
	token := os.Getenv("FIGMA_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("FIGMA_TOKEN env not set — export a read-only Figma personal access token")
	}

	client := &http.Client{Timeout: 120 * time.Second}

	type endpoint struct {
		name string
		url  string
		key  string
	}

	// url.PathEscape ensures fileKey is safe to embed in a URL path (defense in depth).
	escapedKey := url.PathEscape(fileKey)
	endpoints := []endpoint{
		{"components", fmt.Sprintf("https://api.figma.com/v1/files/%s/components", escapedKey), "components"},
		{"component_sets", fmt.Sprintf("https://api.figma.com/v1/files/%s/component_sets", escapedKey), "component_sets"},
		{"styles", fmt.Sprintf("https://api.figma.com/v1/files/%s/styles", escapedKey), "styles"},
	}

	// Filter endpoints by scope.
	fetchVars := scope == "" || scope == "all" || scope == "variables"
	if scope != "" && scope != "all" {
		filtered := endpoints[:0]
		for _, ep := range endpoints {
			if ep.name == scope {
				filtered = append(filtered, ep)
			}
		}
		endpoints = filtered
	}

	// fetchEndpoint performs a single GET to one Figma REST endpoint and returns
	// the items array nested under wrapper.meta[name]. The response body is closed
	// before returning, so callers do not accumulate stacked defers.
	fetchEndpoint := func(ep endpoint) ([]any, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.url, nil)
		if err != nil {
			return nil, fmt.Errorf("new request (%s): %w", ep.name, err)
		}
		req.Header.Set("X-Figma-Token", token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", ep.name, err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body (%s): %w", ep.name, err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("figma API %s returned %d: %s", ep.name, resp.StatusCode, string(body))
		}

		// Figma returns {meta: {components|component_sets|styles: [...]}}
		var wrapper struct {
			Meta map[string]json.RawMessage `json:"meta"`
		}
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return nil, fmt.Errorf("unmarshal %s: %w", ep.name, err)
		}

		raw, ok := wrapper.Meta[ep.name]
		if !ok {
			return nil, nil
		}
		var items []any
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("unmarshal %s items: %w", ep.name, err)
		}
		return items, nil
	}

	// fetchVariables fetches /variables/local which has a different response shape:
	// {meta: {variables: {id: {...}}, variableCollections: {id: {...}}}}
	// Returns (variables, variableCollections, error).
	// A 403 typically means the Figma plan does not support the Variables REST API.
	fetchVariables := func() (map[string]any, map[string]any, error) {
		u := fmt.Sprintf("https://api.figma.com/v1/files/%s/variables/local", escapedKey)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("new request (variables): %w", err)
		}
		req.Header.Set("X-Figma-Token", token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch variables: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("read body (variables): %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("figma API variables returned %d: %s (Variables REST API requires Figma Enterprise plan)", resp.StatusCode, string(body))
		}

		var wrapper struct {
			Meta struct {
				Variables           map[string]any `json:"variables"`
				VariableCollections map[string]any `json:"variableCollections"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return nil, nil, fmt.Errorf("unmarshal variables: %w", err)
		}
		return wrapper.Meta.Variables, wrapper.Meta.VariableCollections, nil
	}

	result := map[string]any{
		"components":          []any{},
		"component_sets":      []any{},
		"styles":              []any{},
		"variables":           map[string]any{},
		"variableCollections": map[string]any{},
	}

	for _, ep := range endpoints {
		items, err := fetchEndpoint(ep)
		if err != nil {
			return nil, err
		}
		if items != nil {
			result[ep.key] = items
		}
	}

	if fetchVars {
		vars, colls, err := fetchVariables()
		if err != nil {
			// Non-fatal: surface the error in the result so callers can report it
			// without aborting the rest of the catalog (components/styles may be valid).
			result["variablesError"] = err.Error()
		} else {
			if vars != nil {
				result["variables"] = vars
			}
			if colls != nil {
				result["variableCollections"] = colls
			}
		}
	}

	return result, nil
}

// executeFetchCatalog is the inner handler that accepts an injected fetcher
// and explicit workDir for testability. The registered handler calls this with
// the production fetcher and os.Getwd().
func executeFetchCatalog(ctx context.Context, fetch catalogFetcher, fileKey, scope, outPath, workDir string) (*mcp.CallToolResult, error) {
	// Validate fileKey before touching the network — defense against URL injection.
	// fileKeyPattern is defined in schema.go and matches ^[A-Za-z0-9_-]+$.
	if !fileKeyPattern.MatchString(fileKey) {
		return mcp.NewToolResultError("fileKey must be alphanumeric (got invalid characters)"), nil
	}

	// Resolve and validate the output path (must be inside workDir).
	resolvedPath, err := resolveOutputPath(outPath, workDir)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Fetch catalog data.
	catalogData, err := fetch(ctx, fileKey, scope)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Assemble the full catalog object to write to disk.
	components, _ := catalogData["components"].([]any)
	componentSets, _ := catalogData["component_sets"].([]any)
	styles, _ := catalogData["styles"].([]any)
	variables, _ := catalogData["variables"].(map[string]any)
	variableCollections, _ := catalogData["variableCollections"].(map[string]any)
	variablesError, _ := catalogData["variablesError"].(string)

	if components == nil {
		components = []any{}
	}
	if componentSets == nil {
		componentSets = []any{}
	}
	if styles == nil {
		styles = []any{}
	}
	if variables == nil {
		variables = map[string]any{}
	}
	if variableCollections == nil {
		variableCollections = map[string]any{}
	}

	// Build byKey map: key -> {name, type, nodeId}
	byKey := make(map[string]any)

	addToByKey := func(items []any, assetType string) {
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key, _ := m["key"].(string)
			if key == "" {
				continue
			}
			name, _ := m["name"].(string)
			nodeID, _ := m["node_id"].(string)
			byKey[key] = map[string]any{
				"name":   name,
				"type":   assetType,
				"nodeId": nodeID,
			}
		}
	}
	addToByKey(components, "COMPONENT")
	addToByKey(componentSets, "COMPONENT_SET")
	addToByKey(styles, "STYLE")

	fullCatalog := map[string]any{
		"libraryFileKey":      fileKey,
		"fetchedAt":           time.Now().UTC().Format(time.RFC3339),
		"components":          components,
		"component_sets":      componentSets,
		"styles":              styles,
		"variables":           variables,
		"variableCollections": variableCollections,
		"byKey":               byKey,
	}
	if variablesError != "" {
		fullCatalog["variablesError"] = variablesError
	}

	// Write full catalog to disk — COMPACT (no indent). Pretty-printing inflated
	// these files ~2-3× (a real catalog hit 6.4 MB / 149K lines) and slowed every
	// jq/python re-parse; the byKey map is the structured search surface, so the
	// on-disk shape doesn't need to be human-pretty.
	catalogJSON, err := json.Marshal(fullCatalog)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal catalog: %v", err)), nil
	}
	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("mkdir: %v", err)), nil
	}
	// 0o600: owner read/write only — shared-account safety.
	if err := os.WriteFile(resolvedPath, catalogJSON, 0o600); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write catalog: %v", err)), nil
	}
	rememberLibraryCatalogKeys(byKey)

	// Slim NDJSON sidecar: one entity per line {key,name,type,nodeId}. This is the
	// HOT-PATH search surface — `rg`/`grep` over line-oriented records needs no
	// full-document parse (each hit is a complete record), and duckdb can
	// read_ndjson() it columnar. The full catalog above is only for full-record fetches.
	ndjsonPath := strings.TrimSuffix(resolvedPath, filepath.Ext(resolvedPath)) + ".ndjson"
	if nd, ndErr := buildCatalogNDJSON(byKey); ndErr == nil {
		_ = os.WriteFile(ndjsonPath, nd, 0o600) // best-effort: sidecar never fails the call
	} else {
		ndjsonPath = ""
	}

	// Build the small handle to return (first 3 component_sets, name+key only).
	maxSample := 3
	if len(componentSets) < maxSample {
		maxSample = len(componentSets)
	}
	sample := make([]map[string]any, 0, maxSample)
	for _, cs := range componentSets[:maxSample] {
		m, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{}
		if n, ok := m["name"].(string); ok {
			entry["name"] = n
		}
		if k, ok := m["key"].(string); ok {
			entry["key"] = k
		}
		sample = append(sample, entry)
	}

	handleData := map[string]any{
		"outPath":    resolvedPath,
		"ndjsonPath": ndjsonPath, // slim {key,name,type,nodeId} per line — grep/duckdb this for the hot path
		"counts": map[string]any{
			"components":          len(components),
			"componentSets":       len(componentSets),
			"styles":              len(styles),
			"variables":           len(variables),
			"variableCollections": len(variableCollections),
		},
		"sample": sample,
	}
	if variablesError != "" {
		handleData["variablesError"] = variablesError
	}
	handle, err := json.Marshal(handleData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal handle: %v", err)), nil
	}
	return mcp.NewToolResultText(string(handle)), nil
}
