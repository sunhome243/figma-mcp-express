import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, applyAutoLayout, applyTextStyleProps, applyLayoutSizing, bulkApply } from "./write-helpers";

export const handleWriteModifyRequest = async (request: any) => {
  switch (request.type) {
    case "set_text": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type !== "TEXT") throw new Error(`Node ${nodeId} is not a TEXT node`);
      const fontName = typeof node.fontName === "symbol"
        ? { family: "Inter", style: "Regular" }
        : node.fontName;
      await figma.loadFontAsync(fontName);
      if (p.text != null) node.characters = p.text;
      // Optional styling (alignment / auto-resize / font / spacing / case / decoration).
      await applyTextStyleProps(node, p);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: node.id,
          name: node.name,
          characters: node.characters,
          textAlignHorizontal: node.textAlignHorizontal,
          textAutoResize: node.textAutoResize,
        },
      };
    }

    case "set_fills": {
      const p = request.params || {};
      // Advanced paints: a direct paints[] (gradient / image / mixed solids) is applied to the
      // node verbatim, bypassing the single-solid shorthand below. For a REUSABLE, tokenizable
      // fill prefer the design-system path — create_paint_style(paints[]) + apply_style_to_node;
      // this is the deliberate one-off "put this gradient on this node now" escape hatch.
      if (Array.isArray(p.paints)) {
        return bulkApply(request, (node) => {
          if (!("fills" in node)) throw new Error(`Node ${node.id} does not support fills`);
          node.fills = p.mode === "append" ? [...(node.fills as Paint[]), ...p.paints] : p.paints;
          return { name: node.name, warning: "direct paints applied — for a reusable/tokenized fill prefer create_paint_style + apply_style_to_node" };
        });
      }
      // Fill + variable + warning are request-level (identical for every node) → build
      // the paint ONCE here, not inside the loop, so setBoundVariableForPaint isn't
      // re-invoked per node. Per-node failures (no fills support) are collected.
      const variable = p.variableId
        ? await figma.variables.getVariableByIdAsync(p.variableId)
        : null;
      if (p.variableId && !variable) throw new Error(`Variable not found: ${p.variableId}`);
      const fillWarning = p.variableId
        ? undefined
        : p.mode === "append"
          ? "raw color used — prefer variableId binding to honor the design-system invariant"
          : "raw color used — prefer variableId binding to honor the design-system invariant; replace mode will also discard any existing variable bindings on this node";
      let fill: Paint = makeSolidPaint(p.color ?? "#000000", p.opacity != null ? p.opacity : undefined);
      if (variable) fill = figma.variables.setBoundVariableForPaint(fill as SolidPaint, "color", variable);
      return bulkApply(request, (node) => {
        if (!("fills" in node)) throw new Error(`Node ${node.id} does not support fills`);
        node.fills = p.mode === "append" ? [...(node.fills as Paint[]), fill] : [fill];
        return fillWarning ? { name: node.name, warning: fillWarning } : { name: node.name };
      });
    }

    case "set_strokes": {
      const p = request.params || {};
      // Advanced paints: a direct paints[] (gradient / image / mixed solids) applied verbatim,
      // bypassing the single-solid shorthand. For a reusable stroke prefer create_paint_style +
      // apply_style_to_node(target:"stroke"); this is the one-off direct path. strokeWeight is
      // still honored alongside.
      if (Array.isArray(p.paints)) {
        return bulkApply(request, (node) => {
          if (!("strokes" in node)) throw new Error(`Node ${node.id} does not support strokes`);
          node.strokes = p.mode === "append" ? [...(node.strokes as Paint[]), ...p.paints] : p.paints;
          if (p.strokeWeight != null) node.strokeWeight = p.strokeWeight;
          return { name: node.name, warning: "direct paints applied — for a reusable/tokenized stroke prefer create_paint_style + apply_style_to_node" };
        });
      }
      // Stroke paint + variable + warning are request-level → build the paint ONCE here
      // (strokeWeight stays per-node). Mirrors set_fills; avoids a per-node API call.
      const variable = p.variableId
        ? await figma.variables.getVariableByIdAsync(p.variableId)
        : null;
      if (p.variableId && !variable) throw new Error(`Variable not found: ${p.variableId}`);
      const strokeWarning = p.variableId
        ? undefined
        : p.mode === "append"
          ? "raw color used — prefer variableId binding to honor the design-system invariant"
          : "raw color used — prefer variableId binding to honor the design-system invariant; replace mode will also discard any existing variable bindings on this node";
      let stroke: Paint = makeSolidPaint(p.color ?? "#000000");
      if (variable) stroke = figma.variables.setBoundVariableForPaint(stroke as SolidPaint, "color", variable);
      return bulkApply(request, (node) => {
        if (!("strokes" in node)) throw new Error(`Node ${node.id} does not support strokes`);
        node.strokes = p.mode === "append" ? [...(node.strokes as Paint[]), stroke] : [stroke];
        if (p.strokeWeight != null) node.strokeWeight = p.strokeWeight;
        return strokeWarning ? { name: node.name, warning: strokeWarning } : { name: node.name };
      });
    }

    case "move_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("x" in n)) { results.push({ nodeId: nid, error: "Node does not support position" }); continue; }
        if (p.x != null) n.x = p.x;
        if (p.y != null) n.y = p.y;
        results.push({ nodeId: nid, x: n.x, y: n.y });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "resize_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      const hasLayoutSizing = p.layoutSizingHorizontal != null || p.layoutSizingVertical != null
        || p.layoutGrow != null || p.layoutAlign != null || p.layoutPositioning != null;
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("resize" in n)) { results.push({ nodeId: nid, error: "Node does not support resize" }); continue; }
        // Explicit px resize only when width/height given — so this tool can also
        // set FILL/HUG without forcing a fixed size.
        if (p.width != null || p.height != null) {
          const w = p.width != null ? p.width : n.width;
          const h = p.height != null ? p.height : n.height;
          n.resize(w, h);
        }
        // Sizing-within-parent (FILL/HUG/grow/align/positioning). Applied AFTER the px
        // resize so FILL/HUG wins. Needs an auto-layout parent → catch + report per node.
        if (hasLayoutSizing) {
          try {
            applyLayoutSizing(n, p);
          } catch (e) {
            results.push({ nodeId: nid, error: `layoutSizing failed (node needs an auto-layout parent): ${String(e)}` });
            continue;
          }
        }
        results.push({ nodeId: nid, width: n.width, height: n.height });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "rename_node": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      node.name = p.name;
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name },
      };
    }

    case "clone_node": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type === "PAGE" || node.type === "DOCUMENT") throw new Error("Cannot clone a page or document node");
      const clone = node.clone();
      if (p.x != null) clone.x = p.x;
      if (p.y != null) clone.y = p.y;
      if (p.parentId) {
        const parent = await getParentNode(p.parentId);
        (parent as any).appendChild(clone);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: clone.id, name: clone.name, type: clone.type, bounds: getBounds(clone) },
      };
    }

    case "set_opacity": {
      const p = request.params || {};
      if (p.opacity == null) throw new Error("opacity is required");
      return bulkApply(request, (n) => {
        if (!("opacity" in n)) throw new Error("Node does not support opacity");
        n.opacity = p.opacity;
        return { opacity: n.opacity };
      });
    }

    case "set_corner_radius": {
      const p = request.params || {};
      return bulkApply(request, (n: any) => {
        if (!("cornerRadius" in n)) throw new Error("Node does not support corner radius");
        if (p.cornerRadius != null) n.cornerRadius = p.cornerRadius;
        if (p.topLeftRadius != null) n.topLeftRadius = p.topLeftRadius;
        if (p.topRightRadius != null) n.topRightRadius = p.topRightRadius;
        if (p.bottomLeftRadius != null) n.bottomLeftRadius = p.bottomLeftRadius;
        if (p.bottomRightRadius != null) n.bottomRightRadius = p.bottomRightRadius;
        const cr = typeof n.cornerRadius === "symbol" ? "mixed" : n.cornerRadius;
        if (cr === "mixed") {
          return {
            cornerRadius: cr,
            topLeftRadius: n.topLeftRadius,
            topRightRadius: n.topRightRadius,
            bottomLeftRadius: n.bottomLeftRadius,
            bottomRightRadius: n.bottomRightRadius,
          };
        }
        return { cornerRadius: cr };
      });
    }

    case "set_auto_layout": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type !== "FRAME") throw new Error(`Node ${nodeId} is not a FRAME`);
      await applyAutoLayout(node, p);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name },
      };
    }

    case "set_visible": {
      const p = request.params || {};
      return bulkApply(request, (n) => {
        if (!("visible" in n)) throw new Error("Node does not support visibility");
        n.visible = p.visible;
        return { visible: n.visible };
      });
    }

    case "lock_nodes":
    case "unlock_nodes": {
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const locked = request.type === "lock_nodes";
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("locked" in n)) { results.push({ nodeId: nid, error: "Node does not support locking" }); continue; }
        n.locked = locked;
        results.push({ nodeId: nid, locked: n.locked });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "rotate_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("rotation" in n)) { results.push({ nodeId: nid, error: "Node does not support rotation" }); continue; }
        n.rotation = p.rotation;
        results.push({ nodeId: nid, rotation: n.rotation });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "reorder_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const validOrders = ["bringToFront", "sendToBack", "bringForward", "sendBackward"];
      if (!validOrders.includes(p.order)) {
        throw new Error(`order must be bringToFront, sendToBack, bringForward, or sendBackward`);
      }
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        const parent = n.parent as any;
        if (!parent || !("children" in parent)) { results.push({ nodeId: nid, error: "Node has no reorderable parent" }); continue; }
        const siblings = parent.children as any[];
        const currentIndex = siblings.indexOf(n);
        let newIndex: number;
        switch (p.order) {
          case "bringToFront":   newIndex = siblings.length - 1; break;
          case "sendToBack":     newIndex = 0; break;
          case "bringForward":   newIndex = Math.min(currentIndex + 1, siblings.length - 1); break;
          case "sendBackward":   newIndex = Math.max(currentIndex - 1, 0); break;
          default:               newIndex = currentIndex;
        }
        parent.insertChild(newIndex, n);
        results.push({ nodeId: nid, index: newIndex });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_blend_mode": {
      const p = request.params || {};
      const VALID_BLEND_MODES = new Set([
        "NORMAL", "MULTIPLY", "SCREEN", "OVERLAY", "DARKEN", "LIGHTEN",
        "COLOR_DODGE", "COLOR_BURN", "HARD_LIGHT", "SOFT_LIGHT", "DIFFERENCE",
        "EXCLUSION", "HUE", "SATURATION", "COLOR", "LUMINOSITY",
        "PASS_THROUGH", "LINEAR_BURN", "LINEAR_DODGE",
      ]);
      if (!p.blendMode || !VALID_BLEND_MODES.has(p.blendMode)) {
        throw new Error(
          `Invalid blend mode: "${p.blendMode}". Must be one of: ${[...VALID_BLEND_MODES].join(", ")}`,
        );
      }
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("blendMode" in n)) { results.push({ nodeId: nid, error: "Node does not support blend mode" }); continue; }
        n.blendMode = p.blendMode;
        results.push({ nodeId: nid, blendMode: n.blendMode });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_constraints": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("constraints" in n)) { results.push({ nodeId: nid, error: "Node does not support constraints" }); continue; }
        const updated: any = { ...n.constraints };
        if (p.horizontal) updated.horizontal = p.horizontal;
        if (p.vertical)   updated.vertical   = p.vertical;
        n.constraints = updated;
        results.push({ nodeId: nid, constraints: n.constraints });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "reparent_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      if (!p.parentId) throw new Error("parentId is required");
      const newParent = await figma.getNodeByIdAsync(p.parentId) as any;
      if (!newParent) throw new Error(`Parent not found: ${p.parentId}`);
      if (!("appendChild" in newParent)) throw new Error(`Node ${p.parentId} cannot contain children`);
      // preserveAbsolutePosition (default true) keeps the node visually put after reparenting,
      // but ONLY when the new parent is not auto-layout: an auto-layout parent positions its
      // children itself and ignores x/y, so a reparented child necessarily takes its laid-out
      // slot — there is nothing to preserve. The correction subtracts parent translation, which
      // is exact for unrotated/unscaled ancestors (the common case); a rotated/scaled parent
      // would need full transform inversion, which we deliberately don't attempt.
      const preserveAbsPos = p.preserveAbsolutePosition !== false;
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        try {
          const parentIsAutoLayout = newParent.layoutMode != null && newParent.layoutMode !== "NONE";
          const canPreserve = preserveAbsPos && !parentIsAutoLayout
            && n.absoluteTransform && newParent.absoluteTransform;
          const absX: number | undefined = canPreserve ? n.absoluteTransform[0][2] : undefined;
          const absY: number | undefined = canPreserve ? n.absoluteTransform[1][2] : undefined;
          newParent.appendChild(n);
          if (canPreserve && absX !== undefined && absY !== undefined) {
            // Parent transform is unchanged by appendChild; convert the saved absolute
            // position back into the new parent's local space.
            n.x = absX - newParent.absoluteTransform[0][2];
            n.y = absY - newParent.absoluteTransform[1][2];
          }
          results.push({ nodeId: nid, newParentId: p.parentId, positionPreserved: canPreserve });
        } catch (e: any) {
          results.push({ nodeId: nid, error: e.message });
        }
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "batch_rename_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        const oldName: string = n.name;
        let newName = oldName;
        if (p.find !== undefined && p.replace !== undefined) {
          if (p.useRegex) {
            try {
              const regex = new RegExp(p.find, p.regexFlags || "g");
              newName = newName.replace(regex, p.replace);
            } catch (e: any) {
              results.push({ nodeId: nid, error: `Invalid regex: ${e.message}` }); continue;
            }
          } else {
            newName = newName.split(p.find).join(p.replace);
          }
        }
        if (p.prefix) newName = p.prefix + newName;
        if (p.suffix) newName = newName + p.suffix;
        n.name = newName;
        results.push({ nodeId: nid, oldName, name: newName });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "find_replace_text": {
      const p = request.params || {};
      if (!p.find) throw new Error("find is required");
      if (p.replace === undefined) throw new Error("replace is required");
      const rootNodeId = request.nodeIds && request.nodeIds[0];
      const root: any = rootNodeId
        ? await figma.getNodeByIdAsync(rootNodeId)
        : figma.currentPage;
      if (!root) throw new Error(`Root node not found: ${rootNodeId}`);
      const textNodes: any[] = [];
      const collect = (node: any) => {
        if (node.type === "TEXT") textNodes.push(node);
        if ("children" in node) (node.children as any[]).forEach(collect);
      };
      collect(root);
      const results: any[] = [];
      for (const tn of textNodes) {
        const originalText: string = tn.characters;
        let newText: string;
        if (p.useRegex) {
          try {
            const regex = new RegExp(p.find, p.regexFlags || "g");
            newText = originalText.replace(regex, p.replace);
          } catch (e: any) {
            results.push({ nodeId: tn.id, nodeName: tn.name, error: `Invalid regex: ${e.message}` });
            continue;
          }
        } else {
          newText = originalText.split(p.find).join(p.replace);
        }
        if (newText !== originalText) {
          const fontName = typeof tn.fontName === "symbol"
            ? { family: "Inter", style: "Regular" }
            : tn.fontName;
          await figma.loadFontAsync(fontName);
          tn.characters = newText;
          results.push({ nodeId: tn.id, nodeName: tn.name, oldText: originalText, newText });
        }
      }
      figma.commitUndo();
      const successCount = results.filter((r: any) => !r.error).length;
      return { type: request.type, requestId: request.requestId, data: { replaced: successCount, results } };
    }

    default:
      return null;
  }
};
