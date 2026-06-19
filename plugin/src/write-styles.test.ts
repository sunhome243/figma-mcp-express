import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteRequest } from "./write-handlers";

// ── Figma global mock ─────────────────────────────────────────────────────────
//
// These tests cover the fork's design-system enhancements that span write-modify
// (set_fills/set_strokes/set_auto_layout/resize_nodes/set_corner_radius) and
// write-styles (bind_variable_to_node). All dispatch through handleWriteRequest,
// the top-level chain, so individual sub-handler file boundaries don't matter.

const SENTINEL_PAINT = { type: "SOLID", color: { r: 0, g: 0, b: 0 }, boundVariables: { color: "sentinel" } };

let mockNodes: Record<string, any>;
let mockVariables: Record<string, any>;
let commitUndoCalled: boolean;
let lastBoundVariableField: string | null;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  lastBoundVariableField = null;
  mockNodes = {};
  mockVariables = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
    variables: {
      getVariableByIdAsync: async (id: string) => mockVariables[id] ?? null,
      // Returns a recognisable sentinel so tests can prove the bound paint
      // (not the raw color paint) was written onto the node.
      setBoundVariableForPaint: (_base: any, field: string, _variable: any) => {
        lastBoundVariableField = field;
        return SENTINEL_PAINT;
      },
    },
    mixed: Symbol("mixed"),
  };
});

// ── set_fills ───────────────────────────────────────────────────────────────

describe("set_fills", () => {
  it("sets a raw color fill and emits the design-system warning", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", type: "RECTANGLE", fills: [] };
    const res = await handleWriteRequest(makeRequest("set_fills", ["1:1"], { color: "#FF0000" }));
    // Bulk-apply envelope: the warning rides inside the per-node result.
    expect(res?.data.results[0].warning).toContain("prefer variableId");
    expect(mockNodes["1:1"].fills).toHaveLength(1);
    expect(mockNodes["1:1"].fills[0].type).toBe("SOLID");
    expect(mockNodes["1:1"].fills[0].color.r).toBeCloseTo(1, 5);
    expect(commitUndoCalled).toBe(true);
  });

  it("binds a variable to the fill and omits the warning when variableId given", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", type: "RECTANGLE", fills: [] };
    mockVariables["var:color/1"] = { id: "var:color/1", name: "colors/primary" };
    const res = await handleWriteRequest(
      makeRequest("set_fills", ["1:1"], { variableId: "var:color/1" })
    );
    // Fork behavior: bound paint (sentinel) written, NO warning.
    expect(res?.data.results[0].warning).toBeUndefined();
    expect(mockNodes["1:1"].fills[0]).toBe(SENTINEL_PAINT);
    expect(lastBoundVariableField).toBe("color");
  });

  it("appends a fill when mode is append", async () => {
    const existing = { type: "SOLID", color: { r: 0, g: 0, b: 0 } };
    mockNodes["1:1"] = { id: "1:1", name: "Box", type: "RECTANGLE", fills: [existing] };
    await handleWriteRequest(makeRequest("set_fills", ["1:1"], { color: "#00FF00", mode: "append" }));
    expect(mockNodes["1:1"].fills).toHaveLength(2);
    expect(mockNodes["1:1"].fills[0]).toBe(existing);
  });

  it("throws when variableId is provided but the variable does not exist", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", type: "RECTANGLE", fills: [] };
    await expect(
      handleWriteRequest(makeRequest("set_fills", ["1:1"], { variableId: "var:missing" }))
    ).rejects.toThrow("Variable not found: var:missing");
  });

  it("collects a per-node error when a node does not support fills (no abort)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "NoFill", type: "SLICE" }; // no "fills" key
    const res = await handleWriteRequest(makeRequest("set_fills", ["1:1"], { color: "#000000" }));
    expect(res?.data.results[0].error).toContain("does not support fills");
  });

  it("throws (request-level) when nodeIds is empty", async () => {
    await expect(
      handleWriteRequest(makeRequest("set_fills", [], { color: "#000000" }))
    ).rejects.toThrow("nodeIds is required");
  });
});

// ── set_strokes ─────────────────────────────────────────────────────────────

describe("set_strokes", () => {
  it("sets a raw stroke and emits the design-system warning", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", type: "RECTANGLE", strokes: [] };
    const res = await handleWriteRequest(makeRequest("set_strokes", ["1:1"], { color: "#0000FF" }));
    // Bulk-apply envelope: the warning rides inside the per-node result (mirrors set_fills).
    expect(res?.data.results[0].warning).toContain("prefer variableId");
    expect(mockNodes["1:1"].strokes[0].color.b).toBeCloseTo(1, 5);
  });

  it("binds a variable to the stroke and applies strokeWeight without a warning", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", type: "RECTANGLE", strokes: [], strokeWeight: 0 };
    mockVariables["var:outline"] = { id: "var:outline", name: "colors/outline" };
    const res = await handleWriteRequest(
      makeRequest("set_strokes", ["1:1"], { variableId: "var:outline", strokeWeight: 2 })
    );
    expect(res?.data.results[0].warning).toBeUndefined();
    expect(mockNodes["1:1"].strokes[0]).toBe(SENTINEL_PAINT);
    expect(mockNodes["1:1"].strokeWeight).toBe(2);
    expect(lastBoundVariableField).toBe("color");
  });

  it("applies the same stroke to EVERY node in nodeIds (all→all bulk)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "A", type: "RECTANGLE", strokes: [] };
    mockNodes["2:2"] = { id: "2:2", name: "B", type: "RECTANGLE", strokes: [] };
    const res = await handleWriteRequest(makeRequest("set_strokes", ["1:1", "2:2"], { color: "#00FF00" }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
    expect(mockNodes["2:2"].strokes).toHaveLength(1);
  });

  it("reports partial success on a mix of valid + unsupported nodes (no abort)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "A", type: "RECTANGLE", strokes: [] };
    mockNodes["7:7"] = { id: "7:7", name: "Slice", type: "SLICE" }; // no "strokes" key
    const res = await handleWriteRequest(makeRequest("set_strokes", ["1:1", "9:9", "7:7"], { color: "#000000" }));
    expect(res?.data.results).toHaveLength(3);
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
    expect(res?.data.results[1].error).toBe("Node not found");
    expect(res?.data.results[2].error).toContain("does not support strokes");
  });

  it("collects a per-node error when a node does not support strokes (no abort)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Text", type: "SLICE" }; // no "strokes" key
    const res = await handleWriteRequest(makeRequest("set_strokes", ["1:1"], { color: "#000000" }));
    expect(res?.data.results[0].error).toContain("does not support strokes");
  });

  it("throws (request-level) when nodeIds is empty", async () => {
    await expect(
      handleWriteRequest(makeRequest("set_strokes", [], { color: "#000000" }))
    ).rejects.toThrow("nodeIds is required");
  });
});

// ── set_auto_layout ─────────────────────────────────────────────────────────

describe("set_auto_layout", () => {
  it("applies FILL/HUG/FIXED-driving layout props to a FRAME", async () => {
    mockNodes["1:1"] = {
      id: "1:1", name: "Row", type: "FRAME", layoutMode: "NONE",
      itemSpacing: 0, paddingLeft: 0,
    };
    const res = await handleWriteRequest(
      makeRequest("set_auto_layout", ["1:1"], {
        layoutMode: "HORIZONTAL",
        itemSpacing: 16,
        paddingLeft: 24,
        primaryAxisAlignItems: "SPACE_BETWEEN",
        counterAxisAlignItems: "CENTER",
      })
    );
    expect(res?.data.id).toBe("1:1");
    expect(mockNodes["1:1"].layoutMode).toBe("HORIZONTAL");
    expect(mockNodes["1:1"].itemSpacing).toBe(16);
    expect(mockNodes["1:1"].paddingLeft).toBe(24);
    expect(mockNodes["1:1"].primaryAxisAlignItems).toBe("SPACE_BETWEEN");
    expect(mockNodes["1:1"].counterAxisAlignItems).toBe("CENTER");
    expect(commitUndoCalled).toBe(true);
  });

  it("skips axis alignment when layoutMode resolves to NONE", async () => {
    // padding is applied unconditionally; axis-align props are gated on layoutMode !== NONE.
    mockNodes["1:1"] = {
      id: "1:1", name: "Shell", type: "FRAME", layoutMode: "NONE",
      paddingTop: 0,
    };
    await handleWriteRequest(
      makeRequest("set_auto_layout", ["1:1"], {
        paddingTop: 8,
        primaryAxisAlignItems: "CENTER",
      })
    );
    expect(mockNodes["1:1"].paddingTop).toBe(8);
    expect(mockNodes["1:1"].primaryAxisAlignItems).toBeUndefined();
  });

  it("applies WRAP counterAxisSpacing only when layoutWrap is WRAP", async () => {
    mockNodes["1:1"] = {
      id: "1:1", name: "Grid", type: "FRAME", layoutMode: "HORIZONTAL",
      counterAxisSpacing: 0,
    };
    await handleWriteRequest(
      makeRequest("set_auto_layout", ["1:1"], { layoutWrap: "WRAP", counterAxisSpacing: 12 })
    );
    expect(mockNodes["1:1"].layoutWrap).toBe("WRAP");
    expect(mockNodes["1:1"].counterAxisSpacing).toBe(12);
  });

  it("throws when node is not a FRAME", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Inst", type: "INSTANCE" };
    await expect(
      handleWriteRequest(makeRequest("set_auto_layout", ["1:1"], { layoutMode: "VERTICAL" }))
    ).rejects.toThrow("is not a FRAME");
  });
});

// ── resize_nodes ────────────────────────────────────────────────────────────

describe("resize_nodes", () => {
  const makeResizable = (id: string) => {
    let hSizing: string | undefined;
    return {
      id, name: `node-${id}`, width: 100, height: 100,
      resize(w: number, h: number) { this.width = w; this.height = h; },
      get layoutSizingHorizontal() { return hSizing; },
      set layoutSizingHorizontal(v: any) { hSizing = v; },
      set layoutSizingVertical(_v: any) {},
    };
  };

  it("resizes a node to explicit width/height", async () => {
    mockNodes["1:1"] = makeResizable("1:1");
    const res = await handleWriteRequest(
      makeRequest("resize_nodes", ["1:1"], { width: 320, height: 64 })
    );
    expect(res?.data.results[0]).toEqual({ nodeId: "1:1", width: 320, height: 64 });
  });

  it("applies layoutSizingHorizontal FILL without forcing a px resize", async () => {
    const node = makeResizable("1:1");
    mockNodes["1:1"] = node;
    const res = await handleWriteRequest(
      makeRequest("resize_nodes", ["1:1"], { layoutSizingHorizontal: "FILL" })
    );
    expect(node.layoutSizingHorizontal).toBe("FILL");
    // No width/height given → original dimensions preserved.
    expect(res?.data.results[0].width).toBe(100);
  });

  it("reports a per-node error when layoutSizing throws (no auto-layout parent)", async () => {
    const node: any = {
      id: "1:1", name: "orphan", width: 100, height: 100,
      resize() {},
      set layoutSizingHorizontal(_v: any) {
        throw new Error("layoutSizing requires an auto-layout parent");
      },
    };
    mockNodes["1:1"] = node;
    const res = await handleWriteRequest(
      makeRequest("resize_nodes", ["1:1"], { layoutSizingHorizontal: "FILL" })
    );
    expect(res?.data.results[0].error).toContain("layoutSizing failed");
  });

  it("reports an error for a node that does not support resize", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "page", type: "PAGE" }; // no "resize" key
    const res = await handleWriteRequest(
      makeRequest("resize_nodes", ["1:1"], { width: 50 })
    );
    expect(res?.data.results[0].error).toBe("Node does not support resize");
  });
});

// ── set_corner_radius ───────────────────────────────────────────────────────

describe("set_corner_radius", () => {
  it("sets a uniform corner radius", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card", cornerRadius: 0 };
    const res = await handleWriteRequest(
      makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 8 })
    );
    expect(mockNodes["1:1"].cornerRadius).toBe(8);
    expect(res?.data.results[0].cornerRadius).toBe(8);
  });

  it("sets per-corner radii from an object of individual corners", async () => {
    mockNodes["1:1"] = {
      id: "1:1", name: "Card", cornerRadius: 0,
      topLeftRadius: 0, topRightRadius: 0, bottomLeftRadius: 0, bottomRightRadius: 0,
    };
    await handleWriteRequest(
      makeRequest("set_corner_radius", ["1:1"], {
        topLeftRadius: 12, topRightRadius: 12, bottomLeftRadius: 4, bottomRightRadius: 4,
      })
    );
    expect(mockNodes["1:1"].topLeftRadius).toBe(12);
    expect(mockNodes["1:1"].bottomRightRadius).toBe(4);
  });

  it("reports an error for a node that does not support corner radius", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Line", type: "LINE" }; // no "cornerRadius" key
    const res = await handleWriteRequest(
      makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 4 })
    );
    expect(res?.data.results[0].error).toBe("Node does not support corner radius");
  });
});

// ── bind_variable_to_node (write-styles) ────────────────────────────────────

describe("bind_variable_to_node", () => {
  it("binds a color variable to a node fill via setBoundVariableForPaint", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [] };
    mockVariables["var:bg"] = { id: "var:bg", name: "colors/background" };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:bg", field: "fillColor" })
    );
    expect(mockNodes["1:1"].fills[0]).toBe(SENTINEL_PAINT);
    expect(res?.data.results[0].field).toBe("fillColor");
    expect(lastBoundVariableField).toBe("color");
  });

  it("binds a non-color field via node.setBoundVariable", async () => {
    let boundField: string | null = null;
    let boundVar: any = null;
    mockNodes["1:1"] = {
      id: "1:1", name: "Frame", itemSpacing: 0,
      setBoundVariable(field: string, variable: any) { boundField = field; boundVar = variable; },
    };
    mockVariables["var:gap"] = { id: "var:gap", name: "spacing/4" };
    await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:gap", field: "itemSpacing" })
    );
    expect(boundField).toBe("itemSpacing");
    expect(boundVar?.id).toBe("var:gap");
  });

  it("throws when the variable does not exist", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [] };
    await expect(
      handleWriteRequest(
        makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:missing", field: "fillColor" })
      )
    ).rejects.toThrow("Variable not found");
  });

  // ── Multi-paint preservation ───────────────────────────────────────────────
  it("fillColor bind preserves fills[1..N] — only index 0 is bound", async () => {
    const fill1 = { type: "SOLID", color: { r: 1, g: 0, b: 0 } };
    const fill2 = { type: "SOLID", color: { r: 0, g: 1, b: 0 } };
    mockNodes["1:1"] = { id: "1:1", name: "Multi", fills: [fill1, fill2] };
    mockVariables["var:c"] = { id: "var:c", name: "colors/primary" };
    await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:c", field: "fillColor" })
    );
    expect(mockNodes["1:1"].fills).toHaveLength(2);
    expect(mockNodes["1:1"].fills[0]).toBe(SENTINEL_PAINT); // bound
    expect(mockNodes["1:1"].fills[1]).toBe(fill2);          // preserved
  });

  it("strokeColor bind preserves strokes[1..N]", async () => {
    const s1 = { type: "SOLID", color: { r: 1, g: 0, b: 0 } };
    const s2 = { type: "SOLID", color: { r: 0, g: 0, b: 1 } };
    mockNodes["1:1"] = { id: "1:1", name: "Multi", strokes: [s1, s2] };
    mockVariables["var:c"] = { id: "var:c", name: "colors/stroke" };
    await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:c", field: "strokeColor" })
    );
    expect(mockNodes["1:1"].strokes).toHaveLength(2);
    expect(mockNodes["1:1"].strokes[0]).toBe(SENTINEL_PAINT);
    expect(mockNodes["1:1"].strokes[1]).toBe(s2);
  });

  it("throws when fills[0] is not SOLID", async () => {
    const gradFill = { type: "GRADIENT_LINEAR", gradientStops: [] };
    mockNodes["1:1"] = { id: "1:1", name: "Grad", fills: [gradFill] };
    mockVariables["var:c"] = { id: "var:c", name: "colors/primary" };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:c", field: "fillColor" })
    );
    // per-node error (not request-level throw)
    expect(res?.data.results[0].error).toContain("SOLID");
  });

  // ── Type-compatibility check for scalar fields ─────────────────────────────
  it("throws per-node error when FLOAT variable bound to visible (needs BOOLEAN)", async () => {
    let boundField: string | null = null;
    mockNodes["1:1"] = {
      id: "1:1", name: "Frame", visible: true,
      setBoundVariable(f: string, _v: any) { boundField = f; },
    };
    mockVariables["var:float"] = { id: "var:float", name: "spacing/4", resolvedType: "FLOAT" };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "visible" })
    );
    expect(res?.data.results[0].error).toContain("BOOLEAN");
    expect(boundField).toBeNull(); // setBoundVariable must NOT have been called
  });

  it("throws per-node error when BOOLEAN variable bound to itemSpacing (needs FLOAT)", async () => {
    let boundField: string | null = null;
    mockNodes["1:1"] = {
      id: "1:1", name: "Frame", itemSpacing: 0,
      setBoundVariable(f: string, _v: any) { boundField = f; },
    };
    mockVariables["var:bool"] = { id: "var:bool", name: "flags/show", resolvedType: "BOOLEAN" };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:bool", field: "itemSpacing" })
    );
    expect(res?.data.results[0].error).toContain("FLOAT");
    expect(boundField).toBeNull();
  });

  it("throws per-node error when non-COLOR variable bound to fillColor (needs COLOR)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 } }] };
    mockVariables["var:float"] = { id: "var:float", name: "spacing/4", resolvedType: "FLOAT" };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "fillColor" })
    );
    expect(res?.data.results[0].error).toContain("COLOR");
  });

  it("rejects a field outside the VariableBindableNodeField allowlist up front", async () => {
    let boundField: string | null = null;
    mockNodes["1:1"] = {
      id: "1:1", name: "Frame", customProp: 0,
      setBoundVariable(f: string, _v: any) { boundField = f; },
    };
    mockVariables["var:x"] = { id: "var:x", name: "custom/val", resolvedType: "FLOAT" };
    // An unlisted field would throw at setBoundVariable on the real API, so we reject it
    // up front rather than letting it through a permissive fallback.
    await expect(handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:x", field: "customProp" })
    )).rejects.toThrow("is not a bindable node field");
    expect(boundField).toBeNull();
  });
});

// ── set_effects (write-styles fixes) ────────────────────────────────────────

describe("set_effects (fixes)", () => {
  const makeEffectNode = (id: string) => ({
    id, name: `node-${id}`, effects: [] as any[],
  });

  // ── showShadowBehindNode handling ─────────────────────────────────────────
  it("DROP_SHADOW preserves showShadowBehindNode=true when provided", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{
        type: "DROP_SHADOW", color: "#000000", opacity: 0.3,
        offsetX: 0, offsetY: 4, radius: 8, spread: 0,
        showShadowBehindNode: true,
      }],
    }));
    expect(mockNodes["1:1"].effects[0].showShadowBehindNode).toBe(true);
  });

  it("DROP_SHADOW defaults showShadowBehindNode to false when omitted", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "DROP_SHADOW", color: "#000000", radius: 4 }],
    }));
    expect(mockNodes["1:1"].effects[0].showShadowBehindNode).toBe(false);
  });

  it("INNER_SHADOW does NOT get showShadowBehindNode", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "INNER_SHADOW", color: "#000000", radius: 4 }],
    }));
    expect("showShadowBehindNode" in mockNodes["1:1"].effects[0]).toBe(false);
  });

  // ── Bulk apply (all nodeIds) ───────────────────────────────────────────────
  it("set_effects applies to ALL nodeIds and returns per-node results", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    mockNodes["2:2"] = makeEffectNode("2:2");
    const res = await handleWriteRequest(makeRequest("set_effects", ["1:1", "2:2"], {
      effects: [{ type: "DROP_SHADOW", color: "#000000", radius: 4 }],
    }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].effects).toHaveLength(1);
    expect(mockNodes["2:2"].effects).toHaveLength(1);
  });

  it("set_effects reports per-node error for missing node without aborting others", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    const res = await handleWriteRequest(makeRequest("set_effects", ["1:1", "9:9"], {
      effects: [{ type: "LAYER_BLUR", radius: 4 }],
    }));
    expect(res?.data.results).toHaveLength(2);
    expect(res?.data.results[0].error).toBeUndefined();
    expect(res?.data.results[1].error).toBe("Node not found");
    expect(mockNodes["1:1"].effects).toHaveLength(1);
  });
});

// ── create_text_style (textCase/paragraphSpacing/paragraphIndent) ────────────

describe("create_text_style — extended typography properties", () => {
  beforeEach(() => {
    // Add mock Figma text style creation support
    (globalThis as any).figma.loadFontAsync = async () => {};
    (globalThis as any).figma.getLocalTextStylesAsync = async () => [];
    const capturedStyle: any = {};
    (globalThis as any).figma.createTextStyle = () => {
      return new Proxy(capturedStyle, {
        set(t, k, v) { t[k as string] = v; return true; },
        get(t, k) { return t[k as string]; },
      });
    };
    (globalThis as any)._capturedTextStyle = capturedStyle;
  });

  it("applies textCase when provided", async () => {
    await handleWriteRequest(makeRequest("create_text_style", [], {
      name: "Heading/H1", textCase: "UPPER",
    }));
    expect((globalThis as any)._capturedTextStyle.textCase).toBe("UPPER");
  });

  it("applies paragraphSpacing when provided", async () => {
    await handleWriteRequest(makeRequest("create_text_style", [], {
      name: "Body/Text", paragraphSpacing: 12,
    }));
    expect((globalThis as any)._capturedTextStyle.paragraphSpacing).toBe(12);
  });

  it("applies paragraphIndent when provided", async () => {
    await handleWriteRequest(makeRequest("create_text_style", [], {
      name: "Body/Indent", paragraphIndent: 16,
    }));
    expect((globalThis as any)._capturedTextStyle.paragraphIndent).toBe(16);
  });

  it("skips textCase/paragraphSpacing/paragraphIndent when not provided", async () => {
    await handleWriteRequest(makeRequest("create_text_style", [], {
      name: "Plain/Style",
    }));
    const s = (globalThis as any)._capturedTextStyle;
    expect(s.textCase).toBeUndefined();
    expect(s.paragraphSpacing).toBeUndefined();
    expect(s.paragraphIndent).toBeUndefined();
  });
});

// ── create_effect_style (multi-effect + blurType discriminant) ───────────────

describe("create_effect_style — multi-effect array", () => {
  beforeEach(() => {
    (globalThis as any).figma.getLocalEffectStylesAsync = async () => [];
    const capturedStyle: any = {};
    (globalThis as any).figma.createEffectStyle = () => {
      return new Proxy(capturedStyle, {
        set(t, k, v) { t[k as string] = v; return true; },
        get(t, k) { return t[k as string]; },
      });
    };
    (globalThis as any)._capturedEffectStyle = capturedStyle;
  });

  it("accepts effects[] array and creates multi-effect style", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "Complex/Shadow",
      effects: [
        { type: "DROP_SHADOW", color: "#000000", opacity: 0.2, offsetX: 0, offsetY: 2, radius: 4, spread: 0 },
        { type: "DROP_SHADOW", color: "#000000", opacity: 0.1, offsetX: 0, offsetY: 8, radius: 16, spread: 0 },
      ],
    }));
    expect((globalThis as any)._capturedEffectStyle.effects).toHaveLength(2);
  });

  it("LAYER_BLUR sets the required blurType discriminant to NORMAL", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "Blur/Card",
      type: "LAYER_BLUR",
      radius: 8,
    }));
    const effect = (globalThis as any)._capturedEffectStyle.effects[0];
    expect(effect.blurType).toBe("NORMAL");
  });

  it("BACKGROUND_BLUR sets the required blurType discriminant to NORMAL", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "BgBlur/Frosted",
      type: "BACKGROUND_BLUR",
      radius: 20,
    }));
    const effect = (globalThis as any)._capturedEffectStyle.effects[0];
    expect(effect.blurType).toBe("NORMAL");
  });

  it("single-effect shorthand still works (backward compat)", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "Shadow/Card",
      type: "DROP_SHADOW",
      color: "#000000",
      opacity: 0.25,
      offsetY: 4,
      radius: 8,
    }));
    expect((globalThis as any)._capturedEffectStyle.effects).toHaveLength(1);
    expect((globalThis as any)._capturedEffectStyle.effects[0].type).toBe("DROP_SHADOW");
  });

  it("single-effect shorthand creates native GLASS effects", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "Glass/Frosted",
      type: "GLASS",
      lightIntensity: 0.7,
      lightAngle: 120,
      refraction: 0.4,
      depth: 12,
      dispersion: 0.2,
      radius: 18,
    }));
    const effect = (globalThis as any)._capturedEffectStyle.effects[0];
    expect(effect.type).toBe("GLASS");
    expect(effect.lightIntensity).toBe(0.7);
    expect(effect.depth).toBe(12);
  });

  it("effects[] creates native NOISE and TEXTURE effects with noiseSizeVector", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "Noise/Texture",
      effects: [
        { type: "TEXTURE", noiseSize: 2, noiseSizeVector: { x: 2, y: 5 }, clipToShape: false },
        { type: "NOISE", noiseType: "MULTITONE", opacity: 0.4, noiseSizeVector: { x: 3, y: 7 } },
      ],
    }));
    const [texture, noise] = (globalThis as any)._capturedEffectStyle.effects;
    expect(texture.type).toBe("TEXTURE");
    expect(texture.noiseSizeVector).toEqual({ x: 2, y: 5 });
    expect(texture.clipToShape).toBe(false);
    expect(noise.type).toBe("NOISE");
    expect(noise.noiseType).toBe("MULTITONE");
    expect(noise.noiseSizeVector).toEqual({ x: 3, y: 7 });
  });

  it("single-effect shorthand creates PROGRESSIVE blur styles", async () => {
    await handleWriteRequest(makeRequest("create_effect_style", [], {
      name: "Blur/Progressive",
      type: "BACKGROUND_BLUR",
      blurType: "PROGRESSIVE",
      startRadius: 2,
      radius: 20,
      startOffset: { x: 0.5, y: 0 },
      endOffset: { x: 0.5, y: 1 },
    }));
    const effect = (globalThis as any)._capturedEffectStyle.effects[0];
    expect(effect.type).toBe("BACKGROUND_BLUR");
    expect(effect.blurType).toBe("PROGRESSIVE");
    expect(effect.startRadius).toBe(2);
    expect(effect.endOffset).toEqual({ x: 0.5, y: 1 });
  });
});

// ── create_paint_style / update_paint_style (multi-paint paints[]) ───────────

describe("create_paint_style / update_paint_style — paints[] array support", () => {
  beforeEach(() => {
    (globalThis as any).figma.getLocalPaintStylesAsync = async () => [];
    const capturedStyle: any = {};
    (globalThis as any).figma.createPaintStyle = () => {
      return new Proxy(capturedStyle, {
        set(t, k, v) { t[k as string] = v; return true; },
        get(t, k) { return t[k as string]; },
      });
    };
    (globalThis as any).figma.getStyleByIdAsync = async (id: string) => {
      if (id === "style:paint:1") {
        const s: any = { id, type: "PAINT", name: "Old/Color", description: "" };
        return s;
      }
      return null;
    };
    (globalThis as any)._capturedPaintStyle = capturedStyle;
  });

  it("create_paint_style accepts paints[] array (gradient/image/solid passthrough)", async () => {
    const gradPaint = {
      type: "GRADIENT_LINEAR",
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [
        { position: 0, color: { r: 1, g: 0, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 1, a: 1 } },
      ],
    };
    await handleWriteRequest(makeRequest("create_paint_style", [], {
      name: "Gradient/Brand",
      paints: [gradPaint],
    }));
    expect((globalThis as any)._capturedPaintStyle.paints).toHaveLength(1);
    expect((globalThis as any)._capturedPaintStyle.paints[0].type).toBe("GRADIENT_LINEAR");
  });

  it("create_paint_style paints[] overrides color shorthand when both given", async () => {
    const solidPaint = { type: "SOLID", color: { r: 0, g: 1, b: 0 } };
    await handleWriteRequest(makeRequest("create_paint_style", [], {
      name: "Override/Test",
      color: "#FF0000",
      paints: [solidPaint],
    }));
    // paints[] should take precedence
    expect((globalThis as any)._capturedPaintStyle.paints[0].type).toBe("SOLID");
    expect((globalThis as any)._capturedPaintStyle.paints[0].color.r).toBeCloseTo(0, 5);
    expect((globalThis as any)._capturedPaintStyle.paints[0].color.g).toBeCloseTo(1, 5);
  });

  it("create_paint_style falls back to color shorthand when paints[] not given", async () => {
    await handleWriteRequest(makeRequest("create_paint_style", [], {
      name: "Solid/Red",
      color: "#FF0000",
    }));
    expect((globalThis as any)._capturedPaintStyle.paints).toHaveLength(1);
    expect((globalThis as any)._capturedPaintStyle.paints[0].type).toBe("SOLID");
  });

  it("update_paint_style accepts paints[] array", async () => {
    const gradPaint = {
      type: "GRADIENT_LINEAR",
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [],
    };
    // update_paint_style uses getStyleByIdAsync — need capturable style
    const mutableStyle: any = { id: "style:paint:1", type: "PAINT", name: "Old", description: "" };
    (globalThis as any).figma.getStyleByIdAsync = async (id: string) => id === "style:paint:1" ? mutableStyle : null;
    await handleWriteRequest(makeRequest("update_paint_style", [], {
      styleId: "style:paint:1",
      paints: [gradPaint],
    }));
    expect(mutableStyle.paints).toHaveLength(1);
    expect(mutableStyle.paints[0].type).toBe("GRADIENT_LINEAR");
  });
});

// ── TASK 3: bind_variable_to_node — non-bindable field rejection ──────────────

describe("bind_variable_to_node — non-bindable field rejection", () => {
  beforeEach(() => {
    mockVariables["var:float"] = { id: "var:float", name: "spacing/4", resolvedType: "FLOAT" };
    mockNodes["1:1"] = {
      id: "1:1", name: "Frame",
      cornerRadius: 0, rotation: 0, x: 0, y: 0,
      paddingLeft: 0, topLeftRadius: 0,
      setBoundVariable(_field: string, _variable: any) {},
    };
  });

  it("throws request-level error for cornerRadius (not in bindable allowlist)", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "cornerRadius" }),
      ),
    ).rejects.toThrow(/is not a bindable node field/);
  });

  it("error for cornerRadius mentions per-corner alternatives", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "cornerRadius" }),
      ),
    ).rejects.toThrow(/topLeftRadius|topRightRadius|bottomLeftRadius|bottomRightRadius/);
  });

  it("throws request-level error for rotation (not bindable)", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "rotation" }),
      ),
    ).rejects.toThrow(/is not a bindable node field/);
  });

  it("throws request-level error for x (not bindable)", async () => {
    await expect(
      handleWriteRequest(
        makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "x" }),
      ),
    ).rejects.toThrow(/is not a bindable node field/);
  });

  it("does NOT throw for paddingLeft (valid FLOAT field in allowlist)", async () => {
    let boundField: string | null = null;
    mockNodes["1:1"].setBoundVariable = (f: string, _v: any) => { boundField = f; };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "paddingLeft" }),
    );
    expect(res?.data.results[0].error).toBeUndefined();
    expect(boundField).toBe("paddingLeft");
  });

  it("does NOT throw for topLeftRadius (valid per-corner FLOAT field in allowlist)", async () => {
    let boundField: string | null = null;
    mockNodes["1:1"].setBoundVariable = (f: string, _v: any) => { boundField = f; };
    const res = await handleWriteRequest(
      makeRequest("bind_variable_to_node", ["1:1"], { variableId: "var:float", field: "topLeftRadius" }),
    );
    expect(res?.data.results[0].error).toBeUndefined();
    expect(boundField).toBe("topLeftRadius");
  });
});

// ── TASK 4: set_effects — blur effects include blurType:"NORMAL" discriminant ──

describe("set_effects — LAYER_BLUR and BACKGROUND_BLUR include blurType:NORMAL", () => {
  const makeEffectNode = (id: string) => ({
    id, name: `node-${id}`, effects: [] as any[],
  });

  it("LAYER_BLUR effect applied to node has blurType:'NORMAL'", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "LAYER_BLUR", radius: 10 }],
    }));
    const effect = mockNodes["1:1"].effects[0];
    expect(effect.type).toBe("LAYER_BLUR");
    expect(effect.blurType).toBe("NORMAL");
  });

  it("BACKGROUND_BLUR effect applied to node has blurType:'NORMAL'", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "BACKGROUND_BLUR", radius: 20 }],
    }));
    const effect = mockNodes["1:1"].effects[0];
    expect(effect.type).toBe("BACKGROUND_BLUR");
    expect(effect.blurType).toBe("NORMAL");
  });

  it("LAYER_BLUR blurType is NORMAL regardless of radius value", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "LAYER_BLUR", radius: 4 }],
    }));
    expect(mockNodes["1:1"].effects[0].blurType).toBe("NORMAL");
  });

  it("mixed effects — blur has blurType, shadow does not have blurType", async () => {
    mockNodes["1:1"] = makeEffectNode("1:1");
    await handleWriteRequest(makeRequest("set_effects", ["1:1"], {
      effects: [
        { type: "DROP_SHADOW", color: "#000000", radius: 4 },
        { type: "LAYER_BLUR", radius: 8 },
      ],
    }));
    expect(mockNodes["1:1"].effects).toHaveLength(2);
    const shadow = mockNodes["1:1"].effects[0];
    const blur = mockNodes["1:1"].effects[1];
    expect("blurType" in shadow).toBe(false); // DROP_SHADOW has no blurType
    expect(blur.blurType).toBe("NORMAL");
  });
});
