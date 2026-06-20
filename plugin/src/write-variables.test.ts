import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteRequest } from "./write-handlers";

// ── Figma global mock ─────────────────────────────────────────────────────────
//
// Covers variable creation (write-variables) plus the fork's Track A library
// variable tools (write-library): import_variable_by_key, set_variable_mode,
// get_remote_variable_collection. All dispatch via handleWriteRequest.

let mockNodes: Record<string, any>;
let mockVariables: Record<string, any>;
let mockCollections: Record<string, any>;
let importedKeys: string[];
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

const makeCollection = (id: string, name: string) => {
  let nextMode = 1;
  return {
    id, name,
    defaultModeId: "mode:default",
    modes: [{ modeId: "mode:default", name: "Mode 1" }],
    renameMode(modeId: string, newName: string) {
      const m = this.modes.find((mm: any) => mm.modeId === modeId);
      if (m) m.name = newName;
    },
    addMode(modeName: string) {
      const modeId = `mode:${nextMode++}`;
      this.modes.push({ modeId, name: modeName });
      return modeId;
    },
    remove() {},
  };
};

beforeEach(() => {
  commitUndoCalled = false;
  importedKeys = [];
  mockNodes = {};
  mockVariables = {};
  mockCollections = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
    variables: {
      createVariableCollection: (name: string) => {
        const col = makeCollection(`col:${name}`, name);
        mockCollections[col.id] = col;
        return col;
      },
      getVariableCollectionByIdAsync: async (id: string) => mockCollections[id] ?? null,
      getVariableByIdAsync: async (id: string) => mockVariables[id] ?? null,
      createVariable: (name: string, collection: any, type: string) => {
        const v: any = {
          id: `var:${name}`,
          name,
          resolvedType: type,
          _values: {} as Record<string, any>,
          setValueForMode(modeId: string, value: any) { this._values[modeId] = value; },
          remove() {},
        };
        mockVariables[v.id] = v;
        return v;
      },
      importVariableByKeyAsync: async (key: string) => {
        importedKeys.push(key);
        return { id: `var:imported:${key}`, name: `imported/${key}`, resolvedType: "COLOR" };
      },
    },
  };
});

// ── create_variable_collection ──────────────────────────────────────────────

describe("create_variable_collection", () => {
  it("creates a collection and returns its modes", async () => {
    const res = await handleWriteRequest(
      makeRequest("create_variable_collection", [], { name: "Theme" })
    );
    expect(res?.data.name).toBe("Theme");
    expect(res?.data.modes[0].name).toBe("Mode 1");
    expect(commitUndoCalled).toBe(true);
  });

  it("renames the initial mode when initialModeName is provided", async () => {
    const res = await handleWriteRequest(
      makeRequest("create_variable_collection", [], { name: "Theme", initialModeName: "Light" })
    );
    expect(res?.data.modes[0].name).toBe("Light");
  });

  it("throws when name is missing", async () => {
    await expect(
      handleWriteRequest(makeRequest("create_variable_collection", [], {}))
    ).rejects.toThrow("name is required");
  });
});

// ── create_variable ─────────────────────────────────────────────────────────

describe("create_variable", () => {
  it("creates a COLOR variable with an initial hex value", async () => {
    mockCollections["col:1"] = makeCollection("col:1", "Theme");
    const res = await handleWriteRequest(
      makeRequest("create_variable", [], {
        name: "primary", collectionId: "col:1", type: "COLOR", value: "#3366FF",
      })
    );
    expect(res?.data.resolvedType).toBe("COLOR");
    const created = mockVariables["var:primary"];
    const stored = created._values["mode:default"];
    expect(stored.b).toBeCloseTo(1, 5);
  });

  it("creates a FLOAT variable from a numeric value", async () => {
    mockCollections["col:1"] = makeCollection("col:1", "Spacing");
    await handleWriteRequest(
      makeRequest("create_variable", [], {
        name: "gap", collectionId: "col:1", type: "FLOAT", value: 16,
      })
    );
    expect(mockVariables["var:gap"]._values["mode:default"]).toBe(16);
  });

  it("throws for an invalid variable type", async () => {
    mockCollections["col:1"] = makeCollection("col:1", "Theme");
    await expect(
      handleWriteRequest(
        makeRequest("create_variable", [], { name: "x", collectionId: "col:1", type: "VECTOR" })
      )
    ).rejects.toThrow("type is required");
  });

  it("throws when the collection does not exist", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("create_variable", [], { name: "x", collectionId: "col:missing", type: "COLOR" })
      )
    ).rejects.toThrow("Collection not found");
  });
});

// ── set_variable_value ──────────────────────────────────────────────────────

describe("set_variable_value", () => {
  it("sets a variable value for a specific mode", async () => {
    mockVariables["var:1"] = {
      id: "var:1", name: "primary", resolvedType: "STRING",
      _values: {} as Record<string, any>,
      setValueForMode(modeId: string, value: any) { this._values[modeId] = value; },
    };
    const res = await handleWriteRequest(
      makeRequest("set_variable_value", [], { variableId: "var:1", modeId: "mode:default", value: "hello" })
    );
    expect(res?.data.variableId).toBe("var:1");
    expect(mockVariables["var:1"]._values["mode:default"]).toBe("hello");
  });

  it("throws when value is missing", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("set_variable_value", [], { variableId: "var:1", modeId: "mode:default" })
      )
    ).rejects.toThrow("value is required");
  });
});

// ── import_variable_by_key (Track A library tool) ───────────────────────────

describe("import_variable_by_key", () => {
  it("imports a library variable via figma.variables.importVariableByKeyAsync", async () => {
    const res = await handleWriteRequest(
      makeRequest("import_variable_by_key", [], { key: "abc123" })
    );
    expect(importedKeys).toEqual(["abc123"]);
    expect(res?.data.id).toBe("var:imported:abc123");
    expect(res?.data.resolvedType).toBe("COLOR");
  });

  it("throws when key is missing", async () => {
    await expect(
      handleWriteRequest(makeRequest("import_variable_by_key", [], {}))
    ).rejects.toThrow("key is required");
  });
});

// ── set_variable_mode (Track A — pins a node to an explicit mode) ────────────

describe("set_variable_mode", () => {
  it("pins a node to an explicit variable mode for a collection", async () => {
    let pinnedCollection: any = null;
    let pinnedMode: string | null = null;
    mockNodes["1:1"] = {
      id: "1:1", name: "Wrapper",
      setExplicitVariableModeForCollection(collection: any, modeId: string) {
        pinnedCollection = collection;
        pinnedMode = modeId;
      },
    };
    mockCollections["col:theme"] = makeCollection("col:theme", "Theme");
    mockCollections["col:theme"].addMode("Dark"); // mode:1
    const res = await handleWriteRequest(
      makeRequest("set_variable_mode", ["1:1"], { collectionId: "col:theme", modeId: "mode:1" })
    );
    expect(pinnedCollection?.id).toBe("col:theme");
    expect(pinnedMode).toBe("mode:1");
    expect(res?.data.modeId).toBe("mode:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("throws when the collection does not exist", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Wrapper", setExplicitVariableModeForCollection() {} };
    await expect(
      handleWriteRequest(
        makeRequest("set_variable_mode", ["1:1"], { collectionId: "col:missing", modeId: "mode:1" })
      )
    ).rejects.toThrow("Collection not found");
  });

  it("throws when modeId is missing", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Wrapper", setExplicitVariableModeForCollection() {} };
    await expect(
      handleWriteRequest(
        makeRequest("set_variable_mode", ["1:1"], { collectionId: "col:theme" })
      )
    ).rejects.toThrow("modeId is required");
  });
});

// ── FIX 1: parseVariableValue VARIABLE_ALIAS guard ──────────────────────────

describe("VARIABLE_ALIAS guard in parseVariableValue", () => {
  it("set_variable_value preserves a VARIABLE_ALIAS object (not NaN/string/false)", async () => {
    mockVariables["var:1"] = {
      id: "var:1", name: "alias-target", resolvedType: "FLOAT",
      variableCollectionId: "col:1",
      _values: {} as Record<string, any>,
      setValueForMode(modeId: string, value: any) { this._values[modeId] = value; },
    };
    mockCollections["col:1"] = makeCollection("col:1", "Theme");
    const aliasValue = { type: "VARIABLE_ALIAS", id: "var:other" };
    await handleWriteRequest(
      makeRequest("set_variable_value", [], { variableId: "var:1", modeId: "mode:default", value: aliasValue })
    );
    const stored = mockVariables["var:1"]._values["mode:default"];
    // Must be preserved as-is, not corrupted to NaN / "[object Object]" / false
    expect(stored).toEqual({ type: "VARIABLE_ALIAS", id: "var:other" });
  });

  it("create_variable with a VARIABLE_ALIAS initial value preserves the alias object (FLOAT variable)", async () => {
    // FLOAT path: parseFloat({type:"VARIABLE_ALIAS",...}) → NaN without guard
    mockCollections["col:1"] = makeCollection("col:1", "Theme");
    const aliasValue = { type: "VARIABLE_ALIAS", id: "var:primary" };
    await handleWriteRequest(
      makeRequest("create_variable", [], {
        name: "alias-var", collectionId: "col:1", type: "FLOAT", value: aliasValue,
      })
    );
    const created = mockVariables["var:alias-var"];
    const stored = created._values["mode:default"];
    expect(stored).toEqual({ type: "VARIABLE_ALIAS", id: "var:primary" });
  });
});

// ── FIX 2: set_variable_value modeId validation ──────────────────────────────

describe("set_variable_value modeId validation", () => {
  beforeEach(() => {
    mockCollections["col:1"] = makeCollection("col:1", "Theme");
    mockVariables["var:1"] = {
      id: "var:1", name: "primary", resolvedType: "STRING",
      variableCollectionId: "col:1",
      _values: {} as Record<string, any>,
      setValueForMode(modeId: string, value: any) { this._values[modeId] = value; },
    };
  });

  it("throws a clear error when modeId is not in the variable's collection", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("set_variable_value", [], {
          variableId: "var:1", modeId: "mode:nonexistent", value: "hello",
        })
      )
    ).rejects.toThrow(/mode.*not found|invalid mode|valid mode/i);
  });

  it("succeeds when modeId is valid for the collection", async () => {
    const res = await handleWriteRequest(
      makeRequest("set_variable_value", [], {
        variableId: "var:1", modeId: "mode:default", value: "ok",
      })
    );
    expect(res?.data.variableId).toBe("var:1");
    expect(mockVariables["var:1"]._values["mode:default"]).toBe("ok");
  });
});

// ── FIX 3: create_variable multi-mode values + modes in response ─────────────

describe("create_variable multi-mode values + modes in response", () => {
  it("sets values for multiple modes via p.values map", async () => {
    const col = makeCollection("col:1", "Theme");
    col.addMode("Dark"); // adds mode:1
    mockCollections["col:1"] = col;
    await handleWriteRequest(
      makeRequest("create_variable", [], {
        name: "bg", collectionId: "col:1", type: "STRING",
        values: { "mode:default": "white", "mode:1": "black" },
      })
    );
    const v = mockVariables["var:bg"];
    expect(v._values["mode:default"]).toBe("white");
    expect(v._values["mode:1"]).toBe("black");
  });

  it("includes modes list in the response data", async () => {
    mockCollections["col:1"] = makeCollection("col:1", "Theme");
    const res = await handleWriteRequest(
      makeRequest("create_variable", [], {
        name: "tok", collectionId: "col:1", type: "FLOAT", value: 4,
      })
    );
    expect(res?.data.modes).toBeDefined();
    expect(Array.isArray(res?.data.modes)).toBe(true);
    expect(res?.data.modes[0]).toHaveProperty("modeId");
    expect(res?.data.modes[0]).toHaveProperty("name");
  });
});

// ── FIX 4: add_variable_mode plan-limit error wrapping ──────────────────────

describe("add_variable_mode plan-limit error", () => {
  it("wraps plan-limit error with a clear message", async () => {
    const col = makeCollection("col:1", "Theme");
    col.addMode = (_name: string) => {
      throw new Error("Limited to 1 modes only");
    };
    mockCollections["col:1"] = col;
    await expect(
      handleWriteRequest(
        makeRequest("add_variable_mode", [], { collectionId: "col:1", modeName: "Dark" })
      )
    ).rejects.toThrow(/Cannot add mode.*plan mode-limit|plan mode-limit reached/i);
  });
});

// ── get_remote_variable_collection (Track A — queries a remote collection) ───

describe("get_remote_variable_collection", () => {
  it("returns the collection's modes and default mode", async () => {
    const col = makeCollection("col:remote", "Remote Theme");
    col.addMode("Dark");
    mockCollections["col:remote"] = col;
    const res = await handleWriteRequest(
      makeRequest("get_remote_variable_collection", [], { collectionId: "col:remote" })
    );
    expect(res?.data.name).toBe("Remote Theme");
    expect(res?.data.defaultModeId).toBe("mode:default");
    expect(res?.data.modes).toHaveLength(2);
  });

  it("throws when the remote collection is not found", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("get_remote_variable_collection", [], { collectionId: "col:gone" })
      )
    ).rejects.toThrow("Collection not found");
  });

  it("returns null for an unrecognised request type", async () => {
    const res = await handleWriteRequest(makeRequest("totally_unknown_op"));
    expect(res).toBeNull();
  });
});

// ── update_variable ───────────────────────────────────────────────────────────

describe("update_variable", () => {
  it("renames, sets scopes, hides, and sets codeSyntax", async () => {
    const syntaxCalls: any[] = [];
    mockVariables["var:1"] = {
      id: "var:1", name: "old", resolvedType: "COLOR",
      scopes: ["ALL_SCOPES"], hiddenFromPublishing: false, codeSyntax: {},
      setVariableCodeSyntax(platform: string, value: string) { syntaxCalls.push({ platform, value }); this.codeSyntax[platform] = value; },
    };
    const res = await handleWriteRequest(makeRequest("update_variable", [], {
      variableId: "var:1", name: "color/primary", scopes: ["TEXT_FILL", "FRAME_FILL"],
      hiddenFromPublishing: true, codeSyntax: { WEB: "colorPrimary", iOS: "ColorPrimary" },
    }));
    expect(mockVariables["var:1"].name).toBe("color/primary");
    expect(mockVariables["var:1"].scopes).toEqual(["TEXT_FILL", "FRAME_FILL"]);
    expect(mockVariables["var:1"].hiddenFromPublishing).toBe(true);
    expect(syntaxCalls).toContainEqual({ platform: "WEB", value: "colorPrimary" });
    expect(syntaxCalls).toContainEqual({ platform: "iOS", value: "ColorPrimary" });
    expect(res?.data.variableId).toBe("var:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("throws when the variable is not found", async () => {
    await expect(handleWriteRequest(makeRequest("update_variable", [], { variableId: "nope" })))
      .rejects.toThrow("Variable not found");
  });

  it("throws when variableId missing", async () => {
    await expect(handleWriteRequest(makeRequest("update_variable", [], {})))
      .rejects.toThrow("variableId is required");
  });

  it("rejects non-string codeSyntax values before mutating", async () => {
    const syntaxCalls: any[] = [];
    mockVariables["var:1"] = {
      id: "var:1", name: "old", resolvedType: "COLOR",
      scopes: ["ALL_SCOPES"], hiddenFromPublishing: false, codeSyntax: {},
      setVariableCodeSyntax(platform: string, value: string) { syntaxCalls.push({ platform, value }); this.codeSyntax[platform] = value; },
    };
    await expect(handleWriteRequest(makeRequest("update_variable", [], {
      variableId: "var:1",
      name: "new-name",
      scopes: ["TEXT_FILL"],
      hiddenFromPublishing: true,
      codeSyntax: { WEB: { token: "bad" } },
    }))).rejects.toThrow("codeSyntax.WEB must be a string");
    expect(syntaxCalls).toEqual([]);
    expect(mockVariables["var:1"].name).toBe("old");
    expect(mockVariables["var:1"].scopes).toEqual(["ALL_SCOPES"]);
    expect(mockVariables["var:1"].hiddenFromPublishing).toBe(false);
    expect(commitUndoCalled).toBe(false);
  });

  it("rejects invalid removeCodeSyntax before mutating metadata", async () => {
    mockVariables["var:1"] = {
      id: "var:1", name: "old", resolvedType: "COLOR",
      scopes: ["ALL_SCOPES"], hiddenFromPublishing: false, codeSyntax: { WEB: "token" },
      setVariableCodeSyntax() {},
      removeVariableCodeSyntax(platform: string) { delete this.codeSyntax[platform]; },
    };
    await expect(handleWriteRequest(makeRequest("update_variable", [], {
      variableId: "var:1",
      name: "new-name",
      hiddenFromPublishing: true,
      removeCodeSyntax: ["MAC"],
    }))).rejects.toThrow("codeSyntax platform must be WEB, ANDROID, or iOS");
    expect(mockVariables["var:1"].name).toBe("old");
    expect(mockVariables["var:1"].hiddenFromPublishing).toBe(false);
    expect(mockVariables["var:1"].codeSyntax).toEqual({ WEB: "token" });
    expect(commitUndoCalled).toBe(false);
  });
});

// ── update_variable_collection ────────────────────────────────────────────────

describe("update_variable_collection", () => {
  it("renames the collection, hides it, and renames a mode", async () => {
    const col: any = makeCollection("col:1", "Tokens");
    col.hiddenFromPublishing = false;
    mockCollections["col:1"] = col;
    const res = await handleWriteRequest(makeRequest("update_variable_collection", [], {
      collectionId: "col:1", name: "Design Tokens", hiddenFromPublishing: true,
      renameMode: { modeId: "mode:default", newName: "Light" },
    }));
    expect(col.name).toBe("Design Tokens");
    expect(col.hiddenFromPublishing).toBe(true);
    expect(col.modes[0].name).toBe("Light");
    expect(res?.data.collectionId).toBe("col:1");
  });

  it("removes a mode", async () => {
    const col: any = makeCollection("col:1", "Tokens");
    col.addMode("Dark");
    col.removeMode = function (modeId: string) { this.modes = this.modes.filter((m: any) => m.modeId !== modeId); };
    mockCollections["col:1"] = col;
    await handleWriteRequest(makeRequest("update_variable_collection", [], {
      collectionId: "col:1", removeMode: "mode:1",
    }));
    expect(col.modes.find((m: any) => m.modeId === "mode:1")).toBeUndefined();
  });

  it("throws a clear error when removing the last mode fails", async () => {
    const col: any = makeCollection("col:1", "Tokens");
    col.removeMode = () => { throw new Error("cannot remove last mode"); };
    mockCollections["col:1"] = col;
    await expect(handleWriteRequest(makeRequest("update_variable_collection", [], {
      collectionId: "col:1", removeMode: "mode:default",
    }))).rejects.toThrow("must keep at least one mode");
  });

  it("throws when the collection is not found", async () => {
    await expect(handleWriteRequest(makeRequest("update_variable_collection", [], { collectionId: "nope" })))
      .rejects.toThrow("Collection not found");
  });

  it("rejects non-string renameMode fields before mutating", async () => {
    const col: any = makeCollection("col:1", "Tokens");
    col.hiddenFromPublishing = false;
    mockCollections["col:1"] = col;
    await expect(handleWriteRequest(makeRequest("update_variable_collection", [], {
      collectionId: "col:1",
      name: "New Tokens",
      hiddenFromPublishing: true,
      renameMode: { modeId: "mode:default", newName: { label: "bad" } },
    }))).rejects.toThrow("renameMode.newName must be a string");
    expect(col.name).toBe("Tokens");
    expect(col.hiddenFromPublishing).toBe(false);
    expect(col.modes[0].name).toBe("Mode 1");
    expect(commitUndoCalled).toBe(false);
  });

  it("rejects last-mode removal before mutating collection metadata", async () => {
    const col: any = makeCollection("col:1", "Tokens");
    col.hiddenFromPublishing = false;
    mockCollections["col:1"] = col;
    await expect(handleWriteRequest(makeRequest("update_variable_collection", [], {
      collectionId: "col:1",
      name: "New Tokens",
      hiddenFromPublishing: true,
      removeMode: "mode:default",
    }))).rejects.toThrow("Cannot remove mode (a collection must keep at least one mode)");
    expect(col.name).toBe("Tokens");
    expect(col.hiddenFromPublishing).toBe(false);
    expect(col.modes).toHaveLength(1);
    expect(commitUndoCalled).toBe(false);
  });
});
