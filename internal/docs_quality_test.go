package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestSkillFrontmatterProductionRules(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatalf("glob skills: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no skills found")
	}

	nameRE := regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	for _, path := range paths {
		path := path
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			body := readTestFile(t, path)
			fm := parseFrontmatter(t, body)

			name := strings.TrimSpace(fm["name"])
			if name == "" {
				t.Fatal("skill frontmatter missing name")
			}
			if !nameRE.MatchString(name) {
				t.Fatalf("skill name %q must use only letters, numbers, and hyphens", name)
			}

			desc := normalizeFrontmatterValue(fm["description"])
			if desc == "" {
				t.Fatal("skill frontmatter missing description")
			}
			if !strings.HasPrefix(desc, "Use when ") {
				t.Fatalf("description must start with %q, got %q", "Use when ", desc)
			}
			if len(desc) > 500 {
				t.Fatalf("description is %d chars, want <= 500", len(desc))
			}
			for _, processHint := range []string{"Load this skill", "Load proactively", "Before the first tool call"} {
				if strings.Contains(desc, processHint) {
					t.Fatalf("description should describe triggers only, not process hint %q: %q", processHint, desc)
				}
			}
		})
	}
}

func TestSkillEntryDocsStayTokenEfficient(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatalf("glob skills: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no skills found")
	}

	const maxWords = 500
	for _, path := range paths {
		body := readTestFile(t, path)
		if words := len(strings.Fields(body)); words > maxWords {
			t.Fatalf("%s has %d words, want <= %d; move details into references", path, words, maxWords)
		}
	}
}

func TestSkillReferencesStaySSOTFriendly(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "batch-recipes.md"))
	for _, forbidden := range []string{
		"Catalog-backed op examples",
		"### `boolean_operation`",
		"### `set_corner_radius`",
		"UNION` | `SUBTRACT`",
		"Demoted",
		"demoted op",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("batch-recipes.md must not mirror op-specific catalog specs; found %q", forbidden)
		}
	}
	for _, required := range []string{"search_batch_ops", "get_batch_op_spec", "batch(validateOnly:true)", "BatchOpCatalog"} {
		if !strings.Contains(body, required) {
			t.Fatalf("batch-recipes.md must route to the live catalog; missing %q", required)
		}
	}
}

func TestFigmaMCPExpressSkillKeepsProductionRules(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "SKILL.md"))
	for _, required := range []string{
		"core",
		"full",
		"search_batch_ops",
		"get_batch_op_spec",
		"batch(validateOnly:true)",
		"Do not write raw Plugin API JS",
		"Read wide-shallow, then targeted-deep",
		"After every write, validate structurally",
		"Build one logical section per batch",
		"channel is mandatory",
		"references/tool-selection.md",
		"references/batch-recipes.md",
		"BatchOpCatalog",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("figma-mcp-express SKILL.md missing production rule %q", required)
		}
	}
}

func TestBatchRecipesDocumentValidationAndErgonomics(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "batch-recipes.md"))
	for _, required := range []string{
		"Progressive discovery flow",
		"Hard rejects before plugin execution",
		"search_batch_ops",
		"get_batch_op_spec",
		"batch(validateOnly:true",
		"script-like keys anywhere",
		"stale aliases such as `characters`",
		"self/forward refs",
		"invalid `map.over`, `map.as`, or `map.do`",
		"named binding refs are only allowed inside `map.do`",
		"string interpolation like",
		"`map.as` must be an identifier and cannot be `index`",
		"Named binding projections such as `$item.children[*].id` are rejected",
		"`map.do` cannot be another `map`",
		"Only ONE `[*]` wildcard",
		"not one giant batch",
		"NOT transactional",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("batch-recipes.md missing production batch guidance %q", required)
		}
	}
	if strings.Contains(body, `"text": "Section $index"`) {
		t.Fatal("batch-recipes.md must not show string interpolation examples; named refs are whole-value only")
	}
}

func TestToolSelectionDocumentsParamKeySearch(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "tool-selection.md"))
	for _, required := range []string{"param key", "fontSize", "componentId"} {
		if !strings.Contains(body, required) {
			t.Fatalf("tool-selection.md must explain search_batch_ops param-key discovery; missing %q", required)
		}
	}
}

func TestDesignPatternReferencesDoNotTeachRawPluginAPIScripts(t *testing.T) {
	patternsDir := filepath.Join("..", "skills", "figma-design-patterns")
	paths, err := filepath.Glob(filepath.Join(patternsDir, "references", "*.md"))
	if err != nil {
		t.Fatalf("glob design-pattern references: %v", err)
	}
	paths = append(paths, filepath.Join(patternsDir, "SKILL.md"))

	forbiddenRE := regexp.MustCompile(`\bfigma\.|\bimportComponentByKeyAsync\b|\bcreateInstance\s*\(|\bappendChild\s*\(|\bsetProperties\s*\(|\bresetOverrides\s*\(|\bsetBoundVariable(?:ForPaint)?\s*\(`)
	for _, path := range paths {
		body := readTestFile(t, path)
		if match := forbiddenRE.FindString(body); match != "" {
			t.Fatalf("%s teaches raw Plugin API script syntax %q; use batch/FigmaPlan wording instead", path, match)
		}
	}
}

func TestPromptsDoNotTeachHiddenToolsAsTopLevelCalls(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "internal", "prompts", "*.go"))
	if err != nil {
		t.Fatalf("glob prompts: %v", err)
	}
	hiddenCallRE := hiddenToolCallRegexp(t)

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "prompts.go") {
			continue
		}
		body := readTestFile(t, path)
		if match := hiddenCallRE.FindString(body); match != "" {
			t.Fatalf("%s teaches hidden/core-unlisted tool as a top-level call: %q; use batch/FigmaPlan wording or core read tools", path, match)
		}
		if strings.Contains(body, "use_figma") || strings.Contains(body, "eval") || strings.Contains(body, "new Function") {
			t.Fatalf("%s must not imply raw Plugin API JS/script execution", path)
		}
	}
}

func TestDocsDoNotTeachHiddenToolsAsTopLevelCalls(t *testing.T) {
	docPaths := []string{
		filepath.Join("..", "README.md"),
		filepath.Join("..", "npm", "README.md"),
		filepath.Join("..", "DEV-SETUP.md"),
	}
	for _, pattern := range []string{
		filepath.Join("..", "skills", "*", "SKILL.md"),
		filepath.Join("..", "skills", "*", "references", "*.md"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		docPaths = append(docPaths, matches...)
	}

	hiddenCallRE := hiddenToolCallRegexp(t)
	for _, path := range docPaths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		body := readTestFile(t, path)
		if match := hiddenCallRE.FindString(body); match != "" {
			t.Fatalf("%s teaches hidden/core-unlisted tool as a top-level call: %q; route through batch/FigmaPlan or live spec discovery", path, match)
		}
	}
}

func TestPublicReadmesDocumentToolProfileAndSchemaMode(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "README.md"),
		filepath.Join("..", "npm", "README.md"),
	} {
		body := readTestFile(t, path)
		for _, required := range []string{
			"FIGMA_MCP_TOOL_PROFILE",
			"FIGMA_MCP_TOOL_SCHEMA_MODE",
			"core",
			"full",
			"compact",
			"verbose",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s must document %q for the production tool-surface contract", path, required)
			}
		}
	}
}

func TestNpmPackageDescriptionMatchesCoreProfilePositioning(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "npm", "package.json"),
		filepath.Join("..", "glama.json"),
	} {
		body := readTestFile(t, path)
		var pkg struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(body), &pkg); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if strings.Contains(pkg.Description, "70 tools") {
			t.Fatalf("%s description must not advertise stale 70-tool default surface: %q", path, pkg.Description)
		}
		for _, required := range []string{"compact", "batch"} {
			if !strings.Contains(strings.ToLower(pkg.Description), required) {
				t.Fatalf("%s description should mention %q positioning, got %q", path, required, pkg.Description)
			}
		}
	}
}

func TestGlamaToolListMatchesCoreSurface(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "glama.json"))
	var doc struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal glama.json: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range doc.Tools {
		got[tool.Name] = true
	}
	for name := range coreToolSurface {
		if !got[name] {
			t.Fatalf("glama.json missing default core tool %q", name)
		}
	}
	for name := range got {
		if !coreToolSurface[name] {
			t.Fatalf("glama.json lists non-core/default-hidden tool %q", name)
		}
	}
}

func TestDocsDoNotMislabelPluginLibraryOpsAsREST(t *testing.T) {
	var bodies []string
	for _, path := range []string{
		filepath.Join("..", "DEV-SETUP.md"),
		filepath.Join("..", "ARCHITECTURE.md"),
	} {
		bodies = append(bodies, readTestFile(t, path))
	}
	for _, forbidden := range []string{
		"REST-path tools (fetch_library_catalog, get_library_variables)",
		"REST catalog (3 new tools)",
		"REST catalog tools (3 new tools)",
		"3 new REST-path tools",
	} {
		for _, body := range bodies {
			if strings.Contains(body, forbidden) {
				t.Fatalf("docs mislabel plugin/team-library ops as REST: found %q", forbidden)
			}
		}
	}
}

func TestPromptsMatchCurrentSemanticContracts(t *testing.T) {
	promptsDir := filepath.Join("..", "internal", "prompts")

	annotation := readTestFile(t, filepath.Join(promptsDir, "annotation_conversion_strategy.go"))
	for _, forbidden := range []string{
		"converting manual annotations to Figma's native annotations",
		"Apply native Figma annotations",
		"After converting annotations",
	} {
		if strings.Contains(annotation, forbidden) {
			t.Fatalf("annotation prompt must stay read-only until an annotation write op exists; found %q", forbidden)
		}
	}

	reactions := readTestFile(t, filepath.Join(promptsDir, "reaction_to_connector_strategy.go"))
	for _, required := range []string{"actions[]", "destinationId", "destination-bearing actions"} {
		if !strings.Contains(reactions, required) {
			t.Fatalf("reaction prompt must describe the current plural actions[] response shape; missing %q", required)
		}
	}
	if strings.Contains(reactions, `"action": {`) || strings.Contains(reactions, "action.type") || strings.Contains(reactions, "action.destinationId") {
		t.Fatalf("reaction prompt must not describe the stale singular action response shape")
	}

	palette := readTestFile(t, filepath.Join(promptsDir, "generate_color_palette.go"))
	if strings.Contains(palette, "modeName \"Value\"") || strings.Contains(palette, "modeName \"Light\"") {
		t.Fatalf("create_variable_collection prompt guidance must use initialModeName, not modeName")
	}
	if !strings.Contains(palette, "initialModeName") {
		t.Fatalf("generate_color_palette prompt must mention initialModeName for create_variable_collection")
	}

	variants := readTestFile(t, filepath.Join(promptsDir, "generate_component_variants.go"))
	if strings.Contains(variants, "font size cannot be changed via MCP") {
		t.Fatalf("component variants prompt must not deny set_text fontSize support")
	}
	if !strings.Contains(variants, "fontSize") {
		t.Fatalf("component variants prompt should mention set_text fontSize support")
	}

	tokens := readTestFile(t, filepath.Join(promptsDir, "design_token_generation_strategy.go"))
	for _, forbidden := range []string{
		`get_design_context(detail="compact") to scan the full node tree`,
		"create_variable_collection → create_variable with type",
		"lineHeight, letterSpacing",
	} {
		if strings.Contains(tokens, forbidden) {
			t.Fatalf("design token prompt must not teach stale schema/discovery guidance %q", forbidden)
		}
	}
	for _, required := range []string{"search_batch_ops", "get_batch_op_spec", "batch(validateOnly:true)", "Create each variable collection once"} {
		if !strings.Contains(tokens, required) {
			t.Fatalf("design token prompt must route writes through catalog discovery; missing %q", required)
		}
	}

	design := readTestFile(t, filepath.Join(promptsDir, "design_strategy.go"))
	if strings.Contains(design, "Use fillColor for backgrounds") {
		t.Fatal("design strategy prompt must not teach fillColor as the background fill mutator")
	}
	if !strings.Contains(design, "batch op set_fills for backgrounds") {
		t.Fatal("design strategy prompt must route background fills through set_fills batch ops")
	}
}

func hiddenToolCallRegexp(t *testing.T) *regexp.Regexp {
	t.Helper()
	var hidden []string
	for name := range batchOpCatalog {
		if name != "map" && !coreToolSurface[name] {
			hidden = append(hidden, regexp.QuoteMeta(name))
		}
	}
	hidden = append(hidden, "get_screenshot", "get_document")
	sort.Strings(hidden)
	return regexp.MustCompile(`\b(` + strings.Join(hidden, "|") + `)\s*\(`)
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func parseFrontmatter(t *testing.T, body string) map[string]string {
	t.Helper()
	lines := strings.Split(body, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatal("skill must start with YAML frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		t.Fatal("skill frontmatter missing closing ---")
	}

	out := map[string]string{}
	for i := 1; i < end; i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  ") || strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == ">-" || value == "|" || value == "|-" {
			var parts []string
			for j := i + 1; j < end; j++ {
				if !strings.HasPrefix(lines[j], "  ") && strings.Contains(lines[j], ":") {
					break
				}
				parts = append(parts, strings.TrimSpace(lines[j]))
				i = j
			}
			value = strings.Join(parts, " ")
		}
		out[key] = strings.Trim(value, `"`)
	}
	return out
}

func normalizeFrontmatterValue(v string) string {
	return strings.Join(strings.Fields(strings.Trim(v, `"`)), " ")
}
