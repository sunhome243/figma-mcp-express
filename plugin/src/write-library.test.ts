import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteLibraryRequest, withImportTimeout, IMPORT_TIMEOUT_MS } from "./write-library";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let createdInstances: any[];
let modePins: Array<{ node: any; collection: any; modeId: string }>;
let loadedFonts: Array<{ family: string; style: string }>;
let setPropertiesCallCount: number;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

// makeInstance creates a mock INSTANCE with optional componentProperties and text children.
// componentProperties models the Figma API: each key has a { type, value } shape.
// textChildren lets us simulate findAll({type: "TEXT"}) returning nodes with fontName.
const makeInstance = (
  id: string,
  name: string,
  opts: {
    componentProperties?: Record<string, { type: string; value?: any }>;
    textChildren?: Array<{ fontName: { family: string; style: string } | symbol }>;
  } = {},
) => {
  const inst: any = {
    id,
    name,
    type: "INSTANCE",
    x: 0,
    y: 0,
    width: 100,
    height: 40,
    _props: {},
    _overridesReset: false,
    _layoutSizingHorizontal: undefined,
    _layoutSizingVertical: undefined,
    componentProperties: opts.componentProperties ?? {},
    _textChildren: opts.textChildren ?? [],
    resize(w: number, h: number) {
      this.width = w;
      this.height = h;
    },
    setProperties(p: any) {
      setPropertiesCallCount++;
      Object.assign(this._props, p);
    },
    resetOverrides() {
      this._overridesReset = true;
    },
    findAll(filter: (n: any) => boolean) {
      // Return fake TEXT descendant nodes matching the filter
      return this._textChildren
        .map((tc: any, i: number) => ({ id: `${id}:txt:${i}`, type: "TEXT", fontName: tc.fontName }))
        .filter(filter);
    },
    set layoutSizingHorizontal(v: string) {
      if (!this.parent) throw new Error("layoutSizing requires a parent");
      this._layoutSizingHorizontal = v;
    },
    set layoutSizingVertical(v: string) {
      if (!this.parent) throw new Error("layoutSizing requires a parent");
      this._layoutSizingVertical = v;
    },
    parent: null,
  };
  createdInstances.push(inst);
  return inst;
};

const makeComponent = (id: string, name: string, variantProperties?: any) => ({
  id,
  name,
  type: "COMPONENT",
  variantProperties: variantProperties ?? null,
  createInstance() {
    return makeInstance(`${id}:inst`, name);
  },
});

const makeComponentSet = (id: string, name: string, variants: any[]) => ({
  id,
  name,
  type: "COMPONENT_SET",
  children: variants,
  defaultVariant: variants[0],
});

// importable registries keyed by figma key
let componentRegistry: Record<string, any>;
let setRegistry: Record<string, any>;
let variableRegistry: Record<string, any>;
let styleRegistry: Record<string, any>;
let collectionRegistry: Record<string, any>;
let teamLibCollections: any[];

const makeContainer = (id: string, name: string) => {
  const node: any = {
    id,
    name,
    type: "FRAME",
    children: [] as any[],
    appendChild(child: any) {
      child.parent = node;
      node.children.push(child);
    },
    insertChild(index: number, child: any) {
      child.parent = node;
      node.children.splice(index, 0, child);
    },
    setExplicitVariableModeForCollection(collection: any, modeId: string) {
      modePins.push({ node, collection, modeId });
    },
  };
  return node;
};

let currentPageNode: any;

beforeEach(() => {
  commitUndoCalled = false;
  createdInstances = [];
  modePins = [];
  loadedFonts = [];
  setPropertiesCallCount = 0;
  mockNodes = {};
  currentPageNode = makeContainer("0:1", "Page 1");

  componentRegistry = {
    "key-button": makeComponent("100:1", "Button"),
  };
  const v1 = makeComponent("200:1", "Badge", { State: "Default" });
  const v2 = makeComponent("200:2", "Badge", { State: "Error" });
  setRegistry = {
    "key-badge-set": makeComponentSet("200:0", "Badge", [v1, v2]),
  };
  variableRegistry = {
    "key-spacing-16": { id: "VariableID:300:1", name: "spacing/16", resolvedType: "FLOAT" },
  };
  styleRegistry = {
    "key-body": { id: "S:400:1", name: "Body/Medium", type: "TEXT" },
  };
  collectionRegistry = {
    "VariableCollectionId:1:1": {
      id: "VariableCollectionId:1:1",
      name: "Theme",
      defaultModeId: "1:0",
      modes: [
        { name: "Light", modeId: "1:0" },
        { name: "Dark", modeId: "1:1" },
      ],
    },
  };
  teamLibCollections = [
    { key: "libcol-1", name: "Colors", libraryName: "Brand" },
  ];

  (globalThis as any).figma = {
    get currentPage() {
      return currentPageNode;
    },
    mixed: Symbol("figma.mixed"),
    loadFontAsync: async (fontName: { family: string; style: string }) => {
      loadedFonts.push(fontName);
    },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    importComponentByKeyAsync: async (key: string) => {
      const c = componentRegistry[key];
      if (!c) throw new Error(`Component with key '${key}' not found`);
      return c;
    },
    importComponentSetByKeyAsync: async (key: string) => {
      const s = setRegistry[key];
      if (!s) throw new Error(`Component set with key '${key}' not found`);
      return s;
    },
    importStyleByKeyAsync: async (key: string) => {
      const s = styleRegistry[key];
      if (!s) throw new Error(`Style with key '${key}' not found`);
      return s;
    },
    variables: {
      // Real API: variable import lives on figma.variables (not top-level).
      importVariableByKeyAsync: async (key: string) => {
        const v = variableRegistry[key];
        if (!v) throw new Error(`Variable with key '${key}' not found`);
        return v;
      },
      getVariableCollectionByIdAsync: async (id: string) => collectionRegistry[id] ?? null,
    },
    teamLibrary: {
      getAvailableLibraryVariableCollectionsAsync: async () => teamLibCollections,
    },
    commitUndo: () => {
      commitUndoCalled = true;
    },
  };
});

// ── import_component_by_key ────────────────────────────────────────────────────

describe("import_component_by_key", () => {
  it("imports a COMPONENT by key", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("import_component_by_key", [], { key: "key-button" }),
    );
    expect(res?.data.type).toBe("COMPONENT");
    expect(res?.data.id).toBe("100:1");
    expect(res?.data.name).toBe("Button");
  });

  it("auto-branches to COMPONENT_SET when component import fails", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("import_component_by_key", [], { key: "key-badge-set" }),
    );
    expect(res?.data.type).toBe("COMPONENT_SET");
    expect(res?.data.defaultVariantId).toBe("200:1");
    expect(res?.data.variants).toHaveLength(2);
    expect(res?.data.variants[1].variantProperties.State).toBe("Error");
  });

  it("honors explicit assetType=COMPONENT_SET", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("import_component_by_key", [], { key: "key-badge-set", assetType: "COMPONENT_SET" }),
    );
    expect(res?.data.type).toBe("COMPONENT_SET");
  });

  it("throws when key missing", async () => {
    await expect(
      handleWriteLibraryRequest(makeRequest("import_component_by_key", [], {})),
    ).rejects.toThrow("key is required");
  });

  it("throws when key not found in either registry", async () => {
    await expect(
      handleWriteLibraryRequest(makeRequest("import_component_by_key", [], { key: "nope" })),
    ).rejects.toThrow();
  });

  // [FIX 5] A non-"not found" error from importComponentByKeyAsync (e.g. network,
  // permission, library disabled) must NOT be masked as a component-set failure.
  // The original error must propagate with its original message.
  it("re-throws original error when importComponentByKeyAsync fails with a non-not-found error", async () => {
    // Override to throw an unrelated error
    (globalThis as any).figma.importComponentByKeyAsync = async (_key: string) => {
      throw new Error("Library not enabled for this file");
    };
    await expect(
      handleWriteLibraryRequest(makeRequest("import_component_by_key", [], { key: "key-button" })),
    ).rejects.toThrow("Library not enabled for this file");
  });

  // [FIX 5] A "not found" error should still fall back to importComponentSetByKeyAsync
  // (existing fallback behavior preserved for type-mismatch case).
  it("falls back to importComponentSetByKeyAsync when importComponentByKeyAsync says not found", async () => {
    (globalThis as any).figma.importComponentByKeyAsync = async (_key: string) => {
      throw new Error("Component with key 'key-badge-set' not found");
    };
    const res = await handleWriteLibraryRequest(
      makeRequest("import_component_by_key", [], { key: "key-badge-set" }),
    );
    expect(res?.data.type).toBe("COMPONENT_SET");
  });
});

// ── import_variable_by_key ─────────────────────────────────────────────────────

describe("import_variable_by_key", () => {
  it("imports a variable and returns resolvedType", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("import_variable_by_key", [], { key: "key-spacing-16" }),
    );
    expect(res?.data.id).toBe("VariableID:300:1");
    expect(res?.data.name).toBe("spacing/16");
    expect(res?.data.resolvedType).toBe("FLOAT");
  });

  it("throws when key missing", async () => {
    await expect(
      handleWriteLibraryRequest(makeRequest("import_variable_by_key", [], {})),
    ).rejects.toThrow("key is required");
  });
});

// ── import_style_by_key ────────────────────────────────────────────────────────

describe("import_style_by_key", () => {
  it("imports a style and returns styleType", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("import_style_by_key", [], { key: "key-body" }),
    );
    expect(res?.data.id).toBe("S:400:1");
    expect(res?.data.styleType).toBe("TEXT");
  });

  it("throws when key missing", async () => {
    await expect(
      handleWriteLibraryRequest(makeRequest("import_style_by_key", [], {})),
    ).rejects.toThrow("key is required");
  });
});

// ── create_instance ────────────────────────────────────────────────────────────

describe("create_instance", () => {
  it("creates an instance from a COMPONENT node and appends to current page", async () => {
    mockNodes["100:1"] = makeComponent("100:1", "Button");
    const res = await handleWriteLibraryRequest(
      makeRequest("create_instance", [], { componentId: "100:1" }),
    );
    expect(res?.data.id).toBe("100:1:inst");
    expect(commitUndoCalled).toBe(true);
    expect(currentPageNode.children).toHaveLength(1);
  });

  it("picks a variant from a COMPONENT_SET by variantProperties", async () => {
    mockNodes["200:0"] = setRegistry["key-badge-set"];
    const res = await handleWriteLibraryRequest(
      makeRequest("create_instance", [], {
        componentId: "200:0",
        variantProperties: { State: "Error" },
      }),
    );
    expect(res?.data.id).toBe("200:2:inst");
  });

  it("uses defaultVariant when a COMPONENT_SET is given without variantProperties", async () => {
    mockNodes["200:0"] = setRegistry["key-badge-set"];
    const res = await handleWriteLibraryRequest(
      makeRequest("create_instance", [], { componentId: "200:0" }),
    );
    expect(res?.data.id).toBe("200:1:inst");
  });

  it("throws when variantProperties match no variant in the set", async () => {
    mockNodes["200:0"] = setRegistry["key-badge-set"];
    await expect(
      handleWriteLibraryRequest(
        makeRequest("create_instance", [], {
          componentId: "200:0",
          variantProperties: { State: "Nonexistent" },
        }),
      ),
    ).rejects.toThrow();
  });

  it("falls back to componentKey import when componentId is unresolvable", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("create_instance", [], { componentId: "999:9", componentKey: "key-button" }),
    );
    expect(res?.data.id).toBe("100:1:inst");
  });

  it("applies properties and layout sizing after append", async () => {
    mockNodes["100:1"] = makeComponent("100:1", "Button");
    await handleWriteLibraryRequest(
      makeRequest("create_instance", [], {
        componentId: "100:1",
        properties: { "Label#1:2": "OK" },
        layoutSizingHorizontal: "FILL",
      }),
    );
    const inst = createdInstances[0];
    expect(inst._props["Label#1:2"]).toBe("OK");
    expect(inst._layoutSizingHorizontal).toBe("FILL");
  });

  it("throws when componentId missing", async () => {
    await expect(
      handleWriteLibraryRequest(makeRequest("create_instance", [], {})),
    ).rejects.toThrow("componentId is required");
  });

  it("throws when component cannot be resolved", async () => {
    await expect(
      handleWriteLibraryRequest(makeRequest("create_instance", [], { componentId: "999:9" })),
    ).rejects.toThrow();
  });

  // [FIX 4] create_instance must load fonts from text descendants before setProperties
  it("loads fonts from TEXT descendants before calling setProperties in create_instance", async () => {
    // The makeComponent's createInstance returns a basic instance — override it here
    // to include text children with fonts.
    const compId = "100:1";
    const comp = {
      id: compId,
      name: "Button",
      type: "COMPONENT",
      variantProperties: null,
      createInstance() {
        return makeInstance(`${compId}:inst`, "Button", {
          textChildren: [{ fontName: { family: "Pretendard", style: "SemiBold" } }],
        });
      },
    };
    mockNodes[compId] = comp;
    await handleWriteLibraryRequest(
      makeRequest("create_instance", [], {
        componentId: compId,
        properties: { "Label#1:2": "Submit" },
      }),
    );
    expect(loadedFonts.some(f => f.family === "Pretendard" && f.style === "SemiBold")).toBe(true);
    expect(setPropertiesCallCount).toBeGreaterThan(0);
  });
});

// ── set_instance_properties ────────────────────────────────────────────────────

describe("set_instance_properties", () => {
  it("sets properties on an instance", async () => {
    const inst = makeInstance("100:1:inst", "Button");
    mockNodes["100:1:inst"] = inst;
    const res = await handleWriteLibraryRequest(
      makeRequest("set_instance_properties", ["100:1:inst"], { properties: { State: "Error" } }),
    );
    expect(inst._props.State).toBe("Error");
    expect(res?.data.results[0].appliedProperties.State).toBe("Error");
    expect(commitUndoCalled).toBe(true);
  });

  it("resets overrides when requested", async () => {
    const inst = makeInstance("100:1:inst", "Button");
    mockNodes["100:1:inst"] = inst;
    await handleWriteLibraryRequest(
      makeRequest("set_instance_properties", ["100:1:inst"], {
        properties: { State: "Default" },
        resetOverrides: true,
      }),
    );
    expect(inst._overridesReset).toBe(true);
  });

  it("collects a per-node error when node is not an INSTANCE (no abort)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME" };
    const res = await handleWriteLibraryRequest(
      makeRequest("set_instance_properties", ["1:1"], { properties: { State: "Error" } }),
    );
    expect(res?.data.results[0].error).toContain("is not a component INSTANCE");
  });

  it("throws when properties missing", async () => {
    const inst = makeInstance("100:1:inst", "Button");
    mockNodes["100:1:inst"] = inst;
    await expect(
      handleWriteLibraryRequest(makeRequest("set_instance_properties", ["100:1:inst"], {})),
    ).rejects.toThrow("properties is required");
  });

  // [FIX 3] SLOT-type keys must be dropped before setProperties — passing them
  // poisons the whole update with "cannotSetSlotProperty".
  it("drops SLOT-type keys before calling setProperties and reports droppedSlotKeys", async () => {
    const inst = makeInstance("100:1:inst", "Button", {
      componentProperties: {
        "Label#1:2": { type: "TEXT", value: "OK" },
        "Icon#1:3": { type: "SLOT" },        // SLOT — must be dropped
        "State#1:4": { type: "VARIANT", value: "Default" },
      },
    });
    mockNodes["100:1:inst"] = inst;
    const res = await handleWriteLibraryRequest(
      makeRequest("set_instance_properties", ["100:1:inst"], {
        properties: { "Label#1:2": "Hello", "Icon#1:3": "some-id", "State#1:4": "Error" },
      }),
    );
    // SLOT key must NOT appear in the applied properties
    expect(inst._props["Icon#1:3"]).toBeUndefined();
    // Non-SLOT keys must still be applied
    expect(inst._props["Label#1:2"]).toBe("Hello");
    expect(inst._props["State#1:4"]).toBe("Error");
    // Result must communicate what was dropped
    expect(res?.data.results[0].droppedSlotKeys).toContain("Icon#1:3");
  });

  // [FIX 4] TEXT-type props require fonts loaded — setProperties without prior
  // loadFontAsync can throw. Ensure fonts are loaded first.
  it("loads fonts from TEXT descendants before calling setProperties", async () => {
    const inst = makeInstance("100:1:inst", "Button", {
      textChildren: [
        { fontName: { family: "Inter", style: "Regular" } },
        { fontName: { family: "Inter", style: "Bold" } },
      ],
    });
    mockNodes["100:1:inst"] = inst;
    await handleWriteLibraryRequest(
      makeRequest("set_instance_properties", ["100:1:inst"], {
        properties: { "Label#1:2": "Hello" },
      }),
    );
    // Both fonts must have been loaded before setProperties
    expect(loadedFonts.some(f => f.family === "Inter" && f.style === "Regular")).toBe(true);
    expect(loadedFonts.some(f => f.family === "Inter" && f.style === "Bold")).toBe(true);
    expect(setPropertiesCallCount).toBeGreaterThan(0);
  });

  // [FIX 4] figma.mixed fontNames must be skipped (not passed to loadFontAsync)
  it("skips figma.mixed fontName when loading fonts", async () => {
    const mixedSymbol = Symbol("figma.mixed");
    (globalThis as any).figma.mixed = mixedSymbol;
    const inst = makeInstance("100:1:inst", "Button", {
      textChildren: [
        { fontName: mixedSymbol },
        { fontName: { family: "Geist", style: "Medium" } },
      ],
    });
    mockNodes["100:1:inst"] = inst;
    await handleWriteLibraryRequest(
      makeRequest("set_instance_properties", ["100:1:inst"], { properties: { "Label#1:2": "X" } }),
    );
    // Only the non-mixed font should have been loaded
    expect(loadedFonts).toHaveLength(1);
    expect(loadedFonts[0]).toEqual({ family: "Geist", style: "Medium" });
  });
});

// ── set_variable_mode ──────────────────────────────────────────────────────────

describe("set_variable_mode", () => {
  it("pins a node to a collection mode", async () => {
    const frame = makeContainer("1:1", "Wrapper");
    mockNodes["1:1"] = frame;
    const res = await handleWriteLibraryRequest(
      makeRequest("set_variable_mode", ["1:1"], {
        collectionId: "VariableCollectionId:1:1",
        modeId: "1:1",
      }),
    );
    expect(modePins).toHaveLength(1);
    expect(modePins[0].modeId).toBe("1:1");
    expect(res?.data.collectionId).toBe("VariableCollectionId:1:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("throws when collection not found", async () => {
    const frame = makeContainer("1:1", "Wrapper");
    mockNodes["1:1"] = frame;
    await expect(
      handleWriteLibraryRequest(
        makeRequest("set_variable_mode", ["1:1"], { collectionId: "missing", modeId: "1:1" }),
      ),
    ).rejects.toThrow("Collection not found");
  });

  it("throws when collectionId missing", async () => {
    mockNodes["1:1"] = makeContainer("1:1", "Wrapper");
    await expect(
      handleWriteLibraryRequest(makeRequest("set_variable_mode", ["1:1"], { modeId: "1:1" })),
    ).rejects.toThrow("collectionId is required");
  });
});

// ── get_remote_variable_collection ─────────────────────────────────────────────

describe("get_remote_variable_collection", () => {
  it("returns collection modes", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("get_remote_variable_collection", [], { collectionId: "VariableCollectionId:1:1" }),
    );
    expect(res?.data.defaultModeId).toBe("1:0");
    expect(res?.data.modes).toHaveLength(2);
    expect(res?.data.modes[1].modeId).toBe("1:1");
  });

  it("throws when collection not found", async () => {
    await expect(
      handleWriteLibraryRequest(
        makeRequest("get_remote_variable_collection", [], { collectionId: "missing" }),
      ),
    ).rejects.toThrow("Collection not found");
  });
});

// ── list_library_variable_collections ──────────────────────────────────────────

describe("list_library_variable_collections", () => {
  it("returns subscribed library variable collections", async () => {
    const res = await handleWriteLibraryRequest(
      makeRequest("list_library_variable_collections", [], {}),
    );
    expect(res?.data.collections).toHaveLength(1);
    expect(res?.data.collections[0].key).toBe("libcol-1");
  });
});

// ── dispatch miss ──────────────────────────────────────────────────────────────

describe("handleWriteLibraryRequest", () => {
  it("returns null for unrelated request types", async () => {
    const res = await handleWriteLibraryRequest(makeRequest("create_frame", [], {}));
    expect(res).toBeNull();
  });
});

// ── TASK 5: create_instance — droppedSlotKeys surfaced in response ─────────────

describe("create_instance — droppedSlotKeys in response (new feature)", () => {
  it("surfaces droppedSlotKeys in response when a SLOT prop is passed", async () => {
    // Build a component whose instance has a SLOT prop in componentProperties
    const compId = "100:2";
    const comp = {
      id: compId,
      name: "Card",
      type: "COMPONENT",
      variantProperties: null,
      createInstance() {
        return makeInstance(`${compId}:inst`, "Card", {
          componentProperties: {
            "Title#1:5": { type: "TEXT", value: "Hello" },
            "Icon#1:6": { type: "SLOT" },       // SLOT — must be dropped
            "Variant#1:7": { type: "VARIANT", value: "Default" },
          },
        });
      },
    };
    mockNodes[compId] = comp;

    const res = await handleWriteLibraryRequest(
      makeRequest("create_instance", [], {
        componentId: compId,
        properties: {
          "Title#1:5": "My Card",
          "Icon#1:6": "some-node-id",  // SLOT key — should be dropped
          "Variant#1:7": "Primary",
        },
      }),
    );

    // SLOT key must appear in droppedSlotKeys
    expect(res?.data.droppedSlotKeys).toBeDefined();
    expect(res?.data.droppedSlotKeys).toContain("Icon#1:6");
    expect(res?.data.id).toBe(`${compId}:inst`);
  });

  it("droppedSlotKeys not present in response when no SLOT props exist", async () => {
    const compId = "100:3";
    const comp = {
      id: compId,
      name: "Button",
      type: "COMPONENT",
      variantProperties: null,
      createInstance() {
        return makeInstance(`${compId}:inst`, "Button", {
          componentProperties: {
            "Label#1:2": { type: "TEXT", value: "Click me" },
          },
        });
      },
    };
    mockNodes[compId] = comp;

    const res = await handleWriteLibraryRequest(
      makeRequest("create_instance", [], {
        componentId: compId,
        properties: { "Label#1:2": "Submit" },
      }),
    );

    // No SLOT props → droppedSlotKeys should not be in data (or be undefined/empty)
    expect(res?.data.droppedSlotKeys).toBeUndefined();
  });

  it("non-SLOT property is applied normally alongside a dropped SLOT key", async () => {
    const compId = "100:4";
    const comp = {
      id: compId,
      name: "Widget",
      type: "COMPONENT",
      variantProperties: null,
      createInstance() {
        return makeInstance(`${compId}:inst`, "Widget", {
          componentProperties: {
            "Label#2:1": { type: "TEXT", value: "default" },
            "Slot#2:2": { type: "SLOT" },
          },
        });
      },
    };
    mockNodes[compId] = comp;

    await handleWriteLibraryRequest(
      makeRequest("create_instance", [], {
        componentId: compId,
        properties: {
          "Label#2:1": "Hello",
          "Slot#2:2": "some-node-id",
        },
      }),
    );

    const inst = createdInstances[createdInstances.length - 1];
    // Non-SLOT prop applied
    expect(inst._props["Label#2:1"]).toBe("Hello");
    // SLOT prop must not be in applied props
    expect(inst._props["Slot#2:2"]).toBeUndefined();
  });
});

// ── withImportTimeout (hung-import guard) ─────────────────────────────────────
// Root cause this fixes: figma.importByKeyAsync can hang forever with no built-in
// timeout/progress; a hung import then occupies the single plugin thread until the
// server's 120s ceiling, and concurrency re-arms that window so it looks permanent.
describe("withImportTimeout (hung-import guard)", () => {
  it("rejects a hung import within the timeout, fast, with a clear message", async () => {
    const neverResolves = new Promise<never>(() => {}); // simulates a hung importByKeyAsync
    const start = Date.now();
    await expect(
      withImportTimeout(neverResolves, "importComponentByKeyAsync(key-x)", 20),
    ).rejects.toThrow(/timed out after 20ms/);
    // settled at ~20ms, NOT after the underlying hang — proves the race fired
    expect(Date.now() - start).toBeLessThan(2000);
  });

  it("passes a resolving import through unchanged (real call wins the race)", async () => {
    const result = await withImportTimeout(
      Promise.resolve({ id: "1:2", name: "Button" }),
      "importComponentByKeyAsync(key-button)",
      20,
    );
    expect(result).toEqual({ id: "1:2", name: "Button" });
  });

  it("propagates the real rejection (not the timeout) when the import rejects first", async () => {
    await expect(
      withImportTimeout(Promise.reject(new Error("library disabled")), "x", 1000),
    ).rejects.toThrow("library disabled");
  });

  it("timeout error message lacks 'not found' so it never triggers the COMPONENT_SET fallback", async () => {
    // importComponentOrSet only falls back to the set importer on "not found"/"not a
    // component" — a timeout must NOT match, else a hung component import would re-hang
    // on the set importer.
    let msg = "";
    try {
      await withImportTimeout(new Promise<never>(() => {}), "importComponentByKeyAsync(k)", 20);
    } catch (e: any) {
      msg = String(e?.message ?? "").toLowerCase();
    }
    expect(msg).not.toContain("not found");
    expect(msg).not.toContain("not a component");
  });

  it("defaults to a 15s ceiling", () => {
    expect(IMPORT_TIMEOUT_MS).toBe(15000);
  });
});
