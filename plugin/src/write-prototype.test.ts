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
