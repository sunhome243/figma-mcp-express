import { describe, it, expect, beforeEach } from "bun:test";
import { handleReadDocumentRequest } from "./read-document";

// ── Figma global mock helpers ─────────────────────────────────────────────────

const makeRequest = (type: string, params?: any, nodeIds?: string[]) => ({
  type,
  requestId: "req-test-42",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

// Build a flat-tree node with N leaf children under a single root.
// All nodes are visible FRAME types with no grandchildren — purely flat.
// The root itself has `children` so the scanner will iterate them.
const makeFlatTree = (rootId: string, childCount: number) => {
  const children = Array.from({ length: childCount }, (_, i) => ({
    id: `child:${i}`,
    name: `Node ${i}`,
    type: "FRAME" as const,
    visible: true,
    x: i,
    y: 0,
    width: 100,
    height: 100,
    // leaf — no children
  }));
  return {
    id: rootId,
    name: "Root",
    type: "FRAME" as const,
    visible: true,
    x: 0,
    y: 0,
    width: 1000,
    height: 1000,
    children,
  };
};

// Build a 2-level tree: root → N frames each with M leaf children.
// Total descendants = N*M children + N parent frames = N*(M+1)
const makeDeepTree = (
  rootId: string,
  branchCount: number,
  leavesPerBranch: number,
  leafType: string = "FRAME",
) => {
  const branches = Array.from({ length: branchCount }, (_, b) => {
    const leaves = Array.from({ length: leavesPerBranch }, (_, l) => ({
      id: `leaf:${b}:${l}`,
      name: `Leaf ${b}-${l}`,
      type: leafType as any,
      visible: true,
      x: l * 10,
      y: b * 10,
      width: 50,
      height: 50,
      characters: leafType === "TEXT" ? `Text ${b}-${l}` : undefined,
      fontSize: leafType === "TEXT" ? 14 : undefined,
      fontName: leafType === "TEXT" ? { family: "Inter", style: "Regular" } : undefined,
    }));
    return {
      id: `branch:${b}`,
      name: `Branch ${b}`,
      type: "FRAME" as const,
      visible: true,
      x: b * 100,
      y: 0,
      width: 500,
      height: 500,
      children: leaves,
    };
  });
  return {
    id: rootId,
    name: "DeepRoot",
    type: "FRAME" as const,
    visible: true,
    x: 0,
    y: 0,
    width: 5000,
    height: 5000,
    children: branches,
  };
};

// ── Setup: record postMessage calls ──────────────────────────────────────────

let posted: any[] = [];

// Replicate Figma's native node.findAllWithCriteria({types}) on the mock tree so
// 7R-1's native-scan path is exercised. Real semantics (per the Plugin API):
//   • DFS pre-order over DESCENDANTS only — EXCLUDES the receiver itself.
//   • Includes invisible nodes and their children (no visibility pruning).
// scan_* honors visibility itself (it post-filters), so the mock must NOT prune.
const attachFindAll = (node: any, parent: any = null) => {
  if (!node || typeof node !== "object") return;
  // Mirror real Figma: every node exposes .parent (needed by scan_*'s ancestor
  // visibility post-filter).
  node.parent = parent;
  // findAllWithCriteria exists ONLY on container nodes (ChildrenMixin); leaf
  // nodes (TextNode/RectangleNode/VectorNode…) do NOT have it. Attach it only to
  // nodes that have a children array so the mock reflects real Figma and the
  // scan handlers' leaf-root fallback path is actually exercised.
  if (Array.isArray(node.children)) {
    node.findAllWithCriteria = ({ types }: { types: string[] }) => {
      // Honor figma.skipInvisibleInstanceChildren like real Figma: when set, do not
      // descend into the children of an INVISIBLE INSTANCE node (the instance node
      // itself is still visited). Other invisible nodes are NOT skipped by the flag.
      const skipInvisibleInst = (globalThis as any).figma
        ?.skipInvisibleInstanceChildren === true;
      const out: any[] = [];
      const walk = (n: any) => {
        if (Array.isArray(n.children)) {
          for (const c of n.children) {
            if (types.includes(c.type)) out.push(c);
            const skipChildren =
              skipInvisibleInst && c.type === "INSTANCE" && c.visible === false;
            if (!skipChildren) walk(c);
          }
        }
      };
      walk(node);
      return out;
    };
    node.children.forEach((c: any) => attachFindAll(c, node));
  }
};

const setupFigma = (rootNode?: any) => {
  posted = [];
  if (rootNode) attachFindAll(rootNode);
  (globalThis as any).figma = {
    skipInvisibleInstanceChildren: false,
    root: { name: "TestFile", children: [] },
    currentPage: rootNode ?? {
      id: "page:1",
      name: "Page 1",
      type: "PAGE",
      children: [],
      selection: [],
    },
    viewport: {
      center: { x: 0, y: 0 },
      zoom: 1,
      bounds: { x: 0, y: 0, width: 1000, height: 1000 },
    },
    getNodeByIdAsync: async (id: string) => {
      if (rootNode && rootNode.id === id) return rootNode;
      // Walk rootNode tree
      if (rootNode) {
        const find = (n: any): any => {
          if (n.id === id) return n;
          if (n.children) {
            for (const c of n.children) {
              const found = find(c);
              if (found) return found;
            }
          }
          return null;
        };
        return find(rootNode);
      }
      return null;
    },
    ui: {
      postMessage: (msg: any) => posted.push(msg),
    },
  };
};

// ── search_nodes: progress tests ─────────────────────────────────────────────

describe("search_nodes — cooperative yielding", () => {
  // LARGE FIXTURE: root with 2000 leaf children (> 800 threshold)
  // Use limit:100000 so the early-exit doesn't stop traversal before 800 nodes.
  it("emits ≥1 progress_update with correct requestId on a large subtree (>800 nodes)", async () => {
    const root = makeFlatTree("root:big", 2000);
    setupFigma(root);

    await handleReadDocumentRequest(
      makeRequest("search_nodes", {
        nodeId: "root:big",
        limit: 100000,
        query: "",
      }),
    );

    const progressMsgs = posted.filter(
      (m) =>
        m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBeGreaterThanOrEqual(1);
    // CRITICAL: progress_update messages must have progress > 0 so the Go bridge
    // routes them to the progress branch rather than the resolution block.
    expect(progressMsgs[0].progress).toBeGreaterThan(0);
  });

  // SMALL FIXTURE: root with 400 leaf children (< 800 threshold)
  // search_nodes has NO unconditional progress_update, so small must emit ZERO.
  it("emits ZERO progress_update on a small subtree (<800 nodes)", async () => {
    const root = makeFlatTree("root:small", 400);
    setupFigma(root);

    await handleReadDocumentRequest(
      makeRequest("search_nodes", {
        nodeId: "root:small",
        limit: 100000,
        query: "",
      }),
    );

    const progressMsgs = posted.filter(
      (m) =>
        m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBe(0);
  });

  // CORRECTNESS: result set must be identical regardless of yielding
  it("returns correct matched nodes despite yielding — result is identical to expected set", async () => {
    // 900 frames + 100 TEXT nodes; query matches "text-node" by name prefix
    const children = [
      ...Array.from({ length: 900 }, (_, i) => ({
        id: `frame:${i}`,
        name: `frame-node-${i}`,
        type: "FRAME" as const,
        visible: true,
        x: i,
        y: 0,
        width: 10,
        height: 10,
      })),
      ...Array.from({ length: 100 }, (_, i) => ({
        id: `text:${i}`,
        name: `text-node-${i}`,
        type: "TEXT" as const,
        visible: true,
        x: i,
        y: 10,
        width: 10,
        height: 10,
      })),
    ];
    const root = {
      id: "root:mixed",
      name: "Root",
      type: "FRAME" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 1000,
      height: 1000,
      children,
    };
    setupFigma(root);

    const res = await handleReadDocumentRequest(
      makeRequest("search_nodes", {
        nodeId: "root:mixed",
        limit: 100000,
        query: "text-node",
      }),
    );

    expect(res?.data.count).toBe(100);
    const ids = res?.data.nodes.map((n: any) => n.id);
    // All 100 text nodes must be present
    for (let i = 0; i < 100; i++) {
      expect(ids).toContain(`text:${i}`);
    }
    // No frame nodes should appear
    for (let i = 0; i < 900; i++) {
      expect(ids).not.toContain(`frame:${i}`);
    }
  });
});

// ── scan_nodes_by_types: progress tests ──────────────────────────────────────

describe("scan_nodes_by_types — native findAllWithCriteria (7R-1)", () => {
  // Native scans no longer emit per-node "processed N nodes" ticks — the native
  // traversal has no JS-side per-node hook. The single unconditional
  // progress_update (progress:10, "Scanning for types") still fires.
  it("emits exactly the single unconditional progress_update, no per-node ticks", async () => {
    const root = makeFlatTree("root:scan-big", 2000);
    setupFigma(root);

    await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "root:scan-big",
        types: ["FRAME"],
      }),
    );

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBe(1);
    expect(progressMsgs[0].progress).toBe(10);
    // No "processed N nodes" threshold ticks — those belonged to the JS walk.
    const processed = progressMsgs.filter(
      (m) => typeof m.message === "string" && m.message.includes("processed"),
    );
    expect(processed.length).toBe(0);
  });

  // BYTE-IDENTICAL: a type-matching, visible root is INCLUDED first (mirrors the
  // old manual findByTypes(root) which visited root before its children).
  it("includes a matching visible root as the first result", async () => {
    const root = makeFlatTree("root:rootmatch", 3); // root + 3 FRAME children, all FRAME
    setupFigma(root);
    const res = await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "root:rootmatch",
        types: ["FRAME"],
      }),
    );
    expect(res?.data.count).toBe(4); // root + 3 children
    expect(res?.data.matchingNodes[0].id).toBe("root:rootmatch");
    expect(res?.data.matchingNodes[0].bbox).toEqual({
      x: 0,
      y: 0,
      width: 1000,
      height: 1000,
    });
  });

  // BYTE-IDENTICAL: a hidden subtree drops the hidden node AND all its
  // descendants — exactly what the old `if (!visible) return` prune did.
  it("prunes a hidden subtree (node + descendants), like the manual walk", async () => {
    const root = {
      id: "root:vis",
      name: "Root",
      type: "FRAME" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      children: [
        {
          id: "visible-branch",
          name: "VisibleBranch",
          type: "FRAME" as const,
          visible: true,
          x: 0,
          y: 0,
          width: 10,
          height: 10,
          children: [
            { id: "vchild", name: "VChild", type: "FRAME" as const, visible: true, x: 0, y: 0, width: 5, height: 5 },
          ],
        },
        {
          id: "hidden-branch",
          name: "HiddenBranch",
          type: "FRAME" as const,
          visible: false, // hidden — this node AND its child must be dropped
          x: 0,
          y: 0,
          width: 10,
          height: 10,
          children: [
            { id: "hchild", name: "HChild", type: "FRAME" as const, visible: true, x: 0, y: 0, width: 5, height: 5 },
          ],
        },
      ],
    };
    setupFigma(root);
    const res = await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "root:vis",
        types: ["FRAME"],
      }),
    );
    const ids = res?.data.matchingNodes.map((n: any) => n.id);
    // root + visible-branch + vchild = 3. hidden-branch and hchild dropped.
    expect(ids).toEqual(["root:vis", "visible-branch", "vchild"]);
  });

  // PERF HINT: the scan sets skipInvisibleInstanceChildren=true during the native
  // traversal and RESTORES it afterward. Byte-identical-safe because the ancestor
  // visibility filter already drops anything under an invisible node, so skipping
  // invisible-instance descendants cannot change the result set.
  it("sets skipInvisibleInstanceChildren during scan and restores it, result unchanged", async () => {
    const root = {
      id: "root:flag",
      name: "Root",
      type: "FRAME" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      children: [
        { id: "v1", name: "V1", type: "FRAME" as const, visible: true, x: 0, y: 0, width: 5, height: 5 },
        {
          id: "hidden-inst",
          name: "HiddenInst",
          type: "INSTANCE" as const,
          visible: false, // invisible instance — flag skips its children
          x: 0,
          y: 0,
          width: 5,
          height: 5,
          children: [
            { id: "inst-child", name: "InstChild", type: "FRAME" as const, visible: true, x: 0, y: 0, width: 2, height: 2 },
          ],
        },
      ],
    };
    setupFigma(root);
    (globalThis as any).figma.skipInvisibleInstanceChildren = false; // pre-state

    const res = await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "root:flag",
        types: ["FRAME"],
      }),
    );

    // Flag restored to its pre-scan value after the scan.
    expect((globalThis as any).figma.skipInvisibleInstanceChildren).toBe(false);
    // Result: root + v1. The invisible instance is itself an INSTANCE (not FRAME),
    // and inst-child is dropped both by the flag AND the ancestor filter.
    const ids = res?.data.matchingNodes.map((n: any) => n.id);
    expect(ids).toEqual(["root:flag", "v1"]);
  });

  // CORRECTNESS: result must include all matching nodes
  it("returns all matching nodes correctly despite yielding", async () => {
    // 1000 frames + 500 texts in a flat layout
    const children = [
      ...Array.from({ length: 1000 }, (_, i) => ({
        id: `frm:${i}`,
        name: `Frame ${i}`,
        type: "FRAME" as const,
        visible: true,
        x: i,
        y: 0,
        width: 10,
        height: 10,
      })),
      ...Array.from({ length: 500 }, (_, i) => ({
        id: `txt:${i}`,
        name: `Text ${i}`,
        type: "TEXT" as const,
        visible: true,
        x: i,
        y: 10,
        width: 10,
        height: 10,
      })),
    ];
    const root = {
      id: "root:type-scan",
      name: "Root",
      type: "FRAME" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 2000,
      height: 200,
      children,
    };
    setupFigma(root);

    const res = await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "root:type-scan",
        types: ["TEXT"],
      }),
    );

    expect(res?.data.count).toBe(500);
    const ids = res?.data.matchingNodes.map((n: any) => n.id);
    for (let i = 0; i < 500; i++) {
      expect(ids).toContain(`txt:${i}`);
    }
    // FRAME nodes must NOT appear
    for (let i = 0; i < 1000; i++) {
      expect(ids).not.toContain(`frm:${i}`);
    }
  });

  // Scan also emits `bounds` (same as bbox) so consumers can use either field
  it("emits both bbox and bounds fields on each match", async () => {
    const root = makeFlatTree("root:bbox-bounds", 1);
    setupFigma(root);
    const res = await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "root:bbox-bounds",
        types: ["FRAME"],
      }),
    );
    const match = res?.data.matchingNodes[0];
    expect(match.bbox).toBeDefined();
    expect(match.bounds).toBeDefined();
    // Both fields are identical
    expect(match.bounds).toEqual(match.bbox);
    // Key fields present
    expect(match.bounds.x).toBeDefined();
    expect(match.bounds.width).toBeDefined();
  });

  // LEAF ROOT (H1 regression): a leaf node has no findAllWithCriteria. The old
  // manual walk returned [root] when root's own type matched. Must not crash.
  it("returns [root] for a matching LEAF root (no findAllWithCriteria)", async () => {
    const leaf = {
      id: "leaf:rect",
      name: "Rect",
      type: "RECTANGLE" as const,
      visible: true,
      x: 5,
      y: 6,
      width: 7,
      height: 8,
      // no children → mock omits findAllWithCriteria, like real Figma
    };
    setupFigma(leaf);
    const res = await handleReadDocumentRequest(
      makeRequest("scan_nodes_by_types", {
        nodeId: "leaf:rect",
        types: ["RECTANGLE"],
      }),
    );
    expect(res?.data.count).toBe(1);
    expect(res?.data.matchingNodes[0].id).toBe("leaf:rect");
    expect(res?.data.matchingNodes[0].bbox).toEqual({ x: 5, y: 6, width: 7, height: 8 });
  });
});

// ── scan_text_nodes: progress tests ──────────────────────────────────────────

describe("scan_text_nodes — native findAllWithCriteria (7R-1)", () => {
  // Native scan: only the single unconditional progress_update fires, no
  // per-node "processed N nodes" ticks.
  it("emits exactly the single unconditional progress_update, no per-node ticks", async () => {
    // 40 branches × 50 TEXT leaves = 2000 descendant nodes
    const root = makeDeepTree("root:txt-scan", 40, 50, "TEXT");
    setupFigma(root);

    await handleReadDocumentRequest(
      makeRequest("scan_text_nodes", {
        nodeId: "root:txt-scan",
      }),
    );

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBe(1);
    expect(progressMsgs[0].progress).toBe(10);
    const processed = progressMsgs.filter(
      (m) => typeof m.message === "string" && m.message.includes("processed"),
    );
    expect(processed.length).toBe(0);
  });

  // CORRECTNESS: all TEXT nodes must be returned
  it("returns all TEXT nodes correctly despite yielding", async () => {
    // 10 branches × 200 TEXT leaves = 2000 text descendants
    const root = makeDeepTree("root:txt-correct", 10, 200, "TEXT");
    setupFigma(root);

    const res = await handleReadDocumentRequest(
      makeRequest("scan_text_nodes", {
        nodeId: "root:txt-correct",
      }),
    );

    // 10 branches × 200 leaves = 2000 text nodes
    expect(res?.data.count).toBe(2000);

    // Spot-check a few IDs
    const ids = res?.data.textNodes.map((n: any) => n.id);
    expect(ids).toContain("leaf:0:0");
    expect(ids).toContain("leaf:9:199");
  });

  // LEAF ROOT (H1 regression): a TEXT leaf has no findAllWithCriteria. The old
  // manual walk returned [root] when root was TEXT. Must not crash.
  it("returns [root] for a TEXT leaf root (no findAllWithCriteria)", async () => {
    const textLeaf = {
      id: "leaf:txt",
      name: "Label",
      type: "TEXT" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 10,
      height: 10,
      characters: "Hello",
      fontSize: 14,
      fontName: { family: "Inter", style: "Regular" },
      // no children → mock omits findAllWithCriteria
    };
    setupFigma(textLeaf);
    const res = await handleReadDocumentRequest(
      makeRequest("scan_text_nodes", { nodeId: "leaf:txt" }),
    );
    expect(res?.data.count).toBe(1);
    expect(res?.data.textNodes[0].id).toBe("leaf:txt");
    expect(res?.data.textNodes[0].characters).toBe("Hello");
  });

  // LEAF ROOT, non-matching: a non-TEXT leaf root yields [] (old walk pushed
  // nothing). Must not crash.
  it("returns [] for a non-TEXT leaf root (no findAllWithCriteria)", async () => {
    const rectLeaf = {
      id: "leaf:rect2",
      name: "Rect",
      type: "RECTANGLE" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 10,
      height: 10,
      // no children → mock omits findAllWithCriteria
    };
    setupFigma(rectLeaf);
    const res = await handleReadDocumentRequest(
      makeRequest("scan_text_nodes", { nodeId: "leaf:rect2" }),
    );
    expect(res?.data.count).toBe(0);
    expect(res?.data.textNodes).toEqual([]);
  });
});

// ── get_node: depth param + cooperative yielding ─────────────────────────────

describe("get_node — depth param", () => {
  // (a) depth:1 returns node + direct children only (no grandchildren)
  it("depth:1 returns node and direct children but no grandchildren", async () => {
    // 5 branches × 10 leaves = 50 grandchildren under root
    const root = makeDeepTree("root:depth", 5, 10);
    setupFigma(root);

    const res = await handleReadDocumentRequest({
      type: "get_node",
      requestId: "req-test-42",
      nodeIds: ["root:depth"],
      params: { depth: 1 },
    });

    expect(res?.data).toBeDefined();
    // Root has children (branches)
    expect(res?.data.children).toHaveLength(5);
    // Each branch child must be truncated: no children array, has childCount instead
    const branch0 = res?.data.children[0];
    expect(branch0.children).toBeUndefined();
    expect(branch0.childCount).toBe(10);
  });

  // (a) full depth (no depth param) returns entire subtree
  it("full depth (no params.depth) returns entire subtree including grandchildren", async () => {
    const root = makeDeepTree("root:full", 3, 4);
    setupFigma(root);

    const res = await handleReadDocumentRequest({
      type: "get_node",
      requestId: "req-test-42",
      nodeIds: ["root:full"],
      params: {},
    });

    expect(res?.data.children).toHaveLength(3);
    // Grandchildren must be present
    expect(res?.data.children[0].children).toHaveLength(4);
    expect(res?.data.children[0].children[0].id).toMatch(/^leaf:/);
  });

  // (b) progress_update emitted on large subtree (>800 nodes)
  it("emits ≥1 progress_update with correct requestId on a large subtree (>800 nodes)", async () => {
    // 2 branches × 1000 leaves = 2000 descendant nodes
    const root = makeDeepTree("root:big-node", 2, 1000);
    setupFigma(root);

    await handleReadDocumentRequest({
      type: "get_node",
      requestId: "req-test-42",
      nodeIds: ["root:big-node"],
      params: {},
    });

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBeGreaterThanOrEqual(1);
    expect(progressMsgs[0].progress).toBeGreaterThan(0);
  });

  // (c) small node emits zero progress
  it("emits zero progress_update on a small node (<800 nodes)", async () => {
    const root = makeDeepTree("root:small-node", 3, 5);
    setupFigma(root);

    await handleReadDocumentRequest({
      type: "get_node",
      requestId: "req-test-42",
      nodeIds: ["root:small-node"],
      params: {},
    });

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBe(0);
  });

  // (d) full-depth output is structurally correct (correctness guard)
  it("full-depth output contains all nodes in the tree (correctness)", async () => {
    const root = makeDeepTree("root:correct", 4, 6);
    setupFigma(root);

    const res = await handleReadDocumentRequest({
      type: "get_node",
      requestId: "req-test-42",
      nodeIds: ["root:correct"],
      params: {},
    });

    expect(res?.data.id).toBe("root:correct");
    expect(res?.data.children).toHaveLength(4);
    // Each branch has 6 leaf children
    for (let b = 0; b < 4; b++) {
      expect(res?.data.children[b].children).toHaveLength(6);
      expect(res?.data.children[b].children[0].id).toBe(`leaf:${b}:0`);
    }
  });
});

// ── get_nodes_info: cooperative yielding ─────────────────────────────────────

describe("get_nodes_info — cooperative yielding", () => {
  // (b) emits ≥1 progress_update on large subtree
  it("emits ≥1 progress_update with correct requestId on large nodes (>800 total nodes)", async () => {
    // Two large branches each with 500 leaves = 1000 total descendant nodes
    const root = makeDeepTree("root:info-big", 2, 500);
    setupFigma(root);

    await handleReadDocumentRequest({
      type: "get_nodes_info",
      requestId: "req-test-42",
      nodeIds: ["branch:0", "branch:1"],
      params: {},
    });

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBeGreaterThanOrEqual(1);
    expect(progressMsgs[0].progress).toBeGreaterThan(0);
  });

  // (c) small nodes emit zero progress
  it("emits zero progress_update for small nodes (<800 total nodes)", async () => {
    const root = makeDeepTree("root:info-small", 3, 5);
    setupFigma(root);

    await handleReadDocumentRequest({
      type: "get_nodes_info",
      requestId: "req-test-42",
      nodeIds: ["branch:0", "branch:1"],
      params: {},
    });

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBe(0);
  });

  // (d) correctness: all requested nodes are in result
  it("returns all requested nodes with full subtree (correctness)", async () => {
    const root = makeDeepTree("root:info-correct", 4, 3);
    setupFigma(root);

    const res = await handleReadDocumentRequest({
      type: "get_nodes_info",
      requestId: "req-test-42",
      nodeIds: ["branch:0", "branch:2"],
      params: {},
    });

    expect(res?.data).toHaveLength(2);
    expect(res?.data[0].id).toBe("branch:0");
    expect(res?.data[1].id).toBe("branch:2");
    // Each branch has 3 leaf children
    expect(res?.data[0].children).toHaveLength(3);
    expect(res?.data[1].children).toHaveLength(3);
  });

  // (e) NEW: depth cap — get_nodes_info previously recursed unbounded (latent
  // timeout on a giant node). A depth param now bounds it, same as get_node:
  // the node at the cap is serialized but its children collapse to childCount.
  it("depth:1 returns each node + direct children only, grandchildren truncated", async () => {
    // 5 branches × 10 leaves under root → request the root with depth:1
    const root = makeDeepTree("root:info-depth", 5, 10);
    setupFigma(root);

    const res = await handleReadDocumentRequest({
      type: "get_nodes_info",
      requestId: "req-test-42",
      nodeIds: ["root:info-depth"],
      params: { depth: 1 },
    });

    expect(res?.data).toHaveLength(1);
    const node = res?.data[0];
    expect(node.children).toHaveLength(5); // direct children present
    const branch0 = node.children[0];
    expect(branch0.children).toBeUndefined(); // grandchildren truncated
    expect(branch0.childCount).toBe(10);
  });
});

// ── get_design_context: extractInstanceOverrides (dedupeComponents:true) ─────
//
// extractInstanceOverrides is invoked inside serializeWithDepth when dedupeComponents=true
// and a node is an INSTANCE. It compares each instance child against the corresponding
// component child, matching by id suffix (instChild.id.split(";").pop()).
//
// The suffix used for matching is the FULL component-child id (e.g. "13:126572", an A:B pair).
// Using only the trailing counter from split(":").pop() causes cross-page collisions when two
// siblings share the same counter but differ in prefix (e.g. "13:126572" vs "9399:126572").

describe("get_design_context — extractInstanceOverrides (dedupeComponents:true)", () => {
  // Helper to build a mock INSTANCE node whose children look like real Figma
  // instance-descendant ids (I<instanceId>;<componentChildId>).
  const makeInstanceNode = (
    instanceId: string,
    children: { componentChildId: string; type: string; characters?: string; visible?: boolean; fills?: any[] }[],
  ) => ({
    id: instanceId,
    name: "MyInstance",
    type: "INSTANCE" as const,
    visible: true,
    x: 0,
    y: 0,
    width: 200,
    height: 100,
    // componentProperties empty so no flattening noise
    componentProperties: {},
    children: children.map(({ componentChildId, type, characters, visible, fills }) => {
      const child: any = {
        id: `I${instanceId};${componentChildId}`,
        name: `Child-${componentChildId}`,
        type,
        visible: visible ?? true,
        x: 0,
        y: 0,
        width: 50,
        height: 20,
      };
      if (type === "TEXT") child.characters = characters ?? "instance text";
      if (fills !== undefined) child.fills = fills;
      return child;
    }),
    getMainComponentAsync: () => Promise.resolve(null), // set per-test via override
  });

  // Helper to build a mock COMPONENT node (the main component of an instance).
  const makeComponentNode = (
    componentId: string,
    children: { id: string; type: string; characters?: string; visible?: boolean; fills?: any[] }[],
  ) => ({
    id: componentId,
    name: "MyComponent",
    type: "COMPONENT" as const,
    visible: true,
    x: 0,
    y: 0,
    width: 200,
    height: 100,
    children: children.map(({ id, type, characters, visible, fills }) => {
      const child: any = {
        id,
        name: `CompChild-${id}`,
        type,
        visible: visible ?? true,
        x: 0,
        y: 0,
        width: 50,
        height: 20,
      };
      if (type === "TEXT") child.characters = characters ?? "component text";
      if (fills !== undefined) child.fills = fills;
      return child;
    }),
  });

  // Test 1: NORMAL MATCH — instance child "I216:103729;13:126572" pairs with
  // component child "13:126572". Override (different characters) is extracted.
  it("normal match: instance child suffix matches component child id → override extracted", async () => {
    const instanceId = "216:103729";
    const componentId = "comp:100";
    const componentChildId = "13:126572";

    const compNode = makeComponentNode(componentId, [
      { id: componentChildId, type: "TEXT", characters: "original text" },
    ]);
    const instNode = makeInstanceNode(instanceId, [
      { componentChildId, type: "TEXT", characters: "overridden text" },
    ]);
    // Wire getMainComponentAsync to return our component
    instNode.getMainComponentAsync = () => Promise.resolve(compNode as any);

    const root = {
      id: "page:dc1",
      name: "Page",
      type: "PAGE" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 1440,
      height: 900,
      children: [instNode],
    };
    setupFigma(root);
    // Patch figma.currentPage.selection so get_design_context uses just our instance
    (globalThis as any).figma.currentPage.selection = [instNode];

    const res = await handleReadDocumentRequest(
      makeRequest("get_design_context", { dedupeComponents: true }),
    );

    const contextNode = res?.data.context[0];
    expect(contextNode).toBeDefined();
    expect(contextNode.type).toBe("INSTANCE");

    // The override must exist: characters changed from "original text" → "overridden text"
    expect(contextNode.overrides).toBeDefined();
    expect(contextNode.overrides).toHaveLength(1);
    const override = contextNode.overrides[0];
    expect(override.id).toBe(`I${instanceId};${componentChildId}`);
    expect(override.characters).toBe("overridden text");
    expect(override.type).toBe("TEXT");
  });

  // Test 2: COLLISION GUARD — two component children share the same trailing counter
  // ("126572") but differ in prefix ("13:126572" vs "9399:126572").
  // The instance child "I216:103729;9399:126572" must pair with "9399:126572",
  // NOT "13:126572". With the buggy split(":").pop() both would match "126572"
  // and the FIRST found would win — the wrong child.
  it("COLLISION GUARD: siblings with same trailing counter but different prefix are disambiguated by full id", async () => {
    const instanceId = "216:103729";
    const componentId = "comp:200";
    const childIdA = "13:126572";     // prefix 13
    const childIdB = "9399:126572";   // prefix 9399 — SAME trailing counter as A

    const compNode = makeComponentNode(componentId, [
      { id: childIdA, type: "TEXT", characters: "text from A" },
      { id: childIdB, type: "TEXT", characters: "text from B" },
    ]);

    // Instance only overrides child B (the 9399: one)
    const instNode = makeInstanceNode(instanceId, [
      { componentChildId: childIdA, type: "TEXT", characters: "text from A" },   // same → no override
      { componentChildId: childIdB, type: "TEXT", characters: "OVERRIDDEN B" }, // changed → override
    ]);
    instNode.getMainComponentAsync = () => Promise.resolve(compNode as any);

    const root = {
      id: "page:dc2",
      name: "Page",
      type: "PAGE" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 1440,
      height: 900,
      children: [instNode],
    };
    setupFigma(root);
    (globalThis as any).figma.currentPage.selection = [instNode];

    const res = await handleReadDocumentRequest(
      makeRequest("get_design_context", { dedupeComponents: true }),
    );

    const contextNode = res?.data.context[0];
    expect(contextNode.overrides).toBeDefined();

    // Only child B should have an override
    expect(contextNode.overrides).toHaveLength(1);
    const override = contextNode.overrides[0];

    // Must pair with childIdB, NOT childIdA
    expect(override.id).toBe(`I${instanceId};${childIdB}`);
    expect(override.characters).toBe("OVERRIDDEN B");

    // Verify childIdA did NOT produce a false override
    const falseOverride = contextNode.overrides.find(
      (o: any) => o.id === `I${instanceId};${childIdA}`,
    );
    expect(falseOverride).toBeUndefined();
  });

  // Test 3: REORDERED CHILDREN — component children are in a different order than
  // instance children. Pairing must be by id, NOT by positional index.
  it("reordered children: pairs by id regardless of position, not by index", async () => {
    const instanceId = "216:103729";
    const componentId = "comp:300";
    const childIdX = "13:200";
    const childIdY = "13:201";

    // Component: X first, Y second
    const compNode = makeComponentNode(componentId, [
      { id: childIdX, type: "TEXT", characters: "original X" },
      { id: childIdY, type: "TEXT", characters: "original Y" },
    ]);

    // Instance: Y first, X second (reversed order)
    // Only Y is overridden
    const instNode = makeInstanceNode(instanceId, [
      { componentChildId: childIdY, type: "TEXT", characters: "overridden Y" }, // position 0
      { componentChildId: childIdX, type: "TEXT", characters: "original X" },   // position 1 — same
    ]);
    instNode.getMainComponentAsync = () => Promise.resolve(compNode as any);

    const root = {
      id: "page:dc3",
      name: "Page",
      type: "PAGE" as const,
      visible: true,
      x: 0,
      y: 0,
      width: 1440,
      height: 900,
      children: [instNode],
    };
    setupFigma(root);
    (globalThis as any).figma.currentPage.selection = [instNode];

    const res = await handleReadDocumentRequest(
      makeRequest("get_design_context", { dedupeComponents: true }),
    );

    const contextNode = res?.data.context[0];

    // Only Y should have an override (X's text is unchanged)
    expect(contextNode.overrides).toBeDefined();
    expect(contextNode.overrides).toHaveLength(1);
    const override = contextNode.overrides[0];
    expect(override.id).toBe(`I${instanceId};${childIdY}`);
    expect(override.characters).toBe("overridden Y");

    // X must NOT produce a false override due to wrong positional pairing
    // (positional: Y at i=0 would pair with X at compNode.children[0], i.e. "original X" vs
    // "overridden Y" → mismatch → wrong override; with id-based pairing Y pairs with Y → correct)
    const falseOverrideX = contextNode.overrides.find(
      (o: any) => o.id === `I${instanceId};${childIdX}`,
    );
    expect(falseOverrideX).toBeUndefined();
  });
});

// ── get_metadata: N/A verification ───────────────────────────────────────────

describe("get_metadata — no traversal (no progress expected)", () => {
  it("returns page list without emitting any progress_update", async () => {
    setupFigma();
    (globalThis as any).figma.root = {
      name: "My File",
      children: [
        { id: "p:1", name: "Page 1" },
        { id: "p:2", name: "Page 2" },
      ],
    };
    (globalThis as any).figma.currentPage = { id: "p:1", name: "Page 1" };

    const res = await handleReadDocumentRequest(makeRequest("get_metadata"));

    expect(res?.data.fileName).toBe("My File");
    expect(res?.data.pageCount).toBe(2);
    const progressMsgs = posted.filter((m) => m.type === "progress_update");
    expect(progressMsgs.length).toBe(0);
  });
});

// ── get_fonts: cooperative yielding (#3) ──────────────────────────────────────
// get_fonts walks the whole page collecting fonts. The walk must yield + emit a
// progress_update so a large page doesn't trip the Go-bridge watchdog, while
// returning the same aggregated font data.

describe("get_fonts — cooperative yielding", () => {
  it("emits ≥1 progress_update (progress > 0) on a large page (>800 nodes)", async () => {
    const root = makeDeepTree("root:fonts", 40, 25, "TEXT"); // 1041 nodes
    setupFigma(root);

    const res = await handleReadDocumentRequest(makeRequest("get_fonts"));

    // Data integrity: all leaves are Inter/Regular → one aggregated entry.
    expect(res?.data.count).toBe(1);
    expect(res?.data.fonts[0].family).toBe("Inter");
    expect(res?.data.fonts[0].style).toBe("Regular");
    expect(res?.data.fonts[0].nodeCount).toBe(1000);

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBeGreaterThanOrEqual(1);
    expect(progressMsgs[0].progress).toBeGreaterThan(0);
  });

  it("emits ZERO progress_update on a small page (<800 nodes)", async () => {
    const root = makeDeepTree("root:fonts-small", 4, 25, "TEXT"); // 105 nodes
    setupFigma(root);
    await handleReadDocumentRequest(makeRequest("get_fonts"));
    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBe(0);
  });
});

// ── get_document: cooperative yielding (#4) ───────────────────────────────────
// get_document serializes the entire current page via serializeNode. Threading a
// per-node tick (via onVisit) keeps the inactivity timer alive on a large page.

describe("get_document — cooperative yielding", () => {
  it("emits ≥1 progress_update (progress > 0) on a large page (>800 nodes)", async () => {
    const root = makeFlatTree("root:doc", 2000); // 2001 nodes
    setupFigma(root);

    const res = await handleReadDocumentRequest(makeRequest("get_document"));
    expect(res?.data).toBeTruthy();

    const progressMsgs = posted.filter(
      (m) => m.type === "progress_update" && m.requestId === "req-test-42",
    );
    expect(progressMsgs.length).toBeGreaterThanOrEqual(1);
    expect(progressMsgs[0].progress).toBeGreaterThan(0);
  });
});

// ── prewarm: parallel cache pre-population, no double-fetch ───────────────────
//
// prewarmReadCaches resolves each unique style id / instance main-component up
// front via Promise.all, then the walk hits the shared cache. The guarantee we
// assert: a style id used by many nodes (+ prewarm) is fetched EXACTLY ONCE, the
// instance main-component EXACTLY ONCE, and the serialized output is correct — so
// prewarm parallelizes without adding redundant lookups or changing output.

describe("prewarm — parallel lookups, no double-fetch", () => {
  const styledChild = (id: string) => ({
    id, name: id, type: "FRAME" as const, visible: true,
    x: 0, y: 0, width: 10, height: 10,
    fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 }, opacity: 1 }],
    fillStyleId: "S1",
  });

  it("get_node: resolves a shared style id once and the instance ref once", async () => {
    const styleCalls: string[] = [];
    const mainCompCalls: string[] = [];
    const inst = {
      id: "i:1", name: "Inst", type: "INSTANCE" as const, visible: true,
      x: 0, y: 0, width: 10, height: 10,
      fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 }, opacity: 1 }],
      fillStyleId: "S1",
      componentProperties: {},
      getMainComponentAsync: async () => {
        mainCompCalls.push("i:1");
        return { id: "mc:1", key: "k1", name: "Comp", remote: true };
      },
    };
    const root = {
      id: "root", name: "Root", type: "FRAME" as const, visible: true,
      x: 0, y: 0, width: 100, height: 100,
      fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 }, opacity: 1 }],
      fillStyleId: "S1",
      children: [styledChild("c1"), styledChild("c2"), inst],
    };
    setupFigma(root);
    (globalThis as any).figma.getStyleByIdAsync = async (id: string) => {
      styleCalls.push(id);
      return { name: `style:${id}` };
    };

    const res = await handleReadDocumentRequest(
      makeRequest("get_node", {}, ["root"]),
    );

    // Correctness: style names + instance ref serialized as expected.
    expect(res?.data.styles.fillStyle).toBe("style:S1");
    expect(res?.data.children).toHaveLength(3);
    expect(res?.data.children[2].mainComponent).toEqual({
      key: "k1", name: "Comp", remote: true,
    });
    // 4 nodes use S1 + prewarm — but exactly ONE fetch (shared cache).
    expect(styleCalls.filter((id) => id === "S1").length).toBe(1);
    // Instance main component fetched exactly once (prewarm, not re-fetched inline).
    expect(mainCompCalls.length).toBe(1);
  });
});

// ── get_design_context dedupeComponents — bounded serialization ──────────────
// The dedupeComponents path keeps a per-level re-walk (so nested INSTANCEs compact),
// and its per-node serializeNode is bounded to maxDepth:1 — only the node's own
// fields + direct-child id list are used; deeper serialization is discarded. Each
// node is therefore serialized as ~1 level → ~2N total body passes, NOT the
// Σ-subtree-size the old UNBOUNDED call cost. An `id` getter counts serializeNode
// bodies; this test would FAIL (count ≈ 547) on the pre-fix unbounded code.
describe("get_design_context dedupeComponents — bounded serialization", () => {
  it("serializes ~2N body passes on a deep frame tree, not O(Σ subtree size)", async () => {
    let idReads = 0;
    const reg = new Map<string, any>();
    const make = (id: string, depth: number, maxDepth: number, branching: number): any => {
      const children =
        depth < maxDepth
          ? Array.from({ length: branching }, (_, i) => make(`${id}-${i}`, depth + 1, maxDepth, branching))
          : [];
      const n: any = { name: id, type: "FRAME", visible: true, x: 0, y: 0, width: 10, height: 10 };
      Object.defineProperty(n, "id", { get() { idReads++; return id; }, enumerable: true });
      if (children.length) n.children = children;
      reg.set(id, n);
      return n;
    };
    const root = make("r", 0, 4, 3); // depth 4, branching 3 → 121 nodes
    const N = reg.size;
    setupFigma(root);
    (globalThis as any).figma.getNodeByIdAsync = async (id: string) => reg.get(id) ?? null;
    (globalThis as any).figma.currentPage.selection = [root];

    idReads = 0;
    const res = await handleReadDocumentRequest({
      type: "get_design_context",
      requestId: "req-dedupe",
      params: { dedupeComponents: true, depth: 50 },
    });

    // Each node serialized as ~1 level → N + (N-1) body passes. Old unbounded code
    // = Σ subtree size (≈ 547 for this tree), which exceeds 2N (242).
    expect(idReads).toBeLessThanOrEqual(2 * N);
    // Correctness: full depth preserved (no truncation at depth 50).
    expect(res?.data.context[0].children).toHaveLength(3);
    expect(res?.data.context[0].children[0].children).toHaveLength(3);
  });
});

// ── get_design_context: nodeId scoping (issue #34) ───────────────────────────
describe("get_design_context — nodeId scoping (issue #34)", () => {
  it("scopes to params.nodeId regardless of the current selection", async () => {
    const root = makeFlatTree("page:root", 3); // children child:0..child:2
    setupFigma(root);
    // Select a DIFFERENT child than the one we will request.
    (globalThis as any).figma.currentPage.selection = [root.children[0]];

    const res = await handleReadDocumentRequest(
      makeRequest("get_design_context", { nodeId: "child:2", depth: 1 }),
    );

    expect(res?.data.context).toHaveLength(1);
    expect(res?.data.context[0].id).toBe("child:2");
  });

  it("falls back to the selection when nodeId is omitted", async () => {
    const root = makeFlatTree("page:root", 2);
    setupFigma(root);
    (globalThis as any).figma.currentPage.selection = [root.children[1]];

    const res = await handleReadDocumentRequest(
      makeRequest("get_design_context", { depth: 1 }),
    );

    expect(res?.data.context[0].id).toBe("child:1");
  });

  it("throws when params.nodeId does not resolve", async () => {
    const root = makeFlatTree("page:root", 1);
    setupFigma(root);

    await expect(
      handleReadDocumentRequest(
        makeRequest("get_design_context", { nodeId: "missing:999" }),
      ),
    ).rejects.toThrow(/not found/i);
  });
});
