import { describe, it, expect } from "bun:test";
import {
  actionLabel,
  activeAgents,
  appendEvent,
  collectAffectedNodeIds,
  isHighlightableRequest,
  MAX_PRESENCE_EVENTS,
  unionActiveNodeIds,
  type PresenceEvent,
} from "./presence";

const ev = (origin: string, ts: number, nodeIds: string[] = ["1:1"], action = "edited 1 node"): PresenceEvent => ({
  origin,
  ts,
  nodeIds,
  action,
});

// Response shapes mirror what the real handlers return (see write-create.ts,
// write-modify.ts, batch.ts): standalone ops wrap their payload in `.data`, and a
// batch nests per-op results under `.data.results[].data`.

describe("collectAffectedNodeIds", () => {
  it("pulls the id from a create_* response", () => {
    const res = {
      type: "create_frame",
      requestId: "r1",
      data: { id: "10:1", name: "Card", type: "FRAME", bounds: { x: 0, y: 0, width: 200, height: 100 } },
    };
    expect(collectAffectedNodeIds(res)).toEqual(["10:1"]);
  });

  it("pulls nodeIds from a multi-node move_nodes response", () => {
    const res = {
      type: "move_nodes",
      requestId: "r2",
      data: { results: [{ nodeId: "10:2", x: 5, y: 5 }, { nodeId: "10:3", x: 8, y: 8 }] },
    };
    expect(collectAffectedNodeIds(res).sort()).toEqual(["10:2", "10:3"]);
  });

  it("recurses into a nested batch response and de-dupes", () => {
    const res = {
      type: "batch",
      requestId: "r3",
      data: {
        results: [
          { i: 0, type: "create_frame", data: { id: "10:1", bounds: {} } },
          { i: 1, type: "set_fills", data: { results: [{ nodeId: "10:1" }, { nodeId: "10:4" }] } },
        ],
        okCount: 2,
        failCount: 0,
      },
    };
    // 10:1 appears twice (created, then filled) → de-duped.
    expect(collectAffectedNodeIds(res).sort()).toEqual(["10:1", "10:4"]);
  });

  it("skips per-node entries that errored", () => {
    const res = {
      type: "resize_nodes",
      requestId: "r4",
      data: {
        results: [
          { nodeId: "10:5", width: 10, height: 10 },
          { nodeId: "10:6", error: "Node not found" },
        ],
      },
    };
    expect(collectAffectedNodeIds(res)).toEqual(["10:5"]);
  });

  it("skips a failed op entry in a batch (error, no data)", () => {
    const res = {
      type: "batch",
      requestId: "r5",
      data: {
        results: [
          { i: 0, type: "create_frame", data: { id: "10:7" } },
          { i: 1, type: "set_fills", error: "color or paints is required" },
        ],
        okCount: 1,
        failCount: 1,
      },
    };
    expect(collectAffectedNodeIds(res)).toEqual(["10:7"]);
  });

  it("returns empty for an error-only response", () => {
    expect(collectAffectedNodeIds({ type: "create_frame", requestId: "r6", error: "boom" })).toEqual([]);
  });

  it("ignores non-node-id-shaped values and unrelated id keys", () => {
    const res = {
      type: "set_fills",
      requestId: "r7",
      // variableId is NOT under an `id`/`nodeId` key, and "abc" isn't node-id-shaped.
      data: { results: [{ nodeId: "10:8", variableId: "VariableID:1:2" }, { nodeId: "abc" }] },
    };
    expect(collectAffectedNodeIds(res)).toEqual(["10:8"]);
  });

  it("accepts instance-sublayer ids", () => {
    const res = { type: "set_text", requestId: "r8", data: { id: "I12:3;45:6" } };
    expect(collectAffectedNodeIds(res)).toEqual(["I12:3;45:6"]);
  });

  it("is defensive against malformed / non-object input", () => {
    expect(collectAffectedNodeIds(null)).toEqual([]);
    expect(collectAffectedNodeIds(undefined)).toEqual([]);
    expect(collectAffectedNodeIds("nope")).toEqual([]);
    expect(collectAffectedNodeIds(42)).toEqual([]);
    expect(collectAffectedNodeIds([])).toEqual([]);
  });
});

describe("isHighlightableRequest", () => {
  it("is true for batch and write verbs", () => {
    expect(isHighlightableRequest("batch")).toBe(true);
    expect(isHighlightableRequest("create_frame")).toBe(true);
    expect(isHighlightableRequest("set_fills")).toBe(true);
    expect(isHighlightableRequest("move_nodes")).toBe(true);
    expect(isHighlightableRequest("import_component_by_key")).toBe(true);
  });

  it("is false for read verbs", () => {
    expect(isHighlightableRequest("get_node")).toBe(false);
    expect(isHighlightableRequest("scan_text_nodes")).toBe(false);
    expect(isHighlightableRequest("search_nodes")).toBe(false);
    expect(isHighlightableRequest("list_channels")).toBe(false);
    expect(isHighlightableRequest("export_tokens")).toBe(false);
    expect(isHighlightableRequest("fetch_library_catalog")).toBe(false);
    expect(isHighlightableRequest("save_screenshots")).toBe(false);
  });

  it("is false for empty / non-string input", () => {
    expect(isHighlightableRequest("")).toBe(false);
    expect(isHighlightableRequest(undefined)).toBe(false);
    expect(isHighlightableRequest(null)).toBe(false);
    expect(isHighlightableRequest(123)).toBe(false);
  });
});

describe("appendEvent", () => {
  it("appends without mutating the input array", () => {
    const a: PresenceEvent[] = [ev("grace", 1)];
    const b = appendEvent(a, ev("theo", 2));
    expect(a).toHaveLength(1); // original untouched (immutability)
    expect(b.map((e) => e.origin)).toEqual(["grace", "theo"]);
  });

  it("caps to the last N events (ring buffer), dropping the oldest", () => {
    let events: PresenceEvent[] = [];
    for (let i = 0; i < MAX_PRESENCE_EVENTS + 5; i++) {
      events = appendEvent(events, ev("grace", i));
    }
    expect(events).toHaveLength(MAX_PRESENCE_EVENTS);
    // Oldest 5 dropped; first remaining ts is index 5.
    expect(events[0].ts).toBe(5);
    expect(events[events.length - 1].ts).toBe(MAX_PRESENCE_EVENTS + 4);
  });

  it("respects a custom cap", () => {
    let events: PresenceEvent[] = [];
    for (let i = 0; i < 6; i++) events = appendEvent(events, ev("x", i), 3);
    expect(events.map((e) => e.ts)).toEqual([3, 4, 5]);
  });
});

describe("actionLabel", () => {
  it("maps verbs from the request type", () => {
    expect(actionLabel("create_frame", 1)).toBe("created 1 node");
    expect(actionLabel("move_nodes", 2)).toBe("moved 2 nodes");
    expect(actionLabel("resize_nodes", 3)).toBe("resized 3 nodes");
    expect(actionLabel("delete_nodes", 1)).toBe("deleted 1 node");
    expect(actionLabel("import_component_by_key", 1)).toBe("imported 1 node");
    expect(actionLabel("set_fills", 4)).toBe("styled 4 nodes");
  });

  it("falls back to 'edited' for batch / unknown / non-string", () => {
    expect(actionLabel("batch", 5)).toBe("edited 5 nodes");
    expect(actionLabel("frobnicate", 1)).toBe("edited 1 node");
    expect(actionLabel(undefined, 2)).toBe("edited 2 nodes");
  });
});

describe("activeAgents", () => {
  it("collapses to the latest event per origin, most-recent first", () => {
    const events = [ev("grace", 10), ev("theo", 30), ev("grace", 20)];
    const agents = activeAgents(events, 30, 60000);
    expect(agents.map((a) => a.origin)).toEqual(["theo", "grace"]); // theo ts=30 first
    expect(agents.find((a) => a.origin === "grace")!.lastTs).toBe(20); // latest grace
  });

  it("flags agents outside the active window as inactive", () => {
    const now = 100000;
    const events = [ev("grace", now - 5000), ev("theo", now - 30000)];
    const agents = activeAgents(events, now, 15000);
    expect(agents.find((a) => a.origin === "grace")!.active).toBe(true);
    expect(agents.find((a) => a.origin === "theo")!.active).toBe(false);
  });

  it("returns empty for no events", () => {
    expect(activeAgents([], 0)).toEqual([]);
  });
});

describe("unionActiveNodeIds", () => {
  it("collects de-duped node ids across only ACTIVE agents", () => {
    const now = 100000;
    const events = [
      ev("grace", now - 1000, ["1:1", "1:2"]),
      ev("theo", now - 2000, ["1:2", "1:3"]),
      ev("zoe", now - 40000, ["9:9"]), // inactive → excluded
    ];
    const union = unionActiveNodeIds(activeAgents(events, now, 15000)).sort();
    expect(union).toEqual(["1:1", "1:2", "1:3"]);
  });

  it("is empty when all agents are idle", () => {
    const now = 100000;
    const events = [ev("grace", now - 60000, ["1:1"])];
    expect(unionActiveNodeIds(activeAgents(events, now, 15000))).toEqual([]);
  });
});
