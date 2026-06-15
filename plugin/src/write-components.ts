import { bulkApply, WRITE_PROGRESS_EVERY } from "./write-helpers";
import { makeProgress } from "./progress";

export const handleWriteComponentRequest = async (request: any) => {
  switch (request.type) {
    case "swap_component": {
      const p = request.params || {};
      // Request-level guards (throw): the single target component is resolved ONCE
      // before the loop so a bad componentId fails the whole op, not per node.
      if ((request.nodeIds || []).length === 0) throw new Error("nodeIds is required");
      if (!p.componentId) throw new Error("componentId is required");
      const component = await figma.getNodeByIdAsync(p.componentId);
      if (!component) throw new Error(`Component not found: ${p.componentId}`);
      if (component.type !== "COMPONENT") throw new Error(`Node ${p.componentId} is not a COMPONENT`);
      // Per-node (collected): swap EVERY instance in nodeIds; wrong-type/missing
      // ids report their own error without aborting the rest (all→all bulk).
      // Use node.swapComponent() instead of node.mainComponent= to preserve
      // text overrides and other instance state (direct assignment clears all overrides).
      return bulkApply(request, (node, nid) => {
        if (node.type !== "INSTANCE") throw new Error(`Node ${nid} is not a component INSTANCE`);
        node.swapComponent(component);
        return { name: node.name, componentId: component.id, componentName: component.name };
      });
    }

    case "detach_instance": {
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid);
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (n.type !== "INSTANCE") { results.push({ nodeId: nid, error: "Node is not an INSTANCE" }); continue; }
        const frame = n.detachInstance();
        results.push({ nodeId: nid, newId: frame.id, name: frame.name });
        await tick(nodeIds.length);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { results },
      };
    }

    case "delete_nodes": {
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid);
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        // Per-node guard: one un-removable node (e.g. an instance child, which
        // Figma natively refuses) must not abort the rest of the batch.
        try {
          n.remove();
          results.push({ nodeId: nid, deleted: true });
        } catch (e: any) {
          let error = e?.message ?? String(e);
          if (/Removing this node is not allowed/i.test(error)) {
            error += " — instance children are structure-locked. To actually remove it: delete it on the master component (propagates to every instance), or detach_instance first then delete on the resulting plain frame. To replace it: swap the nested instance. set_visible:false only HIDES it (the node still exists — not a delete; use only when visual suppression, not removal, is the intent).";
          }
          results.push({ nodeId: nid, error });
        }
        await tick(nodeIds.length);
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "navigate_to_page": {
      const p = request.params || {};
      let page: PageNode | undefined;
      if (p.pageId) {
        const found = await figma.getNodeByIdAsync(p.pageId);
        if (!found) throw new Error(`Page not found: ${p.pageId}`);
        if (found.type !== "PAGE") throw new Error(`Node ${p.pageId} is not a PAGE`);
        page = found as PageNode;
      } else if (p.pageName) {
        page = figma.root.children.find(pg => pg.name === p.pageName) as PageNode | undefined;
        if (!page) throw new Error(`Page not found with name: ${p.pageName}`);
      } else {
        throw new Error("pageId or pageName is required");
      }
      await figma.setCurrentPageAsync(page);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: page.id, name: page.name },
      };
    }

    case "group_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const nodes = await Promise.all(nodeIds.map((id: string) => figma.getNodeByIdAsync(id)));
      const validNodes = nodes.filter((n): n is SceneNode => n !== null && n.type !== "DOCUMENT" && n.type !== "PAGE");
      if (validNodes.length === 0) throw new Error("No valid scene nodes found");
      const parent = validNodes[0].parent;
      if (!parent) throw new Error("Nodes must have a parent");
      // Guard against cross-parent grouping: figma.group would throw or partially
      // mutate if nodes belong to different parents.
      const allSameParent = validNodes.every(n => n.parent && n.parent.id === parent.id);
      if (!allSameParent) throw new Error("All nodes must share the same parent to be grouped");
      const group = figma.group(validNodes, parent as any);
      if (p.name) group.name = p.name;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: group.id, name: group.name, type: group.type },
      };
    }

    case "ungroup_nodes": {
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid);
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (n.type !== "GROUP") { results.push({ nodeId: nid, error: "Node is not a GROUP" }); continue; }
        const group = n as GroupNode;
        const parent = group.parent as any;
        const index = parent.children.indexOf(group);
        const childIds: string[] = [];
        for (const child of [...group.children]) {
          parent.insertChild(index, child as SceneNode);
          childIds.push(child.id);
        }
        group.remove();
        results.push({ nodeId: nid, childIds });
        await tick(nodeIds.length);
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    default:
      return null;
  }
};
