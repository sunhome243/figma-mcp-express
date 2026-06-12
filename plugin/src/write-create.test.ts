import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteCreateRequest } from "./write-create";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let createdComponents: any[];
let convertedFrom: any;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  createdComponents = [];
  convertedFrom = undefined;
  mockNodes = {};
  (globalThis as any).figma = {
    get currentPage() { return { id: "0:1", name: "Page 1", appendChild: () => {} }; },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    // Native in-place conversion — preserves all properties, bound variables, and effects.
    createComponentFromNode: (node: any) => {
      convertedFrom = node;
      const comp: any = {
        id: "comp:new",
        name: node.name,
        type: "COMPONENT",
        x: node.x, y: node.y, width: node.width, height: node.height,
      };
      createdComponents.push(comp);
      return comp;
    },
    commitUndo: () => { commitUndoCalled = true; },
    mixed: Symbol("mixed"),
  };
});

// ── create_component ──────────────────────────────────────────────────────────

describe("create_component", () => {
  it("converts a node to a COMPONENT in place via native createComponentFromNode", async () => {
    const frame = {
      id: "1:1", name: "Card", type: "FRAME",
      x: 10, y: 20, width: 200, height: 100,
    };
    mockNodes["1:1"] = frame;

    const res = await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    expect(res?.data.type).toBe("COMPONENT");
    expect(convertedFrom).toBe(frame);              // native API received the original node
    expect(createdComponents[0].name).toBe("Card"); // name preserved by native conversion
    expect(commitUndoCalled).toBe(true);
  });

  it("applies a custom name when provided", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME", x: 0, y: 0, width: 100, height: 100 };
    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"], { name: "Button" }));
    expect(createdComponents[0].name).toBe("Button");
  });

  it("converts a GROUP too (not frame-only)", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "G", type: "GROUP", x: 0, y: 0, width: 50, height: 50 };
    const res = await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    expect(res?.data.type).toBe("COMPONENT");
    expect(convertedFrom.type).toBe("GROUP");
  });

  it("throws when node is already a COMPONENT", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "COMPONENT" };
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["1:1"]))
    ).rejects.toThrow("already a COMPONENT");
  });

  it("throws when node is an INSTANCE", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "INSTANCE" };
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["1:1"]))
    ).rejects.toThrow("INSTANCE");
  });

  it("throws when nodeId not found", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["9:9"]))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when no nodeId provided", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", []))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── create_section ────────────────────────────────────────────────────────────

describe("create_section", () => {
  let createdSection: any;
  let appendedToParent: any;

  beforeEach(() => {
    createdSection = null;
    appendedToParent = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: { id: "0:1", name: "Page 1", appendChild: (n: any) => { appendedToParent = "page"; } },
      createSection: () => {
        createdSection = {
          id: "section:new", name: "Section", type: "SECTION",
          x: 0, y: 0, width: 200, height: 200,
          resizeWithoutConstraints(w: number, h: number) { this.width = w; this.height = h; },
        };
        return createdSection;
      },
    };
  });

  it("creates a section with a name", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], { name: "Sprint 1" }));
    expect(createdSection.name).toBe("Sprint 1");
    expect(res?.data.type).toBe("SECTION");
    expect(res?.data.id).toBe("section:new");
    expect(commitUndoCalled).toBe(true);
  });

  it("creates a section at a specific position", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], { x: 100, y: 200 }));
    expect(createdSection.x).toBe(100);
    expect(createdSection.y).toBe(200);
  });

  it("creates a section with custom size", async () => {
    await handleWriteCreateRequest(makeRequest("create_section", [], { width: 800, height: 600 }));
    expect(createdSection.width).toBe(800);
    expect(createdSection.height).toBe(600);
  });

  it("creates a section with default values when no params given", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], {}));
    expect(res?.data.id).toBe("section:new");
  });

  // parentId support: appends into a specific parent
  it("appends section into a specific parent when parentId provided", async () => {
    let appendedChild: any = null;
    mockNodes["parent:1"] = {
      id: "parent:1", type: "FRAME",
      appendChild: (n: any) => { appendedChild = n; },
    };
    await handleWriteCreateRequest(makeRequest("create_section", [], { parentId: "parent:1" }));
    expect(appendedChild).toBe(createdSection);
  });

  it("throws when parentId node is not found", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_section", [], { parentId: "nope:999" }))
    ).rejects.toThrow("Parent node not found");
  });

  it("throws when parentId node cannot have children", async () => {
    mockNodes["rect:1"] = { id: "rect:1", type: "RECTANGLE" }; // no appendChild
    await expect(
      handleWriteCreateRequest(makeRequest("create_section", [], { parentId: "rect:1" }))
    ).rejects.toThrow("cannot have children");
  });
});

// ── create_rectangle (per-corner radius) ──────────────────────────────────────

describe("create_rectangle per-corner radius", () => {
  let createdRect: any;

  beforeEach(() => {
    createdRect = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: { id: "0:1", appendChild: () => {} },
      createRectangle: () => {
        createdRect = {
          id: "rect:new", name: "Rectangle", type: "RECTANGLE",
          x: 0, y: 0, width: 100, height: 100,
          fills: [],
          cornerRadius: 0,
          topLeftRadius: 0,
          topRightRadius: 0,
          bottomLeftRadius: 0,
          bottomRightRadius: 0,
          resize(w: number, h: number) { this.width = w; this.height = h; },
          appendChild: () => {},
        };
        return createdRect;
      },
    };
  });

  it("applies uniform cornerRadius", async () => {
    await handleWriteCreateRequest(makeRequest("create_rectangle", [], { cornerRadius: 8 }));
    expect(createdRect.cornerRadius).toBe(8);
  });

  // Per-corner radii
  it("applies per-corner radii when all four are provided", async () => {
    await handleWriteCreateRequest(makeRequest("create_rectangle", [], {
      topLeftRadius: 4, topRightRadius: 8, bottomLeftRadius: 16, bottomRightRadius: 0,
    }));
    expect(createdRect.topLeftRadius).toBe(4);
    expect(createdRect.topRightRadius).toBe(8);
    expect(createdRect.bottomLeftRadius).toBe(16);
    expect(createdRect.bottomRightRadius).toBe(0);
  });

  it("applies individual per-corner radius properties independently", async () => {
    await handleWriteCreateRequest(makeRequest("create_rectangle", [], { topLeftRadius: 12 }));
    expect(createdRect.topLeftRadius).toBe(12);
    // Other corners untouched
    expect(createdRect.topRightRadius).toBe(0);
  });

  it("uniform cornerRadius and per-corner can coexist (uniform first, then override)", async () => {
    await handleWriteCreateRequest(makeRequest("create_rectangle", [], {
      cornerRadius: 8, topLeftRadius: 20,
    }));
    expect(createdRect.cornerRadius).toBe(8);
    expect(createdRect.topLeftRadius).toBe(20);
  });
});

// ── create_frame (cornerRadius/clipsContent/opacity) ─────────────────────────

describe("create_frame additional params", () => {
  let createdFrame: any;

  beforeEach(() => {
    createdFrame = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: { id: "0:1", appendChild: () => {} },
      createFrame: () => {
        createdFrame = {
          id: "frame:new", name: "Frame", type: "FRAME",
          x: 0, y: 0, width: 100, height: 100,
          fills: [],
          cornerRadius: 0,
          clipsContent: true,
          opacity: 1,
          layoutMode: "NONE",
          paddingTop: 0, paddingRight: 0, paddingBottom: 0, paddingLeft: 0, itemSpacing: 0,
          layoutSizingHorizontal: undefined, layoutSizingVertical: undefined,
          resize(w: number, h: number) { this.width = w; this.height = h; },
        };
        return createdFrame;
      },
    };
  });

  // Honor cornerRadius, clipsContent, opacity when provided
  it("honors cornerRadius when provided", async () => {
    await handleWriteCreateRequest(makeRequest("create_frame", [], { cornerRadius: 12 }));
    expect(createdFrame.cornerRadius).toBe(12);
  });

  it("honors clipsContent=false when provided", async () => {
    await handleWriteCreateRequest(makeRequest("create_frame", [], { clipsContent: false }));
    expect(createdFrame.clipsContent).toBe(false);
  });

  it("honors opacity when provided", async () => {
    await handleWriteCreateRequest(makeRequest("create_frame", [], { opacity: 0.5 }));
    expect(createdFrame.opacity).toBe(0.5);
  });

  it("does not override clipsContent default when not provided", async () => {
    await handleWriteCreateRequest(makeRequest("create_frame", [], {}));
    // default stays true (as set in mock)
    expect(createdFrame.clipsContent).toBe(true);
  });

  it("does not override opacity default when not provided", async () => {
    await handleWriteCreateRequest(makeRequest("create_frame", [], {}));
    expect(createdFrame.opacity).toBe(1);
  });
});

// ── import_image (scaleMode validation) ──────────────────────────────────────

describe("import_image scaleMode validation", () => {
  beforeEach(() => {
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: { id: "0:1", appendChild: () => {} },
      createImage: (bytes: Uint8Array) => ({ hash: "img-hash" }),
      createRectangle: () => ({
        id: "rect:img", name: "Rectangle", type: "RECTANGLE",
        x: 0, y: 0, width: 200, height: 200,
        fills: [] as any[],
        resize(w: number, h: number) { this.width = w; this.height = h; },
      }),
    };
  });

  it("accepts valid scaleMode FILL", async () => {
    const res = await handleWriteCreateRequest(makeRequest("import_image", [], {
      imageData: "TWFu", scaleMode: "FILL",
    }));
    expect(res?.data.id).toBe("rect:img");
  });

  it("accepts valid scaleMode FIT", async () => {
    const res = await handleWriteCreateRequest(makeRequest("import_image", [], {
      imageData: "TWFu", scaleMode: "FIT",
    }));
    expect(res?.data.id).toBe("rect:img");
  });

  it("accepts valid scaleMode CROP", async () => {
    const res = await handleWriteCreateRequest(makeRequest("import_image", [], {
      imageData: "TWFu", scaleMode: "CROP",
    }));
    expect(res?.data.id).toBe("rect:img");
  });

  it("accepts valid scaleMode TILE", async () => {
    const res = await handleWriteCreateRequest(makeRequest("import_image", [], {
      imageData: "TWFu", scaleMode: "TILE",
    }));
    expect(res?.data.id).toBe("rect:img");
  });

  // Invalid scaleMode throws a clear error
  it("throws a clear error for an invalid scaleMode", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("import_image", [], {
        imageData: "TWFu", scaleMode: "STRETCH",
      }))
    ).rejects.toThrow(/invalid.*scaleMode|scaleMode.*invalid/i);
  });

  it("throws for any unrecognised scaleMode value", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("import_image", [], {
        imageData: "TWFu", scaleMode: "bogus",
      }))
    ).rejects.toThrow(/scaleMode/i);
  });

  it("defaults to FILL when scaleMode not provided", async () => {
    // Should not throw
    const res = await handleWriteCreateRequest(makeRequest("import_image", [], {
      imageData: "TWFu",
    }));
    expect(res?.data.id).toBe("rect:img");
  });
});
