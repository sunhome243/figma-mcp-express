import { getBounds } from "./serializers";

// Boolean / vector operations. UNION/SUBTRACT/INTERSECT/EXCLUDE combine 2+ shapes
// into a boolean-operation node; FLATTEN rasterises 1+ nodes into a single vector.
// All place the result in the first node's parent unless parentId is given — needed
// for the custom primitives a GAP component requires (diamonds, week-bars) without eval.
export const handleWriteVectorRequest = async (request: any) => {
  switch (request.type) {
    case "boolean_operation": {
      const p = request.params || {};
      const op = p.operation;
      const ids: string[] = request.nodeIds || [];
      if (ids.length === 0) throw new Error("nodeIds is required");
      if (op !== "FLATTEN" && ids.length < 2) {
        throw new Error(`${op} needs at least 2 nodes (got ${ids.length})`);
      }

      const nodes: any[] = [];
      for (const id of ids) {
        const n = await figma.getNodeByIdAsync(id);
        if (!n) throw new Error(`Node not found: ${id}`);
        nodes.push(n);
      }

      // Result goes into an explicit parent, else the first node's current parent.
      let parent: any = nodes[0].parent;
      if (p.parentId) {
        parent = await figma.getNodeByIdAsync(p.parentId);
        if (!parent) throw new Error(`Parent not found: ${p.parentId}`);
        if (!("appendChild" in parent)) throw new Error(`Node ${p.parentId} cannot contain the result`);
      }

      let result: any;
      switch (op) {
        case "UNION":     result = figma.union(nodes, parent); break;
        case "SUBTRACT":  result = figma.subtract(nodes, parent); break;
        case "INTERSECT": result = figma.intersect(nodes, parent); break;
        case "EXCLUDE":   result = figma.exclude(nodes, parent); break;
        case "FLATTEN":   result = figma.flatten(nodes, parent); break;
        default: throw new Error(`operation must be UNION, SUBTRACT, INTERSECT, EXCLUDE, or FLATTEN, got: ${op}`);
      }
      if (p.name) result.name = p.name;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: result.id, name: result.name, type: result.type, bounds: getBounds(result) },
      };
    }
  }
  return null;
};
