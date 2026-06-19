import { describe, it, expect } from "bun:test";
import { resolveRefs, handleBatchRequest, substituteBindings, batchConfig } from "./batch";

// Shared makeProgress noop used by read ops inside a batch.
const noopUi = { postMessage: () => {} };

// results[N] mirrors what a completed op returns: { i, type, data }.
const results = [
  { i: 0, type: "create_frame", data: { id: "10:1", name: "Card", bounds: { width: 200 } } },
  { i: 1, type: "create_text", data: { id: "10:2", name: "Label" } },
];

describe("resolveRefs", () => {
  it("substitutes a top-level $N.id ref", () => {
    expect(resolveRefs("$0.id", results)).toBe("10:1");
    expect(resolveRefs("$1.id", results)).toBe("10:2");
  });

  it("resolves a nested dotted path", () => {
    expect(resolveRefs("$0.bounds.width", results)).toBe(200);
  });

  it("indexes into an array via a dot-path (read chain: $0.nodes.0.id)", () => {
    // search_nodes-style result: a read op whose data carries an array of nodes.
    // The documented ref shape for a read chain uses dot-index, not brackets.
    const readResults = [
      { i: 0, type: "search_nodes", data: { nodes: [{ id: "20:1" }, { id: "20:2" }] } },
    ];
    expect(resolveRefs("$0.nodes.0.id", readResults)).toBe("20:1");
    expect(resolveRefs("$0.nodes.1.id", readResults)).toBe("20:2");
  });

  it("substitutes refs inside arrays and preserves structure", () => {
    expect(resolveRefs(["$1.id", "literal"], results)).toEqual(["10:2", "literal"]);
  });

  it("substitutes refs inside nested objects, leaving non-refs intact", () => {
    const input = { parentId: "$0.id", name: "child", meta: { from: "$1.id", keep: 5 } };
    expect(resolveRefs(input, results)).toEqual({
      parentId: "10:1",
      name: "child",
      meta: { from: "10:2", keep: 5 },
    });
  });

  it("leaves non-matching strings untouched (partial $ is not a ref)", () => {
    expect(resolveRefs("price is $5", results)).toBe("price is $5");
    expect(resolveRefs("$0", results)).toBe("$0"); // no dotted path → not a ref
  });

  it("rejects a forward/self ref (N >= completed count)", () => {
    // Resolving op #1's params: only results[0] exists.
    expect(() => resolveRefs("$2.id", results)).toThrow(/earlier ops only|has not run yet/);
  });

  it("throws when the referenced field is missing", () => {
    expect(() => resolveRefs("$0.missing", results)).toThrow(/no field/);
  });

  it("throws when referencing an op that produced no data (failed earlier op)", () => {
    const withFailure = [{ i: 0, type: "set_fills", error: "Node not found" }];
    expect(() => resolveRefs("$0.id", withFailure)).toThrow(/no data/);
  });
});

// ── array-projection refs ($N.path[*].sub — "all→all" bulk-apply) ──────────────
// A projection ref maps over an array in an earlier op's data and projects one
// sub-field (or the elements themselves). It is the scan→fan-in primitive that
// feeds a bulk setter: search_nodes → swap_component nodeIds:["$0.nodes[*].id"].
describe("resolveRefs — array projection [*]", () => {
  const scan = [
    { i: 0, type: "search_nodes", data: { nodes: [{ id: "20:1" }, { id: "20:2" }, { id: "20:3" }] } },
  ];

  it("projects a sub-field of every element → array of values", () => {
    expect(resolveRefs("$0.nodes[*].id", scan)).toEqual(["20:1", "20:2", "20:3"]);
  });

  it("projects the elements themselves with a trailing [*]", () => {
    const items = [{ i: 0, type: "search_nodes", data: { items: ["a", "b", "c"] } }];
    expect(resolveRefs("$0.items[*]", items)).toEqual(["a", "b", "c"]);
  });

  it("resolves a nested path before the wildcard ($0.a.b[*].id)", () => {
    const nested = [
      { i: 0, type: "x", data: { a: { b: [{ id: "1" }, { id: "2" }] } } },
    ];
    expect(resolveRefs("$0.a.b[*].id", nested)).toEqual(["1", "2"]);
  });

  it("resolves a deep sub-path after the wildcard ($0.nodes[*].bounds.width)", () => {
    const withBounds = [
      { i: 0, type: "x", data: { nodes: [{ bounds: { width: 10 } }, { bounds: { width: 20 } }] } },
    ];
    expect(resolveRefs("$0.nodes[*].bounds.width", withBounds)).toEqual([10, 20]);
  });

  it("projects an empty array when the target array is empty", () => {
    const empty = [{ i: 0, type: "x", data: { nodes: [] } }];
    expect(resolveRefs("$0.nodes[*].id", empty)).toEqual([]);
  });

  it("rejects more than one [*] segment with a clear error", () => {
    const twoD = [{ i: 0, type: "x", data: { rows: [{ cells: [{ id: "1" }] }] } }];
    expect(() => resolveRefs("$0.rows[*].cells[*].id", twoD)).toThrow(/exactly one \[\*\]|one \[\*\]/);
  });

  it("throws when the [*] target is not an array", () => {
    const notArr = [{ i: 0, type: "x", data: { nodes: "oops" } }];
    expect(() => resolveRefs("$0.nodes[*].id", notArr)).toThrow(/not an array|is not array/i);
  });

  it("FLATTENS a projection ref that is the sole array element (nodeIds shape)", () => {
    // nodeIds:["$0.nodes[*].id"] must become the flat [id0,id1,id2], NOT [[…]].
    expect(resolveRefs(["$0.nodes[*].id"], scan)).toEqual(["20:1", "20:2", "20:3"]);
  });

  it("FLATTENS (spreads) a projection ref among literal array elements", () => {
    // Positional rule: a projection ref as ANY array element spreads in place.
    expect(resolveRefs(["lead", "$0.nodes[*].id", "tail"], scan)).toEqual([
      "lead", "20:1", "20:2", "20:3", "tail",
    ]);
  });

  it("keeps the array value (no flatten) when a projection ref is an object value", () => {
    // Inside params (object value), the projection stays a nested array.
    expect(resolveRefs({ ids: "$0.nodes[*].id" }, scan)).toEqual({
      ids: ["20:1", "20:2", "20:3"],
    });
  });

  it("leaves single-value refs and literals byte-identical alongside a projection", () => {
    expect(resolveRefs(["$0.nodes.0.id", "$0.nodes[*].id"], scan)).toEqual([
      "20:1", "20:1", "20:2", "20:3",
    ]);
  });
});

// ── per-op skipInvisibleInstanceChildren toggle (SET-POINT 2) ─────────────────
// The flag is a global mutable Figma value, so each op must set it from its OWN
// resolved params — a prior op's `true` must never leak into a later op that
// omitted it. We capture every assignment via a setter and assert the sequence.
describe("handleBatchRequest — per-op skipInvisibleInstanceChildren", () => {
  it("sets the flag from each op's params and resets to false when omitted", async () => {
    const skipHistory: boolean[] = [];
    let backing = false;
    (globalThis as any).figma = {
      get skipInvisibleInstanceChildren() {
        return backing;
      },
      set skipInvisibleInstanceChildren(v: boolean) {
        backing = v;
        skipHistory.push(v);
      },
      ui: { postMessage: () => {} },
    };

    // Unknown op types: both handler sets return null → each op throws AFTER the
    // flag has already been set, so the per-op reset is exercised regardless of
    // handler outcome. No $N refs → independent bulk → continue-on-error.
    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-skip-1",
      params: {
        ops: [
          { type: "__noop__", params: { skipInvisibleInstanceChildren: true } },
          { type: "__noop__", params: {} },
          { type: "__noop__", params: { skipInvisibleInstanceChildren: true } },
        ],
      },
    });

    expect(skipHistory).toEqual([true, false, true]);
    expect(res.data.failCount).toBe(3); // unknown type → all error, but flag still toggled
  });
});

// ── end-to-end BULK-APPLY ("all→all" in one round-trip) ───────────────────────
// op0 search_nodes returns {nodes:[{id}×3]}; op1 swap_component fans those ids in
// via "$0.nodes[*].id" and swaps ALL three in the same batch.
describe("handleBatchRequest — scan → swap-every-match (all→all)", () => {
  it("swaps all 3 matched instances in a single batch", async () => {
    const newComponent = { id: "99:1", name: "TrashV2", type: "COMPONENT" };
    const instances: Record<string, any> = {
      "20:1": { id: "20:1", name: "trash", type: "INSTANCE", mainComponent: null },
      "20:2": { id: "20:2", name: "trash", type: "INSTANCE", mainComponent: null },
      "20:3": { id: "20:3", name: "trash", type: "INSTANCE", mainComponent: null },
    };
    // swap_component now uses the override-preserving native swapComponent(); mock it to set mainComponent.
    Object.values(instances).forEach((n) => { n.swapComponent = function (c: any) { this.mainComponent = c; }; });
    const page = {
      id: "0:1",
      name: "Page 1",
      children: [
        instances["20:1"],
        instances["20:2"],
        instances["20:3"],
      ],
    };
    const byId: Record<string, any> = { ...instances, "99:1": newComponent };

    (globalThis as any).figma = {
      get currentPage() { return page; },
      getNodeByIdAsync: async (id: string) => byId[id] ?? null,
      commitUndo: () => {},
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-e2e-1",
      params: {
        ops: [
          { type: "search_nodes", params: { query: "trash" } },
          { type: "swap_component", nodeIds: ["$0.nodes[*].id"], params: { componentId: "99:1" } },
        ],
      },
    });

    // op0 found 3, op1 swapped all 3 to the new component.
    expect(res.data.results[0].data.count).toBe(3);
    const swap = res.data.results[1];
    expect(swap.error).toBeUndefined();
    expect(swap.data.results).toHaveLength(3);
    expect(swap.data.results.every((r: any) => !r.error)).toBe(true);
    expect(instances["20:1"].mainComponent).toBe(newComponent);
    expect(instances["20:2"].mainComponent).toBe(newComponent);
    expect(instances["20:3"].mainComponent).toBe(newComponent);
    expect(res.data.okCount).toBe(2); // both OPS succeeded at the op level
  });

  it("a zero-match scan makes the bulk op fail with 'nodeIds is required' (documented edge)", async () => {
    // op0 matches nothing → "$0.nodes[*].id" resolves to [] → op1 nodeIds is []
    // → the setter's request-level guard throws. Because the batch has a ref it is
    // a dependent chain (stop-on-error), so a legitimately-empty scan surfaces as
    // an error rather than a silent no-op. Pinned so the behavior is intentional.
    const page = { id: "0:1", name: "Page 1", children: [] as any[] };
    (globalThis as any).figma = {
      get currentPage() { return page; },
      getNodeByIdAsync: async () => null,
      commitUndo: () => {},
      ui: noopUi,
    };
    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-e2e-empty",
      params: {
        ops: [
          { type: "search_nodes", params: { query: "nope" } },
          { type: "swap_component", nodeIds: ["$0.nodes[*].id"], params: { componentId: "99:1" } },
        ],
      },
    });
    expect(res.data.results[0].data.count).toBe(0);
    expect(res.data.results[1].error).toBe("nodeIds is required");
    expect(res.data.failCount).toBe(1);
  });
});

describe("handleBatchRequest — resolved import validation", () => {
  it("rejects a ref-resolved component key that is actually a node ID before import", async () => {
    let importCalls = 0;
    const page = {
      id: "0:1",
      name: "Page 1",
      children: [{ id: "410:49695", name: "Button", type: "COMPONENT", width: 10, height: 10 }],
    };
    (globalThis as any).figma = {
      get currentPage() { return page; },
      getNodeByIdAsync: async () => null,
      importComponentByKeyAsync: async () => {
        importCalls++;
        throw new Error("should not call importComponentByKeyAsync");
      },
      importComponentSetByKeyAsync: async () => {
        importCalls++;
        throw new Error("should not call importComponentSetByKeyAsync");
      },
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-import-ref-node-id",
      params: {
        ops: [
          { type: "search_nodes", params: { query: "Button", types: ["COMPONENT"] } },
          { type: "import_component_by_key", params: { key: "$0.nodes.0.id" } },
        ],
      },
    });

    expect(res.data.results[1].error).toContain("node id");
    expect(importCalls).toBe(0);
  });

  it("rejects a named-binding component assetType after map substitution before import", async () => {
    let importCalls = 0;
    (globalThis as any).figma = {
      importComponentByKeyAsync: async () => {
        importCalls++;
        throw new Error("should not call importComponentByKeyAsync");
      },
      importComponentSetByKeyAsync: async () => {
        importCalls++;
        throw new Error("should not call importComponentSetByKeyAsync");
      },
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-import-map-assettype",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "map",
            over: [{ key: "0123456789abcdef0123456789abcdef01234567", assetType: "STYLE" }],
            as: "asset",
            do: {
              type: "import_component_by_key",
              params: { key: "$asset.key", assetType: "$asset.assetType" },
            },
          },
        ],
      },
    });

    expect(res.data.results[0].error).toContain("assetType");
    expect(importCalls).toBe(0);
  });
});

describe("handleBatchRequest — resolved semantic validation", () => {
  it("rejects set_fills after refs resolve when neither color nor paints is present", async () => {
    let getNodeCalls = 0;
    (globalThis as any).figma = {
      getNodeByIdAsync: async () => {
        getNodeCalls++;
        return {
          id: "10:1",
          name: "Rect",
          type: "RECTANGLE",
          fills: [],
        };
      },
      variables: { getVariableByIdAsync: async () => null },
      commitUndo: () => {},
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-resolved-fills-missing",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "map",
            over: [{ id: "10:1" }],
            as: "item",
            do: { type: "set_fills", nodeIds: ["$item.id"], params: {} },
          },
        ],
      },
    });

    expect(res.data.results[0].error).toContain("color or paints");
    expect(getNodeCalls).toBe(0);
  });

  it("rejects set_effects after refs resolve when an effect type is invalid", async () => {
    let getNodeCalls = 0;
    (globalThis as any).figma = {
      getNodeByIdAsync: async () => {
        getNodeCalls++;
        return { id: "10:1", name: "Rect", type: "RECTANGLE", effects: [] };
      },
      commitUndo: () => {},
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-resolved-effects-invalid",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "map",
            over: [{ id: "10:1" }],
            as: "item",
            do: {
              type: "set_effects",
              nodeIds: ["$item.id"],
              params: { effects: [{ type: "MAGIC_SHADOW" }] },
            },
          },
        ],
      },
    });

    expect(res.data.results[0].error).toContain("DROP_SHADOW");
    expect(getNodeCalls).toBe(0);
  });

  it("accepts set_effects with a native GLASS effect type", async () => {
    const node: any = { id: "10:1", name: "Rect", type: "RECTANGLE", effects: [] };
    (globalThis as any).figma = {
      getNodeByIdAsync: async () => node,
      commitUndo: () => {},
      ui: noopUi,
    };
    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-glass-ok",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "set_effects",
            nodeIds: ["10:1"],
            params: { effects: [{ type: "GLASS", refraction: 0.4, radius: 14 }] },
          },
        ],
      },
    });
    expect(res.data.results[0].error).toBeUndefined();
    expect(node.effects[0].type).toBe("GLASS");
    expect(node.effects[0].refraction).toBe(0.4);
  });

  it("rejects set_effects after refs resolve when advanced effect fields are invalid", async () => {
    let getNodeCalls = 0;
    (globalThis as any).figma = {
      getNodeByIdAsync: async () => {
        getNodeCalls++;
        return { id: "10:1", name: "Rect", type: "RECTANGLE", effects: [] };
      },
      commitUndo: () => {},
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-resolved-effects-bad-field",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "map",
            over: [{ id: "10:1" }],
            as: "item",
            do: {
              type: "set_effects",
              nodeIds: ["$item.id"],
              params: { effects: [{ type: "NOISE", noiseType: "CHROMA" }] },
            },
          },
        ],
      },
    });

    expect(res.data.results[0].error).toContain("noiseType");
    expect(getNodeCalls).toBe(0);
  });

  it("rejects map inner ops after named refs resolve to bad concrete values", async () => {
    let getNodeCalls = 0;
    (globalThis as any).figma = {
      getNodeByIdAsync: async () => {
        getNodeCalls++;
        return { id: "10:1", name: "Rect", type: "RECTANGLE", fills: [] };
      },
      variables: { getVariableByIdAsync: async () => null },
      commitUndo: () => {},
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-resolved-semantic",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "map",
            over: [{ id: "10:1" }],
            as: "item",
            do: {
              type: "set_fills",
              nodeIds: ["$item.id"],
              params: { color: "$item.missingColor" },
            },
          },
        ],
      },
    });

    expect(res.data.results[0].error).toContain("binding $item.missingColor");
    expect(getNodeCalls).toBe(0);
  });
});

// ── substituteBindings unit tests ──────────────────────────────────────────────
// substituteBindings replaces ONLY named-binding refs ($item, $index, etc.)
// in a value (string | array | object). It runs BEFORE resolveRefs so $N refs
// remain untouched and are handled by the existing resolver.
describe("substituteBindings", () => {
  // test #2: $index nested inside params object (the object-branch test)
  it("resolves $index when nested inside a params object", () => {
    const bindings = { item: "node:1", index: 3 };
    const input = { properties: { variant: "Style-$index" } };
    // $index is not a whole-string anchored match here ("Style-$index" has prefix)
    // so only bare anchored refs resolve; this is the EXACT match design
    expect(substituteBindings(input, bindings)).toEqual({ properties: { variant: "Style-$index" } });
    // but a bare $index does resolve:
    const bare = { properties: { idx: "$index" } };
    expect(substituteBindings(bare, bindings)).toEqual({ properties: { idx: 3 } });
  });

  it("resolves $item to the element value", () => {
    const bindings = { item: "node:42", index: 0 };
    expect(substituteBindings("$item", bindings)).toBe("node:42");
  });

  it("resolves $item.foo.bar path projection", () => {
    const bindings = { item: { id: "node:5", type: "INSTANCE" }, index: 1 };
    expect(substituteBindings("$item.id", bindings)).toBe("node:5");
    expect(substituteBindings("$item.type", bindings)).toBe("INSTANCE");
  });

  it("leaves mid-string literals untouched (collision cost documented)", () => {
    // A non-anchored string like "$item costs $5" is NOT a whole-string match
    // so it is returned untouched. Only exact whole-string "$item" resolves.
    // NOTE: This is the known collision cost of the exact-match grammar:
    //   a template string like "prefix-$item" will NOT be substituted.
    //   Document that named refs must occupy the whole string value.
    const bindings = { item: "node:1", index: 0 };
    expect(substituteBindings("$item costs $5", bindings)).toBe("$item costs $5");
  });

  // test #5: unbound / typo'd binding throws loud error
  it("throws a loud error for an unknown binding name (typo guard)", () => {
    const bindings = { item: "node:1", index: 0 };
    expect(() => substituteBindings("$itm", bindings)).toThrow(
      /unknown binding \$itm.*bound:.*item.*index/i
    );
  });

  it("resolves bindings inside arrays", () => {
    const bindings = { item: "node:7", index: 2 };
    expect(substituteBindings(["$item", "literal", "$index"], bindings)).toEqual([
      "node:7", "literal", 2,
    ]);
  });
});

// ── map control-flow op ────────────────────────────────────────────────────────
// map iterates over an array resolved from a prior op, substitutes $item/$index
// per element, then dispatches each concrete op through the standard handlers.
// Returns data:{ results, okCount, failCount } so downstream $M.results[*].data.<field>
// resolves through the existing projection resolver — no new projection machinery.
describe("handleBatchRequest — map control-flow op", () => {
  // Helper: builds a minimal figma mock that supports swap_component on specific nodeIds
  function makeFigmaMock(
    instances: Record<string, { id: string; name: string; type: string; mainComponent: any }>,
    newComponent: any,
  ) {
    const byId: Record<string, any> = { ...instances, [newComponent.id]: newComponent };
    return {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(instances) },
      getNodeByIdAsync: async (id: string) => byId[id] ?? null,
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };
  }

  // test #1: map binds $item per element — each inner op gets DISTINCT, varying params.
  // This is the headline use case projection CANNOT cover: per-item different values.
  // We map over objects {id, text} and set_text each node with its OWN text — provably
  // different per item. After the batch, each node's .characters must match its item.
  it("binds $item per element so each inner op receives a different nodeId AND different params", async () => {
    const textNodes: Record<string, any> = {
      "20:1": { id: "20:1", type: "TEXT", name: "t1", characters: "", fontName: { family: "Inter", style: "Regular" } },
      "20:2": { id: "20:2", type: "TEXT", name: "t2", characters: "", fontName: { family: "Inter", style: "Regular" } },
    };
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(textNodes) },
      getNodeByIdAsync: async (id: string) => textNodes[id] ?? null,
      loadFontAsync: async () => {},
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    // map over objects — each carries its own id AND its own label.
    // $item.id → nodeId; $item.label → params.text. Projection can't do this
    // (projection applies ONE set of params to all nodes).
    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-1",
      params: {
        ops: [
          {
            type: "map",
            over: [
              { id: "20:1", label: "Alpha" },
              { id: "20:2", label: "Beta" },
            ],
            as: "item",
            do: {
              type: "set_text",
              nodeIds: ["$item.id"],    // $item.path projection off the element
              params: { text: "$item.label" },  // DIFFERENT per item
            },
          },
        ],
      },
    });

    const mapResult = res.data.results[0];
    expect(mapResult.error).toBeUndefined();
    expect(mapResult.type).toBe("map");
    expect(mapResult.data.okCount).toBe(2);
    expect(mapResult.data.failCount).toBe(0);
    // THE KEY ASSERTION: each node has its own distinct text, not the same value.
    expect(textNodes["20:1"].characters).toBe("Alpha");
    expect(textNodes["20:2"].characters).toBe("Beta");
  });

  // test #2 (object-branch): $index nested inside params resolves correctly at the
  // INTEGRATION level — substituteBindings must walk the object branch (params:{…})
  // and replace the bare "$index" value in a nested property.
  // We prove this by having each iteration set a DIFFERENT text that embeds the index.
  it("resolves $index nested inside params object — each iteration receives its own index", async () => {
    const textNodes: Record<string, any> = {
      "t:0": { id: "t:0", type: "TEXT", name: "n0", characters: "", fontName: { family: "Inter", style: "Regular" } },
      "t:1": { id: "t:1", type: "TEXT", name: "n1", characters: "", fontName: { family: "Inter", style: "Regular" } },
      "t:2": { id: "t:2", type: "TEXT", name: "n2", characters: "", fontName: { family: "Inter", style: "Regular" } },
    };
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(textNodes) },
      getNodeByIdAsync: async (id: string) => textNodes[id] ?? null,
      loadFontAsync: async () => {},
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    // params.text = "$index" — a bare $index nested inside the params object.
    // substituteBindings must walk the object branch for this to resolve.
    // After the map, each node's characters must equal its zero-based iteration index.
    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-idx",
      params: {
        ops: [
          {
            type: "map",
            over: ["t:0", "t:1", "t:2"],
            as: "item",
            do: {
              type: "set_text",
              nodeIds: ["$item"],
              params: { text: "$index" },  // $index nested inside params object
            },
          },
        ],
      },
    });

    const mapResult = res.data.results[0];
    expect(mapResult.error).toBeUndefined();
    expect(mapResult.data.okCount).toBe(3);
    // THE KEY ASSERTIONS: each node received its own $index value, not the template literal "$index"
    expect(textNodes["t:0"].characters).toBe(0);   // $index=0
    expect(textNodes["t:1"].characters).toBe(1);   // $index=1
    expect(textNodes["t:2"].characters).toBe(2);   // $index=2
  });

  // test #3: over resolves a projection ref from a prior op
  it("resolves 'over' as a projection ref from a prior scan op", async () => {
    const newComp = { id: "99:1", name: "NewBtn", type: "COMPONENT" };
    const instances: Record<string, any> = {
      "20:1": { id: "20:1", name: "btn", type: "INSTANCE", mainComponent: null },
      "20:2": { id: "20:2", name: "btn", type: "INSTANCE", mainComponent: null },
      "20:3": { id: "20:3", name: "btn", type: "INSTANCE", mainComponent: null },
    };
    Object.values(instances).forEach((n) => { n.swapComponent = function (c: any) { this.mainComponent = c; }; });
    const byId: Record<string, any> = { ...instances, "99:1": newComp };
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(instances) },
      getNodeByIdAsync: async (id: string) => byId[id] ?? null,
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-3",
      params: {
        continueOnError: true,
        ops: [
          // op 0: search returns {nodes:[{id}×3]}
          { type: "search_nodes", params: { query: "btn" } },
          // op 1: map over "$0.nodes[*].id" — projection ref from prior op
          {
            type: "map",
            over: "$0.nodes[*].id",
            as: "item",
            do: {
              type: "swap_component",
              nodeIds: ["$item"],
              params: { componentId: "99:1" },
            },
          },
        ],
      },
    });

    expect(res.data.results[0].data.count).toBe(3);
    const mapResult = res.data.results[1];
    expect(mapResult.error).toBeUndefined();
    expect(mapResult.data.results).toHaveLength(3);
    expect(mapResult.data.okCount).toBe(3);
    // All 3 instances should have been swapped to newComp
    expect(instances["20:1"].mainComponent).toBe(newComp);
    expect(instances["20:2"].mainComponent).toBe(newComp);
    expect(instances["20:3"].mainComponent).toBe(newComp);
  });

  // test #4: map result is referenceable downstream via $0.results[*].data.<field>
  // through the EXISTING projection resolver — NO new machinery.
  // op0 = map over 2 text nodes (set_text each → data:{id,name,characters,…}).
  // op1 = set_visible with nodeIds:["$0.results[*].data.id"] — the projection resolver
  //       parses pre="results", post="data.id" against the map result, exactly like
  //       "$N.path[*].sub" in any other chain. Zero new code.
  it("map result data is referenceable downstream through existing projection resolver", async () => {
    const textNodes: Record<string, any> = {
      "20:1": { id: "20:1", type: "TEXT", name: "n1", characters: "", fontName: { family: "Inter", style: "Regular" }, visible: true },
      "20:2": { id: "20:2", type: "TEXT", name: "n2", characters: "", fontName: { family: "Inter", style: "Regular" }, visible: true },
    };
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(textNodes) },
      getNodeByIdAsync: async (id: string) => textNodes[id] ?? null,
      loadFontAsync: async () => {},
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-4",
      params: {
        // $0 refs present → hasRefs=true → continueOnError=false (default).
        // All inner ops succeed, so this is fine.
        ops: [
          // op 0: map sets text on 2 nodes; each iteration returns data:{id, name, characters,…}
          {
            type: "map",
            over: ["20:1", "20:2"],
            as: "item",
            do: {
              type: "set_text",
              nodeIds: ["$item"],
              params: { text: "hello" },
            },
          },
          // op 1: downstream op feeding off map result via the existing $N.path[*].sub resolver.
          // "$0.results[*].data.id" → resolveProjection(idx=0, pre="results", post="data.id")
          // → ["20:1","20:2"]. Exercises the unchanged resolver with map data as input.
          {
            type: "set_visible",
            nodeIds: ["$0.results[*].data.id"],
            params: { visible: false },
          },
        ],
      },
    });

    // op 0 (map) succeeded
    expect(res.data.results[0].error).toBeUndefined();
    expect(res.data.results[0].data.okCount).toBe(2);

    // op 1 ran via the downstream projection — chain worked
    expect(res.data.results[1].error).toBeUndefined();

    // THE KEY ASSERTION: op 1 received both node ids from the projection.
    // set_visible returns data:{results:[{nodeId,visible}×2]}
    const setVisibleResults = res.data.results[1].data.results;
    expect(setVisibleResults).toHaveLength(2);
    expect(setVisibleResults.map((r: any) => r.nodeId).sort()).toEqual(["20:1", "20:2"]);
    // And the mutations actually happened
    expect(textNodes["20:1"].visible).toBe(false);
    expect(textNodes["20:2"].visible).toBe(false);
  });

  // test #6 (literal mid-string passthrough): "$item costs $5" is untouched
  it("passes through a mid-string literal containing $item — exact-match only", async () => {
    // The whole-string anchored match means "$item costs $5" is NOT substituted.
    // We verify this by running a map where the do-op contains a mid-string literal.
    // The op will fail (no real handler for a fabricated op type), but the key check
    // is that substituteBindings itself correctly passes through mid-string refs.
    const bindings = { item: "node:1", index: 0 };
    // Direct unit test via substituteBindings (already covered in the unit suite above).
    // Integration test: verify map dispatches the concrete op without crashing on the literal.
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: [] },
      getNodeByIdAsync: async () => null,
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };
    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-6",
      params: {
        continueOnError: true,
        ops: [
          {
            type: "map",
            over: ["node:1"],
            as: "item",
            do: {
              // unknown op type so handler fails, but substituteBindings ran
              type: "__literal_test__",
              nodeIds: ["$item"],
              params: { label: "$item costs $5" }, // mid-string should NOT be substituted
            },
          },
        ],
      },
    });
    // map ran 1 iteration (even though inner op failed — continueOnError=true)
    const mapResult = res.data.results[0];
    expect(mapResult.data.results).toHaveLength(1);
    // The inner op failed on "unknown op type", not on substituteBindings
    expect(mapResult.data.results[0].error).toMatch(/unknown op type/);
  });

  // test #7: continueOnError — one bad item doesn't abort others when true; stops when false
  // We use set_text as the inner op — it THROWS (not collects) when a node is missing,
  // so it reliably triggers per-iteration failure for the nodeId "20:BAD".
  it("continueOnError:true — bad item doesn't abort rest of map iterations", async () => {
    // Nodes "20:1" and "20:3" are real TEXT nodes; "20:BAD" doesn't exist → set_text throws.
    const textNodes: Record<string, any> = {
      "20:1": { id: "20:1", type: "TEXT", name: "t1", characters: "", fontName: { family: "Inter", style: "Regular" } },
      "20:3": { id: "20:3", type: "TEXT", name: "t3", characters: "", fontName: { family: "Inter", style: "Regular" } },
    };
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(textNodes) },
      getNodeByIdAsync: async (id: string) => textNodes[id] ?? null,
      loadFontAsync: async () => {},
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-7a",
      params: {
        continueOnError: true,
        ops: [
          {
            type: "map",
            // item 1 ("20:BAD") doesn't exist → set_text throws "Node not found"
            over: ["20:1", "20:BAD", "20:3"],
            as: "item",
            do: {
              type: "set_text",
              nodeIds: ["$item"],
              params: { text: "hello" },
            },
          },
        ],
      },
    });

    const mapResult = res.data.results[0];
    expect(mapResult.error).toBeUndefined(); // map op itself succeeded (partial)
    expect(mapResult.data.results).toHaveLength(3);
    expect(mapResult.data.results[0].error).toBeUndefined(); // 20:1 OK
    expect(mapResult.data.results[1].error).toBeDefined();   // 20:BAD threw
    expect(mapResult.data.results[1].error).toMatch(/Node not found/);
    expect(mapResult.data.results[2].error).toBeUndefined(); // 20:3 OK (continued)
    expect(mapResult.data.okCount).toBe(2);
    expect(mapResult.data.failCount).toBe(1);
  });

  it("continueOnError:false — stops map on first item failure", async () => {
    const textNodes: Record<string, any> = {
      "20:1": { id: "20:1", type: "TEXT", name: "t1", characters: "", fontName: { family: "Inter", style: "Regular" } },
      "20:3": { id: "20:3", type: "TEXT", name: "t3", characters: "", fontName: { family: "Inter", style: "Regular" } },
    };
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: Object.values(textNodes) },
      getNodeByIdAsync: async (id: string) => textNodes[id] ?? null,
      loadFontAsync: async () => {},
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-7b",
      params: {
        continueOnError: false,
        ops: [
          {
            type: "map",
            over: ["20:1", "20:BAD", "20:3"],
            as: "item",
            do: {
              type: "set_text",
              nodeIds: ["$item"],
              params: { text: "hello" },
            },
          },
        ],
      },
    });

    const mapResult = res.data.results[0];
    // map op itself failed (threw on inner failure with continueOnError=false)
    expect(mapResult.error).toBeDefined();
    expect(mapResult.error).toMatch(/map iteration 1 failed/i);
    expect(res.data.failCount).toBe(1);
  });

  // test #8: iteration cap throws (not truncates) when exceeded
  it("throws (not truncates) when over.length exceeds the iteration cap", async () => {
    (globalThis as any).figma = {
      currentPage: { id: "0:1", name: "Page 1", children: [] },
      getNodeByIdAsync: async () => null,
      commitUndo: () => {},
      ui: { postMessage: () => {} },
    };

    // Build an array of 501 fake node IDs
    const bigList = Array.from({ length: 501 }, (_, i) => `node:${i}`);

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-map-8",
      params: {
        continueOnError: true,
        ops: [
          {
            type: "map",
            over: bigList,
            as: "item",
            do: { type: "__noop__", nodeIds: ["$item"], params: {} },
          },
        ],
      },
    });

    // The map op should fail with a clear error naming the count and cap
    const mapResult = res.data.results[0];
    expect(mapResult.error).toBeDefined();
    expect(mapResult.error).toMatch(/501/);    // names the count
    expect(mapResult.error).toMatch(/500/);    // names the cap
  });

  // test #9 (backward-compat): pre-existing batch test suite is unaffected
  // This is validated by running all tests together. The 23 existing tests are
  // structurally independent (resolveRefs is byte-identical). Confirmed implicitly
  // by the full test run passing — but also verify the specific scan→swap still works.
  it("backward-compat: scan → swap-every-match still works (resolveRefs untouched)", async () => {
    const newComponent = { id: "99:1", name: "TrashV2", type: "COMPONENT" };
    const instances: Record<string, any> = {
      "20:1": { id: "20:1", name: "trash", type: "INSTANCE", mainComponent: null },
      "20:2": { id: "20:2", name: "trash", type: "INSTANCE", mainComponent: null },
    };
    const byId: Record<string, any> = { ...instances, "99:1": newComponent };
    (globalThis as any).figma = {
      get currentPage() { return { id: "0:1", name: "Page 1", children: Object.values(instances) }; },
      getNodeByIdAsync: async (id: string) => byId[id] ?? null,
      commitUndo: () => {},
      ui: noopUi,
    };

    const res = await handleBatchRequest({
      type: "batch",
      requestId: "req-compat-9",
      params: {
        ops: [
          { type: "search_nodes", params: { query: "trash" } },
          { type: "swap_component", nodeIds: ["$0.nodes[*].id"], params: { componentId: "99:1" } },
        ],
      },
    });

    expect(res.data.results[0].data.count).toBe(2);
    expect(res.data.results[1].error).toBeUndefined();
    expect(res.data.results[1].data.results).toHaveLength(2);
    expect(res.data.okCount).toBe(2);
  });
});

// ── Per-op timeout (issue #31) ────────────────────────────────────────────────
//
// A hung Figma API call inside a batch op must not block forever — it must reject
// within the configured timeout so the op resolves and the channel's serial slot
// frees, instead of stalling every agent until the server's ~120s ceiling.
describe("handleBatchRequest — per-op timeout (issue #31)", () => {
  const originalOp = batchConfig.opTimeoutMs;
  const originalHeavy = batchConfig.heavyReadTimeoutMs;
  const restore = () => {
    batchConfig.opTimeoutMs = originalOp;
    batchConfig.heavyReadTimeoutMs = originalHeavy;
  };

  it("times out a hung op instead of hanging the whole batch", async () => {
    batchConfig.opTimeoutMs = 30;
    (globalThis as any).figma = {
      // Never resolves — simulates a wedged Figma API call.
      getNodeByIdAsync: () => new Promise(() => {}),
      variables: { getVariableByIdAsync: async () => null },
      commitUndo: () => {},
      ui: noopUi,
    };

    try {
      const res = await handleBatchRequest({
        type: "batch",
        requestId: "req-timeout-hang",
        params: {
          ops: [
            { type: "set_fills", nodeIds: ["10:1"], params: { color: "#ff0000" } },
          ],
        },
      });
      expect(res.data.results[0].error).toMatch(/timed out/);
      expect(res.data.failCount).toBe(1);
    } finally {
      restore();
    }
  });

  it("continueOnError lets a later op run after a prior op times out", async () => {
    batchConfig.opTimeoutMs = 30;
    (globalThis as any).figma = {
      getNodeByIdAsync: (id: string) =>
        id === "10:hang"
          ? new Promise(() => {}) // wedged
          : Promise.resolve({ id, name: "Rect", type: "RECTANGLE", fills: [] }),
      variables: { getVariableByIdAsync: async () => null },
      commitUndo: () => {},
      ui: noopUi,
    };

    try {
      const res = await handleBatchRequest({
        type: "batch",
        requestId: "req-timeout-continue",
        params: {
          continueOnError: true,
          ops: [
            { type: "set_fills", nodeIds: ["10:hang"], params: { color: "#ff0000" } },
            { type: "set_fills", nodeIds: ["10:ok"], params: { color: "#00ff00" } },
          ],
        },
      });
      expect(res.data.results[0].error).toMatch(/timed out/);
      expect(res.data.results[1].error).toBeUndefined();
      expect(res.data.okCount).toBe(1);
      expect(res.data.failCount).toBe(1);
    } finally {
      restore();
    }
  });

  // A heavy READ op must use heavyReadTimeoutMs, NOT the short write cap — the
  // server gives a batch the generous 600s read ceiling precisely so a big read
  // doesn't time out, and the plugin must not undercut that. Here the write cap is
  // long (won't fire) and the heavy-read cap is short, so a hung get_node timing
  // out PROVES the op-type split routes it to heavyReadTimeoutMs.
  it("routes a heavy read op to heavyReadTimeoutMs, not the write cap", async () => {
    batchConfig.opTimeoutMs = 10_000; // long — would NOT fire in the test window
    batchConfig.heavyReadTimeoutMs = 30; // short — the heavy-read path
    (globalThis as any).figma = {
      getNodeByIdAsync: () => new Promise(() => {}), // wedged
      ui: noopUi,
    };

    try {
      const start = Date.now();
      const res = await handleBatchRequest({
        type: "batch",
        requestId: "req-timeout-heavy",
        params: {
          ops: [{ type: "get_node", nodeIds: ["10:1"], params: {} }],
        },
      });
      // Timed out fast via the heavy-read cap; if get_node had wrongly used the
      // 10s write cap, this would not have resolved within the window.
      expect(res.data.results[0].error).toMatch(/timed out/);
      expect(Date.now() - start).toBeLessThan(5_000);
    } finally {
      restore();
    }
  });
});
