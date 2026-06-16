// Small shared node-tree helpers used across read/write handlers.

// Walk up to the owning PAGE node. Prototypes (reactions, flowStartingPoints)
// are per-page, so several handlers need to resolve a node's containing page.
// Returns null if the node has no PAGE ancestor (e.g. a detached node).
export function owningPage(node: any): any {
  let cur: any = node;
  while (cur && cur.type !== "PAGE") cur = cur.parent;
  return cur ?? null;
}
