import { describe, it, expect, beforeEach } from "bun:test";
import { handleReadPrototypeRequest } from "./read-prototype";

const makeRequest = (type: string, nodeIds?: string[]) => ({
  type,
  requestId: "req-proto-1",
  nodeIds: nodeIds ?? [],
  params: {},
});

let mockNodes: Record<string, any>;
let page: any;

// Build a small page: Login frame with a "Next" button -> Home (NAVIGATE), and a
// "Menu" button -> Drawer (OVERLAY). Drawer carries read-only overlay config.
beforeEach(() => {
  const nextButton: any = {
    id: "1:10", name: "Next", type: "INSTANCE",
    reactions: [{
      trigger: { type: "ON_CLICK" },
      actions: [{ type: "NODE", navigation: "NAVIGATE", destinationId: "1:3",
        transition: { type: "PUSH", direction: "RIGHT", duration: 0.3, easing: { type: "EASE_OUT" } } }],
    }],
  };
  const menuButton: any = {
    id: "1:11", name: "Menu", type: "INSTANCE",
    reactions: [{
      trigger: { type: "ON_CLICK" },
      actions: [{ type: "NODE", navigation: "OVERLAY", destinationId: "1:4" }],
    }],
  };
  const drawer: any = {
    id: "1:4", name: "Drawer", type: "FRAME", reactions: [],
    overlayPositionType: "CENTER",
    overlayBackground: { type: "NONE" },
    overlayBackgroundInteraction: "NONE",
  };
  const home: any = { id: "1:3", name: "Home", type: "FRAME", reactions: [] };
  const login: any = {
    id: "1:2", name: "Login", type: "FRAME", reactions: [],
    children: [nextButton, menuButton],
  };
  page = {
    id: "0:1", name: "Page 1", type: "PAGE", parent: null,
    flowStartingPoints: [{ nodeId: "1:2", name: "Onboarding" }],
    prototypeStartNode: login,
    children: [login, home, drawer],
    // findAll walks all descendants and applies the predicate (mirrors Figma's native API)
    findAll: (pred: (n: any) => boolean) => {
      const all = [login, nextButton, menuButton, home, drawer];
      return all.filter(pred);
    },
  };
  // Parent pointers so owningPage() can resolve the page via ancestry.
  login.parent = page; home.parent = page; drawer.parent = page;
  nextButton.parent = login; menuButton.parent = login;
  mockNodes = {
    "0:1": page, "1:2": login, "1:3": home, "1:4": drawer, "1:10": nextButton, "1:11": menuButton,
  };
  (globalThis as any).figma = {
    currentPage: page,
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    ui: { postMessage: () => {} },
  };
});

describe("get_prototype", () => {
  it("returns null for non-get_prototype requests", async () => {
    expect(await handleReadPrototypeRequest(makeRequest("get_reactions"))).toBeNull();
  });

  it("reads page flow starting points and prototype start node", async () => {
    const res = await handleReadPrototypeRequest(makeRequest("get_prototype"));
    expect(res?.data.pageId).toBe("0:1");
    expect(res?.data.flowStartingPoints).toEqual([{ nodeId: "1:2", name: "Onboarding" }]);
    expect(res?.data.prototypeStartNodeId).toBe("1:2");
  });

  it("builds source->destination edges with names resolved", async () => {
    const res = await handleReadPrototypeRequest(makeRequest("get_prototype"));
    expect(res?.data.edgeCount).toBe(2);
    const navEdge = res?.data.edges.find((e: any) => e.sourceId === "1:10");
    expect(navEdge.navigation).toBe("NAVIGATE");
    expect(navEdge.destinationId).toBe("1:3");
    expect(navEdge.destinationName).toBe("Home");
    expect(navEdge.transition.type).toBe("PUSH");
  });

  it("reports read-only overlay config for OVERLAY destinations", async () => {
    const res = await handleReadPrototypeRequest(makeRequest("get_prototype"));
    expect(res?.data.overlays).toHaveLength(1);
    expect(res?.data.overlays[0]).toMatchObject({
      nodeId: "1:4", name: "Drawer", overlayPositionType: "CENTER",
      overlayBackgroundInteraction: "NONE",
    });
  });

  it("scopes to a subtree when nodeIds provided", async () => {
    // Scope to the Home frame (no reactions) -> no edges.
    const home = mockNodes["1:3"];
    home.findAll = () => [];
    const res = await handleReadPrototypeRequest(makeRequest("get_prototype", ["1:3"]));
    expect(res?.data.edgeCount).toBe(0);
    expect(res?.data.pageId).toBe("0:1"); // page still resolved via ancestry
  });

  it("throws when a scoped node is not found", async () => {
    await expect(
      handleReadPrototypeRequest(makeRequest("get_prototype", ["9:9"]))
    ).rejects.toThrow(/not found/i);
  });

  it("builds an edge from a legacy singular `action` reaction", async () => {
    // Old files store a singular `action` instead of the `actions` array.
    mockNodes["1:10"].reactions = [{
      trigger: { type: "ON_CLICK" },
      action: { type: "NODE", navigation: "NAVIGATE", destinationId: "1:3" },
    }];
    mockNodes["1:11"].reactions = [];
    const res = await handleReadPrototypeRequest(makeRequest("get_prototype"));
    const edge = res?.data.edges.find((e: any) => e.sourceId === "1:10");
    expect(edge.actionType).toBe("NODE");
    expect(edge.destinationId).toBe("1:3");
    expect(edge.destinationName).toBe("Home");
  });

  it("dedupes reaction nodes across overlapping scoped roots", async () => {
    // Scope to both the page-level login frame and its child button; the child
    // must not be counted twice.
    const login = mockNodes["1:2"];
    login.findAll = (pred: (n: any) => boolean) =>
      [mockNodes["1:10"], mockNodes["1:11"]].filter(pred);
    const res = await handleReadPrototypeRequest(makeRequest("get_prototype", ["1:2", "1:10"]));
    const fromNext = res?.data.edges.filter((e: any) => e.sourceId === "1:10");
    expect(fromNext).toHaveLength(1);
  });
});
