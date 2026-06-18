import { describe, it, expect, beforeEach } from "bun:test";
import {
  serializeNode,
  serializeStyles,
  serializeComponentRef,
  deduplicateStyles,
} from "./serializers";

// ── Instrumented figma mock — counts async lookups for memoization proof ─────

let styleCalls: string[] = [];
let mainComponentCalls = 0;

beforeEach(() => {
  styleCalls = [];
  mainComponentCalls = 0;
  (globalThis as any).figma = {
    getStyleByIdAsync: async (id: string) => {
      styleCalls.push(id);
      return { name: `style:${id}` };
    },
  };
});

// A small fixture tree where N nodes SHARE the same fillStyleId so the
// per-read memo can demonstrably collapse N lookups → 1.
const makeSharedStyleTree = () => ({
  id: "root",
  name: "Root",
  type: "FRAME",
  x: 0,
  y: 0,
  width: 100,
  height: 100,
  fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }],
  fillStyleId: "S_SHARED",
  children: [
    {
      id: "a",
      name: "A",
      type: "FRAME",
      x: 0,
      y: 0,
      width: 10,
      height: 10,
      fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }],
      fillStyleId: "S_SHARED",
    },
    {
      id: "b",
      name: "B",
      type: "FRAME",
      x: 0,
      y: 0,
      width: 10,
      height: 10,
      fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }],
      fillStyleId: "S_SHARED",
    },
  ],
});

// ── GOLDEN: serializeNode output (the byte-identical contract) ───────────────

describe("serializeNode — golden output (byte-identical contract)", () => {
  it("serializes a shared-style tree to a stable JSON shape", async () => {
    const out = await serializeNode(makeSharedStyleTree());
    // Captured from the unchanged build BEFORE the memoization refactor.
    expect(JSON.stringify(out)).toBe(
      JSON.stringify({
        id: "root",
        name: "Root",
        type: "FRAME",
        bounds: { x: 0, y: 0, width: 100, height: 100 },
        styles: { fillStyle: "style:S_SHARED", fills: ["#ff0000"] },
        children: [
          {
            id: "a",
            name: "A",
            type: "FRAME",
            bounds: { x: 0, y: 0, width: 10, height: 10 },
            styles: { fillStyle: "style:S_SHARED", fills: ["#ff0000"] },
          },
          {
            id: "b",
            name: "B",
            type: "FRAME",
            bounds: { x: 0, y: 0, width: 10, height: 10 },
            styles: { fillStyle: "style:S_SHARED", fills: ["#ff0000"] },
          },
        ],
      }),
    );
  });
});

// ── 7R-2: per-read memoization of getStyleByIdAsync ──────────────────────────

describe("serializeStyles — per-read style cache (7R-2)", () => {
  it("collapses N lookups of the same style id to 1 when a cache is shared", async () => {
    const cache = new Map<string, any>();
    const tree = makeSharedStyleTree();
    // Serialize root + both children sharing one shared cache.
    await serializeStyles(tree, cache);
    await serializeStyles(tree.children[0], cache);
    await serializeStyles(tree.children[1], cache);
    // Without the cache this is 3 calls; with it, exactly 1.
    expect(styleCalls.filter((id) => id === "S_SHARED").length).toBe(1);
  });

  it("caches null/undefined results too (no repeat lookup on a miss)", async () => {
    (globalThis as any).figma.getStyleByIdAsync = async (id: string) => {
      styleCalls.push(id);
      return null;
    };
    const cache = new Map<string, any>();
    const node = {
      fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 }, opacity: 1 }],
      fillStyleId: "MISS",
    };
    await serializeStyles(node, cache);
    await serializeStyles(node, cache);
    expect(styleCalls.filter((id) => id === "MISS").length).toBe(1);
  });

  it("default (no cache arg) stays byte-identical for existing callers", async () => {
    const a = await serializeStyles(makeSharedStyleTree());
    const b = await serializeStyles(makeSharedStyleTree());
    expect(JSON.stringify(a)).toBe(JSON.stringify(b));
    expect(a).toEqual({ fillStyle: "style:S_SHARED", fills: ["#ff0000"] });
  });
});

// ── 7R-2: serializeNode threads ONE cache across the whole subtree ───────────

describe("serializeNode — shares a style cache across the subtree (7R-2)", () => {
  it("resolves a shared style id once for the whole tree", async () => {
    await serializeNode(makeSharedStyleTree());
    expect(styleCalls.filter((id) => id === "S_SHARED").length).toBe(1);
  });
});

// ── 7R-2: serializeComponentRef memoization via shared cache ─────────────────

describe("serializeComponentRef — per-read component cache (7R-2)", () => {
  const makeInstance = (id: string, mcId: string) => ({
    id,
    name: id,
    type: "INSTANCE",
    getMainComponentAsync: async () => {
      mainComponentCalls++;
      return { id: mcId, key: `key:${mcId}`, name: `comp:${mcId}` };
    },
  });

  // Achievable contract: the SAME instance resolved twice within one read hits
  // the cache the second time. Under documentAccess:"dynamic-page" the main
  // component id is only known AFTER the await, so two DISTINCT instances
  // sharing a component cannot share a pre-call key — the memo is keyed by the
  // instance node id and collapses same-instance re-resolution (the real
  // get_design_context double-resolve), not cross-instance sharing.
  it("collapses repeated lookups of the SAME instance via a shared cache", async () => {
    const cache = new Map<string, any>();
    const inst = makeInstance("i1", "MC");
    const r1 = await serializeComponentRef(inst, cache);
    const r2 = await serializeComponentRef(inst, cache);
    expect(mainComponentCalls).toBe(1);
    // Includes remote field + the master node id (issue #29); surfacing sites
    // (mainComponent / componentRef) strip the id back out to {key,name,remote}.
    expect(r1).toEqual({ key: "key:MC", name: "comp:MC", remote: false, id: "MC" });
    expect(r2).toEqual({ key: "key:MC", name: "comp:MC", remote: false, id: "MC" });
  });

  it("default (no cache) preserves existing per-call behavior", async () => {
    const ref = await serializeComponentRef(makeInstance("x", "Z"));
    // Carries the master node id alongside remote (issue #29).
    expect(ref).toEqual({ key: "key:Z", name: "comp:Z", remote: false, id: "Z" });
  });

  it("reflects remote:true when mc.remote is true", async () => {
    const remoteInst = {
      id: "ri1",
      name: "ri1",
      type: "INSTANCE",
      getMainComponentAsync: async () => {
        mainComponentCalls++;
        return { id: "MC:remote", key: "key:remote", name: "comp:remote", remote: true };
      },
    };
    const ref = await serializeComponentRef(remoteInst);
    expect(ref?.remote).toBe(true);
  });
});

// ── 6B-1: in-place pass-2 dedup stays byte-identical ─────────────────────────

describe("deduplicateStyles — golden output after in-place pass-2 (6B-1)", () => {
  const sharedFillTree = () => ({
    id: "r",
    name: "r",
    type: "FRAME",
    styles: { fills: ["#abcabc"] },
    children: [
      { id: "c1", name: "c1", type: "FRAME", styles: { fills: ["#abcabc"] } },
      { id: "c2", name: "c2", type: "FRAME", styles: { fills: ["#abcabc"] } },
    ],
  });

  it("produces the stable s1 ref naming + globalVars.styles shape", () => {
    const { tree, globalVars } = deduplicateStyles(sharedFillTree());
    expect(JSON.stringify({ tree, globalVars })).toBe(
      JSON.stringify({
        tree: {
          id: "r",
          name: "r",
          type: "FRAME",
          styles: { fills: "s1" },
          children: [
            { id: "c1", name: "c1", type: "FRAME", styles: { fills: "s1" } },
            { id: "c2", name: "c2", type: "FRAME", styles: { fills: "s1" } },
          ],
        },
        globalVars: { styles: { s1: ["#abcabc"] } },
      }),
    );
  });

  it("preserves object key order (id,name,type,styles,children) after rewrite", () => {
    const { tree } = deduplicateStyles(sharedFillTree());
    expect(Object.keys(tree)).toEqual(["id", "name", "type", "styles", "children"]);
    expect(Object.keys(tree.children[0])).toEqual(["id", "name", "type", "styles"]);
  });
});
