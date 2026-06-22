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

			for key := range fm {
				if key != "name" && key != "description" {
					t.Fatalf("skill frontmatter must contain only name and description; got extra field %q", key)
				}
			}

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
			for _, forbiddenHeading := range []string{"# When to Use", "# When to use", "## When to Use", "## When to use"} {
				if strings.Contains(body, forbiddenHeading) {
					t.Fatalf("skill trigger guidance belongs in description, not body heading %q", forbiddenHeading)
				}
			}
		})
	}
}

func TestSkillFoldersStayLeanAndDiscoverable(t *testing.T) {
	skillDirs, err := filepath.Glob(filepath.Join("..", "skills", "*"))
	if err != nil {
		t.Fatalf("glob skill dirs: %v", err)
	}
	if len(skillDirs) == 0 {
		t.Fatal("no skills found")
	}

	forbiddenDocNames := map[string]bool{
		"CHANGELOG.md":          true,
		"INSTALLATION_GUIDE.md": true,
		"QUICK_REFERENCE.md":    true,
		"README.md":             true,
	}

	for _, skillDir := range skillDirs {
		skillDir := skillDir
		t.Run(filepath.Base(skillDir), func(t *testing.T) {
			entries, err := os.ReadDir(skillDir)
			if err != nil {
				t.Fatalf("read skill dir: %v", err)
			}

			for _, entry := range entries {
				if forbiddenDocNames[entry.Name()] {
					t.Fatalf("skill folders should not contain auxiliary docs such as %s; keep guidance in SKILL.md or references/", entry.Name())
				}
				if !entry.IsDir() {
					continue
				}
				switch entry.Name() {
				case "agents", "assets", "references", "scripts":
				default:
					t.Fatalf("unexpected skill top-level directory %q; keep resources in agents/, assets/, references/, or scripts/", entry.Name())
				}
			}

			skillBody := readTestFile(t, filepath.Join(skillDir, "SKILL.md"))
			refs, err := filepath.Glob(filepath.Join(skillDir, "references", "*.md"))
			if err != nil {
				t.Fatalf("glob references: %v", err)
			}
			for _, ref := range refs {
				rel := filepath.ToSlash(filepath.Join("references", filepath.Base(ref)))
				if !strings.Contains(skillBody, rel) {
					t.Fatalf("reference %s must be directly linked from SKILL.md for progressive disclosure", rel)
				}
			}

			nestedRefs, err := filepath.Glob(filepath.Join(skillDir, "references", "*", "*.md"))
			if err != nil {
				t.Fatalf("glob nested references: %v", err)
			}
			if len(nestedRefs) > 0 {
				t.Fatalf("references should stay one level deep from SKILL.md; found nested reference %s", nestedRefs[0])
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

func TestLongSkillReferencesHaveQuickNavigation(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "skills", "*", "references", "*.md"))
	if err != nil {
		t.Fatalf("glob skill references: %v", err)
	}

	for _, path := range paths {
		body := readTestFile(t, path)
		lines := strings.Split(body, "\n")
		if len(lines) <= 100 {
			continue
		}
		n := 25
		if len(lines) < n {
			n = len(lines)
		}
		head := strings.ToLower(strings.Join(lines[:n], "\n"))
		if !strings.Contains(head, "quick navigation") && !strings.Contains(head, "table of contents") {
			t.Fatalf("%s has %d lines; long skill references must expose quick navigation near the top", path, len(lines))
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

func TestPrototypeSkillIsCompanionDomainLayer(t *testing.T) {
	prototype := readTestFile(t, filepath.Join("..", "skills", "figma-prototype", "SKILL.md"))
	for _, required := range []string{
		"Companion domain skill for figma-mcp-express prototype work",
		"does not redefine MCP execution",
		"use `figma-mcp-express` for tool discovery, batch syntax, origin/channel",
		"load the `figma-mcp-express` skill first",
	} {
		if !strings.Contains(prototype, required) {
			t.Fatalf("figma-prototype SKILL.md must present itself as a companion domain layer; missing %q", required)
		}
	}

	express := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "SKILL.md"))
	if !strings.Contains(express, "Load companion skill `figma-prototype`; keep MCP execution discipline here") {
		t.Fatal("figma-mcp-express SKILL.md must route prototype domain work to figma-prototype without moving MCP execution rules")
	}
}

func TestMultiAgentSkillDocumentsOriginRosterAndOrchestrator(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "multi-agent.md")) +
		"\n" + readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "presence.md"))
	for _, required := range []string{
		"`wolfgang`, `grace`, `theo`, `sunho`, `zoe`, `taewon`, `emma`, `alex`, `rick`",
		"not a free-form string",
		"sessionId+origin",
		"orchestrator's own origin is `wolfgang`",
		"Do not use `sunho` for the orchestrator",
		"Do not reuse one `origin` across concurrent agents",
		"Agent 3 -> owns frame C -> origin: \"zoe\"",
		"status is optional in the schema, not optional in the workflow",
		"Do not skip `set_presence` because `status` is optional",
		"Actively call `set_presence` at dispatch and workflow transitions",
		"Pass `origin` on every `batch` call",
		"batch(channel:\"auto-2\", origin:\"theo\", ops:[create_frame...])",
		"Use exactly the origin assigned to you",
		"Do not pick a random roster enum",
		"`origin` works on plugin reads, writes, and batch",
		"`fetch_library_catalog`",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("multi-agent/presence refs missing origin roster/orchestrator rule %q", required)
		}
	}
	if strings.Contains(body, "Agent 3 → owns frame C → origin: \"sunho\"") {
		t.Fatal("multi-agent/presence refs must not model the orchestrator/third worker as sunho; use wolfgang for orchestrator and distinct worker origins")
	}
	if strings.Contains(body, "batch(channel:\"auto-2\", ops:[create_frame") {
		t.Fatal("multi-agent/presence refs must not show batch examples without outer origin")
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
		"batch(channel:",
		"Never put `channel` inside `ops[*].params`",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("batch-recipes.md missing production batch guidance %q", required)
		}
	}
	if strings.Contains(body, `"text": "Section $index"`) {
		t.Fatal("batch-recipes.md must not show string interpolation examples; named refs are whole-value only")
	}
}

func TestToolsDocDoesNotExposeChannelInsideCatalogBackedBatchOps(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "TOOLS.md"))
	currentHeading := ""
	inBatchOp := false
	for i, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "### ") {
			currentHeading = line
			inBatchOp = strings.Contains(line, "[BATCH OP]")
			continue
		}
		if inBatchOp && strings.HasPrefix(line, "| channel ") {
			t.Fatalf("TOOLS.md line %d: %s must not document per-op channel; use outer batch channel only", i+1, currentHeading)
		}
	}
	if !strings.Contains(body, "Pass `channel` on the outer `batch` call") {
		t.Fatal("TOOLS.md must document outer batch channel routing")
	}
}

func TestDocsTrackAPIGapCoverageSurface(t *testing.T) {
	tools := readTestFile(t, filepath.Join("..", "TOOLS.md"))
	for _, heading := range []string{
		"### create_line",
		"### create_polygon",
		"### create_star",
		"### import_svg",
		"### create_table",
		"### set_text_range",
		"### update_variable",
		"### update_variable_collection",
		"### set_constraints",
		"### get_image_by_hash",
		"### get_file_thumbnail",
		"### get_dev_resources",
		"### resolve_variable_for_consumer",
		"### get_selection_colors",
		"### create_video",
		"### create_gif",
		"### create_link_preview",
		"### create_vector",
		"### create_slice",
		"### create_page_divider",
		"### create_text_path",
		"### set_file_thumbnail",
		"### add_dev_resource",
		"### edit_dev_resource",
		"### delete_dev_resource",
		"### reorder_local_style",
		"### reorder_local_style_folder",
		"### create_variable_alias",
		"### bind_variable_to_effect",
		"### bind_variable_to_layout_grid",
	} {
		if !strings.Contains(tools, heading) {
			t.Fatalf("TOOLS.md must document new API-gap surface %q", heading)
		}
	}
	if strings.Contains(tools, "### set_constraints [BATCH OP]") {
		t.Fatal("TOOLS.md must document promoted set_constraints as a top-level tool, not batch-only")
	}

	gotchas := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "gotchas.md"))
	for _, forbidden := range []string{
		"NO `import_svg`",
		"create_vector_from_svg",
		"Plugin runtime required",
		"use_figma",
	} {
		if strings.Contains(gotchas, forbidden) {
			t.Fatalf("gotchas.md must not describe SVG import as missing now that import_svg exists; found %q", forbidden)
		}
	}
	for _, required := range []string{"SVG", "`import_svg`", "batch"} {
		if !strings.Contains(gotchas, required) {
			t.Fatalf("gotchas.md must document SVG ingestion through import_svg; missing %q", required)
		}
	}
}

func TestToolsDocTracksNativeEffectsSurface(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "TOOLS.md"))
	for _, required := range []string{
		"### set_effects",
		"### create_effect_style",
		"GLASS",
		"NOISE",
		"TEXTURE",
		"PROGRESSIVE",
		"noiseSizeVector",
		"startOffset",
		"endOffset",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("TOOLS.md must document native/progressive effect surface; missing %q", required)
		}
	}
}

func TestSkillDocsDoNotReferenceRemovedWorkflowSections(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "multi-agent.md"))
	if strings.Contains(body, "SKILL.md § Workflow") {
		t.Fatal("multi-agent.md must not point readers at removed SKILL.md Workflow sections")
	}
	if !strings.Contains(body, "Reference Router") {
		t.Fatal("multi-agent.md should point readers at the current SKILL.md Reference Router")
	}
}

func TestNpmReadmeHasSingleLimitationsSection(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "npm", "README.md"))
	count := strings.Count(body, "\n## Limitations\n") + strings.Count(body, "\n## Known limitations\n")
	if count != 1 {
		t.Fatalf("npm/README.md must have exactly one limitations section, got %d", count)
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

func TestDocsDistinguishGenericFloatingFromScrollPinning(t *testing.T) {
	autoLayout := readTestFile(t, filepath.Join("..", "skills", "figma-design-patterns", "references", "auto-layout.md"))
	for _, required := range []string{
		"Floating children inside auto-layout",
		"`resize_nodes`",
		"`layoutPositioning:\"ABSOLUTE\"`",
		"Do **not** use `pin_child`",
		"x/y cannot bind to variables",
		"do not promise token-bound x/y",
	} {
		if !strings.Contains(autoLayout, required) {
			t.Fatalf("auto-layout.md must distinguish generic floating from scroll pinning; missing %q", required)
		}
	}

	constraints := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "platform-constraints.md"))
	for _, required := range []string{
		"Existing floating child needs ABSOLUTE",
		"`set_auto_layout` with `layoutPositioning`",
		"batch/FigmaPlan `resize_nodes` op on the child with `layoutPositioning:\"ABSOLUTE\"`",
		"Float glass tab bar or decorative blob",
	} {
		if !strings.Contains(constraints, required) {
			t.Fatalf("platform-constraints.md must route existing absolute children to resize_nodes; missing %q", required)
		}
	}

	scroll := readTestFile(t, filepath.Join("..", "skills", "figma-prototype", "references", "prototype-scroll.md"))
	for _, required := range []string{
		"`pin_child` is only for prototype scroll-fixed children",
		"not the generic way to",
		"`layoutPositioning:\"ABSOLUTE\"`",
	} {
		if !strings.Contains(scroll, required) {
			t.Fatalf("prototype-scroll.md must narrow pin_child to scroll-fixed use; missing %q", required)
		}
	}
}

func TestDesignSkillRequiresProductionFigmaFeatures(t *testing.T) {
	skill := readTestFile(t, filepath.Join("..", "skills", "figma-design-patterns", "SKILL.md"))
	for _, required := range []string{
		"Figma-native production features",
		"variables/modes",
		"components",
		"variants/properties",
		"auto layout/grid",
		"prototypes",
		"annotations/dev resources",
		"Visual lookalikes",
	} {
		if !strings.Contains(skill, required) {
			t.Fatalf("figma-design-patterns SKILL.md must require production Figma feature use; missing %q", required)
		}
	}

	handoff := readTestFile(t, filepath.Join("..", "skills", "figma-design-patterns", "references", "handoff-checklist.md"))
	for _, required := range []string{
		"Figma-native production features",
		"native features, not visual lookalikes",
		"variables, modes, text/paint/effect styles",
		"component variant/property",
		"annotations/dev resources",
	} {
		if !strings.Contains(handoff, required) {
			t.Fatalf("handoff-checklist.md must gate production Figma feature use; missing %q", required)
		}
	}

	toolSelection := readTestFile(t, filepath.Join("..", "skills", "figma-mcp-express", "references", "tool-selection.md"))
	for _, required := range []string{
		"Figma-native production features",
		"auto layout/grid/constraints",
		"variables/modes",
		"component variants/properties",
		"annotations",
		"dev resources",
	} {
		if !strings.Contains(toolSelection, required) {
			t.Fatalf("tool-selection.md must nudge production Figma feature use; missing %q", required)
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

func TestChangeLogRecordsMeasuredToolListTokenSavings(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "CHANGELOG.md"))
	for _, required := range []string{
		"vkhanhqui/figma-mcp-go@fe6cd768",
		"73 tools",
		"51,125 bytes",
		"12,214 tokens",
		"8,931 tokens",
		"73.1%",
		"36,552 bytes",
		"71.5%",
		"v1.0.3",
		"84.2%",
		"70 tools",
		"90,038 bytes",
		"20,822 tokens",
		"21 tools",
		"3,283",
		"o200k_base",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("CHANGELOG.md must document measured tools/list savings; missing %q", required)
		}
	}
}

func TestPublicReadmesDescribeUpstreamToolSurfaceBaselineWithoutReleaseMetrics(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "README.md"),
		filepath.Join("..", "npm", "README.md"),
	} {
		body := readTestFile(t, path)
		for _, required := range []string{
			"vkhanhqui/figma-mcp-go@fe6cd768",
			"73 tools",
			"12,214",
			"21 tools",
			"3,283",
			"73.1%",
			"o200k_base",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s must describe the measured upstream tool-surface baseline; missing %q", path, required)
			}
		}
		for _, forbidden := range []string{
			"20,822 tokens",
			"84.2%",
			"v1.0.3's 70 tools",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s should not expose release-specific token metrics in README prose; found %q", path, forbidden)
			}
		}
	}
}

func TestDevSetupDocumentsBatchSafetyCaps(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "DEV-SETUP.md"))
	for _, required := range []string{
		"FIGMA_MCP_BATCH_MAX_OPS",
		"FIGMA_MCP_BATCH_MAX_BYTES",
		"fail-fast rejection",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("DEV-SETUP.md must document batch safety cap %q", required)
		}
	}
}

func TestToolsDocDocumentsCoreSurfaceContract(t *testing.T) {
	body := readTestFile(t, filepath.Join("..", "TOOLS.md"))
	for _, required := range []string{
		"FIGMA_MCP_TOOL_PROFILE=core",
		"compact 22-tool MCP surface",
		"`set_presence`",
		"batch",
		"FigmaPlan",
		"search_batch_ops",
		"get_batch_op_spec",
		"batch(validateOnly:true)",
		"FIGMA_MCP_TOOL_PROFILE=full",
		"full top-level compatibility surface",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("TOOLS.md must document production tool-surface contract; missing %q", required)
		}
	}
	if strings.Contains(body, "| status ") {
		t.Fatal("TOOLS.md must not document status as a batch param; use set_presence")
	}
	if !strings.Contains(body, "Manual `status` and `task` go through `set_presence`, not `batch`") {
		t.Fatal("TOOLS.md must route manual status/task through set_presence")
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

func TestPublishedManifestVersionsStayInSync(t *testing.T) {
	paths := []string{
		filepath.Join("..", "npm", "package.json"),
		filepath.Join("..", ".claude-plugin", "plugin.json"),
		filepath.Join("..", ".codex-plugin", "plugin.json"),
	}
	versions := map[string]string{}
	for _, path := range paths {
		body := readTestFile(t, path)
		var manifest struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(body), &manifest); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if manifest.Version == "" {
			t.Fatalf("%s missing version", path)
		}
		versions[path] = manifest.Version
	}
	var want string
	for _, path := range paths {
		if want == "" {
			want = versions[path]
			continue
		}
		if versions[path] != want {
			t.Fatalf("manifest version mismatch: %s=%s, want %s (all manifests must match)", path, versions[path], want)
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
	skillRefsDir := filepath.Join("..", "skills", "figma-mcp-express", "references")

	// ── Generator prompts (still registered as MCP prompts) ──────────────────

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

	// ── Folded guidance — contracts now verified in skill reference files ─────
	// (annotation_conversion_strategy, reaction_to_connector_strategy, and
	// design_strategy were deprecated; their content lives in skill references.)

	workflowRecipes := readTestFile(t, filepath.Join(skillRefsDir, "workflow-recipes.md"))
	prototypeAudit := readTestFile(t, filepath.Join("..", "skills", "figma-prototype", "references", "prototype-audit.md"))

	// Annotation recipe must stay read-only: no annotation-write language.
	for _, forbidden := range []string{
		"converting manual annotations to Figma's native annotations",
		"Apply native Figma annotations",
		"After converting annotations",
	} {
		if strings.Contains(workflowRecipes, forbidden) {
			t.Fatalf("annotation recipe must stay read-only until an annotation write op exists; found %q", forbidden)
		}
	}

	if strings.Contains(workflowRecipes, "Reaction flow map") {
		t.Fatal("prototype reaction maps belong in figma-prototype/references/prototype-audit.md, not figma-mcp-express workflow-recipes.md")
	}
	if !strings.Contains(prototypeAudit, "Reaction flow map") {
		t.Fatal("prototype-audit.md must own the reaction flow map recipe")
	}

	// Reaction recipe must describe the current plural actions[] response shape.
	for _, required := range []string{"actions[]", "destinationId", "destination-bearing actions"} {
		if !strings.Contains(prototypeAudit, required) {
			t.Fatalf("reaction recipe must describe the current plural actions[] response shape; missing %q", required)
		}
	}
	if strings.Contains(prototypeAudit, `"action": {`) || strings.Contains(prototypeAudit, "action.type") || strings.Contains(prototypeAudit, "action.destinationId") {
		t.Fatalf("reaction recipe must not describe the stale singular action response shape")
	}

	toolSelection := readTestFile(t, filepath.Join(skillRefsDir, "tool-selection.md"))

	// Design structure guidance must route background fills through set_fills.
	if strings.Contains(toolSelection, "Use fillColor for backgrounds") {
		t.Fatal("tool-selection reference must not teach fillColor as the background fill mutator")
	}
	if !strings.Contains(toolSelection, "batch op set_fills for backgrounds") {
		t.Fatal("tool-selection reference must route background fills through set_fills batch ops")
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
