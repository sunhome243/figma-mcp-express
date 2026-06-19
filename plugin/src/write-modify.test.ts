import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteModifyRequest } from "./write-modify";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── set_fills ─────────────────────────────────────────────────────────────────

describe("set_fills", () => {
  it("binds a variable when variableId is provided", async () => {
    let boundCall: any = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      variables: {
        getVariableByIdAsync: async (id: string) => ({ id, name: "colors/primary" }),
        setBoundVariableForPaint: (paint: any, field: string, variable: any) => {
          boundCall = { paint, field, variable };
          return { ...paint, boundVariables: { [field]: { type: "VARIABLE_ALIAS", id: variable.id } } };
        },
      },
    };
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { variableId: "VariableID:1:5", color: "#ff0000" }),
    );
    expect(boundCall.field).toBe("color");
    expect(boundCall.variable.id).toBe("VariableID:1:5");
    expect(mockNodes["1:1"].fills[0].boundVariables.color.id).toBe("VariableID:1:5");
    expect(res?.data.results[0].warning).toBeUndefined();
    expect(commitUndoCalled).toBe(true);
  });

  it("returns a warning when a raw color is used without variableId", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { color: "#00ff00" }),
    );
    expect(res?.data.results[0].warning).toContain("raw color used");
    expect(mockNodes["1:1"].fills).toHaveLength(1);
  });

  it("throws when variableId resolves to no variable (no silent unbind)", async () => {
    let bindCalled = false;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      variables: {
        getVariableByIdAsync: async () => null,
        setBoundVariableForPaint: () => {
          bindCalled = true;
          return {};
        },
      },
    };
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [] };
    await expect(
      handleWriteModifyRequest(makeRequest("set_fills", ["1:1"], { variableId: "VariableID:bad" })),
    ).rejects.toThrow("Variable not found: VariableID:bad");
    expect(bindCalled).toBe(false);
    expect(mockNodes["1:1"].fills).toHaveLength(0);
  });

  it("applies the same fill to EVERY node in nodeIds (all→all bulk)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "A", fills: [] };
    mockNodes["2:2"] = { id: "2:2", name: "B", fills: [] };
    mockNodes["3:3"] = { id: "3:3", name: "NoFill" }; // no fills key → per-node error
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1", "2:2", "3:3"], { color: "#112233" }),
    );
    expect(res?.data.results).toHaveLength(3);
    expect(mockNodes["1:1"].fills).toHaveLength(1);
    expect(mockNodes["2:2"].fills).toHaveLength(1);
    expect(res?.data.results[2].error).toContain("does not support fills");
  });
});

// ── set_strokes ───────────────────────────────────────────────────────────────

describe("set_strokes", () => {
  it("binds a variable when variableId is provided", async () => {
    let boundCall: any = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      variables: {
        getVariableByIdAsync: async (id: string) => ({ id, name: "colors/outline" }),
        setBoundVariableForPaint: (paint: any, field: string, variable: any) => {
          boundCall = { paint, field, variable };
          return { ...paint, boundVariables: { [field]: { type: "VARIABLE_ALIAS", id: variable.id } } };
        },
      },
    };
    mockNodes["1:1"] = { id: "1:1", name: "Rect", strokes: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { variableId: "VariableID:2:7", color: "#000000" }),
    );
    expect(boundCall.field).toBe("color");
    expect(boundCall.variable.id).toBe("VariableID:2:7");
    expect(mockNodes["1:1"].strokes[0].boundVariables.color.id).toBe("VariableID:2:7");
    expect(res?.data.results[0].warning).toBeUndefined();
    expect(commitUndoCalled).toBe(true);
  });

  it("returns a warning when a raw color is used without variableId", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", strokes: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { color: "#123456" }),
    );
    expect(res?.data.results[0].warning).toContain("raw color used");
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
  });
});

// ── set_opacity ───────────────────────────────────────────────────────────────

describe("set_opacity", () => {
  it("sets opacity on a node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0.5 }));
    expect(res?.data.results[0].opacity).toBe(0.5);
    expect(mockNodes["1:1"].opacity).toBe(0.5);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets opacity to 0", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0 }));
    expect(res?.data.results[0].opacity).toBe(0);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["9:9"], { opacity: 0.5 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without opacity support", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Page" }; // no opacity property
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0.5 }));
    expect(res?.data.results[0].error).toContain("does not support opacity");
  });

  it("handles multiple nodeIds", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    mockNodes["2:2"] = { id: "2:2", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1", "2:2"], { opacity: 0.25 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].opacity).toBe(0.25);
    expect(mockNodes["2:2"].opacity).toBe(0.25);
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("set_opacity", [], { opacity: 0.5 }))).rejects.toThrow();
  });
});

// ── set_corner_radius ─────────────────────────────────────────────────────────

describe("set_corner_radius", () => {
  it("sets uniform cornerRadius", async () => {
    mockNodes["1:1"] = { id: "1:1", cornerRadius: 0 };
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 8 }));
    expect(mockNodes["1:1"].cornerRadius).toBe(8);
    expect(res?.data.results[0].cornerRadius).toBe(8);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets per-corner radii independently", async () => {
    mockNodes["1:1"] = {
      id: "1:1", cornerRadius: 0,
      topLeftRadius: 0, topRightRadius: 0, bottomLeftRadius: 0, bottomRightRadius: 0,
    };
    await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], {
      topLeftRadius: 8, topRightRadius: 0, bottomLeftRadius: 8, bottomRightRadius: 0,
    }));
    expect(mockNodes["1:1"].topLeftRadius).toBe(8);
    expect(mockNodes["1:1"].topRightRadius).toBe(0);
    expect(mockNodes["1:1"].bottomLeftRadius).toBe(8);
    expect(mockNodes["1:1"].bottomRightRadius).toBe(0);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["9:9"], { cornerRadius: 4 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without cornerRadius support", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Text" }; // no cornerRadius property
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 4 }));
    expect(res?.data.results[0].error).toContain("does not support corner radius");
  });

  it("handles multiple nodeIds", async () => {
    mockNodes["1:1"] = { id: "1:1", cornerRadius: 0 };
    mockNodes["2:2"] = { id: "2:2", cornerRadius: 0 };
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1", "2:2"], { cornerRadius: 12 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].cornerRadius).toBe(12);
    expect(mockNodes["2:2"].cornerRadius).toBe(12);
  });

  it("returns null for unrecognised type", async () => {
    const res = await handleWriteModifyRequest(makeRequest("unknown_op"));
    expect(res).toBeNull();
  });
});

// ── set_visible ───────────────────────────────────────────────────────────────

describe("set_visible", () => {
  it("hides a node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", visible: true };
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1"], { visible: false }));
    expect(mockNodes["1:1"].visible).toBe(false);
    expect(res?.data.results[0].visible).toBe(false);
    expect(commitUndoCalled).toBe(true);
  });

  it("shows a hidden node", async () => {
    mockNodes["1:1"] = { id: "1:1", visible: false };
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1"], { visible: true }));
    expect(mockNodes["1:1"].visible).toBe(true);
    expect(res?.data.results[0].visible).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["9:9"], { visible: false }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without visibility support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no visible property
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1"], { visible: false }));
    expect(res?.data.results[0].error).toContain("does not support visibility");
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", visible: true };
    mockNodes["2:2"] = { id: "2:2", visible: true };
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1", "2:2"], { visible: false }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].visible).toBe(false);
    expect(mockNodes["2:2"].visible).toBe(false);
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("set_visible", [], { visible: false }))).rejects.toThrow();
  });
});

// ── lock_nodes / unlock_nodes ─────────────────────────────────────────────────

describe("lock_nodes", () => {
  it("locks a node", async () => {
    mockNodes["1:1"] = { id: "1:1", locked: false };
    const res = await handleWriteModifyRequest(makeRequest("lock_nodes", ["1:1"]));
    expect(mockNodes["1:1"].locked).toBe(true);
    expect(res?.data.results[0].locked).toBe(true);
    expect(commitUndoCalled).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("lock_nodes", ["9:9"]));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without locked support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no locked property
    const res = await handleWriteModifyRequest(makeRequest("lock_nodes", ["1:1"]));
    expect(res?.data.results[0].error).toContain("does not support locking");
  });
});

describe("unlock_nodes", () => {
  it("unlocks a node", async () => {
    mockNodes["1:1"] = { id: "1:1", locked: true };
    const res = await handleWriteModifyRequest(makeRequest("unlock_nodes", ["1:1"]));
    expect(mockNodes["1:1"].locked).toBe(false);
    expect(res?.data.results[0].locked).toBe(false);
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", locked: true };
    mockNodes["2:2"] = { id: "2:2", locked: true };
    const res = await handleWriteModifyRequest(makeRequest("unlock_nodes", ["1:1", "2:2"]));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].locked).toBe(false);
    expect(mockNodes["2:2"].locked).toBe(false);
  });
});

// ── rotate_nodes ──────────────────────────────────────────────────────────────

describe("rotate_nodes", () => {
  it("rotates a node", async () => {
    mockNodes["1:1"] = { id: "1:1", rotation: 0 };
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1"], { rotation: 45 }));
    expect(mockNodes["1:1"].rotation).toBe(45);
    expect(res?.data.results[0].rotation).toBe(45);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets negative rotation", async () => {
    mockNodes["1:1"] = { id: "1:1", rotation: 0 };
    await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1"], { rotation: -90 }));
    expect(mockNodes["1:1"].rotation).toBe(-90);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["9:9"], { rotation: 45 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without rotation support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no rotation property
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1"], { rotation: 45 }));
    expect(res?.data.results[0].error).toContain("does not support rotation");
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", rotation: 0 };
    mockNodes["2:2"] = { id: "2:2", rotation: 0 };
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1", "2:2"], { rotation: 90 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].rotation).toBe(90);
    expect(mockNodes["2:2"].rotation).toBe(90);
  });
});

// ── reorder_nodes ─────────────────────────────────────────────────────────────

describe("reorder_nodes", () => {
  const makeParent = (children: any[]) => ({
    children,
    insertChild(index: number, child: any) {
      const i = this.children.indexOf(child);
      if (i !== -1) this.children.splice(i, 1);
      this.children.splice(index, 0, child);
    },
  });

  it("brings node to front", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    parent.children = [nodeA, nodeB];
    mockNodes["1:1"] = nodeA;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "bringToFront" }));
    expect(res?.data.results[0].index).toBe(1);
    expect(parent.children[1]).toBe(nodeA);
    expect(commitUndoCalled).toBe(true);
  });

  it("sends node to back", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    parent.children = [nodeA, nodeB];
    mockNodes["2:2"] = nodeB;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["2:2"], { order: "sendToBack" }));
    expect(res?.data.results[0].index).toBe(0);
    expect(parent.children[0]).toBe(nodeB);
  });

  it("brings forward one step", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    const nodeC = { id: "3:3", parent };
    parent.children = [nodeA, nodeB, nodeC];
    mockNodes["1:1"] = nodeA;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "bringForward" }));
    expect(res?.data.results[0].index).toBe(1);
  });

  it("sends backward one step", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    parent.children = [nodeA, nodeB];
    mockNodes["2:2"] = nodeB;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["2:2"], { order: "sendBackward" }));
    expect(res?.data.results[0].index).toBe(0);
  });

  it("throws for invalid order", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    await expect(handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "invalid" }))).rejects.toThrow();
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["9:9"], { order: "bringToFront" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without parent", async () => {
    mockNodes["1:1"] = { id: "1:1", parent: null };
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "bringToFront" }));
    expect(res?.data.results[0].error).toContain("no reorderable parent");
  });
});

// ── set_blend_mode ────────────────────────────────────────────────────────────

describe("resize_nodes — min/max constraints", () => {
  it("sets min/max width/height on a node", async () => {
    mockNodes["1:1"] = { id: "1:1", width: 100, height: 100, resize: function (w: number, h: number) { this.width = w; this.height = h; }, minWidth: null, maxWidth: null, minHeight: null, maxHeight: null };
    const res = await handleWriteModifyRequest(makeRequest("resize_nodes", ["1:1"], { minWidth: 50, maxWidth: 300, minHeight: 40, maxHeight: 600 }));
    expect(mockNodes["1:1"].minWidth).toBe(50);
    expect(mockNodes["1:1"].maxWidth).toBe(300);
    expect(mockNodes["1:1"].minHeight).toBe(40);
    expect(mockNodes["1:1"].maxHeight).toBe(600);
    expect(res?.data.results[0].nodeId).toBe("1:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("clears a constraint when null is passed", async () => {
    mockNodes["1:1"] = { id: "1:1", width: 100, height: 100, resize: function (w: number, h: number) { this.width = w; this.height = h; }, minWidth: 80 };
    await handleWriteModifyRequest(makeRequest("resize_nodes", ["1:1"], { minWidth: null }));
    expect(mockNodes["1:1"].minWidth).toBeNull();
  });

  it("ignores min/max on nodes that don't expose the field", async () => {
    mockNodes["1:1"] = { id: "1:1", width: 100, height: 100, resize: function (w: number, h: number) { this.width = w; this.height = h; } };
    const res = await handleWriteModifyRequest(makeRequest("resize_nodes", ["1:1"], { minWidth: 50 }));
    expect(res?.data.results[0].nodeId).toBe("1:1");
    expect("minWidth" in mockNodes["1:1"]).toBe(false);
  });

  it("reports a per-node error (does not abort) when a min/max assignment throws", async () => {
    // Node A: minWidth setter throws (Figma rejects non-positive / non-auto-layout).
    const throwing: any = { id: "1:1", width: 100, height: 100, resize() {}, get minWidth() { return null; }, set minWidth(_v: any) { throw new Error("minWidth must be positive"); } };
    // Node B: valid.
    const ok: any = { id: "2:2", width: 100, height: 100, resize() {}, minWidth: null };
    mockNodes["1:1"] = throwing;
    mockNodes["2:2"] = ok;
    const res = await handleWriteModifyRequest(makeRequest("resize_nodes", ["1:1", "2:2"], { minWidth: 50 }));
    // The whole request must still resolve, with a per-node error for A and success for B.
    expect(res?.data.results).toHaveLength(2);
    expect(res?.data.results[0].error).toContain("min/max constraint failed");
    expect(res?.data.results[1].nodeId).toBe("2:2");
    expect(ok.minWidth).toBe(50);
  });
});

describe("set_blend_mode", () => {
  it("sets blend mode on a node", async () => {
    mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: "MULTIPLY" }));
    expect(mockNodes["1:1"].blendMode).toBe("MULTIPLY");
    expect(res?.data.results[0].blendMode).toBe("MULTIPLY");
    expect(commitUndoCalled).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["9:9"], { blendMode: "MULTIPLY" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without blend mode support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no blendMode property
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: "MULTIPLY" }));
    expect(res?.data.results[0].error).toContain("does not support blend mode");
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
    mockNodes["2:2"] = { id: "2:2", blendMode: "NORMAL" };
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1", "2:2"], { blendMode: "SCREEN" }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].blendMode).toBe("SCREEN");
    expect(mockNodes["2:2"].blendMode).toBe("SCREEN");
  });
});

// ── set_constraints ───────────────────────────────────────────────────────────

describe("set_constraints", () => {
  it("sets horizontal constraint", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };
    const res = await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { horizontal: "CENTER" }));
    expect(mockNodes["1:1"].constraints.horizontal).toBe("CENTER");
    expect(mockNodes["1:1"].constraints.vertical).toBe("MIN"); // unchanged
    expect(commitUndoCalled).toBe(true);
  });

  it("sets vertical constraint", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };
    await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { vertical: "MAX" }));
    expect(mockNodes["1:1"].constraints.vertical).toBe("MAX");
    expect(mockNodes["1:1"].constraints.horizontal).toBe("MIN"); // unchanged
  });

  it("sets both constraints simultaneously", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };
    await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { horizontal: "STRETCH", vertical: "STRETCH" }));
    expect(mockNodes["1:1"].constraints.horizontal).toBe("STRETCH");
    expect(mockNodes["1:1"].constraints.vertical).toBe("STRETCH");
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_constraints", ["9:9"], { horizontal: "CENTER" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without constraints support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no constraints property
    const res = await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { horizontal: "CENTER" }));
    expect(res?.data.results[0].error).toContain("does not support constraints");
  });
});

// ── reparent_nodes ────────────────────────────────────────────────────────────

describe("reparent_nodes", () => {
  it("moves a node to a new parent", async () => {
    const children: any[] = [];
    const newParent = { id: "2:2", appendChild: (n: any) => children.push(n) };
    mockNodes["1:1"] = { id: "1:1", name: "Node" };
    mockNodes["2:2"] = newParent;
    const res = await handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }));
    expect(children).toHaveLength(1);
    expect(res?.data.results[0].newParentId).toBe("2:2");
    expect(commitUndoCalled).toBe(true);
  });

  it("throws if parentId is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], {}))).rejects.toThrow("parentId is required");
  });

  it("throws if parent node not found", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "9:9" }))).rejects.toThrow("Parent not found");
  });

  it("throws if parent cannot contain children", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    mockNodes["2:2"] = { id: "2:2" }; // no appendChild
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }))).rejects.toThrow("cannot contain children");
  });

  it("reports error for missing child node", async () => {
    const newParent = { id: "2:2", appendChild: () => {} };
    mockNodes["2:2"] = newParent;
    const res = await handleWriteModifyRequest(makeRequest("reparent_nodes", ["9:9"], { parentId: "2:2" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });
});

// ── batch_rename_nodes ────────────────────────────────────────────────────────

describe("batch_rename_nodes", () => {
  it("renames with find/replace", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Button/Primary" };
    mockNodes["2:2"] = { id: "2:2", name: "Button/Secondary" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1", "2:2"], {
      find: "Button", replace: "Btn",
    }));
    expect(mockNodes["1:1"].name).toBe("Btn/Primary");
    expect(mockNodes["2:2"].name).toBe("Btn/Secondary");
    expect(res?.data.results[0].oldName).toBe("Button/Primary");
    expect(res?.data.results[0].name).toBe("Btn/Primary");
    expect(commitUndoCalled).toBe(true);
  });

  it("adds prefix and suffix", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      prefix: "UI/", suffix: "_v2",
    }));
    expect(mockNodes["1:1"].name).toBe("UI/Card_v2");
    expect(res?.data.results[0].name).toBe("UI/Card_v2");
  });

  it("renames using regex", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame 123" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      find: "\\d+", replace: "X", useRegex: true,
    }));
    expect(mockNodes["1:1"].name).toBe("Frame X");
  });

  it("captures regex error per-node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      find: "[invalid", replace: "X", useRegex: true,
    }));
    expect(res?.data.results[0].error).toContain("Invalid regex");
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["9:9"], { prefix: "x" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("batch_rename_nodes", [], { prefix: "x" }))).rejects.toThrow();
  });
});

// ── NEW TESTS (TDD RED phase) ─────────────────────────────────────────────────

// set_corner_radius — mixed cornerRadius serializes as "mixed" + returns per-corner values
describe("set_corner_radius — mixed radius handling", () => {
  it("returns 'mixed' string (not Symbol) when per-corner radii differ", async () => {
    const MIXED = Symbol("figma.mixed");
    mockNodes["1:1"] = {
      id: "1:1", cornerRadius: MIXED,
      topLeftRadius: 0, topRightRadius: 8, bottomLeftRadius: 0, bottomRightRadius: 8,
    };
    const res = await handleWriteModifyRequest(
      makeRequest("set_corner_radius", ["1:1"], { topRightRadius: 8, bottomRightRadius: 8 }),
    );
    const r = res?.data.results[0];
    expect(typeof r.cornerRadius).toBe("string");
    expect(r.cornerRadius).toBe("mixed");
  });

  it("returns per-corner values when cornerRadius is mixed", async () => {
    const MIXED = Symbol("figma.mixed");
    mockNodes["1:1"] = {
      id: "1:1", cornerRadius: MIXED,
      topLeftRadius: 4, topRightRadius: 8, bottomLeftRadius: 0, bottomRightRadius: 0,
    };
    const res = await handleWriteModifyRequest(
      makeRequest("set_corner_radius", ["1:1"], {}),
    );
    const r = res?.data.results[0];
    expect(r.topLeftRadius).toBe(4);
    expect(r.topRightRadius).toBe(8);
    expect(r.bottomLeftRadius).toBe(0);
    expect(r.bottomRightRadius).toBe(0);
  });

  it("does NOT return per-corner values when cornerRadius is a plain number", async () => {
    mockNodes["1:1"] = {
      id: "1:1", cornerRadius: 8,
      topLeftRadius: 8, topRightRadius: 8, bottomLeftRadius: 8, bottomRightRadius: 8,
    };
    const res = await handleWriteModifyRequest(
      makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 8 }),
    );
    const r = res?.data.results[0];
    expect(r.cornerRadius).toBe(8);
    expect(r.topLeftRadius).toBeUndefined();
  });
});

// clone_node — throws on PAGE/DOCUMENT nodes
describe("clone_node — page/document guard", () => {
  it("throws when trying to clone a PAGE node", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "PAGE", name: "Page 1", clone: () => ({}) };
    await expect(
      handleWriteModifyRequest(makeRequest("clone_node", ["1:1"], {})),
    ).rejects.toThrow("Cannot clone a page or document node");
  });

  it("throws when trying to clone a DOCUMENT node", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "DOCUMENT", name: "Document", clone: () => ({}) };
    await expect(
      handleWriteModifyRequest(makeRequest("clone_node", ["1:1"], {})),
    ).rejects.toThrow("Cannot clone a page or document node");
  });

  it("still clones a FRAME node successfully", async () => {
    mockNodes["1:1"] = {
      id: "1:1", type: "FRAME", name: "MyFrame",
      clone: () => ({ id: "2:1", name: "MyFrame", type: "FRAME" }),
    };
    const res = await handleWriteModifyRequest(makeRequest("clone_node", ["1:1"], {}));
    expect(res?.data.id).toBe("2:1");
  });
});

// set_opacity — null/undefined opacity throws instead of silently setting 0
describe("set_opacity — null guard", () => {
  it("throws when opacity param is omitted (undefined)", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    // params={} means p.opacity is undefined
    await expect(
      handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], {})),
    ).rejects.toThrow("opacity is required");
  });

  it("still sets opacity 0 when explicitly passed as 0", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0 }));
    expect(mockNodes["1:1"].opacity).toBe(0);
    expect(res?.data.results[0].opacity).toBe(0);
  });
});

// set_blend_mode — enum validation
describe("set_blend_mode — enum validation", () => {
  it("throws for an invalid blendMode value", async () => {
    mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
    await expect(
      handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: "INVALID_MODE" })),
    ).rejects.toThrow(/invalid blend mode/i);
  });

  it("accepts all valid blend modes", async () => {
    const validModes = [
      "NORMAL", "MULTIPLY", "SCREEN", "OVERLAY", "DARKEN", "LIGHTEN",
      "COLOR_DODGE", "COLOR_BURN", "HARD_LIGHT", "SOFT_LIGHT", "DIFFERENCE",
      "EXCLUSION", "HUE", "SATURATION", "COLOR", "LUMINOSITY",
      "PASS_THROUGH", "LINEAR_BURN", "LINEAR_DODGE",
    ];
    for (const mode of validModes) {
      mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
      const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: mode }));
      expect(mockNodes["1:1"].blendMode).toBe(mode);
    }
  });

  it("throws for empty string blendMode", async () => {
    mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
    await expect(
      handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: "" })),
    ).rejects.toThrow(/invalid blend mode/i);
  });
});

// set_fills / set_strokes — warning includes mention of variable bindings on replace mode
describe("set_fills — replace mode variable binding warning", () => {
  it("warning text mentions variable bindings when not in append mode", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { color: "#ff0000" }),
    );
    const warning: string = res?.data.results[0].warning ?? "";
    expect(warning).toBeTruthy();
    // The warning should mention that existing variable bindings are replaced
    expect(warning).toMatch(/variable/i);
  });

  it("set_strokes warning mentions variable bindings when not in append mode", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", strokes: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { color: "#000000" }),
    );
    const warning: string = res?.data.results[0].warning ?? "";
    expect(warning).toBeTruthy();
    expect(warning).toMatch(/variable/i);
  });
});

// reparent_nodes — preserveAbsolutePosition (default true)
describe("reparent_nodes — preserveAbsolutePosition", () => {
  const makeAbsTransform = (x: number, y: number): [[number, number, number], [number, number, number]] => [
    [1, 0, x],
    [0, 1, y],
  ];

  it("preserves absolute position by default (preserveAbsolutePosition=true)", async () => {
    // Node at absolute (200, 300). Old parent at (0,0). New parent at (100,150).
    // After reparent, node.x/y should be adjusted to remain visually at (200,300).
    const node = {
      id: "1:1", name: "Node",
      x: 200, y: 300,
      absoluteTransform: makeAbsTransform(200, 300),
    };
    const newParent = {
      id: "2:2",
      absoluteTransform: makeAbsTransform(100, 150),
      appendChild: (n: any) => {
        // Simulate Figma: after appendChild, x/y become parent-local (unchanged in mock)
        // The handler must correct them.
      },
    };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    const res = await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }),
    );
    expect(res?.data.results[0].newParentId).toBe("2:2");
    // Expected local position: (200-100, 300-150) = (100, 150)
    expect(node.x).toBe(100);
    expect(node.y).toBe(150);
  });

  it("skips position correction when preserveAbsolutePosition=false", async () => {
    const node = {
      id: "1:1", name: "Node",
      x: 200, y: 300,
      absoluteTransform: makeAbsTransform(200, 300),
    };
    const newParent = {
      id: "2:2",
      absoluteTransform: makeAbsTransform(100, 150),
      appendChild: (_n: any) => {},
    };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2", preserveAbsolutePosition: false }),
    );
    // Should NOT correct the position
    expect(node.x).toBe(200);
    expect(node.y).toBe(300);
  });

  it("still reparents correctly with no absoluteTransform (graceful fallback)", async () => {
    const node = { id: "1:1", name: "Node", x: 50, y: 60 };
    const newParent = { id: "2:2", appendChild: (_n: any) => {} };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    const res = await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }),
    );
    expect(res?.data.results[0].newParentId).toBe("2:2");
  });
});

// ── find_replace_text ─────────────────────────────────────────────────────────

describe("find_replace_text", () => {
  beforeEach(() => {
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: {
        type: "PAGE",
        children: [],
      },
      loadFontAsync: async () => {},
    };
  });

  it("replaces text in matching TEXT nodes", async () => {
    const textNode = {
      id: "1:1", name: "Label", type: "TEXT", characters: "Hello World",
      fontName: { family: "Inter", style: "Regular" },
    };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "World", replace: "Figma" }));
    expect(textNode.characters).toBe("Hello Figma");
    expect(res?.data.replaced).toBe(1);
    expect(res?.data.results[0].newText).toBe("Hello Figma");
    expect(commitUndoCalled).toBe(true);
  });

  it("skips nodes where text does not match", async () => {
    const textNode = { id: "1:1", name: "Label", type: "TEXT", characters: "Goodbye", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "Hello", replace: "Hi" }));
    expect(res?.data.replaced).toBe(0);
    expect(textNode.characters).toBe("Goodbye");
  });

  it("searches recursively through nested children", async () => {
    const textNode = { id: "2:2", name: "Nested", type: "TEXT", characters: "foo bar", fontName: { family: "Inter", style: "Regular" } };
    const frame = { id: "1:1", type: "FRAME", children: [textNode] };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [frame] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "foo", replace: "baz" }));
    expect(textNode.characters).toBe("baz bar");
    expect(res?.data.replaced).toBe(1);
  });

  it("supports scoped search within a subtree when nodeId provided", async () => {
    const textNode = { id: "2:2", name: "Inner", type: "TEXT", characters: "target", fontName: { family: "Inter", style: "Regular" } };
    const frame = { id: "1:1", type: "FRAME", children: [textNode] };
    mockNodes["1:1"] = frame;
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", ["1:1"], { find: "target", replace: "done" }));
    expect(textNode.characters).toBe("done");
    expect(res?.data.replaced).toBe(1);
  });

  it("does NOT edit TEXT inside a component master, but edits instances (issue #33)", async () => {
    const masterText = { id: "m:t", type: "TEXT", characters: "내 경기", fontName: { family: "Inter", style: "Regular" } };
    const master = { id: "c:1", type: "COMPONENT", children: [masterText] };
    const instanceText = { id: "i:t", type: "TEXT", characters: "내 경기", fontName: { family: "Inter", style: "Regular" } };
    const instance = { id: "i:1", type: "INSTANCE", children: [instanceText] };
    const frame = { id: "f:1", type: "FRAME", children: [instance] };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [master, frame] };

    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "내 경기", replace: "내 경기 목록" }));

    expect(masterText.characters).toBe("내 경기"); // master untouched — no global propagation
    expect(instanceText.characters).toBe("내 경기 목록"); // instance override applied
    expect(res?.data.replaced).toBe(1);
  });

  it("edits component master text when the root is explicitly scoped to it (issue #33)", async () => {
    const masterText = { id: "m:t", type: "TEXT", characters: "Old", fontName: { family: "Inter", style: "Regular" } };
    const master = { id: "c:1", type: "COMPONENT", children: [masterText] };
    mockNodes["c:1"] = master;

    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", ["c:1"], { find: "Old", replace: "New" }));

    expect(masterText.characters).toBe("New");
    expect(res?.data.replaced).toBe(1);
  });

  it("uses regex when useRegex is true", async () => {
    const textNode = { id: "1:1", name: "Label", type: "TEXT", characters: "Price: $99", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "\\$\\d+", replace: "$199", useRegex: true }));
    expect(textNode.characters).toBe("Price: $199");
    expect(res?.data.replaced).toBe(1);
  });

  it("captures regex error per-node", async () => {
    const textNode = { id: "1:1", type: "TEXT", characters: "hello", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "[bad", replace: "x", useRegex: true }));
    expect(res?.data.replaced).toBe(0);
    expect(res?.data.results[0].error).toContain("Invalid regex");
  });

  it("throws if find is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("find_replace_text", [], { replace: "x" }))).rejects.toThrow("find is required");
  });

  it("throws if replace is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "x" }))).rejects.toThrow("replace is required");
  });
});

// ── set_text styling (1A) ───────────────────────────────────────────────────────

describe("set_text styling", () => {
  let loadedFonts: any[];
  beforeEach(() => {
    loadedFonts = [];
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      loadFontAsync: async (f: any) => { loadedFonts.push(f); },
    };
  });

  const makeText = (id = "1:1") => ({
    id, name: "Label", type: "TEXT", characters: "hi",
    fontName: { family: "Inter", style: "Regular" },
  });

  it("applies alignment + auto-resize without changing text", async () => {
    const t = makeText();
    mockNodes["1:1"] = t;
    const res = await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], {
      textAlignHorizontal: "CENTER", textAutoResize: "HEIGHT",
    }));
    expect(t.textAlignHorizontal).toBe("CENTER");
    expect(t.textAutoResize).toBe("HEIGHT");
    expect(t.characters).toBe("hi"); // unchanged — restyle only
    expect(res?.data.textAlignHorizontal).toBe("CENTER");
    expect(commitUndoCalled).toBe(true);
  });

  it("changes the font by loading the new family/style first", async () => {
    const t = makeText();
    mockNodes["1:1"] = t;
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], {
      fontFamily: "Geist", fontStyle: "Bold", fontSize: 20,
    }));
    expect(loadedFonts).toContainEqual({ family: "Geist", style: "Bold" });
    expect(t.fontName).toEqual({ family: "Geist", style: "Bold" });
    expect(t.fontSize).toBe(20);
  });

  it("sets spacing, case and decoration", async () => {
    const t = makeText();
    mockNodes["1:1"] = t;
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], {
      letterSpacingValue: 2, lineHeightValue: 24, lineHeightUnit: "PIXELS",
      textCase: "UPPER", textDecoration: "UNDERLINE",
    }));
    expect(t.letterSpacing).toEqual({ value: 2, unit: "PIXELS" });
    expect(t.lineHeight).toEqual({ value: 24, unit: "PIXELS" });
    expect(t.textCase).toBe("UPPER");
    expect(t.textDecoration).toBe("UNDERLINE");
  });

  it("rejects a non-TEXT node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME" };
    await expect(handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { textAlignHorizontal: "LEFT" })))
      .rejects.toThrow("is not a TEXT node");
  });

  it("sets whole-node paragraph/truncation/leadingTrim props", async () => {
    const t: any = makeText();
    mockNodes["1:1"] = t;
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], {
      paragraphIndent: 12, paragraphSpacing: 8, listSpacing: 4,
      textTruncation: "ENDING", maxLines: 2, leadingTrim: "CAP_HEIGHT",
      hangingPunctuation: true, hangingList: true,
    }));
    expect(t.paragraphIndent).toBe(12);
    expect(t.paragraphSpacing).toBe(8);
    expect(t.listSpacing).toBe(4);
    expect(t.textTruncation).toBe("ENDING");
    expect(t.maxLines).toBe(2);
    expect(t.leadingTrim).toBe("CAP_HEIGHT");
    expect(t.hangingPunctuation).toBe(true);
    expect(t.hangingList).toBe(true);
  });

  it("clears maxLines when null is passed", async () => {
    const t: any = makeText();
    t.maxLines = 3;
    mockNodes["1:1"] = t;
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { maxLines: null }));
    expect(t.maxLines).toBeNull();
  });

  it("links a named text style via setTextStyleIdAsync", async () => {
    let linkedId: string | null = null;
    const t: any = { ...makeText(), setTextStyleIdAsync: async (id: string) => { linkedId = id; } };
    mockNodes["1:1"] = t;
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { textStyleId: "S:123" }));
    expect(linkedId).toBe("S:123");
  });
});

// ── set_text_range (per-span styling) ─────────────────────────────────────────

describe("set_text_range", () => {
  let loadedFonts: any[];
  const makeRangeText = (id = "1:1") => {
    const calls: Record<string, any> = {};
    return {
      node: {
        id, name: "Para", type: "TEXT", characters: "Hello world",
        fontName: { family: "Inter", style: "Regular" },
        getRangeAllFontNames: (_s: number, _e: number) => [{ family: "Inter", style: "Regular" }],
        setRangeFontName: (s: number, e: number, v: any) => { calls.fontName = { s, e, v }; },
        setRangeFontSize: (s: number, e: number, v: any) => { calls.fontSize = { s, e, v }; },
        setRangeFills: (s: number, e: number, v: any) => { calls.fills = { s, e, v }; },
        setRangeTextCase: (s: number, e: number, v: any) => { calls.textCase = { s, e, v }; },
        setRangeTextDecoration: (s: number, e: number, v: any) => { calls.textDecoration = { s, e, v }; },
        setRangeLetterSpacing: (s: number, e: number, v: any) => { calls.letterSpacing = { s, e, v }; },
        setRangeLineHeight: (s: number, e: number, v: any) => { calls.lineHeight = { s, e, v }; },
        setRangeHyperlink: (s: number, e: number, v: any) => { calls.hyperlink = { s, e, v }; },
        setRangeListOptions: (s: number, e: number, v: any) => { calls.listOptions = { s, e, v }; },
        setRangeIndentation: (s: number, e: number, v: any) => { calls.indentation = { s, e, v }; },
      },
      calls,
    };
  };

  beforeEach(() => {
    loadedFonts = [];
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      loadFontAsync: async (f: any) => { loadedFonts.push(f); },
    };
  });

  it("applies font, size and color to a range and loads covering fonts first", async () => {
    const { node, calls } = makeRangeText();
    mockNodes["1:1"] = node;
    await handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 0, endOffset: 5, fontFamily: "Geist", fontStyle: "Bold", fontSize: 18, color: "#FF0000",
    }));
    expect(loadedFonts).toContainEqual({ family: "Inter", style: "Regular" }); // covering font
    expect(loadedFonts).toContainEqual({ family: "Geist", style: "Bold" });    // new font
    expect(calls.fontName).toEqual({ s: 0, e: 5, v: { family: "Geist", style: "Bold" } });
    expect(calls.fontSize.v).toBe(18);
    expect(calls.fills.v[0].type).toBe("SOLID");
    expect(commitUndoCalled).toBe(true);
  });

  it("sets a URL hyperlink on the range", async () => {
    const { node, calls } = makeRangeText();
    mockNodes["1:1"] = node;
    await handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 6, endOffset: 11, hyperlink: { url: "https://x.com" },
    }));
    expect(calls.hyperlink.v).toEqual({ type: "URL", value: "https://x.com" });
  });

  it("clears a hyperlink when null", async () => {
    const { node, calls } = makeRangeText();
    mockNodes["1:1"] = node;
    await handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 0, endOffset: 5, hyperlink: null,
    }));
    expect(calls.hyperlink).toEqual({ s: 0, e: 5, v: null });
  });

  it("applies list options and indentation", async () => {
    const { node, calls } = makeRangeText();
    mockNodes["1:1"] = node;
    await handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 0, endOffset: 11, listOptions: { type: "ORDERED" }, indentation: 2,
    }));
    expect(calls.listOptions.v).toEqual({ type: "ORDERED" });
    expect(calls.indentation.v).toBe(2);
  });

  it("rejects an invalid range", async () => {
    const { node } = makeRangeText();
    mockNodes["1:1"] = node;
    await expect(handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 5, endOffset: 5,
    }))).rejects.toThrow("Invalid range");
  });

  it("rejects a range past the end of the text", async () => {
    const { node } = makeRangeText();
    mockNodes["1:1"] = node;
    await expect(handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 0, endOffset: 99,
    }))).rejects.toThrow("Invalid range");
  });

  it("rejects a non-TEXT node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME" };
    await expect(handleWriteModifyRequest(makeRequest("set_text_range", ["1:1"], {
      startOffset: 0, endOffset: 1,
    }))).rejects.toThrow("is not a TEXT node");
  });
});

// ── resize_nodes layout sizing (1B) ─────────────────────────────────────────────

describe("resize_nodes layout sizing", () => {
  it("sets FILL/HUG without forcing a px resize", async () => {
    let resized = false;
    mockNodes["1:1"] = {
      id: "1:1", width: 100, height: 40, parent: { layoutMode: "HORIZONTAL" },
      resize: () => { resized = true; },
      _h: undefined as any, _v: undefined as any,
      set layoutSizingHorizontal(v: string) { this._h = v; },
      set layoutSizingVertical(v: string) { this._v = v; },
    };
    const res = await handleWriteModifyRequest(makeRequest("resize_nodes", ["1:1"], {
      layoutSizingHorizontal: "FILL", layoutSizingVertical: "HUG",
    }));
    expect(resized).toBe(false); // no width/height → no resize
    expect(mockNodes["1:1"]._h).toBe("FILL");
    expect(mockNodes["1:1"]._v).toBe("HUG");
    expect(res?.data.results[0].error).toBeUndefined();
  });

  it("reports a clear error when the node has no auto-layout parent", async () => {
    mockNodes["1:1"] = {
      id: "1:1", width: 100, height: 40,
      resize: () => {},
      set layoutSizingHorizontal(_v: string) { throw new Error("layoutSizing requires a parent that has auto-layout"); },
    };
    const res = await handleWriteModifyRequest(makeRequest("resize_nodes", ["1:1"], { layoutSizingHorizontal: "FILL" }));
    expect(res?.data.results[0].error).toContain("auto-layout parent");
  });
});

// ── TASK 1: set_fills / set_strokes — direct paints[] passthrough ─────────────

describe("set_fills — direct paints[] passthrough (new feature)", () => {
  it("applies a gradient paint verbatim to node.fills when paints[] is provided", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [] };
    const gradient = {
      type: "GRADIENT_LINEAR",
      gradientStops: [
        { position: 0, color: { r: 1, g: 0, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 1, a: 1 } },
      ],
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
    };
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { paints: [gradient] }),
    );
    expect(mockNodes["1:1"].fills).toHaveLength(1);
    expect(mockNodes["1:1"].fills[0]).toBe(gradient);
    expect(mockNodes["1:1"].fills[0].type).toBe("GRADIENT_LINEAR");
    expect(commitUndoCalled).toBe(true);
  });

  it("result contains a warning mentioning create_paint_style when paints[] used", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [] };
    const gradient = { type: "GRADIENT_LINEAR", gradientStops: [], gradientTransform: [[1,0,0],[0,1,0]] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { paints: [gradient] }),
    );
    const warning: string = res?.data.results[0].warning ?? "";
    expect(warning).toBeTruthy();
    expect(warning).toMatch(/create_paint_style/);
  });

  it("stacks onto existing fills when mode is append and paints[] is provided", async () => {
    const existing = { type: "SOLID", color: { r: 0, g: 0, b: 0 } };
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [existing] };
    const gradient = { type: "GRADIENT_LINEAR", gradientStops: [], gradientTransform: [[1,0,0],[0,1,0]] };
    await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { paints: [gradient], mode: "append" }),
    );
    expect(mockNodes["1:1"].fills).toHaveLength(2);
    expect(mockNodes["1:1"].fills[0]).toBe(existing);
    expect(mockNodes["1:1"].fills[1]).toBe(gradient);
  });

  it("replaces existing fills when mode is not append (default) and paints[] is provided", async () => {
    const existing = { type: "SOLID", color: { r: 0, g: 0, b: 0 } };
    mockNodes["1:1"] = { id: "1:1", name: "Rect", fills: [existing] };
    const img = { type: "IMAGE", scaleMode: "FILL", imageHash: "abc" };
    await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { paints: [img] }),
    );
    expect(mockNodes["1:1"].fills).toHaveLength(1);
    expect(mockNodes["1:1"].fills[0]).toBe(img);
  });

  it("reports per-node error when node does not support fills (even with paints[])", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "NoFill" }; // no fills property
    const gradient = { type: "GRADIENT_LINEAR", gradientStops: [], gradientTransform: [[1,0,0],[0,1,0]] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { paints: [gradient] }),
    );
    expect(res?.data.results[0].error).toContain("does not support fills");
  });
});

describe("set_strokes — direct paints[] passthrough (new feature)", () => {
  it("applies a gradient stroke paint verbatim when paints[] provided", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", strokes: [] };
    const gradient = {
      type: "GRADIENT_LINEAR",
      gradientStops: [
        { position: 0, color: { r: 1, g: 0, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 1, a: 1 } },
      ],
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
    };
    const res = await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { paints: [gradient] }),
    );
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
    expect(mockNodes["1:1"].strokes[0]).toBe(gradient);
    expect(res?.data.results[0].warning).toMatch(/create_paint_style/);
    expect(commitUndoCalled).toBe(true);
  });

  it("honors strokeWeight alongside paints[] passthrough", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Rect", strokes: [], strokeWeight: 0 };
    const paint = { type: "SOLID", color: { r: 0, g: 0, b: 0 } };
    await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { paints: [paint], strokeWeight: 3 }),
    );
    expect(mockNodes["1:1"].strokes[0]).toBe(paint);
    expect(mockNodes["1:1"].strokeWeight).toBe(3);
  });

  it("appends stroke paints when mode is append and paints[] provided", async () => {
    const existing = { type: "SOLID", color: { r: 0, g: 0, b: 0 } };
    mockNodes["1:1"] = { id: "1:1", name: "Rect", strokes: [existing] };
    const newPaint = { type: "GRADIENT_RADIAL", gradientStops: [], gradientTransform: [[1,0,0],[0,1,0]] };
    await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { paints: [newPaint], mode: "append" }),
    );
    expect(mockNodes["1:1"].strokes).toHaveLength(2);
    expect(mockNodes["1:1"].strokes[0]).toBe(existing);
    expect(mockNodes["1:1"].strokes[1]).toBe(newPaint);
  });
});

// ── TASK 2: reparent_nodes — auto-layout gating for preserveAbsolutePosition ──

describe("reparent_nodes — auto-layout parent gating (new feature)", () => {
  const makeAbsTransform = (x: number, y: number): [[number, number, number], [number, number, number]] => [
    [1, 0, x],
    [0, 1, y],
  ];

  it("(a) corrects x/y and returns positionPreserved:true when parent layoutMode is NONE", async () => {
    // Node at abs (200, 300), parent at abs (100, 150) → corrected local = (100, 150)
    const node = {
      id: "1:1", name: "Node",
      x: 200, y: 300,
      absoluteTransform: makeAbsTransform(200, 300),
    };
    const newParent = {
      id: "2:2",
      layoutMode: "NONE",
      absoluteTransform: makeAbsTransform(100, 150),
      appendChild: (_n: any) => {},
    };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    const res = await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }),
    );
    // positionPreserved is canPreserve (truthy when position was preserved)
    expect(res?.data.results[0].positionPreserved).toBeTruthy();
    expect(node.x).toBe(100);
    expect(node.y).toBe(150);
  });

  it("(b) does NOT correct x/y and returns positionPreserved:false when parent is auto-layout (HORIZONTAL)", async () => {
    const node = {
      id: "1:1", name: "Node",
      x: 200, y: 300,
      absoluteTransform: makeAbsTransform(200, 300),
    };
    const newParent = {
      id: "2:2",
      layoutMode: "HORIZONTAL",
      absoluteTransform: makeAbsTransform(100, 150),
      appendChild: (_n: any) => {},
    };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    const res = await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }),
    );
    // Auto-layout parent ignores x/y, so the correction must be skipped
    expect(res?.data.results[0].positionPreserved).toBe(false);
    // x/y must be untouched
    expect(node.x).toBe(200);
    expect(node.y).toBe(300);
  });

  it("(b) does NOT correct x/y when parent is auto-layout (VERTICAL)", async () => {
    const node = {
      id: "1:1", name: "Node",
      x: 50, y: 60,
      absoluteTransform: makeAbsTransform(50, 60),
    };
    const newParent = {
      id: "2:2",
      layoutMode: "VERTICAL",
      absoluteTransform: makeAbsTransform(10, 20),
      appendChild: (_n: any) => {},
    };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    const res = await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }),
    );
    expect(res?.data.results[0].positionPreserved).toBe(false);
    expect(node.x).toBe(50); // unchanged
    expect(node.y).toBe(60); // unchanged
  });

  it("(c) preserveAbsolutePosition:false → x/y not corrected even for layoutMode:NONE parent", async () => {
    const node = {
      id: "1:1", name: "Node",
      x: 200, y: 300,
      absoluteTransform: makeAbsTransform(200, 300),
    };
    const newParent = {
      id: "2:2",
      layoutMode: "NONE",
      absoluteTransform: makeAbsTransform(100, 150),
      appendChild: (_n: any) => {},
    };
    mockNodes["1:1"] = node;
    mockNodes["2:2"] = newParent;

    const res = await handleWriteModifyRequest(
      makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2", preserveAbsolutePosition: false }),
    );
    expect(res?.data.results[0].positionPreserved).toBe(false);
    expect(node.x).toBe(200); // must NOT be corrected
    expect(node.y).toBe(300);
  });
});
