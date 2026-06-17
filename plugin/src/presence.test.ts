import { describe, it, expect } from "bun:test";
import {
  activeAgents,
  collectAffectedNodeIds,
  isExpired,
  isHighlightableRequest,
  isStickyStatus,
  mergeQueued,
  opStatus,
  statusLabel,
  unionActiveNodeIds,
  type PresenceEvent,
} from "./presence";

const ev = (
  origin: string,
  ts: number,
  nodeIds: string[] = ["1:1"],
  status = "building",
  label = "Building…",
): PresenceEvent => ({ origin, ts, nodeIds, status, label });

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

describe("isExpired", () => {
  const now = 1_000_000;

  it("expires an AUTO status once it is quiet past the remove window", () => {
    expect(isExpired("building", now - 10_000, now)).toBe(false); // fresh
    expect(isExpired("building", now - 100_000, now)).toBe(false); // away, not yet gone
    expect(isExpired("building", now - 200_000, now)).toBe(true); // past 150s → gone
  });

  it("never expires a STICKY (LLM-set) status", () => {
    expect(isExpired("reviewing", now - 10_000_000, now)).toBe(false);
    expect(isExpired("done", now - 10_000_000, now)).toBe(false);
  });

  it("respects a custom remove window", () => {
    expect(isExpired("scanning", now - 5000, now, 3000)).toBe(true);
    expect(isExpired("scanning", now - 2000, now, 3000)).toBe(false);
  });
});

describe("opStatus", () => {
  it("maps token ops to theming (before the generic import_ prefix)", () => {
    expect(opStatus("set_bound_variable")).toBe("theming");
    expect(opStatus("create_variable")).toBe("theming");
    expect(opStatus("import_variable_by_key")).toBe("theming");
    expect(opStatus("import_style_by_key")).toBe("theming");
  });

  it("maps component imports to importing", () => {
    expect(opStatus("import_component_by_key")).toBe("importing");
  });

  it("maps screenshots and read verbs to their auto statuses", () => {
    expect(opStatus("save_screenshots")).toBe("screenshotting");
    expect(opStatus("get_node")).toBe("scanning");
    expect(opStatus("scan_text_nodes")).toBe("scanning");
    expect(opStatus("search_nodes")).toBe("scanning");
    expect(opStatus("list_channels")).toBe("scanning");
    expect(opStatus("fetch_library_catalog")).toBe("scanning");
  });

  it("defaults mutating ops (and batch/unknown) to building", () => {
    expect(opStatus("create_frame")).toBe("building");
    expect(opStatus("set_fills")).toBe("building");
    expect(opStatus("move_nodes")).toBe("building");
    expect(opStatus("batch")).toBe("building");
    expect(opStatus(undefined)).toBe("building");
  });
});

describe("statusLabel", () => {
  it("flavors building by op type", () => {
    expect(statusLabel("building", { opType: "create_frame" })).toBe("Building…");
    expect(statusLabel("building", { opType: "set_fills" })).toBe("Styling…");
    expect(statusLabel("building", { opType: "move_nodes" })).toBe("Moving…");
    expect(statusLabel("building", { opType: "resize_nodes" })).toBe("Resizing…");
    expect(statusLabel("building", { opType: "delete_nodes" })).toBe("Removing…");
    expect(statusLabel("building")).toBe("Building…");
  });

  it("renders auto + LLM-set statuses", () => {
    expect(statusLabel("importing")).toBe("⤵ Importing…");
    expect(statusLabel("screenshotting")).toBe("📸 Capturing…");
    expect(statusLabel("scanning")).toBe("🔍 Looking around…");
    expect(statusLabel("theming")).toBe("🎨 Theming…");
    expect(statusLabel("away")).toBe("💤 Away");
    expect(statusLabel("escalated")).toBe("🛑 Escalated");
    expect(statusLabel("approved")).toBe("Approved ✓");
    expect(statusLabel("reviewing")).toBe("Reviewing…");
  });

  it("numbers a queued row from queuePos", () => {
    expect(statusLabel("queued", { queuePos: 2 })).toBe("Queued · #2");
    expect(statusLabel("queued")).toBe("Queued");
  });
});

describe("isStickyStatus", () => {
  it("is true only for LLM-set statuses", () => {
    for (const s of ["thinking", "waiting_review", "reviewing", "approved", "escalated", "done"])
      expect(isStickyStatus(s)).toBe(true);
    for (const s of ["building", "importing", "scanning", "queued", "idle", "away", "error"])
      expect(isStickyStatus(s)).toBe(false);
  });
});

describe("activeAgents", () => {
  it("collapses to the latest event per origin, most-recent first", () => {
    const events = [ev("grace", 10), ev("theo", 30), ev("grace", 20)];
    const agents = activeAgents(events, 30, { activeWindowMs: 60000 });
    expect(agents.map((a) => a.origin)).toEqual(["theo", "grace"]); // theo ts=30 first
    expect(agents.find((a) => a.origin === "grace")!.lastTs).toBe(20); // latest grace
  });

  it("carries status + label through for an active agent", () => {
    const events = [ev("grace", 100, ["1:1"], "reviewing", "Reviewing…")];
    const a = activeAgents(events, 110, { activeWindowMs: 15000 })[0];
    expect(a.status).toBe("reviewing");
    expect(a.label).toBe("Reviewing…");
  });

  it("decays an AUTO status: active → idle → away", () => {
    const now = 1_000_000;
    const fresh = activeAgents([ev("g", now - 10_000)], now)[0]; // ≤30s → active
    expect(fresh.status).toBe("building");
    expect(fresh.active).toBe(true);

    const idle = activeAgents([ev("g", now - 45_000)], now)[0]; // 30–60s → idle
    expect(idle.status).toBe("idle");
    expect(idle.label).toBe("Idle");
    expect(idle.active).toBe(false);

    const away = activeAgents([ev("g", now - 100_000)], now)[0]; // >60s → away
    expect(away.status).toBe("away");
    expect(away.label).toBe("💤 Away");
  });

  it("does NOT decay a STICKY (LLM-set) status", () => {
    const now = 1_000_000;
    const a = activeAgents([ev("g", now - 200_000, ["1:1"], "reviewing", "Reviewing…")], now)[0];
    expect(a.status).toBe("reviewing"); // sticky — survives the away window
    expect(a.label).toBe("Reviewing…");
  });

  it("carries forward the latest NON-EMPTY nodeIds across a status-only ping", () => {
    // grace edits 1:1, then sends a status-only ping (no nodes) → nodeIds preserved.
    const events = [
      ev("grace", 10, ["1:1", "1:2"], "building", "Building…"),
      ev("grace", 20, [], "waiting_review", "Waiting for review"),
    ];
    const a = activeAgents(events, 25)[0];
    expect(a.status).toBe("waiting_review");
    expect(a.nodeIds.sort()).toEqual(["1:1", "1:2"]); // not the empty latest event
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
    const union = unionActiveNodeIds(activeAgents(events, now, { activeWindowMs: 15000 })).sort();
    expect(union).toEqual(["1:1", "1:2", "1:3"]);
  });

  it("is empty when all agents are idle", () => {
    const now = 100000;
    const events = [ev("grace", now - 60000, ["1:1"])];
    expect(unionActiveNodeIds(activeAgents(events, now, { activeWindowMs: 15000 }))).toEqual([]);
  });
});

describe("mergeQueued", () => {
  const now = 1_000_000;

  it("adds queued origins that have no activity row", () => {
    const agents = activeAgents([ev("grace", now - 1000)], now);
    const merged = mergeQueued(agents, ["theo", "zoe"], now);
    const theo = merged.find((a) => a.origin === "theo")!;
    expect(theo.status).toBe("queued");
    expect(theo.label).toBe("Queued · #1");
    expect(merged.find((a) => a.origin === "zoe")!.queuePos).toBe(2);
  });

  it("overrides a STALE (non-active) row with queued, but leaves active builders alone", () => {
    const agents = activeAgents(
      [ev("grace", now - 1000), ev("theo", now - 60_000)], // grace active, theo idle
      now,
    );
    const merged = mergeQueued(agents, ["theo", "grace"], now);
    // theo is idle → becomes queued; grace is actively building → unchanged.
    expect(merged.find((a) => a.origin === "theo")!.status).toBe("queued");
    expect(merged.find((a) => a.origin === "grace")!.status).toBe("building");
  });

  it("is a no-op when nothing is queued", () => {
    const agents = activeAgents([ev("grace", now - 1000)], now);
    expect(mergeQueued(agents, [], now)).toEqual(agents);
  });
});
