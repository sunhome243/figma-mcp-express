// Benchmark: OLD per-level re-walk vs NEW depth-aware single walk (read serialization).
//
// Run: `bun run plugin/bench/serialize-bench.ts` (from the plugin/ dir, or adjust the path).
//
// Reproduces the v2.1.0 read-serialization win. Before the fix, the get_node /
// get_nodes_info / get_design_context handlers called the already-recursive
// serializeNode once PER LEVEL (a wrapper re-walk) and re-fetched every child by id —
// so a node at depth d was fully serialized d+1 times (O(N·D)). serializeNode is now
// depth-aware and walks the subtree once (O(N)). This bench instruments both on
// synthetic trees and prints machine-independent work counts plus wall-clock.
//
// Metrics:
//   serialize-body passes — counted via an `id` getter that fires once per
//     serializeNode body, so the count == number of node serializations. OLD ==
//     Σ subtreeSize(v); NEW == N. The ratio ≈ average subtree depth.
//   getNodeByIdAsync calls — the wrapper re-fetched each child by id every level;
//     NEW reads node.children directly, eliminating all N−1 redundant fetches.
//   wall-clock — with INSTANT mock I/O, so it UNDERSTATES the live gain (in the real
//     plugin every property read crosses the C++/JS boundary and each eliminated
//     fetch is a real async round-trip).
import { serializeNode } from "../src/serializers";

let idReads = 0;
let getNodeCalls = 0;
const registry = new Map<string, any>();

function makeNode(id: string, depth: number, maxDepth: number, branching: number): any {
  const children =
    depth < maxDepth
      ? Array.from({ length: branching }, (_, i) => makeNode(`${id}-${i}`, depth + 1, maxDepth, branching))
      : [];
  const node: any = {
    name: "n",
    type: "FRAME",
    x: 0, y: 0, width: 100, height: 100,
    fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }],
    fillStyleId: "S_SHARED",
  };
  Object.defineProperty(node, "id", { get() { idReads++; return id; }, enumerable: true });
  if (children.length) node.children = children;
  registry.set(id, node);
  return node;
}

(globalThis as any).figma = {
  getStyleByIdAsync: async () => ({ name: "style" }),
  getNodeByIdAsync: async (id: string) => { getNodeCalls++; return registry.get(id) ?? null; },
};

// OLD get_node handler, reconstructed verbatim: serializeNode(n) (unbounded, ==
// original) per level, then re-fetch each child by id and recurse. One shared caches
// object across the walk, exactly like the handler.
async function oldWrapper(n: any, depth: number, caches: any): Promise<any> {
  const serialized = await serializeNode(n, caches);
  if (depth >= 50 && serialized.children) {
    return Object.assign({}, serialized, { children: undefined, childCount: n.children ? n.children.length : 0 });
  }
  if (serialized.children) {
    const childNodes = await Promise.all(
      serialized.children.map((c: any) => (globalThis as any).figma.getNodeByIdAsync(c.id)),
    );
    const sc = await Promise.all(childNodes.filter((c: any) => c).map((c: any) => oldWrapper(c, depth + 1, caches)));
    return Object.assign({}, serialized, { children: sc });
  }
  return serialized;
}

async function run(label: string, depth: number, branching: number) {
  registry.clear();
  const root = makeNode("r", 0, depth, branching);
  const N = registry.size;

  idReads = 0; getNodeCalls = 0;
  let t = Bun.nanoseconds();
  await oldWrapper(root, 0, { styles: new Map(), components: new Map() });
  const oldMs = (Bun.nanoseconds() - t) / 1e6;
  const oldIdReads = idReads, oldGetNode = getNodeCalls;

  idReads = 0; getNodeCalls = 0;
  t = Bun.nanoseconds();
  await serializeNode(root, { styles: new Map(), components: new Map() }, undefined, { maxDepth: 50 });
  const newMs = (Bun.nanoseconds() - t) / 1e6;
  const newIdReads = idReads, newGetNode = getNodeCalls;

  console.log(`\n=== ${label}: depth=${depth}, branching=${branching}, N=${N.toLocaleString()} nodes ===`);
  console.log(`serialize-body passes : OLD ${oldIdReads.toLocaleString()}  NEW ${newIdReads.toLocaleString()}  → ${(oldIdReads / newIdReads).toFixed(1)}x less work`);
  console.log(`getNodeByIdAsync calls : OLD ${oldGetNode.toLocaleString()}  NEW ${newGetNode.toLocaleString()}  (all N-1 redundant child re-fetches eliminated)`);
  console.log(`wall-clock (mock I/O)  : OLD ${oldMs.toFixed(1)}ms  NEW ${newMs.toFixed(1)}ms  → ${(oldMs / newMs).toFixed(1)}x (understates live gain)`);
}

await run("balanced binary", 10, 2);
await run("UI-ish (wider)", 6, 4);
await run("deep spine", 14, 2);
