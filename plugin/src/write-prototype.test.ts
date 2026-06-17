import { describe, it, expect, beforeEach } from "bun:test";
import { handleWritePrototypeRequest } from "./write-prototype";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

const clickNavigate: any = {
  trigger: { type: "ON_CLICK" },
  action: {
    type: "NAVIGATE",
    destinationId: "1:3",
    transition: { type: "DISSOLVE", duration: 0.3, easing: { type: "EASE_OUT" } },
    preserveScrollPosition: false,
  },
};

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── set_reactions ─────────────────────────────────────────────────────────────

describe("set_reactions", () => {
  it("replaces all reactions (default mode)", async () => {
    const existing = [{ trigger: { type: "ON_HOVER" }, action: { type: "BACK" } }];
    let stored: any[] = [...existing];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(1);
    expect(mockNodes["1:2"].reactions[0].trigger.type).toBe("ON_CLICK");
    expect(res?.data.reactionCount).toBe(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("appends to existing reactions when mode is append", async () => {
    const existing = [{ trigger: { type: "ON_HOVER" }, action: { type: "BACK" } }];
    let stored: any[] = [...existing];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate], mode: "append" })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(2);
    expect(res?.data.reactionCount).toBe(2);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets empty reactions array (clears all via replace)", async () => {
    let stored: any[] = [clickNavigate];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(0);
    expect(res?.data.reactionCount).toBe(0);
  });

  it("throws when node not found", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", ["9:9"], { reactions: [clickNavigate] }))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when node does not support reactions", async () => {
    mockNodes["1:2"] = { id: "1:2", name: "Document" }; // no reactions property
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate] }))
    ).rejects.toThrow("does not support reactions");
  });

  it("throws when nodeId is missing", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", [], { reactions: [] }))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── remove_reactions ──────────────────────────────────────────────────────────

describe("remove_reactions", () => {
  it("removes all reactions when indices omitted", async () => {
    let stored: any[] = [clickNavigate, { trigger: { type: "ON_HOVER" }, action: { type: "BACK" } }];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(makeRequest("remove_reactions", ["1:2"], {}));

    expect(mockNodes["1:2"].reactions).toHaveLength(0);
    expect(res?.data.removed).toBe(2);
    expect(res?.data.reactionCount).toBe(0);
    expect(commitUndoCalled).toBe(true);
  });

  it("removes all reactions when indices is empty array", async () => {
    let stored: any[] = [clickNavigate];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("remove_reactions", ["1:2"], { indices: [] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(0);
    expect(res?.data.removed).toBe(1);
  });

  it("removes only specified indices, keeps others", async () => {
    const r0 = { trigger: { type: "ON_CLICK" }, action: { type: "BACK" } };
    const r1 = clickNavigate;
    const r2 = { trigger: { type: "ON_HOVER" }, action: { type: "CLOSE" } };
    let stored: any[] = [r0, r1, r2];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("remove_reactions", ["1:2"], { indices: [0, 2] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(1);
    expect(mockNodes["1:2"].reactions[0].trigger.type).toBe("ON_CLICK"); // r1 (was index 1) remains
    expect(res?.data.removed).toBe(2);
    expect(res?.data.reactionCount).toBe(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("throws when node not found", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("remove_reactions", ["9:9"], {}))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when node does not support reactions", async () => {
    mockNodes["1:2"] = { id: "1:2", name: "Document" };
    await expect(
      handleWritePrototypeRequest(makeRequest("remove_reactions", ["1:2"], {}))
    ).rejects.toThrow("does not support reactions");
  });

  it("throws when nodeId is missing", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("remove_reactions", [], {}))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── FIX 5: set_reactions trigger + action validation ─────────────────────────

describe("set_reactions input validation", () => {
  beforeEach(() => {
    let stored: any[] = [];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };
  });

  it("throws when a reaction has a null trigger", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{ trigger: null, actions: [{ type: "BACK" }] }],
        })
      )
    ).rejects.toThrow(/trigger/i);
  });

  it("throws when a reaction is missing a trigger entirely", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{ actions: [{ type: "BACK" }] }],
        })
      )
    ).rejects.toThrow(/trigger/i);
  });

  it("throws when NAVIGATE action is missing destinationId", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{
            trigger: { type: "ON_CLICK" },
            actions: [{ type: "NODE", navigation: "NAVIGATE" }], // missing destinationId
          }],
        })
      )
    ).rejects.toThrow(/destinationId/i);
  });

  it("throws when URL action is missing url", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{
            trigger: { type: "ON_CLICK" },
            actions: [{ type: "URL" }], // missing url
          }],
        })
      )
    ).rejects.toThrow(/url/i);
  });

  it("does NOT throw for unknown action types (only validate well-known)", async () => {
    // An unknown action type should pass through without error (future-proof)
    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{ type: "CUSTOM_UNKNOWN_ACTION", someField: "value" }],
        }],
      })
    );
    expect(res?.data.reactionCount).toBe(1);
  });

  it("accepts a valid NAVIGATE reaction without throwing", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{ type: "NODE", navigation: "NAVIGATE", destinationId: "1:3" }],
        }],
      })
    );
    expect(res?.data.reactionCount).toBe(1);
  });

  it("accepts a valid URL reaction without throwing", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_HOVER" },
          actions: [{ type: "URL", url: "https://example.com" }],
        }],
      })
    );
    expect(res?.data.reactionCount).toBe(1);
  });
});

// ── set_reactions — extended action types (Plugin API gap coverage) ───────────

describe("set_reactions — extended action types", () => {
  let captured: any[] = [];

  beforeEach(() => {
    captured = [];
    let stored: any[] = [];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { captured = r; stored = r; mockNodes["1:2"].reactions = r; },
    };
  });

  it("accepts SET_VARIABLE_MODE with collection + mode (dark-mode toggle)", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{ type: "SET_VARIABLE_MODE", variableCollectionId: "VC:1", variableModeId: "M:dark" }],
        }],
      })
    );
    expect(res?.data.reactionCount).toBe(1);
    expect(captured[0].actions[0].type).toBe("SET_VARIABLE_MODE");
  });

  it("throws when SET_VARIABLE_MODE is missing variableModeId", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{
            trigger: { type: "ON_CLICK" },
            actions: [{ type: "SET_VARIABLE_MODE", variableCollectionId: "VC:1" }],
          }],
        })
      )
    ).rejects.toThrow(/variableModeId/i);
  });

  it("throws when SET_VARIABLE is missing variableId", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{ trigger: { type: "ON_CLICK" }, actions: [{ type: "SET_VARIABLE" }] }],
        })
      )
    ).rejects.toThrow(/variableId/i);
  });

  it("throws when CONDITIONAL is missing conditionalBlocks array", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{ trigger: { type: "ON_CLICK" }, actions: [{ type: "CONDITIONAL" }] }],
        })
      )
    ).rejects.toThrow(/conditionalBlocks/i);
  });

  it("preserves OVERLAY navigation extra fields (overlayRelativePosition, resetScrollPosition)", async () => {
    await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{
            type: "NODE", destinationId: "9:9", navigation: "OVERLAY",
            overlayRelativePosition: { x: 10, y: 20 }, resetScrollPosition: true,
          }],
        }],
      })
    );
    const action = captured[0].actions[0];
    expect(action.navigation).toBe("OVERLAY");
    expect(action.overlayRelativePosition).toEqual({ x: 10, y: 20 });
    expect(action.resetScrollPosition).toBe(true);
  });

  it("preserves URL openInNewTab and ON_KEY_DOWN trigger pass-through", async () => {
    await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_KEY_DOWN", device: "KEYBOARD", keyCodes: [13] },
          actions: [{ type: "URL", url: "https://example.com", openInNewTab: true }],
        }],
      })
    );
    expect(captured[0].trigger.type).toBe("ON_KEY_DOWN");
    expect(captured[0].actions[0].openInNewTab).toBe(true);
  });
});

// ── set_prototype_start ───────────────────────────────────────────────────────

describe("set_prototype_start", () => {
  let page: any;

  beforeEach(() => {
    page = {
      id: "0:1", name: "Page 1", type: "PAGE", parent: null, flowStartingPoints: [],
    };
    const frameA = { id: "1:2", name: "Login", type: "FRAME", parent: page };
    const frameB = { id: "1:3", name: "Home", type: "FRAME", parent: page };
    mockNodes = { "0:1": page, "1:2": frameA, "1:3": frameB };
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
      commitUndo: () => { commitUndoCalled = true; },
    };
  });

  it("sets a single flow starting point with explicit name", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:2"], { names: ["Onboarding"] })
    );
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:2", name: "Onboarding" }]);
    expect(res?.data.pageId).toBe("0:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("defaults the flow name to the frame name when names omitted", async () => {
    await handleWritePrototypeRequest(makeRequest("set_prototype_start", ["1:2"], {}));
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:2", name: "Login" }]);
  });

  it("replaces existing starting points by default", async () => {
    page.flowStartingPoints = [{ nodeId: "9:9", name: "Old" }];
    await handleWritePrototypeRequest(makeRequest("set_prototype_start", ["1:2"], {}));
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:2", name: "Login" }]);
  });

  it("append mode adds without duplicating existing nodeIds", async () => {
    page.flowStartingPoints = [{ nodeId: "1:2", name: "Login" }];
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:2", "1:3"], { mode: "append" })
    );
    expect(page.flowStartingPoints).toEqual([
      { nodeId: "1:2", name: "Login" },
      { nodeId: "1:3", name: "Home" },
    ]);
  });

  it("remove mode drops only the given start point and keeps the rest", async () => {
    page.flowStartingPoints = [
      { nodeId: "1:2", name: "Login" },
      { nodeId: "1:3", name: "Home" },
    ];
    const res = await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:2"], { mode: "remove" })
    );
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:3", name: "Home" }]);
    expect(res?.data.flowStartingPoints).toEqual([{ nodeId: "1:3", name: "Home" }]);
    expect(commitUndoCalled).toBe(true);
  });

  it("remove mode drops several start points at once", async () => {
    page.flowStartingPoints = [
      { nodeId: "1:2", name: "Login" },
      { nodeId: "1:3", name: "Home" },
      { nodeId: "9:9", name: "Other" },
    ];
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:2", "1:3"], { mode: "remove" })
    );
    expect(page.flowStartingPoints).toEqual([{ nodeId: "9:9", name: "Other" }]);
  });

  it("remove mode can empty the list by removing the last start point", async () => {
    page.flowStartingPoints = [{ nodeId: "1:2", name: "Login" }];
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:2"], { mode: "remove" })
    );
    expect(page.flowStartingPoints).toEqual([]);
  });

  it("remove mode is a no-op when the nodeId is not a current start point", async () => {
    page.flowStartingPoints = [{ nodeId: "1:3", name: "Home" }];
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:2"], { mode: "remove" })
    );
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:3", name: "Home" }]);
  });

  it("remove mode tolerates a deleted frame (removes its dangling start point)", async () => {
    // "7:7" is NOT in mockNodes — the frame was deleted, but its start point lingers.
    // The strict replace/append path would throw "Node not found"; remove must clean it.
    page.flowStartingPoints = [
      { nodeId: "7:7", name: "Dangling" },
      { nodeId: "1:3", name: "Home" },
    ];
    (globalThis as any).figma.currentPage = page;
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["7:7"], { mode: "remove" })
    );
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:3", name: "Home" }]);
  });

  it("clear mode empties the current page's starting points with no nodeId", async () => {
    page.flowStartingPoints = [{ nodeId: "1:2", name: "Login" }];
    (globalThis as any).figma.currentPage = page;
    const res = await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", [], { mode: "clear" })
    );
    expect(page.flowStartingPoints).toEqual([]);
    expect(res?.data.flowStartingPoints).toEqual([]);
    expect(res?.data.pageId).toBe("0:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("clear mode resolves the page from the first nodeId when given", async () => {
    page.flowStartingPoints = [{ nodeId: "1:2", name: "Login" }];
    (globalThis as any).figma.currentPage = { id: "9:9", name: "Wrong", flowStartingPoints: [] };
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_start", ["1:3"], { mode: "clear" })
    );
    expect(page.flowStartingPoints).toEqual([]);
  });

  it("clear mode throws when nodeIds span different pages (no silent single-page clear)", async () => {
    page.flowStartingPoints = [{ nodeId: "1:2", name: "Login" }];
    const otherPage = { id: "0:2", name: "Page 2", type: "PAGE", parent: null, flowStartingPoints: [] };
    mockNodes["2:2"] = { id: "2:2", name: "Settings", type: "FRAME", parent: otherPage };
    await expect(
      handleWritePrototypeRequest(makeRequest("set_prototype_start", ["1:2", "2:2"], { mode: "clear" }))
    ).rejects.toThrow(/same page/i);
    // The ambiguity error must fire BEFORE any page is cleared.
    expect(page.flowStartingPoints).toEqual([{ nodeId: "1:2", name: "Login" }]);
  });

  it("throws when a node is on a different page", async () => {
    const otherPage = { id: "0:2", name: "Page 2", type: "PAGE", parent: null };
    mockNodes["2:2"] = { id: "2:2", name: "Settings", type: "FRAME", parent: otherPage };
    await expect(
      handleWritePrototypeRequest(makeRequest("set_prototype_start", ["1:2", "2:2"], {}))
    ).rejects.toThrow(/same page/i);
  });

  it("throws when no nodeId is provided (non-clear mode)", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_prototype_start", [], {}))
    ).rejects.toThrow(/nodeId/i);
  });

  it("throws when node not found", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_prototype_start", ["7:7"], {}))
    ).rejects.toThrow(/not found/i);
  });

  it("throws when a node has no owning page (detached)", async () => {
    mockNodes["3:3"] = { id: "3:3", name: "Detached", type: "FRAME", parent: null };
    await expect(
      handleWritePrototypeRequest(makeRequest("set_prototype_start", ["3:3"], {}))
    ).rejects.toThrow(/not on a page/i);
  });
});

// ── unknown type ──────────────────────────────────────────────────────────────

describe("unknown type", () => {
  it("returns null for unrecognised type", async () => {
    const res = await handleWritePrototypeRequest(makeRequest("unknown_prototype_op"));
    expect(res).toBeNull();
  });
});

// ── TASK 6: set_reactions — NODE action normalization ─────────────────────────

describe("set_reactions — NODE action normalization (new feature)", () => {
  let capturedReactions: any[] = [];

  beforeEach(() => {
    capturedReactions = [];
    let stored: any[] = [];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => {
        capturedReactions = r;
        stored = r;
        mockNodes["1:2"].reactions = r;
      },
    };
  });

  it("NODE action with only destinationId is normalized with navigation:NAVIGATE and transition:null", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{ type: "NODE", destinationId: "5:5" }],
        }],
      })
    );
    // Should NOT throw
    expect(res?.data.reactionCount).toBe(1);
    // The action forwarded to setReactionsAsync should be normalized
    const action = capturedReactions[0].actions[0];
    expect(action.type).toBe("NODE");
    expect(action.destinationId).toBe("5:5");
    expect(action.navigation).toBe("NAVIGATE");
    expect(action.transition).toBeNull();
  });

  it("NODE action with explicit navigation preserves the provided value", async () => {
    await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{ type: "NODE", destinationId: "5:5", navigation: "OVERLAY" }],
        }],
      })
    );
    const action = capturedReactions[0].actions[0];
    expect(action.navigation).toBe("OVERLAY");
    expect(action.transition).toBeNull(); // still defaulted
  });

  it("NODE action with explicit transition preserves the provided transition", async () => {
    const transition = { type: "DISSOLVE", duration: 0.3, easing: { type: "EASE_OUT" } };
    await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], {
        reactions: [{
          trigger: { type: "ON_CLICK" },
          actions: [{ type: "NODE", destinationId: "5:5", transition }],
        }],
      })
    );
    const action = capturedReactions[0].actions[0];
    expect(action.transition).toEqual(transition);
    expect(action.navigation).toBe("NAVIGATE"); // defaulted
  });

  it("null trigger reaction still throws (negative: guard not bypassed by normalization)", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{ trigger: null, actions: [{ type: "NODE", destinationId: "5:5" }] }],
        })
      )
    ).rejects.toThrow(/trigger/i);
  });

  it("URL action without url still throws (negative: normalization does not swallow URL validation)", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{
            trigger: { type: "ON_CLICK" },
            actions: [{ type: "URL" }],  // missing url
          }],
        })
      )
    ).rejects.toThrow(/url/i);
  });

  it("NODE action without destinationId still throws", async () => {
    await expect(
      handleWritePrototypeRequest(
        makeRequest("set_reactions", ["1:2"], {
          reactions: [{
            trigger: { type: "ON_CLICK" },
            actions: [{ type: "NODE" }], // no destinationId
          }],
        })
      )
    ).rejects.toThrow(/destinationId/i);
  });
});

// ── set_overflow ────────────────────────────────────────────────────────────────

describe("set_overflow", () => {
  beforeEach(() => {
    mockNodes["1:2"] = { id: "1:2", name: "Body", overflowDirection: "NONE", clipsContent: false };
  });

  it("sets the scroll direction on a frame", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_overflow", ["1:2"], { overflowDirection: "VERTICAL" })
    );
    expect(mockNodes["1:2"].overflowDirection).toBe("VERTICAL");
    expect(res?.data.overflowDirection).toBe("VERTICAL");
    expect(commitUndoCalled).toBe(true);
  });

  it("toggles clipsContent alongside the direction", async () => {
    await handleWritePrototypeRequest(
      makeRequest("set_overflow", ["1:2"], { overflowDirection: "BOTH", clipsContent: true })
    );
    expect(mockNodes["1:2"].clipsContent).toBe(true);
  });

  it("rejects an invalid overflowDirection", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_overflow", ["1:2"], { overflowDirection: "DIAGONAL" }))
    ).rejects.toThrow(/overflowDirection/i);
  });

  it("throws when the node does not support overflowDirection", async () => {
    mockNodes["3:3"] = { id: "3:3", name: "Vector" };
    await expect(
      handleWritePrototypeRequest(makeRequest("set_overflow", ["3:3"], { overflowDirection: "VERTICAL" }))
    ).rejects.toThrow(/does not support overflowDirection/i);
  });
});

// ── set_fixed_children ──────────────────────────────────────────────────────────

describe("set_fixed_children", () => {
  beforeEach(() => {
    mockNodes["1:2"] = {
      id: "1:2", name: "Screen", numberOfFixedChildren: 0,
      children: [{ id: "c1" }, { id: "c2" }, { id: "c3" }],
    };
  });

  it("sets the leading fixed-children count", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_fixed_children", ["1:2"], { numberOfFixedChildren: 2 })
    );
    expect(mockNodes["1:2"].numberOfFixedChildren).toBe(2);
    expect(res?.data.numberOfFixedChildren).toBe(2);
    expect(commitUndoCalled).toBe(true);
  });

  it("rejects a count larger than the child count", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_fixed_children", ["1:2"], { numberOfFixedChildren: 5 }))
    ).rejects.toThrow(/exceeds/i);
  });

  it("rejects a non-integer / negative count", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_fixed_children", ["1:2"], { numberOfFixedChildren: -1 }))
    ).rejects.toThrow(/non-negative integer/i);
  });
});

// ── pin_child ───────────────────────────────────────────────────────────────────

describe("pin_child", () => {
  let parent: any;
  let child: any;
  // Model Figma's insertChild: remove the child from its current slot, then re-insert
  // at the given index (so indices reindex, like the real API).
  const makeParent = (id: string, fixed: number, kids: any[]) => {
    const inserts: any[] = [];
    const p: any = {
      id, name: "Screen", numberOfFixedChildren: fixed, children: kids, inserts,
      insertChild: (index: number, c: any) => {
        const at = p.children.indexOf(c);
        if (at > -1) p.children.splice(at, 1);
        p.children.splice(index, 0, c);
        inserts.push({ index, child: c });
      },
    };
    kids.forEach((k) => { k.parent = p; });
    return p;
  };

  beforeEach(() => {
    child = { id: "c1", name: "BottomTabBar", layoutPositioning: "AUTO" };
    const body = { id: "b", name: "Body" };
    parent = makeParent("1:2", 0, [body, child]);
    mockNodes["c1"] = child;
  });

  it("sets the child ABSOLUTE, moves it into the fixed band, and bumps the count", async () => {
    const res = await handleWritePrototypeRequest(makeRequest("pin_child", ["c1"]));
    expect(child.layoutPositioning).toBe("ABSOLUTE");
    expect(parent.inserts[0].index).toBe(0);
    expect(parent.children[0]).toBe(child);
    expect(parent.numberOfFixedChildren).toBe(1);
    expect(res?.data.parentId).toBe("1:2");
    expect(res?.data.numberOfFixedChildren).toBe(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("appends into the existing fixed band rather than overwriting it", async () => {
    const header = { id: "h", name: "Header" };
    parent = makeParent("1:2", 1, [header, { id: "b", name: "Body" }, child]);
    await handleWritePrototypeRequest(makeRequest("pin_child", ["c1"]));
    expect(parent.inserts[0].index).toBe(1);
    expect(parent.numberOfFixedChildren).toBe(2);
  });

  it("is idempotent — re-pinning an already-fixed child does not over-count", async () => {
    await handleWritePrototypeRequest(makeRequest("pin_child", ["c1"]));
    expect(parent.numberOfFixedChildren).toBe(1);
    parent.inserts.length = 0;
    await handleWritePrototypeRequest(makeRequest("pin_child", ["c1"]));
    expect(parent.numberOfFixedChildren).toBe(1); // not 2
    expect(parent.inserts).toHaveLength(0); // no second insert
  });

  it("never lets the fixed count exceed the child count", async () => {
    parent = makeParent("1:2", 1, [child]); // count already == child count
    await handleWritePrototypeRequest(makeRequest("pin_child", ["c1"]));
    expect(parent.numberOfFixedChildren).toBe(1);
  });

  it("throws when the parent does not support fixed children", async () => {
    mockNodes["c2"] = { id: "c2", name: "Orphan", parent: { id: "g", type: "GROUP" } };
    await expect(
      handleWritePrototypeRequest(makeRequest("pin_child", ["c2"]))
    ).rejects.toThrow(/does not support fixed children/i);
  });
});

// ── set_prototype_background ──────────────────────────────────────────────────────

describe("set_prototype_background", () => {
  let page: any;
  beforeEach(() => {
    page = { id: "0:1", name: "Page 1", type: "PAGE", prototypeBackgrounds: [] };
    (globalThis as any).figma.currentPage = page;
  });

  it("sets a single solid color on the current page", async () => {
    const res = await handleWritePrototypeRequest(
      makeRequest("set_prototype_background", [], { color: "#101014" })
    );
    expect(page.prototypeBackgrounds).toHaveLength(1);
    expect(page.prototypeBackgrounds[0].type).toBe("SOLID");
    expect(res?.data.pageId).toBe("0:1");
    expect(commitUndoCalled).toBe(true);
  });

  it("applies opacity to the solid color when given", async () => {
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_background", [], { color: "#101014", opacity: 0.5 })
    );
    expect(page.prototypeBackgrounds[0].opacity).toBe(0.5);
  });

  it("clears the prototype background with mode clear", async () => {
    page.prototypeBackgrounds = [{ type: "SOLID", color: { r: 1, g: 1, b: 1 } }];
    await handleWritePrototypeRequest(makeRequest("set_prototype_background", [], { mode: "clear" }));
    expect(page.prototypeBackgrounds).toHaveLength(0);
  });

  it("resolves the owning page of a passed node", async () => {
    const otherPage: any = { id: "0:2", name: "Page 2", type: "PAGE", prototypeBackgrounds: [] };
    mockNodes["9:9"] = { id: "9:9", name: "Frame", parent: otherPage };
    await handleWritePrototypeRequest(
      makeRequest("set_prototype_background", ["9:9"], { color: "#000000" })
    );
    expect(otherPage.prototypeBackgrounds).toHaveLength(1);
    expect(page.prototypeBackgrounds).toHaveLength(0);
  });

  it("throws when neither color nor clear mode is given", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_prototype_background", [], {}))
    ).rejects.toThrow(/color/i);
  });
});
