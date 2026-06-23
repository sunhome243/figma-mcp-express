import { describe, it, expect, beforeEach } from "bun:test";
import {
  hexToRgb,
  makeSolidPaint,
  applyAutoLayout,
  base64ToBytes,
  getParentNode,
  bulkApply,
} from "./write-helpers";

// ── figma.variables mock ──────────────────────────────────────────────────────
// Used by applyAutoLayout variable-binding tests.
let mockVariables: Record<string, any>;

const setupVariablesMock = () => {
  mockVariables = {};
  (globalThis as any).figma = {
    ...(globalThis as any).figma,
    variables: {
      getVariableByIdAsync: async (id: string) => mockVariables[id] ?? null,
    },
  };
};

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockCurrentPage: any;
let mockGetNodeByIdAsync: (id: string) => Promise<any>;

beforeEach(() => {
  mockCurrentPage = { id: "0:1", name: "Page 1" };
  mockGetNodeByIdAsync = async (_id: string) => null;
  (globalThis as any).figma = {
    get currentPage() { return mockCurrentPage; },
    getNodeByIdAsync: (id: string) => mockGetNodeByIdAsync(id),
  };
});

// ── hexToRgb ──────────────────────────────────────────────────────────────────

describe("hexToRgb", () => {
  it("converts 6-char hex to rgb with alpha 1", () => {
    const result = hexToRgb("#ff0000");
    expect(result.r).toBeCloseTo(1);
    expect(result.g).toBe(0);
    expect(result.b).toBe(0);
    expect(result.a).toBe(1);
  });

  it("converts black #000000", () => {
    const result = hexToRgb("#000000");
    expect(result.r).toBe(0);
    expect(result.g).toBe(0);
    expect(result.b).toBe(0);
    expect(result.a).toBe(1);
  });

  it("converts white #ffffff", () => {
    const result = hexToRgb("#ffffff");
    expect(result.r).toBeCloseTo(1);
    expect(result.g).toBeCloseTo(1);
    expect(result.b).toBeCloseTo(1);
    expect(result.a).toBe(1);
  });

  it("converts 8-char hex with alpha", () => {
    // #ff000080 → alpha = 0x80 / 255 ≈ 0.502
    const result = hexToRgb("#ff000080");
    expect(result.r).toBeCloseTo(1);
    expect(result.a).toBeCloseTo(128 / 255);
  });

  it("works without leading #", () => {
    const result = hexToRgb("00ff00");
    expect(result.g).toBeCloseTo(1);
  });
});

// ── makeSolidPaint ────────────────────────────────────────────────────────────

describe("makeSolidPaint", () => {
  it("creates a solid paint from a hex string", () => {
    const paint = makeSolidPaint("#ff0000");
    expect(paint.type).toBe("SOLID");
    expect((paint.color as any).r).toBeCloseTo(1);
    expect((paint as any).opacity).toBeUndefined();
  });

  it("omits opacity when alpha is 1", () => {
    const paint = makeSolidPaint("#ffffff");
    expect((paint as any).opacity).toBeUndefined();
  });

  it("sets opacity when alpha < 1", () => {
    // #ff000080 → a ≈ 128/255
    const paint = makeSolidPaint("#ff000080");
    expect((paint as any).opacity).toBeCloseTo(128 / 255);
  });

  it("uses opacityOverride over alpha channel", () => {
    const paint = makeSolidPaint("#ff000080", 0.25);
    expect((paint as any).opacity).toBe(0.25);
  });

  it("creates paint from an object color input", () => {
    const paint = makeSolidPaint({ r: 0, g: 1, b: 0, a: 1 });
    expect(paint.type).toBe("SOLID");
    expect((paint.color as any).g).toBeCloseTo(1);
    expect((paint as any).opacity).toBeUndefined();
  });

  it("uses opacity from object input when a < 1", () => {
    const paint = makeSolidPaint({ r: 0, g: 0, b: 1, a: 0.5 });
    expect((paint as any).opacity).toBe(0.5);
  });

  it("defaults a to 1 when not provided in object input", () => {
    const paint = makeSolidPaint({ r: 0, g: 0, b: 1 });
    expect((paint as any).opacity).toBeUndefined();
  });
});

// ── applyAutoLayout ───────────────────────────────────────────────────────────

describe("applyAutoLayout", () => {
  // Ensure a minimal figma.variables mock is present so the async variable-binding
  // path doesn't throw when tests don't pass any *VariableId params.
  beforeEach(() => {
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      variables: {
        getVariableByIdAsync: async (_id: string) => null,
      },
    };
  });

  const makeFrame = () => ({
    layoutMode: "NONE" as string,
    paddingTop: 0,
    paddingRight: 0,
    paddingBottom: 0,
    paddingLeft: 0,
    itemSpacing: 0,
    primaryAxisAlignItems: undefined as any,
    counterAxisAlignItems: undefined as any,
    primaryAxisSizingMode: undefined as any,
    counterAxisSizingMode: undefined as any,
    layoutWrap: undefined as any,
    counterAxisSpacing: undefined as any,
    counterAxisAlignContent: undefined as any,
    gridRowCount: undefined as any,
    gridColumnCount: undefined as any,
    gridRowGap: undefined as any,
    gridColumnGap: undefined as any,
    minWidth: undefined as any,
    maxWidth: undefined as any,
    minHeight: undefined as any,
    maxHeight: undefined as any,
    overflowDirection: undefined as any,
    strokesIncludedInLayout: undefined as any,
    itemReverseZIndex: undefined as any,
    setBoundVariable: (_field: string, _v: any) => {},
  });

  it("sets layoutMode", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, { layoutMode: "HORIZONTAL" });
    expect(frame.layoutMode).toBe("HORIZONTAL");
  });

  it("sets padding values", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, { paddingTop: 8, paddingRight: 16, paddingBottom: 8, paddingLeft: 16 });
    expect(frame.paddingTop).toBe(8);
    expect(frame.paddingRight).toBe(16);
  });

  it("sets itemSpacing", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, { itemSpacing: 12 });
    expect(frame.itemSpacing).toBe(12);
  });

  it("sets axis alignment when layoutMode is not NONE", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "HORIZONTAL",
      primaryAxisAlignItems: "CENTER",
      counterAxisAlignItems: "MIN",
      primaryAxisSizingMode: "FIXED",
      counterAxisSizingMode: "AUTO",
      layoutWrap: "NO_WRAP",
    });
    expect(frame.primaryAxisAlignItems).toBe("CENTER");
    expect(frame.counterAxisAlignItems).toBe("MIN");
    expect(frame.primaryAxisSizingMode).toBe("FIXED");
    expect(frame.counterAxisSizingMode).toBe("AUTO");
    expect(frame.layoutWrap).toBe("NO_WRAP");
  });

  it("does not set axis props when layoutMode is NONE", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      primaryAxisAlignItems: "CENTER",
    });
    expect(frame.primaryAxisAlignItems).toBeUndefined();
  });

  it("sets counterAxisSpacing only when layoutWrap is WRAP", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "HORIZONTAL",
      layoutWrap: "WRAP",
      counterAxisSpacing: 8,
    });
    expect(frame.counterAxisSpacing).toBe(8);
  });

  it("skips counterAxisSpacing when not WRAP", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "HORIZONTAL",
      layoutWrap: "NO_WRAP",
      counterAxisSpacing: 8,
    });
    expect(frame.counterAxisSpacing).toBeUndefined();
  });

  it("sets counterAxisAlignContent only when WRAP", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "HORIZONTAL",
      layoutWrap: "WRAP",
      counterAxisAlignContent: "SPACE_BETWEEN",
    });
    expect(frame.counterAxisAlignContent).toBe("SPACE_BETWEEN");
  });

  it("sets GRID layout props and skips flex axis props", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "GRID",
      gridRowCount: 2,
      gridColumnCount: 3,
      gridRowGap: 8,
      gridColumnGap: 16,
      primaryAxisAlignItems: "CENTER", // should be ignored in GRID mode
    });
    expect(frame.layoutMode).toBe("GRID");
    expect(frame.gridRowCount).toBe(2);
    expect(frame.gridColumnCount).toBe(3);
    expect(frame.gridRowGap).toBe(8);
    expect(frame.gridColumnGap).toBe(16);
    expect(frame.primaryAxisAlignItems).toBeUndefined();
  });

  it("sets frame min/max constraints", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "VERTICAL",
      minWidth: 100,
      maxWidth: 400,
      minHeight: 50,
      maxHeight: 800,
    });
    expect(frame.minWidth).toBe(100);
    expect(frame.maxWidth).toBe(400);
    expect(frame.minHeight).toBe(50);
    expect(frame.maxHeight).toBe(800);
  });

  it("clears a min/max constraint when null is passed", () => {
    const frame = makeFrame();
    frame.minWidth = 100;
    applyAutoLayout(frame as any, { minWidth: null });
    expect(frame.minWidth).toBeNull();
  });

  it("sets overflowDirection, strokesIncludedInLayout, itemReverseZIndex", () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "VERTICAL",
      overflowDirection: "VERTICAL",
      strokesIncludedInLayout: true,
      itemReverseZIndex: true,
    });
    expect(frame.overflowDirection).toBe("VERTICAL");
    expect(frame.strokesIncludedInLayout).toBe(true);
    expect(frame.itemReverseZIndex).toBe(true);
  });

  it("skips flex-only itemSpacing in GRID mode", async () => {
    const frame = makeFrame();
    applyAutoLayout(frame as any, {
      layoutMode: "GRID",
      itemSpacing: 16,        // flex-only — must be ignored in GRID
      gridColumnGap: 12,      // the correct GRID gap
    });
    expect(frame.itemSpacing).toBe(0);     // unchanged from default
    expect(frame.gridColumnGap).toBe(12);
  });

  it("skips itemSpacing variable binding in GRID mode", async () => {
    const bound: Record<string, any> = {};
    const frame = { ...makeFrame(), setBoundVariable: (f: string, v: any) => { bound[f] = v; } };
    (globalThis as any).figma.variables.getVariableByIdAsync = async (id: string) => ({ id });
    await applyAutoLayout(frame as any, { layoutMode: "GRID", itemSpacingVariableId: "var:gap" });
    expect(bound.itemSpacing).toBeUndefined();
  });

  it("binds grid gap variables", async () => {
    const bound: Record<string, any> = {};
    const frame = { ...makeFrame(), layoutMode: "GRID", setBoundVariable: (f: string, v: any) => { bound[f] = v; } };
    (globalThis as any).figma.variables.getVariableByIdAsync = async (id: string) => ({ id });
    await applyAutoLayout(frame as any, {
      layoutMode: "GRID",
      gridRowGapVariableId: "var:gap",
      gridColumnGapVariableId: "var:gap2",
    });
    expect(bound.gridRowGap).toEqual({ id: "var:gap" });
    expect(bound.gridColumnGap).toEqual({ id: "var:gap2" });
  });
});

// ── applyAutoLayout — NaN guard ───────────────────────────────────────────────

describe("applyAutoLayout NaN guard", () => {
  const makeFrame = () => ({
    layoutMode: "NONE" as string,
    paddingTop: 0 as any,
    paddingRight: 0 as any,
    paddingBottom: 0 as any,
    paddingLeft: 0 as any,
    itemSpacing: 0 as any,
    primaryAxisAlignItems: undefined as any,
    counterAxisAlignItems: undefined as any,
    primaryAxisSizingMode: undefined as any,
    counterAxisSizingMode: undefined as any,
    layoutWrap: undefined as any,
    counterAxisSpacing: undefined as any,
    setBoundVariable: (_field: string, _v: any) => {},
  });

  // Stray string for a numeric padding field must NOT write NaN
  it("skips paddingTop when a non-finite string is provided", async () => {
    const frame = makeFrame();
    await applyAutoLayout(frame as any, { paddingTop: "not-a-number" });
    expect(Number.isFinite(frame.paddingTop)).toBe(true); // stays at 0
    expect(frame.paddingTop).toBe(0);
  });

  it("skips itemSpacing when a non-finite string is provided", async () => {
    const frame = makeFrame();
    await applyAutoLayout(frame as any, { itemSpacing: "variableId:123" });
    expect(Number.isFinite(frame.itemSpacing)).toBe(true);
    expect(frame.itemSpacing).toBe(0);
  });

  it("still sets padding when a valid number is provided", async () => {
    const frame = makeFrame();
    await applyAutoLayout(frame as any, { paddingLeft: 16 });
    expect(frame.paddingLeft).toBe(16);
  });
});

// ── applyAutoLayout — variable binding ────────────────────────────────────────

describe("applyAutoLayout variable binding", () => {
  const makeBindableFrame = () => {
    const bound: Record<string, any> = {};
    return {
      frame: {
        layoutMode: "HORIZONTAL" as string,
        paddingTop: 0 as any,
        paddingRight: 0 as any,
        paddingBottom: 0 as any,
        paddingLeft: 0 as any,
        itemSpacing: 0 as any,
        primaryAxisAlignItems: undefined as any,
        counterAxisAlignItems: undefined as any,
        primaryAxisSizingMode: undefined as any,
        counterAxisSizingMode: undefined as any,
        layoutWrap: undefined as any,
        counterAxisSpacing: undefined as any,
        setBoundVariable(field: string, v: any) { bound[field] = v; },
      },
      bound,
    };
  };

  beforeEach(setupVariablesMock);

  // When paddingLeftVariableId is provided, resolve the variable and call setBoundVariable
  it("binds paddingLeft variable when paddingLeftVariableId provided", async () => {
    const spVar = { id: "var:sp6", name: "spacing/6" };
    mockVariables["var:sp6"] = spVar;
    const { frame, bound } = makeBindableFrame();
    await applyAutoLayout(frame as any, { paddingLeftVariableId: "var:sp6" });
    expect(bound["paddingLeft"]).toBe(spVar);
    expect(frame.paddingLeft).toBe(0); // raw value unchanged
  });

  it("binds itemSpacing variable when itemSpacingVariableId provided", async () => {
    const gapVar = { id: "var:sp4", name: "spacing/4" };
    mockVariables["var:sp4"] = gapVar;
    const { frame, bound } = makeBindableFrame();
    await applyAutoLayout(frame as any, { itemSpacingVariableId: "var:sp4" });
    expect(bound["itemSpacing"]).toBe(gapVar);
  });

  it("binds all four padding variables independently", async () => {
    const v = { id: "var:sp2", name: "spacing/2" };
    mockVariables["var:sp2"] = v;
    const { frame, bound } = makeBindableFrame();
    await applyAutoLayout(frame as any, {
      paddingTopVariableId: "var:sp2",
      paddingRightVariableId: "var:sp2",
      paddingBottomVariableId: "var:sp2",
      paddingLeftVariableId: "var:sp2",
    });
    expect(bound["paddingTop"]).toBe(v);
    expect(bound["paddingRight"]).toBe(v);
    expect(bound["paddingBottom"]).toBe(v);
    expect(bound["paddingLeft"]).toBe(v);
  });

  it("throws (or skips gracefully) when variableId is not found", async () => {
    const { frame, bound } = makeBindableFrame();
    // Variable not in mock — should not bind and not crash
    await applyAutoLayout(frame as any, { paddingLeftVariableId: "var:missing" });
    expect(bound["paddingLeft"]).toBeUndefined();
  });

  it("plain number path still works alongside variable params", async () => {
    const v = { id: "var:sp4", name: "spacing/4" };
    mockVariables["var:sp4"] = v;
    const { frame, bound } = makeBindableFrame();
    await applyAutoLayout(frame as any, {
      paddingTop: 8,
      paddingLeftVariableId: "var:sp4",
    });
    expect(frame.paddingTop).toBe(8);
    expect(bound["paddingLeft"]).toBe(v);
  });
});

// ── base64ToBytes ─────────────────────────────────────────────────────────────

describe("base64ToBytes", () => {
  it("decodes a known base64 string", () => {
    // "Man" → TWFu
    const bytes = base64ToBytes("TWFu");
    expect(bytes).toEqual(new Uint8Array([77, 97, 110]));
  });

  it("decodes base64 with single padding", () => {
    // "Ma" → TWE=
    const bytes = base64ToBytes("TWE=");
    expect(bytes).toEqual(new Uint8Array([77, 97]));
  });

  it("decodes base64 with double padding", () => {
    // "M" → TQ==
    const bytes = base64ToBytes("TQ==");
    expect(bytes).toEqual(new Uint8Array([77]));
  });

  it("decodes a longer string", () => {
    // "Hello" → SGVsbG8=
    const bytes = base64ToBytes("SGVsbG8=");
    expect(Array.from(bytes)).toEqual([72, 101, 108, 108, 111]);
  });

  it("strips non-base64 characters (e.g. newlines)", () => {
    const bytes = base64ToBytes("TW\nFu");
    expect(bytes).toEqual(new Uint8Array([77, 97, 110]));
  });
});

// ── bulkApply progress heartbeat ──────────────────────────────────────────────
// A single bulk op over a large nodeIds array runs serially on the one JS thread
// with no batch-level tick in between. bulkApply must emit a progress_update
// during the loop so the Go-bridge inactivity timer is reset.

describe("bulkApply progress heartbeat", () => {
  let posted: any[];
  beforeEach(() => {
    posted = [];
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => ({ id, name: `n${id}` }),
      commitUndo: () => {},
      ui: { postMessage: (msg: any) => posted.push(msg) },
    };
  });

  it("emits ≥1 progress_update (progress > 0) over a large nodeIds array", async () => {
    const nodeIds = Array.from({ length: 60 }, (_, i) => `1:${i}`);
    const res = await bulkApply(
      { type: "set_fills", requestId: "req-bulk-1", nodeIds },
      (_n, _id) => ({ ok: true }),
    );
    expect(res.data.results).toHaveLength(60);
    const ticks = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-bulk-1",
    );
    expect(ticks.length).toBeGreaterThanOrEqual(1);
    expect(ticks.every((m) => m.progress > 0)).toBe(true);
  });

  it("emits zero progress_update for a single-node apply (no overhead on the hot path)", async () => {
    await bulkApply(
      { type: "set_fills", requestId: "req-bulk-2", nodeIds: ["1:1"] },
      (_n, _id) => ({ ok: true }),
    );
    const ticks = posted.filter((m) => m.type === "progress_update");
    expect(ticks.length).toBe(0);
  });

  it("respects the cadence boundary (49 → 0 ticks, 50 → 1 tick)", async () => {
    const run = async (rid: string, count: number) => {
      await bulkApply(
        { type: "set_fills", requestId: rid, nodeIds: Array.from({ length: count }, (_, i) => `1:${i}`) },
        () => ({ ok: true }),
      );
      return posted.filter((m) => m.type === "progress_update" && m.requestId === rid).length;
    };
    expect(await run("under", 49)).toBe(0);
    expect(await run("at", 50)).toBe(1);
  });

  it("still ticks even when every node errors (timer must survive a failing batch)", async () => {
    await bulkApply(
      { type: "set_fills", requestId: "req-err", nodeIds: Array.from({ length: 50 }, (_, i) => `1:${i}`) },
      () => { throw new Error("nope"); },
    );
    const ticks = posted.filter((m) => m.type === "progress_update" && m.requestId === "req-err");
    expect(ticks.length).toBe(1);
  });
});

// ── bulkApply parallel prefetch ──────────────────────────────────────────────
// Fetches are issued for ALL nodes before any mutation runs (parallel prefetch);
// mutations then run sequentially, preserving result order and per-node errors.
describe("bulkApply parallel prefetch", () => {
  it("issues every getNodeByIdAsync before the first apply, and keeps order", async () => {
    const events: string[] = [];
    let resolveCount = 0;
    (globalThis as any).figma = {
      // Defer resolution so we can observe that ALL fetches are in flight before
      // any apply runs — a serial await-per-node loop would interleave fetch→apply.
      getNodeByIdAsync: async (id: string) => {
        events.push(`fetch:${id}`);
        await Promise.resolve();
        resolveCount++;
        return id === "1:miss" ? null : { id, name: `n${id}` };
      },
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    const res = await bulkApply(
      { type: "set_fills", requestId: "req-pf", nodeIds: ["1:0", "1:miss", "1:2"] },
      (n, _id) => { events.push(`apply:${n.id}`); return { ok: true }; },
    );

    // All three fetches were issued before any apply event.
    const firstApply = events.findIndex((e) => e.startsWith("apply:"));
    const fetchEvents = events.slice(0, firstApply).filter((e) => e.startsWith("fetch:"));
    expect(fetchEvents).toEqual(["fetch:1:0", "fetch:1:miss", "fetch:1:2"]);
    // Order preserved; the missing node yields an ordered "Node not found" entry,
    // and only the found nodes are applied.
    expect(res.data.results.map((r: any) => r.nodeId)).toEqual(["1:0", "1:miss", "1:2"]);
    expect(res.data.results[1].error).toBe("Node not found");
    expect(events.filter((e) => e.startsWith("apply:"))).toEqual(["apply:1:0", "apply:1:2"]);
  });
});

// ── getParentNode ─────────────────────────────────────────────────────────────

describe("getParentNode", () => {
  it("returns currentPage when no parentId given", async () => {
    const result = await getParentNode(undefined);
    expect(result).toBe(mockCurrentPage);
  });

  it("throws when parentId node is not found", async () => {
    await expect(getParentNode("1:999")).rejects.toThrow("Parent node not found: 1:999");
  });

  it("throws when found node cannot have children", async () => {
    mockGetNodeByIdAsync = async () => ({ id: "1:2", name: "rect" }); // no appendChild
    await expect(getParentNode("1:2")).rejects.toThrow("cannot have children");
  });

  it("returns node when it supports appendChild", async () => {
    const parentNode = { id: "1:3", name: "frame", appendChild: () => {} };
    mockGetNodeByIdAsync = async () => parentNode;
    const result = await getParentNode("1:3");
    expect(result).toBe(parentNode);
  });
});
