import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteComponentRequest } from "./write-components";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let navigatedTo: any;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

let groupCalledWithParent: any;

// Helper to create a mock INSTANCE with override tracking and swapComponent support.
// swapComponent preserves overrides; direct mainComponent= assignment clears them.
const makeMockInstance = (id: string, name: string, overrides: Record<string, any> = {}) => {
  const inst: any = {
    id,
    name,
    type: "INSTANCE",
    mainComponent: null,
    _swapComponentCalledWith: null as any,
    // Simulate real Figma: overrides survive swapComponent but are cleared on
    // direct mainComponent= assignment (per the Figma typings).
    _overrides: { ...overrides },
    swapComponent(component: any) {
      this._swapComponentCalledWith = component;
      this.mainComponent = component;
      // overrides preserved — _overrides untouched
    },
    set _mainComponentRaw(c: any) {
      // simulate clear-overrides on direct assignment
      this._overrides = {};
      this._mainComponentActual = c;
    },
  };
  // Override mainComponent as a real property that delegates clearing
  Object.defineProperty(inst, "mainComponent", {
    get() { return this._mainComponentActual ?? null; },
    set(c: any) {
      this._mainComponentActual = c;
      this._overrides = {}; // Figma clears overrides on direct assignment
    },
    configurable: true,
  });
  // restore swapComponent to NOT clear overrides
  inst.swapComponent = function(component: any) {
    this._swapComponentCalledWith = component;
    this._mainComponentActual = component; // bypass the override-clearing setter
  };
  return inst;
};

beforeEach(() => {
  commitUndoCalled = false;
  navigatedTo = null;
  mockNodes = {};
  groupCalledWithParent = null;
  (globalThis as any).figma = {
    get currentPage() { return { id: "0:1", name: "Page 1" }; },
    setCurrentPageAsync: async (page: any) => { navigatedTo = page; },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    group: (nodes: any[], parent: any) => {
      groupCalledWithParent = parent;
      const group = { id: "grp:1", name: "Group 1", type: "GROUP", children: [...nodes] };
      (parent as any).children = (parent as any).children ?? [];
      (parent as any).children.push(group);
      return group;
    },
    root: {
      children: [
        { id: "0:1", name: "Page 1", type: "PAGE" },
        { id: "0:2", name: "Page 2", type: "PAGE" },
      ],
    },
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── swap_component (bulk: loops over ALL nodeIds) ─────────────────────────────

describe("swap_component", () => {
  const setupComponent = () => {
    mockNodes["99:1"] = { id: "99:1", name: "NewBtn", type: "COMPONENT" };
  };

  it("swaps a single instance (1-element loop, structured result)", async () => {
    setupComponent();
    mockNodes["1:1"] = makeMockInstance("1:1", "Btn");
    const res = await handleWriteComponentRequest(
      makeRequest("swap_component", ["1:1"], { componentId: "99:1" }),
    );
    expect(res?.data.results).toHaveLength(1);
    expect(res?.data.results[0].nodeId).toBe("1:1");
    expect(res?.data.results[0].componentId).toBe("99:1");
    expect(mockNodes["1:1"].mainComponent).toBe(mockNodes["99:1"]);
    expect(commitUndoCalled).toBe(true);
  });

  it("swaps EVERY instance in nodeIds (bulk all→all)", async () => {
    setupComponent();
    mockNodes["1:1"] = makeMockInstance("1:1", "A");
    mockNodes["2:2"] = makeMockInstance("2:2", "B");
    mockNodes["3:3"] = makeMockInstance("3:3", "C");
    const res = await handleWriteComponentRequest(
      makeRequest("swap_component", ["1:1", "2:2", "3:3"], { componentId: "99:1" }),
    );
    expect(res?.data.results).toHaveLength(3);
    expect(res?.data.results.every((r: any) => !r.error)).toBe(true);
    expect(mockNodes["1:1"].mainComponent).toBe(mockNodes["99:1"]);
    expect(mockNodes["2:2"].mainComponent).toBe(mockNodes["99:1"]);
    expect(mockNodes["3:3"].mainComponent).toBe(mockNodes["99:1"]);
  });

  it("reports partial success on a mix of valid + invalid ids (no abort)", async () => {
    setupComponent();
    mockNodes["1:1"] = makeMockInstance("1:1", "A");
    mockNodes["7:7"] = { id: "7:7", name: "F", type: "FRAME" }; // not an INSTANCE
    const res = await handleWriteComponentRequest(
      makeRequest("swap_component", ["1:1", "9:9", "7:7"], { componentId: "99:1" }),
    );
    expect(res?.data.results).toHaveLength(3);
    expect(res?.data.results[0].componentId).toBe("99:1"); // valid → swapped
    expect(res?.data.results[1].error).toBe("Node not found"); // missing id
    expect(res?.data.results[2].error).toContain("not a component INSTANCE"); // wrong type
    expect(mockNodes["1:1"].mainComponent).toBe(mockNodes["99:1"]);
  });

  // [FIX 1] swapComponent must preserve overrides — direct mainComponent= would clear them.
  it("preserves overrides when swapping (uses swapComponent, not mainComponent=)", async () => {
    setupComponent();
    mockNodes["1:1"] = makeMockInstance("1:1", "Btn", { "Label": "Keep me" });
    const inst = mockNodes["1:1"];
    // Sanity: before swap the override is present
    expect(inst._overrides["Label"]).toBe("Keep me");
    await handleWriteComponentRequest(
      makeRequest("swap_component", ["1:1"], { componentId: "99:1" }),
    );
    // swapComponent should be the method called — NOT direct mainComponent=
    expect(inst._swapComponentCalledWith).toBe(mockNodes["99:1"]);
    // Overrides must NOT have been cleared
    expect(inst._overrides["Label"]).toBe("Keep me");
  });

  it("throws (request-level) when componentId is missing", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "A", type: "INSTANCE" };
    await expect(
      handleWriteComponentRequest(makeRequest("swap_component", ["1:1"], {})),
    ).rejects.toThrow("componentId is required");
  });

  it("throws (request-level) when nodeIds is empty", async () => {
    setupComponent();
    await expect(
      handleWriteComponentRequest(makeRequest("swap_component", [], { componentId: "99:1" })),
    ).rejects.toThrow("nodeIds is required");
  });

  it("throws (request-level) when the target component is missing or wrong type", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "A", type: "INSTANCE" };
    await expect(
      handleWriteComponentRequest(makeRequest("swap_component", ["1:1"], { componentId: "no:no" })),
    ).rejects.toThrow("Component not found");
  });
});

// ── navigate_to_page ──────────────────────────────────────────────────────────

describe("navigate_to_page", () => {
  it("navigates by pageId", async () => {
    mockNodes["0:2"] = { id: "0:2", name: "Page 2", type: "PAGE" };
    const res = await handleWriteComponentRequest(makeRequest("navigate_to_page", [], { pageId: "0:2" }));
    expect(navigatedTo?.id).toBe("0:2");
    expect(res?.data.id).toBe("0:2");
    expect(res?.data.name).toBe("Page 2");
  });

  it("navigates by pageName", async () => {
    const res = await handleWriteComponentRequest(makeRequest("navigate_to_page", [], { pageName: "Page 2" }));
    expect(navigatedTo?.name).toBe("Page 2");
    expect(res?.data.name).toBe("Page 2");
  });

  it("throws when pageId node not found", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("navigate_to_page", [], { pageId: "9:9" }))
    ).rejects.toThrow("Page not found: 9:9");
  });

  it("throws when pageId node is not a PAGE", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME" };
    await expect(
      handleWriteComponentRequest(makeRequest("navigate_to_page", [], { pageId: "1:1" }))
    ).rejects.toThrow("is not a PAGE");
  });

  it("throws when pageName not found", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("navigate_to_page", [], { pageName: "Nonexistent" }))
    ).rejects.toThrow("Page not found");
  });

  it("throws when neither pageId nor pageName provided", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("navigate_to_page", [], {}))
    ).rejects.toThrow("pageId or pageName is required");
  });
});

// ── group_nodes ───────────────────────────────────────────────────────────────

describe("group_nodes", () => {
  it("groups nodes and returns the GROUP", async () => {
    const parent = { id: "0:1", children: [] as any[], appendChild: () => {} };
    mockNodes["1:1"] = { id: "1:1", type: "RECTANGLE", parent };
    mockNodes["2:2"] = { id: "2:2", type: "RECTANGLE", parent };
    const res = await handleWriteComponentRequest(makeRequest("group_nodes", ["1:1", "2:2"]));
    expect(res?.data.type).toBe("GROUP");
    expect(commitUndoCalled).toBe(true);
  });

  it("applies custom name to the group", async () => {
    const parent = { id: "0:1", children: [] as any[], appendChild: () => {} };
    mockNodes["1:1"] = { id: "1:1", type: "FRAME", parent };
    mockNodes["2:2"] = { id: "2:2", type: "FRAME", parent };
    const res = await handleWriteComponentRequest(
      makeRequest("group_nodes", ["1:1", "2:2"], { name: "My Group" })
    );
    expect(res?.data.name).toBe("My Group");
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteComponentRequest(makeRequest("group_nodes", []))).rejects.toThrow("nodeIds is required");
  });

  it("throws when no valid nodes found", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("group_nodes", ["9:9", "8:8"]))
    ).rejects.toThrow("No valid scene nodes found");
  });

  // [FIX 2] cross-parent validation — nodes from different parents must fail before
  // figma.group is called (partial mutation / throw risk).
  it("throws when nodes belong to different parents (cross-parent guard)", async () => {
    const parentA = { id: "p:A", children: [] as any[], appendChild: () => {} };
    const parentB = { id: "p:B", children: [] as any[], appendChild: () => {} };
    mockNodes["1:1"] = { id: "1:1", type: "RECTANGLE", parent: parentA };
    mockNodes["2:2"] = { id: "2:2", type: "RECTANGLE", parent: parentB };
    await expect(
      handleWriteComponentRequest(makeRequest("group_nodes", ["1:1", "2:2"]))
    ).rejects.toThrow(/same parent/i);
    // figma.group must NOT have been called
    expect(groupCalledWithParent).toBeNull();
  });
});

// ── ungroup_nodes ─────────────────────────────────────────────────────────────

describe("ungroup_nodes", () => {
  it("ungroups a GROUP node and returns child IDs", async () => {
    const child1 = { id: "3:1", type: "RECTANGLE" };
    const child2 = { id: "3:2", type: "RECTANGLE" };
    let removed = false;
    const parent = {
      id: "0:1",
      children: [] as any[],
      insertChild(_idx: number, child: any) { this.children.push(child); },
    };
    const group = {
      id: "grp:1", type: "GROUP",
      children: [child1, child2],
      parent,
      remove() { removed = true; },
    };
    parent.children = [group];
    mockNodes["grp:1"] = group;

    const res = await handleWriteComponentRequest(makeRequest("ungroup_nodes", ["grp:1"]));
    expect(res?.data.results[0].childIds).toEqual(["3:1", "3:2"]);
    expect(removed).toBe(true);
    expect(commitUndoCalled).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteComponentRequest(makeRequest("ungroup_nodes", ["9:9"]));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error when node is not a GROUP", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "FRAME" };
    const res = await handleWriteComponentRequest(makeRequest("ungroup_nodes", ["1:1"]));
    expect(res?.data.results[0].error).toBe("Node is not a GROUP");
  });

  it("throws for empty nodeIds", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("ungroup_nodes", []))
    ).rejects.toThrow("nodeIds is required");
  });

  it("returns null for unrecognised type", async () => {
    const res = await handleWriteComponentRequest(makeRequest("unknown_op"));
    expect(res).toBeNull();
  });
});

// ── delete_nodes (per-node partial success + actionable hint) ─────────────────

describe("delete_nodes", () => {
  it("deletes valid nodes and reports a per-node error WITHOUT aborting the rest", async () => {
    let removedA = false, removedC = false;
    mockNodes["a:1"] = { id: "a:1", name: "A", type: "FRAME", remove() { removedA = true; } };
    // An instance child / property-backing node: Figma natively throws on remove().
    mockNodes["I1:1;2:2"] = {
      id: "I1:1;2:2", name: "locked", type: "FRAME",
      remove() { throw new Error("Removing this node is not allowed"); },
    };
    mockNodes["c:3"] = { id: "c:3", name: "C", type: "FRAME", remove() { removedC = true; } };

    const res = await handleWriteComponentRequest(
      makeRequest("delete_nodes", ["a:1", "I1:1;2:2", "c:3"]),
    );
    const results = res?.data.results;
    expect(results).toHaveLength(3);
    // the good nodes are deleted even though the middle one threw (no abort)
    expect(removedA).toBe(true);
    expect(removedC).toBe(true);
    expect(results[0]).toEqual({ nodeId: "a:1", deleted: true });
    expect(results[2]).toEqual({ nodeId: "c:3", deleted: true });
    expect(commitUndoCalled).toBe(true);
  });

  it("attaches an actionable hint to the 'Removing this node is not allowed' error", async () => {
    mockNodes["I1:1;2:2"] = {
      id: "I1:1;2:2", name: "locked", type: "FRAME",
      remove() { throw new Error("Removing this node is not allowed"); },
    };
    const res = await handleWriteComponentRequest(
      makeRequest("delete_nodes", ["I1:1;2:2"]),
    );
    const r = res?.data.results[0];
    expect(r.nodeId).toBe("I1:1;2:2");
    expect(r.deleted).toBeUndefined();
    expect(r.error).toContain("Removing this node is not allowed");
    expect(r.error).toContain("detach_instance");
  });

  it("reports not-found per node", async () => {
    const res = await handleWriteComponentRequest(makeRequest("delete_nodes", ["nope:1"]));
    expect(res?.data.results[0]).toEqual({ nodeId: "nope:1", error: "Node not found" });
  });

  it("throws for empty nodeIds", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("delete_nodes", []))
    ).rejects.toThrow("nodeIds is required");
  });
});
