// Page-level prototype flow reader. Unlike get_reactions (one node's raw reactions),
// get_prototype walks a whole page (or a scoped subtree) and returns the flow GRAPH:
// every reaction-bearing node as a set of source -> destination edges, plus the page's
// flow starting points and the read-only overlay configuration of overlay destinations.
//
// Reactions live on ANY node (buttons, instances, shapes), not just top-level frames —
// enumerating frames alone misses the real graph, so we collect reaction-bearing
// descendants across the subtree.

import { makeProgress } from "./progress";
import { owningPage } from "./node-utils";

// A node has prototype connections worth graphing when it carries reactions.
// Array.isArray(undefined) is already false, so no separate "reactions" in n guard.
const hasReactions = (n: any) => Array.isArray(n.reactions) && n.reactions.length > 0;

// One edge per (source node, reaction, action). A single node can carry several
// reactions, and a reaction can carry several actions, so the graph is flattened here.
interface PrototypeEdge {
  sourceId: string;
  sourceName: string;
  sourceType: string;
  trigger: any;
  actionType: string;
  navigation?: string;
  destinationId?: string | null;
  destinationName?: string | null;
  transition?: any;
  url?: string;
}

interface OverlayConfig {
  nodeId: string;
  name: string;
  overlayPositionType: string | null;
  overlayBackground: any;
  overlayBackgroundInteraction: string | null;
}

export const handleReadPrototypeRequest = async (request: any) => {
  switch (request.type) {
    case "get_prototype":
      return getPrototype(request);
    default:
      return null;
  }
};

type DestResolver = (id: string | null | undefined) => Promise<any>;

// Resolve the scope: explicit nodeIds (subtrees, all on one page) or the whole
// current page. Returns the roots to traverse and the owning page.
const resolveScope = async (nodeIds: string[]): Promise<{ roots: any[]; page: any }> => {
  if (nodeIds.length === 0) {
    return { roots: [figma.currentPage], page: figma.currentPage };
  }
  const roots: any[] = [];
  let page: any = null;
  for (const id of nodeIds) {
    const node = await figma.getNodeByIdAsync(id);
    if (!node) throw new Error(`Node not found: ${id}`);
    roots.push(node);
    const ownerPage = owningPage(node);
    if (ownerPage == null) continue;
    if (page == null) page = ownerPage;
    else if (page.id !== ownerPage.id) {
      throw new Error("All scoped nodes must be on the same page");
    }
  }
  return { roots, page };
};

// Flatten one reaction-bearing node into its edges (one per reaction × action).
const buildEdgesForNode = async (
  node: any,
  resolveDest: DestResolver,
  overlayDestIds: Set<string>
): Promise<PrototypeEdge[]> => {
  const edges: PrototypeEdge[] = [];
  for (const reaction of node.reactions as any[]) {
    // Support both the current `actions` array and the legacy singular `action`.
    const actions: any[] = reaction.actions ?? (reaction.action != null ? [reaction.action] : []);
    for (const action of actions) {
      const actionType: string = action?.type ?? "UNKNOWN";
      const edge: PrototypeEdge = {
        sourceId: node.id,
        sourceName: node.name,
        sourceType: node.type,
        trigger: reaction.trigger,
        actionType,
      };
      if (actionType === "NODE") {
        edge.navigation = action.navigation;
        edge.destinationId = action.destinationId ?? null;
        const dest = await resolveDest(action.destinationId);
        edge.destinationName = dest ? dest.name : null;
        edge.transition = action.transition ?? null;
        if (action.navigation === "OVERLAY" && action.destinationId) {
          overlayDestIds.add(action.destinationId);
        }
      } else if (actionType === "URL") {
        edge.url = action.url;
      }
      edges.push(edge);
    }
  }
  return edges;
};

const getPrototype = async (request: any) => {
  const nodeIds: string[] = request.nodeIds ?? [];
  const { roots, page } = await resolveScope(nodeIds);

  // Heartbeat before the synchronous findAll walk so the Go-bridge inactivity
  // timer is reset ahead of the work.
  const tick = makeProgress(request.requestId, "get_prototype", 1);
  figma.ui.postMessage({
    type: "progress_update",
    requestId: request.requestId,
    progress: 10,
    message: "Reading prototype flow...",
  });
  await new Promise((r) => setTimeout(r, 0));

  // Collect reaction-bearing nodes across every root. findAll runs the native
  // traversal and invokes the predicate per node; reactions are not a type criterion,
  // so a predicate walk is the documented way to filter on them. Dedupe by id so
  // overlapping scoped roots (a parent and its descendant) don't double-count.
  const reactionNodesById = new Map<string, any>();
  for (const root of roots) {
    if (hasReactions(root)) reactionNodesById.set(root.id, root);
    if (typeof root.findAll === "function") {
      for (const n of root.findAll(hasReactions)) reactionNodesById.set(n.id, n);
    }
  }

  // Resolve each unique destination node once. Both edge building (for the name)
  // and the overlay pass (for read-only overlay config) read the same nodes, so we
  // cache the NODE — not just its name — to avoid a second serial round-trip per
  // overlay destination (the live plugin is a serial single-owner resource).
  const destCache = new Map<string, any>();
  const resolveDest: DestResolver = async (id) => {
    if (id == null) return null;
    if (destCache.has(id)) return destCache.get(id);
    const dest = await figma.getNodeByIdAsync(id);
    destCache.set(id, dest);
    return dest;
  };

  // Each NODE edge awaits a destination lookup; tick per source node so a large
  // flow graph keeps the bridge timer alive through the serial fetches (the
  // get_fonts / search_nodes per-item heartbeat pattern, not the single-native-call
  // scan_* pattern).
  const edges: PrototypeEdge[] = [];
  const overlayDestIds = new Set<string>();
  for (const node of reactionNodesById.values()) {
    await tick();
    edges.push(...(await buildEdgesForNode(node, resolveDest, overlayDestIds)));
  }

  // Overlay destinations carry read-only position/scrim/dismiss config the auto-wire
  // skill must read and respect (it cannot set these via the Plugin API). Report them
  // so a consumer can flag a dropdown/sheet still sitting at the default CENTER. These
  // nodes were already fetched during edge building, so resolveDest is a cache hit.
  const overlays: OverlayConfig[] = [];
  for (const id of overlayDestIds) {
    const dest: any = await resolveDest(id);
    if (!dest) continue;
    overlays.push({
      nodeId: dest.id,
      name: dest.name,
      overlayPositionType: "overlayPositionType" in dest ? dest.overlayPositionType : null,
      overlayBackground: "overlayBackground" in dest ? dest.overlayBackground : null,
      overlayBackgroundInteraction:
        "overlayBackgroundInteraction" in dest ? dest.overlayBackgroundInteraction : null,
    });
  }

  const flowStartingPoints = page && page.flowStartingPoints ? [...page.flowStartingPoints] : [];
  const prototypeStartNodeId =
    page && page.prototypeStartNode ? page.prototypeStartNode.id : null;

  return {
    type: request.type,
    requestId: request.requestId,
    data: {
      pageId: page ? page.id : null,
      pageName: page ? page.name : null,
      flowStartingPoints,
      prototypeStartNodeId,
      reactionNodeCount: reactionNodesById.size,
      edgeCount: edges.length,
      edges,
      overlays,
    },
  };
};
