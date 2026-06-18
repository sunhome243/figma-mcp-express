import { describe, it, expect, beforeEach } from "bun:test";
import { handleReadDocumentRequest } from "./read-document";

// ── Figma global mock ─────────────────────────────────────────────────────────
//
// codegen detail enriches the FULL serialization with:
//   - autoLayout  (only on auto-layout frames)
//   - tokens      (resolved bound-variable names, incl. paint-level color bindings)
//   - componentRef (only on INSTANCE nodes → {key, name})
//   - codeConnect  (when componentRef.key is in the passed codeConnectMap)

let mockNodes: Record<string, any>;
let mockVariables: Record<string, { name: string }>;

const registerTree = (node: any) => {
  mockNodes[node.id] = node;
  if (Array.isArray(node.children)) node.children.forEach(registerTree);
};

const makeRequest = (params?: any) => ({
  type: "get_design_context",
  requestId: "req-codegen-1",
  params: params ?? {},
});

beforeEach(() => {
  mockNodes = {};
  mockVariables = {
    "VariableID:1": { name: "spacing/16" },
    "VariableID:2": { name: "spacing/8" },
    "VariableID:color1": { name: "colors/primary" },
    "VariableID:stroke1": { name: "colors/border" },
  };

  (globalThis as any).figma = {
    currentPage: {
      id: "0:1",
      name: "Page 1",
      type: "PAGE",
      // selection assigned per-test
      selection: [] as any[],
    },
    root: { name: "Test File", children: [{ id: "0:1", name: "Page 1" }] },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    getStyleByIdAsync: async () => null,
    variables: {
      getVariableByIdAsync: async (id: string) => mockVariables[id] ?? null,
    },
  };
});

// ── autoLayout extraction ─────────────────────────────────────────────────────

describe("codegen — autoLayout", () => {
  it("extracts auto-layout fields from a HORIZONTAL frame", async () => {
    const frame = {
      id: "1:1",
      name: "Row",
      type: "FRAME",
      x: 0, y: 0, width: 200, height: 40,
      layoutMode: "HORIZONTAL",
      primaryAxisAlignItems: "SPACE_BETWEEN",
      counterAxisAlignItems: "CENTER",
      itemSpacing: 8,
      layoutWrap: "NO_WRAP",
      layoutSizingHorizontal: "FILL",
      layoutSizingVertical: "HUG",
      paddingTop: 4, paddingRight: 16, paddingBottom: 4, paddingLeft: 16,
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen", depth: 2 }));
    const node = res!.data.context[0];

    expect(node.autoLayout).toBeDefined();
    expect(node.autoLayout.layoutMode).toBe("HORIZONTAL");
    expect(node.autoLayout.primaryAxisAlignItems).toBe("SPACE_BETWEEN");
    expect(node.autoLayout.counterAxisAlignItems).toBe("CENTER");
    expect(node.autoLayout.itemSpacing).toBe(8);
    expect(node.autoLayout.layoutWrap).toBe("NO_WRAP");
    expect(node.autoLayout.layoutSizingHorizontal).toBe("FILL");
    expect(node.autoLayout.layoutSizingVertical).toBe("HUG");
    expect(node.autoLayout.padding).toEqual({ top: 4, right: 16, bottom: 4, left: 16 });
  });

  it("omits autoLayout when layoutMode is NONE", async () => {
    const frame = {
      id: "1:2", name: "Plain", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      layoutMode: "NONE",
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    expect(res!.data.context[0].autoLayout).toBeUndefined();
  });

  it("omits undefined autoLayout sub-fields", async () => {
    const frame = {
      id: "1:3", name: "Partial", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      layoutMode: "VERTICAL",
      itemSpacing: 12,
      // no align/wrap/sizing/padding fields
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const al = res!.data.context[0].autoLayout;
    expect(al.layoutMode).toBe("VERTICAL");
    expect(al.itemSpacing).toBe(12);
    expect("primaryAxisAlignItems" in al).toBe(false);
    expect("padding" in al).toBe(false);
  });
});

// ── tokens resolution ─────────────────────────────────────────────────────────

describe("codegen — tokens", () => {
  it("resolves scalar boundVariables to token names", async () => {
    const frame = {
      id: "2:1", name: "Padded", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      layoutMode: "NONE",
      boundVariables: {
        paddingLeft: { type: "VARIABLE_ALIAS", id: "VariableID:1" },
        itemSpacing: { type: "VARIABLE_ALIAS", id: "VariableID:2" },
      },
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const tokens = res!.data.context[0].tokens;
    expect(tokens.paddingLeft).toBe("spacing/16");
    expect(tokens.itemSpacing).toBe("spacing/8");
  });

  it("resolves paint-level fill color binding to fills.0.color", async () => {
    const frame = {
      id: "2:2", name: "Filled", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      layoutMode: "NONE",
      fills: [
        {
          type: "SOLID",
          color: { r: 0.1, g: 0.2, b: 0.3 },
          boundVariables: { color: { id: "VariableID:color1" } },
        },
      ],
      strokes: [
        {
          type: "SOLID",
          color: { r: 0, g: 0, b: 0 },
          boundVariables: { color: { id: "VariableID:stroke1" } },
        },
      ],
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const tokens = res!.data.context[0].tokens;
    expect(tokens["fills.0.color"]).toBe("colors/primary");
    expect(tokens["strokes.0.color"]).toBe("colors/border");
  });

  it("skips bound ids that do not resolve and omits tokens when empty", async () => {
    const frame = {
      id: "2:3", name: "Unresolved", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      layoutMode: "NONE",
      boundVariables: {
        paddingLeft: { type: "VARIABLE_ALIAS", id: "VariableID:missing" },
      },
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    expect(res!.data.context[0].tokens).toBeUndefined();
  });

  it("does not crash on array-valued boundVariables fields", async () => {
    const frame = {
      id: "2:4", name: "ArrayBound", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      layoutMode: "NONE",
      boundVariables: {
        // fills as an array of aliases — must be skipped, paint-level walk handles colors
        fills: [{ type: "VARIABLE_ALIAS", id: "VariableID:color1" }],
        paddingTop: { type: "VARIABLE_ALIAS", id: "VariableID:1" },
      },
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const tokens = res!.data.context[0].tokens;
    expect(tokens.paddingTop).toBe("spacing/16");
    expect("fills" in tokens).toBe(false);
  });
});

// ── componentRef + codeConnect ────────────────────────────────────────────────

describe("codegen — componentRef and codeConnect", () => {
  const makeInstance = () => {
    const mc = { id: "C:1", key: "abc123", name: "Button" };
    const inst = {
      id: "3:1", name: "Button Instance", type: "INSTANCE",
      x: 0, y: 0, width: 80, height: 32,
      getMainComponentAsync: async () => mc,
      children: [],
    };
    return { mc, inst };
  };

  it("attaches componentRef {key, name, remote} for an INSTANCE", async () => {
    const { inst } = makeInstance();
    registerTree(inst);
    figma.currentPage.selection = [inst];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const node = res!.data.context[0];
    // remote:false because makeInstance's mc has no .remote property → defaults to false
    expect(node.componentRef).toEqual({ key: "abc123", name: "Button", remote: false });
  });

  it("attaches codeConnect when key is in codeConnectMap", async () => {
    const { inst } = makeInstance();
    registerTree(inst);
    figma.currentPage.selection = [inst];

    const codeConnectMap = {
      abc123: { component: "Button", import: "@/ui/button" },
    };
    const res = await handleReadDocumentRequest(
      makeRequest({ detail: "codegen", codeConnectMap }),
    );
    const node = res!.data.context[0];
    expect(node.codeConnect).toEqual({ component: "Button", import: "@/ui/button" });
  });

  it("does not attach codeConnect when key is absent from the map", async () => {
    const { inst } = makeInstance();
    registerTree(inst);
    figma.currentPage.selection = [inst];

    const res = await handleReadDocumentRequest(
      makeRequest({ detail: "codegen", codeConnectMap: { other: {} } }),
    );
    expect(res!.data.context[0].codeConnect).toBeUndefined();
  });

  it("does not attach componentRef to non-INSTANCE nodes", async () => {
    const frame = {
      id: "3:2", name: "Frame", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100, layoutMode: "NONE", children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    expect(res!.data.context[0].componentRef).toBeUndefined();
  });
});

// ── depth + text + full regression ────────────────────────────────────────────

describe("codegen — depth, text, and full regression", () => {
  it("enriches a child node within depth and keeps text characters", async () => {
    const text = {
      id: "4:2", name: "Label", type: "TEXT",
      x: 0, y: 0, width: 50, height: 20,
      characters: "Hello",
      fontName: { family: "Geist", style: "Regular" },
      fontSize: 14,
      boundVariables: { fontSize: { type: "VARIABLE_ALIAS", id: "VariableID:2" } },
    };
    const root = {
      id: "4:1", name: "Container", type: "FRAME",
      x: 0, y: 0, width: 200, height: 100,
      layoutMode: "VERTICAL", itemSpacing: 8,
      children: [text],
    };
    registerTree(root);
    figma.currentPage.selection = [root];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen", depth: 2 }));
    const node = res!.data.context[0];
    expect(node.autoLayout.layoutMode).toBe("VERTICAL");
    const child = node.children[0];
    expect(child.characters).toBe("Hello");
    expect(child.tokens.fontSize).toBe("spacing/8");
  });

  // KEY-ORDER GOLDEN (byte-identical contract): enrichForCodegen appends
  // autoLayout → tokens → componentRef → codeConnect AFTER the base keys, and the
  // base object ends with `children`. A single-walk refactor MUST preserve this
  // exact order on BOTH the parent and every enriched child, or the serialized
  // JSON bytes change. Locked here before the depth-aware refactor.
  it("emits codegen keys in a stable order, after children (parent + child)", async () => {
    const childInstance = {
      id: "9:2", name: "Btn", type: "INSTANCE",
      x: 0, y: 0, width: 80, height: 32,
      layoutMode: "HORIZONTAL", itemSpacing: 8,
      boundVariables: { itemSpacing: { type: "VARIABLE_ALIAS", id: "VariableID:2" } },
      componentProperties: {},
      getMainComponentAsync: async () => ({ id: "9:100", key: "btnkey", name: "Button", remote: true }),
      children: [],
    };
    const root = {
      id: "9:1", name: "Root", type: "FRAME",
      x: 0, y: 0, width: 200, height: 100,
      layoutMode: "VERTICAL", itemSpacing: 8,
      boundVariables: { paddingLeft: { type: "VARIABLE_ALIAS", id: "VariableID:1" } },
      children: [childInstance],
    };
    registerTree(root);
    figma.currentPage.selection = [root];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen", depth: 2 }));
    const node = res!.data.context[0];
    // Parent: base keys … children, then autoLayout, then tokens.
    expect(Object.keys(node)).toEqual([
      "id", "name", "type", "bounds", "styles", "children", "autoLayout", "tokens",
    ]);
    // Child INSTANCE: base … mainComponent, mainComponentId (issue #29), then
    // children, then enrich keys incl. componentRef.
    const child = node.children[0];
    expect(Object.keys(child)).toEqual([
      "id", "name", "type", "bounds", "styles", "mainComponent", "mainComponentId", "children",
      "autoLayout", "tokens", "componentRef",
    ]);
  });

  it("full detail has NONE of the codegen keys (regression)", async () => {
    const frame = {
      id: "5:1", name: "Row", type: "FRAME",
      x: 0, y: 0, width: 200, height: 40,
      layoutMode: "HORIZONTAL", itemSpacing: 8,
      boundVariables: { paddingLeft: { type: "VARIABLE_ALIAS", id: "VariableID:1" } },
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "full" }));
    const node = res!.data.context[0];
    expect(node.autoLayout).toBeUndefined();
    expect(node.tokens).toBeUndefined();
    expect(node.componentRef).toBeUndefined();
    expect(node.codeConnect).toBeUndefined();
  });
});

// ── extractInstanceOverrides: instance-child → component-child id matching ────

describe("get_design_context — extractInstanceOverrides id matching", () => {
  it("matches the overridden child by its component-child id when position differs", async () => {
    // Real Figma instance-descendant ids are `I<instanceId>;<componentChildId>`, where the
    // segment after `;` is the FULL component-child id (an `A:B` pair). Component children are
    // ordered [c1, c2, c3]; the instance drops c1 and keeps [c2, c3], so position no longer
    // lines up — matching must key on the id, not the index.
    const compChild1 = { id: "13:101", name: "Label1", type: "TEXT", characters: "Original1", visible: true, opacity: 1, fills: [] };
    const compChild2 = { id: "13:102", name: "Label2", type: "TEXT", characters: "Original2", visible: true, opacity: 1, fills: [] };
    const compChild3 = { id: "13:103", name: "Label3", type: "TEXT", characters: "Original3", visible: true, opacity: 1, fills: [] };
    const mc = {
      id: "13:100",
      key: "mc-key",
      name: "MyComp",
      children: [compChild1, compChild2, compChild3],
    };
    const instChild2 = { id: "I9:200;13:102", name: "Label2", type: "TEXT", characters: "Original2", visible: true, opacity: 1, fills: [] };
    const instChild3 = { id: "I9:200;13:103", name: "Label3", type: "TEXT", characters: "Overridden3", visible: true, opacity: 1, fills: [] };
    const inst = {
      id: "I9:200",
      name: "MyComp Instance",
      type: "INSTANCE",
      x: 0, y: 0, width: 200, height: 100,
      getMainComponentAsync: async () => mc,
      children: [instChild2, instChild3],
    };
    (mockNodes as any)["I9:200"] = inst;
    (mockNodes as any)["I9:200;13:102"] = instChild2;
    (mockNodes as any)["I9:200;13:103"] = instChild3;
    figma.currentPage.selection = [inst];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "full", dedupeComponents: true }));
    const node = res!.data.context[0];
    const overrides = node.overrides ?? [];
    // instChild3 (id ...;13:103) pairs with compChild3 → characters changed → recorded.
    const override3 = overrides.find((o: any) => o.id === "I9:200;13:103");
    expect(override3).toBeDefined();
    expect(override3.characters).toBe("Overridden3");
    // instChild2 (id ...;13:102) pairs with compChild2 → unchanged → not recorded.
    const override2 = overrides.find((o: any) => o.id === "I9:200;13:102");
    expect(override2).toBeUndefined();
  });
});

describe("codegen — typography array bindings, effects, graceful degradation", () => {
  it("resolves array-valued text-typography token bindings from element [0]", async () => {
    const text = {
      id: "6:1", name: "Heading", type: "TEXT",
      x: 0, y: 0, width: 80, height: 24,
      characters: "Title",
      fontName: { family: "Geist", style: "Bold" },
      fontSize: 18,
      // Real Plugin API: text-typography fields bind as VariableAlias[] (per-range).
      boundVariables: {
        fontSize: [{ type: "VARIABLE_ALIAS", id: "VariableID:2" }],
        lineHeight: [{ type: "VARIABLE_ALIAS", id: "VariableID:1" }],
      },
    };
    registerTree(text);
    figma.currentPage.selection = [text];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const node = res!.data.context[0];
    expect(node.tokens.fontSize).toBe("spacing/8");
    expect(node.tokens.lineHeight).toBe("spacing/16");
  });

  it("resolves effect-level (shadow) token bindings", async () => {
    const card = {
      id: "7:1", name: "Card", type: "FRAME",
      x: 0, y: 0, width: 200, height: 100,
      effects: [
        { type: "DROP_SHADOW", boundVariables: { radius: { type: "VARIABLE_ALIAS", id: "VariableID:1" } } },
      ],
      children: [],
    };
    registerTree(card);
    figma.currentPage.selection = [card];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const node = res!.data.context[0];
    expect(node.tokens["effects.0.radius"]).toBe("spacing/16");
  });

  it("degrades gracefully when getVariableByIdAsync throws (no abort)", async () => {
    figma.variables.getVariableByIdAsync = async () => {
      throw new Error("dynamic-page access error");
    };
    const frame = {
      id: "8:1", name: "Row", type: "FRAME",
      x: 0, y: 0, width: 200, height: 40,
      layoutMode: "HORIZONTAL", itemSpacing: 8,
      boundVariables: { paddingLeft: { type: "VARIABLE_ALIAS", id: "VariableID:1" } },
      children: [],
    };
    registerTree(frame);
    figma.currentPage.selection = [frame];

    const res = await handleReadDocumentRequest(makeRequest({ detail: "codegen" }));
    const node = res!.data.context[0];
    // serialization still succeeds; the unresolved token is simply absent.
    expect(node.autoLayout.layoutMode).toBe("HORIZONTAL");
    expect(node.tokens).toBeUndefined();
  });
});
