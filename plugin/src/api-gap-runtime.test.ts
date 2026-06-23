import { describe, it, expect, beforeEach } from "bun:test";
import { handleReadRequest } from "./read-handlers";
import { handleWriteRequest } from "./write-handlers";

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-api-gap",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

let mockNodes: Record<string, any>;
let mockVariables: Record<string, any>;
let mockStyles: Record<string, any>;
let commitUndoCalled: boolean;

beforeEach(() => {
  mockNodes = {};
  mockVariables = {};
  mockStyles = {};
  commitUndoCalled = false;
  (globalThis as any).figma = {
    currentPage: { selection: [] },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    getStyleByIdAsync: async (id: string) => mockStyles[id] ?? null,
    getImageByHash: (_hash: string) => null,
    getFileThumbnailNodeAsync: async () => null,
    setFileThumbnailNodeAsync: async (_node: any) => {},
    getSelectionColors: () => null,
    moveLocalPaintStyleAfter: (_target: any, _reference: any) => {},
    moveLocalTextStyleAfter: (_target: any, _reference: any) => {},
    moveLocalEffectStyleAfter: (_target: any, _reference: any) => {},
    moveLocalGridStyleAfter: (_target: any, _reference: any) => {},
    moveLocalPaintFolderAfter: (_target: string, _reference: string | null) => {},
    moveLocalTextFolderAfter: (_target: string, _reference: string | null) => {},
    moveLocalEffectFolderAfter: (_target: string, _reference: string | null) => {},
    moveLocalGridFolderAfter: (_target: string, _reference: string | null) => {},
    commitUndo: () => { commitUndoCalled = true; },
    variables: {
      getVariableByIdAsync: async (id: string) => mockVariables[id] ?? null,
      createVariableAliasByIdAsync: async (id: string) => ({ type: "VARIABLE_ALIAS", id }),
      setBoundVariableForEffect: (effect: any, field: string, variable: any) => ({
        ...effect,
        boundVariables: { ...(effect.boundVariables ?? {}), [field]: { type: "VARIABLE_ALIAS", id: variable.id } },
      }),
      setBoundVariableForLayoutGrid: (grid: any, field: string, variable: any) => ({
        ...grid,
        boundVariables: { ...(grid.boundVariables ?? {}), [field]: { type: "VARIABLE_ALIAS", id: variable.id } },
      }),
    },
  };
});

describe("media and thumbnail readback APIs", () => {
  it("gets image metadata and bytes by hash", async () => {
    (globalThis as any).figma.getImageByHash = (hash: string) => ({
      hash,
      getSizeAsync: async () => ({ width: 64, height: 32 }),
      getBytesAsync: async () => new Uint8Array([77, 97, 110]),
    });
    const res = await handleReadRequest(makeRequest("get_image_by_hash", [], { hash: "abc" }));
    expect(res?.data).toEqual({ hash: "abc", width: 64, height: 32, bytesBase64: "TWFu" });
  });

  it("returns null data for a missing image hash", async () => {
    const res = await handleReadRequest(makeRequest("get_image_by_hash", [], { hash: "missing" }));
    expect(res?.data).toEqual({ hash: "missing", image: null });
  });

  it("gets and sets the file thumbnail node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Cover", type: "FRAME" };
    let thumbnailNode: any = mockNodes["1:1"];
    (globalThis as any).figma.getFileThumbnailNodeAsync = async () => thumbnailNode;
    (globalThis as any).figma.setFileThumbnailNodeAsync = async (node: any) => { thumbnailNode = node; };

    const got = await handleReadRequest(makeRequest("get_file_thumbnail"));
    expect(got?.data).toEqual({ nodeId: "1:1", name: "Cover", type: "FRAME" });

    const cleared = await handleWriteRequest(makeRequest("set_file_thumbnail", [], { nodeId: null }));
    expect(cleared?.data).toEqual({ nodeId: null, cleared: true });
    expect(thumbnailNode).toBeNull();
  });
});

describe("dev resources", () => {
  it("gets dev resources from a node including children when requested", async () => {
    mockNodes["1:1"] = {
      id: "1:1",
      getDevResourcesAsync: async (options: any) => [
        { nodeId: "1:1", name: options.includeChildren ? "Spec" : "Local", url: "https://example.com/spec" },
      ],
    };
    const res = await handleReadRequest(makeRequest("get_dev_resources", [], { nodeId: "1:1", includeChildren: true }));
    expect(res?.data.resources[0]).toMatchObject({ nodeId: "1:1", name: "Spec" });
  });

  it("adds, edits, and deletes dev resources on a node", async () => {
    const calls: any[] = [];
    mockNodes["1:1"] = {
      id: "1:1",
      addDevResourceAsync: async (url: string, name?: string) => calls.push(["add", url, name]),
      editDevResourceAsync: async (currentUrl: string, value: any) => calls.push(["edit", currentUrl, value]),
      deleteDevResourceAsync: async (url: string) => calls.push(["delete", url]),
    };
    await handleWriteRequest(makeRequest("add_dev_resource", [], { nodeId: "1:1", url: "https://a", name: "A" }));
    await handleWriteRequest(makeRequest("edit_dev_resource", [], { nodeId: "1:1", currentUrl: "https://a", url: "https://b", name: "B" }));
    await handleWriteRequest(makeRequest("delete_dev_resource", [], { nodeId: "1:1", url: "https://b" }));
    expect(calls).toEqual([
      ["add", "https://a", "A"],
      ["edit", "https://a", { url: "https://b", name: "B" }],
      ["delete", "https://b"],
    ]);
    expect(commitUndoCalled).toBe(true);
  });
});

describe("style organization and selection colors", () => {
  it("returns selection colors from the native API", async () => {
    (globalThis as any).figma.getSelectionColors = () => ({
      paints: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }],
      styles: [{ id: "S:1", name: "Brand/Red" }],
    });
    const res = await handleReadRequest(makeRequest("get_selection_colors"));
    expect(res?.data.colors.paints[0].type).toBe("SOLID");
    expect(res?.data.colors.styles[0].name).toBe("Brand/Red");
  });

  it("reorders local styles and folders with explicit style/folder types", async () => {
    const calls: any[] = [];
    mockStyles["S:target"] = { id: "S:target", name: "B", type: "PAINT" };
    mockStyles["S:ref"] = { id: "S:ref", name: "A", type: "PAINT" };
    (globalThis as any).figma.moveLocalPaintStyleAfter = (target: any, reference: any) => calls.push(["style", target.id, reference.id]);
    (globalThis as any).figma.moveLocalPaintFolderAfter = (target: string, reference: string | null) => calls.push(["folder", target, reference]);

    await handleWriteRequest(makeRequest("reorder_local_style", [], {
      styleType: "PAINT", styleId: "S:target", afterStyleId: "S:ref",
    }));
    await handleWriteRequest(makeRequest("reorder_local_style_folder", [], {
      styleType: "PAINT", folder: "Brand/Secondary", afterFolder: "Brand/Primary",
    }));
    expect(calls).toEqual([
      ["style", "S:target", "S:ref"],
      ["folder", "Brand/Secondary", "Brand/Primary"],
    ]);
  });

  it("rejects reorder_local_style when styleType does not match the target style", async () => {
    const calls: any[] = [];
    mockStyles["S:target"] = { id: "S:target", name: "B", type: "PAINT" };
    (globalThis as any).figma.moveLocalTextStyleAfter = (target: any, reference: any) => calls.push(["text", target.id, reference?.id ?? null]);

    await expect(handleWriteRequest(makeRequest("reorder_local_style", [], {
      styleType: "TEXT", styleId: "S:target",
    }))).rejects.toThrow("styleType TEXT does not match style S:target type PAINT");
    expect(calls).toEqual([]);
    expect(commitUndoCalled).toBe(false);
  });

  it("rejects reorder_local_style when styleType does not match afterStyleId", async () => {
    const calls: any[] = [];
    mockStyles["S:target"] = { id: "S:target", name: "B", type: "PAINT" };
    mockStyles["S:ref"] = { id: "S:ref", name: "Heading", type: "TEXT" };
    (globalThis as any).figma.moveLocalPaintStyleAfter = (target: any, reference: any) => calls.push(["paint", target.id, reference.id]);

    await expect(handleWriteRequest(makeRequest("reorder_local_style", [], {
      styleType: "PAINT", styleId: "S:target", afterStyleId: "S:ref",
    }))).rejects.toThrow("styleType PAINT does not match style S:ref type TEXT");
    expect(calls).toEqual([]);
    expect(commitUndoCalled).toBe(false);
  });
});

describe("variable helper APIs", () => {
  it("creates a variable alias by id", async () => {
    const res = await handleWriteRequest(makeRequest("create_variable_alias", [], { variableId: "var:1" }));
    expect(res?.data.alias).toEqual({ type: "VARIABLE_ALIAS", id: "var:1" });
  });

  it("resolves a variable for a consumer node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME" };
    mockVariables["var:1"] = {
      id: "var:1",
      name: "spacing/gap",
      resolveForConsumer: (node: any) => ({ value: 16, resolvedType: "FLOAT", nodeId: node.id }),
    };
    const res = await handleReadRequest(makeRequest("resolve_variable_for_consumer", [], { variableId: "var:1", nodeId: "1:1" }));
    expect(res?.data.resolved).toEqual({ value: 16, resolvedType: "FLOAT", nodeId: "1:1" });
  });

  it("removes variable code syntax during update_variable", async () => {
    const removed: string[] = [];
    mockVariables["var:1"] = {
      id: "var:1",
      name: "token",
      scopes: [],
      hiddenFromPublishing: false,
      codeSyntax: { WEB: "token" },
      removeVariableCodeSyntax: (platform: string) => removed.push(platform),
    };
    await handleWriteRequest(makeRequest("update_variable", [], { variableId: "var:1", removeCodeSyntax: ["WEB"] }));
    expect(removed).toEqual(["WEB"]);
  });

  it("binds variables to effect and layout-grid objects", async () => {
    mockVariables["var:float"] = { id: "var:float", resolvedType: "FLOAT" };
    const effectResult = await handleWriteRequest(makeRequest("bind_variable_to_effect", [], {
      effect: { type: "DROP_SHADOW", radius: 8 }, field: "radius", variableId: "var:float",
    }));
    const gridResult = await handleWriteRequest(makeRequest("bind_variable_to_layout_grid", [], {
      layoutGrid: { pattern: "GRID", sectionSize: 8 }, field: "sectionSize", variableId: "var:float",
    }));
    expect(effectResult?.data.effect.boundVariables.radius.id).toBe("var:float");
    expect(gridResult?.data.layoutGrid.boundVariables.sectionSize.id).toBe("var:float");
  });
});
