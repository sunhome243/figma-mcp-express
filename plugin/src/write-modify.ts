import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, applyAutoLayout, applyTextStyleProps, applyLayoutSizing, bulkApply, WRITE_PROGRESS_EVERY } from "./write-helpers";
import { makeProgress } from "./progress";

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

    case "set_text_range": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type !== "TEXT") throw new Error(`Node ${nodeId} is not a TEXT node`);
      const len = node.characters.length;
      const start = Number(p.startOffset);
      const end = Number(p.endOffset);
      if (!Number.isInteger(start) || !Number.isInteger(end)) {
        throw new Error("startOffset and endOffset must be integers");
      }
      if (start < 0 || end > len || start >= end) {
        throw new Error(`Invalid range [${start}, ${end}) for text of length ${len} (need 0 <= start < end <= length)`);
      }
      // Load EVERY font already present in the range before any mutation — a range can
      // span multiple fonts, and setRange* throws if any covering font is unloaded.
      const rangeFonts: FontName[] = node.getRangeAllFontNames(start, end);
      for (const f of rangeFonts) await figma.loadFontAsync(f);
      // Font change: resolve target family/style (fall back to the range's first font),
      // load the NEW font, then apply.
      if (p.fontFamily != null || p.fontStyle != null) {
        const base = rangeFonts[0] || { family: "Inter", style: "Regular" };
        const family = p.fontFamily != null ? String(p.fontFamily) : base.family;
        const style = p.fontStyle != null ? String(p.fontStyle) : base.style;
        await figma.loadFontAsync({ family, style });
        node.setRangeFontName(start, end, { family, style });
      }
      if (p.fontSize != null) node.setRangeFontSize(start, end, Number(p.fontSize));
      if (p.color != null) node.setRangeFills(start, end, [makeSolidPaint(p.color)]);
      if (p.textCase != null) node.setRangeTextCase(start, end, p.textCase);
      if (p.textDecoration != null) node.setRangeTextDecoration(start, end, p.textDecoration);
      if (p.letterSpacingValue != null) {
        node.setRangeLetterSpacing(start, end, { value: Number(p.letterSpacingValue), unit: p.letterSpacingUnit || "PIXELS" });
      }
      if (p.lineHeightUnit === "AUTO") node.setRangeLineHeight(start, end, { unit: "AUTO" });
      else if (p.lineHeightValue != null) {
        node.setRangeLineHeight(start, end, { value: Number(p.lineHeightValue), unit: p.lineHeightUnit || "PIXELS" });
      }
      if (p.hyperlink !== undefined) {
        if (p.hyperlink === null) node.setRangeHyperlink(start, end, null);
        else if (p.hyperlink.url) node.setRangeHyperlink(start, end, { type: "URL", value: String(p.hyperlink.url) });
        else if (p.hyperlink.nodeId) node.setRangeHyperlink(start, end, { type: "NODE", value: String(p.hyperlink.nodeId) });
      }
      if (p.listOptions != null) node.setRangeListOptions(start, end, { type: p.listOptions.type || "NONE" });
      if (p.indentation != null) node.setRangeIndentation(start, end, Number(p.indentation));
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name, startOffset: start, endOffset: end },
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
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("x" in n)) { results.push({ nodeId: nid, error: "Node does not support position" }); continue; }
        if (p.x != null) n.x = p.x;
        if (p.y != null) n.y = p.y;
        results.push({ nodeId: nid, x: n.x, y: n.y });
        await tick(nodeIds.length);
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
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
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
        // Responsive min/max constraints (null clears). Valid on frames and on
        // auto-layout children; Figma throws if the value is non-positive or the node
        // isn't in an auto-layout context — keep that per-node so one bad node doesn't
        // abort the whole bulk request (matches the per-node error model above).
        const hasMinMax = p.minWidth !== undefined || p.maxWidth !== undefined
          || p.minHeight !== undefined || p.maxHeight !== undefined;
        if (hasMinMax) {
          const num = (v: any) => { const x = Number(v); return Number.isFinite(x) ? x : null; };
          try {
            if (p.minWidth !== undefined && "minWidth" in n) n.minWidth = p.minWidth === null ? null : num(p.minWidth);
            if (p.maxWidth !== undefined && "maxWidth" in n) n.maxWidth = p.maxWidth === null ? null : num(p.maxWidth);
            if (p.minHeight !== undefined && "minHeight" in n) n.minHeight = p.minHeight === null ? null : num(p.minHeight);
            if (p.maxHeight !== undefined && "maxHeight" in n) n.maxHeight = p.maxHeight === null ? null : num(p.maxHeight);
          } catch (e) {
            results.push({ nodeId: nid, error: `min/max constraint failed (must be positive; node must be an auto-layout frame or child): ${String(e)}` });
            continue;
          }
        }
        results.push({ nodeId: nid, width: n.width, height: n.height });
        await tick(nodeIds.length);
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

    case "set_file_thumbnail": {
      const p = request.params || {};
      if (typeof figma.setFileThumbnailNodeAsync !== "function") {
        throw new Error("setFileThumbnailNodeAsync is unavailable in this Figma host");
      }
      let node: any = null;
      if (p.nodeId != null) {
        node = await figma.getNodeByIdAsync(String(p.nodeId));
        if (!node) throw new Error(`Node not found: ${p.nodeId}`);
        if (!["FRAME", "COMPONENT", "COMPONENT_SET", "SECTION"].includes(node.type)) {
          throw new Error("file thumbnail node must be FRAME, COMPONENT, COMPONENT_SET, SECTION, or null");
        }
      }
      await figma.setFileThumbnailNodeAsync(node);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: node ? { nodeId: node.id, name: node.name, type: node.type } : { nodeId: null, cleared: true },
      };
    }

    case "add_dev_resource":
    case "edit_dev_resource":
    case "delete_dev_resource": {
      const p = request.params || {};
      if (!p.nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(String(p.nodeId)) as any;
      if (!node) throw new Error(`Node not found: ${p.nodeId}`);
      if (request.type === "add_dev_resource") {
        if (!p.url) throw new Error("url is required");
        if (typeof node.addDevResourceAsync !== "function") throw new Error(`Node ${p.nodeId} does not support dev resources`);
        await node.addDevResourceAsync(String(p.url), p.name != null ? String(p.name) : undefined);
      } else if (request.type === "edit_dev_resource") {
        if (!p.currentUrl) throw new Error("currentUrl is required");
        if (typeof node.editDevResourceAsync !== "function") throw new Error(`Node ${p.nodeId} does not support dev resources`);
        const next: any = {};
        if (p.url != null) next.url = String(p.url);
        if (p.name != null) next.name = String(p.name);
        if (Object.keys(next).length === 0) throw new Error("edit_dev_resource requires url or name");
        await node.editDevResourceAsync(String(p.currentUrl), next);
      } else {
        if (!p.url) throw new Error("url is required");
        if (typeof node.deleteDevResourceAsync !== "function") throw new Error(`Node ${p.nodeId} does not support dev resources`);
        await node.deleteDevResourceAsync(String(p.url));
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { nodeId: node.id, ok: true } };
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
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("locked" in n)) { results.push({ nodeId: nid, error: "Node does not support locking" }); continue; }
        n.locked = locked;
        results.push({ nodeId: nid, locked: n.locked });
        await tick(nodeIds.length);
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "rotate_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("rotation" in n)) { results.push({ nodeId: nid, error: "Node does not support rotation" }); continue; }
        n.rotation = p.rotation;
        results.push({ nodeId: nid, rotation: n.rotation });
        await tick(nodeIds.length);
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
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
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
        await tick(nodeIds.length);
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
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("blendMode" in n)) { results.push({ nodeId: nid, error: "Node does not support blend mode" }); continue; }
        n.blendMode = p.blendMode;
        results.push({ nodeId: nid, blendMode: n.blendMode });
        await tick(nodeIds.length);
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_constraints": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
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
        await tick(nodeIds.length);
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
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
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
        await tick(nodeIds.length);
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "batch_rename_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
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
        await tick(nodeIds.length);
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
      // Never edit TEXT inside a component master — a master edit propagates to
      // every instance on the page (issue #33). Skip COMPONENT/COMPONENT_SET
      // subtrees unless the caller explicitly scoped the root to one. Editing TEXT
      // inside an INSTANCE stays a local override, so instances are safe to walk.
      const collect = (node: any, isRoot: boolean) => {
        if (!isRoot && (node.type === "COMPONENT" || node.type === "COMPONENT_SET")) {
          return;
        }
        if (node.type === "TEXT") textNodes.push(node);
        if ("children" in node) {
          (node.children as any[]).forEach((c) => collect(c, false));
        }
      };
      collect(root, true);
      const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
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
        await tick(textNodes.length);
      }
      figma.commitUndo();
      const successCount = results.filter((r: any) => !r.error).length;
      return { type: request.type, requestId: request.requestId, data: { replaced: successCount, results } };
    }

    default:
      return null;
  }
};
