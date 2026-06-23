import { describe, it, expect, beforeEach } from "bun:test";
import { handleReadStyleRequest } from "./read-styles";

// ── Figma global mock ─────────────────────────────────────────────────────────

const makeRequest = (type: string, params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: [],
  params: params ?? {},
});

beforeEach(() => {
  (globalThis as any).figma = {
    variables: {
      getLocalVariableCollectionsAsync: async () => [],
      getVariableByIdAsync: async () => null,
    },
    getLocalPaintStylesAsync: async () => [],
    getLocalTextStylesAsync: async () => [],
    getLocalEffectStylesAsync: async () => [],
    getLocalGridStylesAsync: async () => [],
    getStyleByIdAsync: async () => null,
  };
});

// ── export_tokens ─────────────────────────────────────────────────────────────

describe("export_tokens", () => {
  it("returns empty token object when there are no variables or styles", async () => {
    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "json" }));
    expect(res?.data.tokens).toBeDefined();
    expect(typeof res?.data.tokens).toBe("object");
    expect(Object.keys(res?.data.tokens)).toHaveLength(0);
  });

  it("returns :root CSS block even when empty", async () => {
    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "css" }));
    expect(res?.data.css).toBeDefined();
    expect(res?.data.css).toContain(":root {");
    expect(res?.data.css).toContain("}");
  });

  it("defaults to JSON when format is not specified", async () => {
    const res = await handleReadStyleRequest(makeRequest("export_tokens", {}));
    expect(res?.data.tokens).toBeDefined();
  });

  it("builds nested JSON token tree from variable names using / as separator", async () => {
    (globalThis as any).figma.variables = {
      getLocalVariableCollectionsAsync: async () => [
        {
          id: "col:1",
          name: "Brand",
          modes: [{ modeId: "m1", name: "Default" }],
          variableIds: ["var:1"],
        },
      ],
      getVariableByIdAsync: async (id: string) =>
        id === "var:1"
          ? {
              id: "var:1",
              name: "Primary/Blue",
              resolvedType: "COLOR",
              valuesByMode: { m1: { r: 0, g: 0.47, b: 1, a: 1 } },
            }
          : null,
    };

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "json" }));
    expect(res?.data.tokens["Brand"]).toBeDefined();
    expect(res?.data.tokens["Brand"]["Primary"]["Blue"]).toBeDefined();
    expect(res?.data.tokens["Brand"]["Primary"]["Blue"].type).toBe("COLOR");
    expect(res?.data.tokens["Brand"]["Primary"]["Blue"].value["Default"]).toBeDefined();
  });

  it("emits per-mode values in JSON output", async () => {
    (globalThis as any).figma.variables = {
      getLocalVariableCollectionsAsync: async () => [
        {
          id: "col:1",
          name: "Spacing",
          modes: [
            { modeId: "m1", name: "Default" },
            { modeId: "m2", name: "Dense" },
          ],
          variableIds: ["var:1"],
        },
      ],
      getVariableByIdAsync: async (id: string) =>
        id === "var:1"
          ? {
              id: "var:1",
              name: "base",
              resolvedType: "FLOAT",
              valuesByMode: { m1: 8, m2: 4 },
            }
          : null,
    };

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "json" }));
    const token = res?.data.tokens["Spacing"]["base"];
    expect(token.value["Default"]).toBe(8);
    expect(token.value["Dense"]).toBe(4);
  });

  it("emits CSS custom property with kebab-case name from / separator", async () => {
    (globalThis as any).figma.variables = {
      getLocalVariableCollectionsAsync: async () => [
        {
          id: "col:1",
          name: "Spacing",
          modes: [{ modeId: "m1", name: "Default" }],
          variableIds: ["var:1"],
        },
      ],
      getVariableByIdAsync: async (id: string) =>
        id === "var:1"
          ? {
              id: "var:1",
              name: "spacing/base",
              resolvedType: "FLOAT",
              valuesByMode: { m1: 8 },
            }
          : null,
    };

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "css" }));
    expect(res?.data.css).toContain("--spacing-base: 8;");
  });

  it("emits rgba() for COLOR variables with alpha < 1", async () => {
    (globalThis as any).figma.variables = {
      getLocalVariableCollectionsAsync: async () => [
        {
          id: "col:1",
          name: "Brand",
          modes: [{ modeId: "m1", name: "Default" }],
          variableIds: ["var:1"],
        },
      ],
      getVariableByIdAsync: async (id: string) =>
        id === "var:1"
          ? {
              id: "var:1",
              name: "overlay",
              resolvedType: "COLOR",
              valuesByMode: { m1: { r: 0, g: 0, b: 0, a: 0.5 } },
            }
          : null,
    };

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "css" }));
    expect(res?.data.css).toContain("rgba(0, 0, 0, 0.50)");
  });

  it("includes solid paint styles under _styles.paint in JSON", async () => {
    (globalThis as any).figma.getLocalPaintStylesAsync = async () => [
      {
        id: "s:1",
        name: "Neutral/Gray",
        paints: [{ type: "SOLID", color: { r: 0.5, g: 0.5, b: 0.5 }, opacity: 1 }],
      },
    ];

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "json" }));
    expect(res?.data.tokens["_styles"]).toBeDefined();
    const gray = res?.data.tokens["_styles"].paint["Neutral"]["Gray"];
    expect(gray.type).toBe("COLOR");
    expect(gray.value).toMatch(/^#[0-9a-f]{6}$/);
  });

  it("includes solid paint styles as CSS custom properties", async () => {
    (globalThis as any).figma.getLocalPaintStylesAsync = async () => [
      {
        id: "s:1",
        name: "brand/primary",
        paints: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }],
      },
    ];

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "css" }));
    expect(res?.data.css).toContain("--brand-primary:");
  });

  it("skips non-solid paint styles", async () => {
    (globalThis as any).figma.getLocalPaintStylesAsync = async () => [
      {
        id: "s:2",
        name: "Gradient/Blue",
        paints: [{ type: "GRADIENT_LINEAR" }],
      },
    ];

    const res = await handleReadStyleRequest(makeRequest("export_tokens", { format: "json" }));
    // No _styles key since nothing was added
    expect(res?.data.tokens["_styles"]).toBeUndefined();
  });
});

// ── get_local_components ──────────────────────────────────────────────────────

// Helpers to build page/node fixtures matching the Figma plugin API shape
const makeComponent = (id: string, name: string, key: string, parentId?: string) => ({
  type: "COMPONENT" as const,
  id,
  name,
  key,
  componentSetId: parentId ?? null,
  variantProperties: parentId ? { Variant: "Default" } : null,
  parent: parentId ? { type: "COMPONENT_SET", id: parentId } : null,
});

const makeComponentSet = (id: string, name: string, key: string, defaultVariantKey?: string) => ({
  type: "COMPONENT_SET" as const,
  id,
  name,
  key,
  parent: null,
  // Figma plugin API: defaultVariant is the default component within the set
  ...(defaultVariantKey ? { defaultVariant: { key: defaultVariantKey } } : {}),
});

const makePage = (id: string, name: string, nodes: any[]) => ({
  id,
  name,
  type: "PAGE" as const,
  parent: { type: "DOCUMENT", id: "doc:1" },
  loadAsync: async () => {},
  findAllWithCriteria: ({ types }: { types: string[] }) =>
    nodes.filter((n) => types.includes(n.type)),
});

// Two-page fixture: page A has a component set + variant, page B has a standalone component
const pageA = makePage("page:A", "Icons", [
  makeComponentSet("set:1", "Button", "key-set-1", "key-cmp-1-default"),
  makeComponent("cmp:1", "Button/Primary", "key-cmp-1", "set:1"),
]);

const pageB = makePage("page:B", "Forms", [
  makeComponent("cmp:2", "Input", "key-cmp-2"),
]);

const setupTwoPageFigma = () => {
  (globalThis as any).figma = {
    ...((globalThis as any).figma ?? {}),
    variables: {
      getLocalVariableCollectionsAsync: async () => [],
      getVariableByIdAsync: async () => null,
    },
    getLocalPaintStylesAsync: async () => [],
    getLocalTextStylesAsync: async () => [],
    getLocalEffectStylesAsync: async () => [],
    getLocalGridStylesAsync: async () => [],
    getStyleByIdAsync: async () => null,
    root: { children: [pageA, pageB] },
    getNodeByIdAsync: async (id: string) => {
      if (id === "page:A") return pageA;
      if (id === "page:B") return pageB;
      return null;
    },
    ui: { postMessage: () => {} },
  };
};

describe("get_local_components", () => {
  beforeEach(setupTwoPageFigma);

  it("without pageId: scans all pages and returns components from every page", async () => {
    const res = await handleReadStyleRequest(makeRequest("get_local_components"));
    expect(res?.data).toBeDefined();
    const { count, components, componentSets } = res!.data;

    // Both pages' components present
    const ids = components.map((c: any) => c.id);
    expect(ids).toContain("cmp:1");
    expect(ids).toContain("cmp:2");

    // Component set from page A present
    const setIds = componentSets.map((s: any) => s.id);
    expect(setIds).toContain("set:1");

    // Total count reflects all components (not sets)
    expect(count).toBe(2);

    // No pageId field in whole-file branch
    expect(res!.data.pageId).toBeUndefined();
  });

  it("without pageId: component has correct shape (id, name, key, componentSetId, variantProperties)", async () => {
    const res = await handleReadStyleRequest(makeRequest("get_local_components"));
    const variant = res!.data.components.find((c: any) => c.id === "cmp:1");
    expect(variant).toBeDefined();
    expect(variant.key).toBe("key-cmp-1");
    expect(variant.componentSetId).toBe("set:1");
    expect(variant.variantProperties).toEqual({ Variant: "Default" });
  });

  it("with valid pageId: returns ONLY that page's components and excludes other pages", async () => {
    const res = await handleReadStyleRequest(
      makeRequest("get_local_components", { pageId: "page:A" }),
    );
    expect(res?.data).toBeDefined();
    const { count, components, componentSets, pageId, pageName } = res!.data;

    // Only page A's component (cmp:1) included
    const ids = components.map((c: any) => c.id);
    expect(ids).toContain("cmp:1");
    // Page B's component (cmp:2) must NOT be present
    expect(ids).not.toContain("cmp:2");

    // Page A's component set present
    const setIds = componentSets.map((s: any) => s.id);
    expect(setIds).toContain("set:1");

    expect(count).toBe(1);
    expect(pageId).toBe("page:A");
    expect(pageName).toBe("Icons");
  });

  it("with valid pageId for page B: returns only page B's standalone component", async () => {
    const res = await handleReadStyleRequest(
      makeRequest("get_local_components", { pageId: "page:B" }),
    );
    const { components, componentSets, pageId, pageName } = res!.data;

    const ids = components.map((c: any) => c.id);
    expect(ids).toContain("cmp:2");
    expect(ids).not.toContain("cmp:1");
    expect(componentSets).toHaveLength(0);
    expect(pageId).toBe("page:B");
    expect(pageName).toBe("Forms");
  });

  it("uses document-level scan to recover masters missed by page traversal", async () => {
    const documentOnlyMaster = {
      ...makeComponent(
        "cmp:document-only",
        "Nested/Orphan Master",
        "key-document-only",
      ),
      parent: {
        id: "page:A",
        name: "Icons",
        type: "PAGE",
        parent: { type: "DOCUMENT", id: "doc:1" },
      },
    };
    let loadedAllPages = false;
    (globalThis as any).figma.loadAllPagesAsync = async () => {
      loadedAllPages = true;
    };
    (globalThis as any).figma.root.findAllWithCriteria = ({ types }: { types: string[] }) => [
      ...pageA.findAllWithCriteria({ types }),
      ...pageB.findAllWithCriteria({ types }),
      documentOnlyMaster,
    ].filter((n) => types.includes(n.type));

    const res = await handleReadStyleRequest(makeRequest("get_local_components"));
    const ids = res!.data.components.map((c: any) => c.id);

    expect(loadedAllPages).toBe(true);
    expect(ids).toContain("cmp:document-only");
    expect(
      res!.data.components.find((c: any) => c.id === "cmp:document-only"),
    ).toMatchObject({
      id: "cmp:document-only",
      name: "Nested/Orphan Master",
      key: "key-document-only",
      componentSetId: null,
      variantProperties: null,
    });
    expect(res!.data.count).toBe(3);
  });

  it("deduplicates components returned by document-level recovery scan", async () => {
    (globalThis as any).figma.loadAllPagesAsync = async () => {};
    (globalThis as any).figma.root.findAllWithCriteria = ({ types }: { types: string[] }) => [
      ...pageA.findAllWithCriteria({ types }),
      ...pageA.findAllWithCriteria({ types }),
      ...pageB.findAllWithCriteria({ types }),
      makeComponent("cmp:1", "Button/Primary Duplicate", "key-cmp-1-duplicate", "set:1"),
      makeComponentSet("set:1", "Button Duplicate", "key-set-1-duplicate"),
    ].filter((n) => types.includes(n.type));

    const res = await handleReadStyleRequest(makeRequest("get_local_components"));
    const componentIds = res!.data.components.map((c: any) => c.id);
    const componentSetIds = res!.data.componentSets.map((s: any) => s.id);

    expect(componentIds.filter((id: string) => id === "cmp:1")).toHaveLength(1);
    expect(componentIds.filter((id: string) => id === "cmp:2")).toHaveLength(1);
    expect(componentSetIds.filter((id: string) => id === "set:1")).toHaveLength(1);
    expect(res!.data.count).toBe(2);
  });

  it("keeps pageId on the bounded page traversal path", async () => {
    const documentOnlyA = {
      ...makeComponent("cmp:document-a", "Recovered A", "key-document-a"),
      parent: {
        id: "page:A",
        name: "Icons",
        type: "PAGE",
        parent: { type: "DOCUMENT", id: "doc:1" },
      },
    };
    const documentOnlyB = {
      ...makeComponent("cmp:document-b", "Recovered B", "key-document-b"),
      parent: {
        id: "page:B",
        name: "Forms",
        type: "PAGE",
        parent: { type: "DOCUMENT", id: "doc:1" },
      },
    };
    (globalThis as any).figma.loadAllPagesAsync = async () => {
      throw new Error("page-scoped scans must not load all pages");
    };
    (globalThis as any).figma.root.findAllWithCriteria = () => [
      ...pageA.findAllWithCriteria({ types: ["COMPONENT", "COMPONENT_SET"] }),
      ...pageB.findAllWithCriteria({ types: ["COMPONENT", "COMPONENT_SET"] }),
      documentOnlyA,
      documentOnlyB,
    ];

    const res = await handleReadStyleRequest(
      makeRequest("get_local_components", { pageId: "page:A" }),
    );
    const ids = res!.data.components.map((c: any) => c.id);

    expect(ids).not.toContain("cmp:document-a");
    expect(ids).not.toContain("cmp:document-b");
    expect(res!.data.count).toBe(1);
    expect(res!.data.pageId).toBe("page:A");
  });

  it("component set includes defaultVariantKey when defaultVariant is present", async () => {
    const res = await handleReadStyleRequest(makeRequest("get_local_components"));
    const setWithDefault = res!.data.componentSets.find((s: any) => s.id === "set:1");
    expect(setWithDefault).toBeDefined();
    // set:1 was created with defaultVariantKey "key-cmp-1-default"
    expect(setWithDefault.defaultVariantKey).toBe("key-cmp-1-default");
  });

  it("component set omits defaultVariantKey when no defaultVariant — no noise", async () => {
    // pageB has only a standalone component, no component set with defaultVariant
    const res = await handleReadStyleRequest(
      makeRequest("get_local_components", { pageId: "page:B" }),
    );
    // No component sets on page B
    expect(res!.data.componentSets).toHaveLength(0);
  });

  it("with missing pageId: throws 'Page not found: <id>'", async () => {
    await expect(
      handleReadStyleRequest(
        makeRequest("get_local_components", { pageId: "page:MISSING" }),
      ),
    ).rejects.toThrow("Page not found: page:MISSING");
  });

  it("with non-PAGE node id: throws 'Page not found: <id>'", async () => {
    // Override getNodeByIdAsync to return a FRAME node
    (globalThis as any).figma.getNodeByIdAsync = async (id: string) => {
      if (id === "frame:1") return { id: "frame:1", name: "Frame", type: "FRAME" };
      return null;
    };
    await expect(
      handleReadStyleRequest(
        makeRequest("get_local_components", { pageId: "frame:1" }),
      ),
    ).rejects.toThrow("Page not found: frame:1");
  });
});
