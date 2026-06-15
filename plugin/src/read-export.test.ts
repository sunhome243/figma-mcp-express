import { describe, it, expect, beforeEach } from "bun:test";
import { handleReadRequest } from "./read-handlers";

// ── Figma global mock ─────────────────────────────────────────────────────────
//
// Covers the read-export tools: get_screenshot (single + multi node, format/scale,
// selection fallback, DOCUMENT/PAGE filtering) and export_frames_to_pdf.
//
// NOTE: there is no `save_screenshots` handler in this codebase — only
// get_screenshot and export_frames_to_pdf exist in read-export.ts. The brief's
// save_screenshots cases are folded into thorough get_screenshot coverage.

let mockNodes: Record<string, any>;
let mockSelection: any[];
let lastExportSettings: any[];
let posted: any[];

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

const makeExportable = (id: string, name: string, type = "FRAME") => ({
  id, name, type, width: 200, height: 120,
  async exportAsync(settings: any) {
    lastExportSettings.push({ id, settings });
    // Bytes are irrelevant to behavior; base64Encode is stubbed deterministically.
    return new Uint8Array([1, 2, 3]);
  },
});

beforeEach(() => {
  lastExportSettings = [];
  mockNodes = {};
  mockSelection = [];
  posted = [];
  (globalThis as any).figma = {
    get currentPage() { return { id: "0:1", name: "Page 1", get selection() { return mockSelection; } }; },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    base64Encode: (_bytes: Uint8Array) => "QkFTRTY0", // deterministic stub
    ui: { postMessage: (msg: any) => posted.push(msg) },
  };
});

const progressTicks = (requestId: string) =>
  posted.filter(
    (m) => m.type === "progress_update" && m.requestId === requestId,
  );

// ── get_screenshot ──────────────────────────────────────────────────────────

describe("get_screenshot", () => {
  it("exports a single node as PNG at default scale 2", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Hero");
    const res = await handleReadRequest(makeRequest("get_screenshot", ["1:1"]));
    expect(res?.data.exports).toHaveLength(1);
    const exp = res?.data.exports[0];
    expect(exp.nodeId).toBe("1:1");
    expect(exp.nodeName).toBe("Hero");
    expect(exp.format).toBe("PNG");
    expect(exp.base64).toBe("QkFTRTY0");
    expect(exp.width).toBe(200);
    expect(lastExportSettings[0].settings).toEqual({ format: "PNG", constraint: { type: "SCALE", value: 2 } });
  });

  it("honors a custom scale for PNG/JPG exports", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Hero");
    await handleReadRequest(makeRequest("get_screenshot", ["1:1"], { scale: 4 }));
    expect(lastExportSettings[0].settings.constraint.value).toBe(4);
  });

  it("exports SVG without a scale constraint", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Icon");
    const res = await handleReadRequest(makeRequest("get_screenshot", ["1:1"], { format: "SVG" }));
    expect(res?.data.exports[0].format).toBe("SVG");
    expect(lastExportSettings[0].settings).toEqual({ format: "SVG" });
  });

  it("exports JPG with a scale constraint", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Photo");
    await handleReadRequest(makeRequest("get_screenshot", ["1:1"], { format: "JPG", scale: 1 }));
    expect(lastExportSettings[0].settings).toEqual({ format: "JPG", constraint: { type: "SCALE", value: 1 } });
  });

  it("exports multiple nodeIds and returns one entry per node", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "A");
    mockNodes["2:2"] = makeExportable("2:2", "B");
    const res = await handleReadRequest(makeRequest("get_screenshot", ["1:1", "2:2"]));
    expect(res?.data.exports).toHaveLength(2);
    expect(res?.data.exports.map((e: any) => e.nodeId)).toEqual(["1:1", "2:2"]);
  });

  it("filters out DOCUMENT and PAGE nodes from the export set", async () => {
    mockNodes["0:0"] = { id: "0:0", name: "Document", type: "DOCUMENT" };
    mockNodes["0:1"] = { id: "0:1", name: "Page 1", type: "PAGE" };
    mockNodes["1:1"] = makeExportable("1:1", "Frame");
    const res = await handleReadRequest(makeRequest("get_screenshot", ["0:0", "0:1", "1:1"]));
    expect(res?.data.exports).toHaveLength(1);
    expect(res?.data.exports[0].nodeId).toBe("1:1");
  });

  it("falls back to the current page selection when no nodeIds are given", async () => {
    mockSelection = [makeExportable("3:3", "Selected")];
    const res = await handleReadRequest(makeRequest("get_screenshot", []));
    expect(res?.data.exports).toHaveLength(1);
    expect(res?.data.exports[0].nodeId).toBe("3:3");
  });

  it("throws when there are no nodes to export", async () => {
    await expect(
      handleReadRequest(makeRequest("get_screenshot", []))
    ).rejects.toThrow("No nodes to export");
  });

  // Single-threaded survival: each frame export is expensive and blocks the JS
  // thread. A progress_update per frame (progress > 0) resets the Go-bridge
  // inactivity timer so a multi-frame export is not killed by the watchdog.
  it("emits a progress_update per frame (progress > 0) during a multi-node export", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "A");
    mockNodes["2:2"] = makeExportable("2:2", "B");
    mockNodes["3:3"] = makeExportable("3:3", "C");
    const res = await handleReadRequest(makeRequest("get_screenshot", ["1:1", "2:2", "3:3"]));
    expect(res?.data.exports).toHaveLength(3);
    const ticks = progressTicks("req-test-1");
    expect(ticks.length).toBeGreaterThanOrEqual(3);
    // CRITICAL: progress must be > 0 or the Go bridge ignores the tick.
    expect(ticks.every((m) => m.progress > 0)).toBe(true);
  });

  it("ticks exactly once per frame and preserves export order/data (serial refactor)", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "A");
    mockNodes["2:2"] = makeExportable("2:2", "B");
    const res = await handleReadRequest(makeRequest("get_screenshot", ["1:1", "2:2"]));
    // Data integrity after the Promise.all → serial-loop refactor.
    expect(res?.data.exports.map((e: any) => e.nodeId)).toEqual(["1:1", "2:2"]);
    expect(res?.data.exports.every((e: any) => e.base64 === "QkFTRTY0")).toBe(true);
    // every=1 → one tick per exported frame.
    expect(progressTicks("req-test-1").length).toBe(2);
  });

  it("still ticks for SVG (no scale constraint path)", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Icon");
    mockNodes["2:2"] = makeExportable("2:2", "Icon2");
    await handleReadRequest(makeRequest("get_screenshot", ["1:1", "2:2"], { format: "SVG" }));
    expect(progressTicks("req-test-1").length).toBe(2);
  });
});

// ── export_frames_to_pdf ────────────────────────────────────────────────────

describe("export_frames_to_pdf", () => {
  it("exports a single frame to PDF", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Cover");
    const res = await handleReadRequest(makeRequest("export_frames_to_pdf", ["1:1"]));
    expect(res?.data.frames).toHaveLength(1);
    expect(res?.data.frames[0]).toEqual({ nodeId: "1:1", nodeName: "Cover", base64: "QkFTRTY0" });
    expect(lastExportSettings[0].settings).toEqual({ format: "PDF" });
  });

  it("exports multiple frames preserving order", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Page A");
    mockNodes["2:2"] = makeExportable("2:2", "Page B");
    mockNodes["3:3"] = makeExportable("3:3", "Page C");
    const res = await handleReadRequest(makeRequest("export_frames_to_pdf", ["1:1", "2:2", "3:3"]));
    expect(res?.data.frames.map((f: any) => f.nodeName)).toEqual(["Page A", "Page B", "Page C"]);
  });

  it("emits a progress_update per frame (progress > 0) during a multi-frame PDF export", async () => {
    mockNodes["1:1"] = makeExportable("1:1", "Page A");
    mockNodes["2:2"] = makeExportable("2:2", "Page B");
    mockNodes["3:3"] = makeExportable("3:3", "Page C");
    await handleReadRequest(makeRequest("export_frames_to_pdf", ["1:1", "2:2", "3:3"]));
    const ticks = progressTicks("req-test-1");
    expect(ticks.length).toBeGreaterThanOrEqual(3);
    expect(ticks.every((m) => m.progress > 0)).toBe(true);
  });

  it("throws when nodeIds is empty", async () => {
    await expect(
      handleReadRequest(makeRequest("export_frames_to_pdf", []))
    ).rejects.toThrow("nodeIds is required");
  });

  it("throws when a node is not found", async () => {
    await expect(
      handleReadRequest(makeRequest("export_frames_to_pdf", ["9:9"]))
    ).rejects.toThrow("not found or is not exportable");
  });

  it("throws when a target node is a PAGE (not exportable)", async () => {
    mockNodes["0:1"] = { id: "0:1", name: "Page 1", type: "PAGE" };
    await expect(
      handleReadRequest(makeRequest("export_frames_to_pdf", ["0:1"]))
    ).rejects.toThrow("not found or is not exportable");
  });

  it("returns null for an unrecognised read request type", async () => {
    const res = await handleReadRequest(makeRequest("unknown_read_op"));
    expect(res).toBeNull();
  });
});
