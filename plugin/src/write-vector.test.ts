import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteVectorRequest } from "./write-vector";

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let lastOp: { fn: string; nodes: any[]; parent: any } | null;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type, requestId: "req-test-1", nodeIds: nodeIds ?? [], params: params ?? {},
});

const result = (name: string) => ({
  id: "9:9", name, type: "BOOLEAN_OPERATION",
  x: 0, y: 0, width: 10, height: 10, absoluteBoundingBox: { x: 0, y: 0, width: 10, height: 10 },
});

beforeEach(() => {
  commitUndoCalled = false;
  lastOp = null;
  mockNodes = {
    "1:1": { id: "1:1", name: "A", parent: { id: "0:1" } },
    "1:2": { id: "1:2", name: "B", parent: { id: "0:1" } },
  };
  const mk = (fn: string) => (nodes: any[], parent: any) => {
    lastOp = { fn, nodes, parent };
    return result(fn);
  };
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
    union: mk("UNION"),
    subtract: mk("SUBTRACT"),
    intersect: mk("INTERSECT"),
    exclude: mk("EXCLUDE"),
    flatten: mk("FLATTEN"),
  };
});

describe("boolean_operation", () => {
  it("unions 2 nodes into the first node's parent", async () => {
    const res = await handleWriteVectorRequest(makeRequest("boolean_operation", ["1:1", "1:2"], { operation: "UNION" }));
    expect(lastOp?.fn).toBe("UNION");
    expect(lastOp?.nodes.map((n: any) => n.id)).toEqual(["1:1", "1:2"]);
    expect(lastOp?.parent.id).toBe("0:1"); // first node's parent
    expect(res?.data.id).toBe("9:9");
    expect(commitUndoCalled).toBe(true);
  });

  it("flattens a single node", async () => {
    const res = await handleWriteVectorRequest(makeRequest("boolean_operation", ["1:1"], { operation: "FLATTEN", name: "diamond" }));
    expect(lastOp?.fn).toBe("FLATTEN");
    expect(res?.data.name).toBe("diamond");
  });

  it("uses an explicit parentId when given", async () => {
    mockNodes["2:0"] = { id: "2:0", name: "Box", appendChild: () => {} };
    await handleWriteVectorRequest(makeRequest("boolean_operation", ["1:1", "1:2"], { operation: "EXCLUDE", parentId: "2:0" }));
    expect(lastOp?.parent.id).toBe("2:0");
  });

  it("rejects a boolean op with fewer than 2 nodes", async () => {
    await expect(handleWriteVectorRequest(makeRequest("boolean_operation", ["1:1"], { operation: "UNION" })))
      .rejects.toThrow("at least 2 nodes");
  });

  it("rejects an unknown operation", async () => {
    await expect(handleWriteVectorRequest(makeRequest("boolean_operation", ["1:1", "1:2"], { operation: "MERGE" })))
      .rejects.toThrow("operation must be");
  });

  it("returns null for an unrelated request type", async () => {
    expect(await handleWriteVectorRequest(makeRequest("set_fills", ["1:1"], {}))).toBeNull();
  });
});
