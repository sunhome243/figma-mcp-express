import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, base64ToBytes, applyAutoLayout, applyTextStyleProps, applyLayoutSizing } from "./write-helpers";

export const handleWriteCreateRequest = async (request: any) => {
  switch (request.type) {
    case "create_frame": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const frame = figma.createFrame();
      frame.resize(p.width || 100, p.height || 100);
      frame.x = p.x != null ? p.x : 0;
      frame.y = p.y != null ? p.y : 0;
      if (p.name) frame.name = p.name;
      if (p.fillColor) frame.fills = [makeSolidPaint(p.fillColor)];
      // Apply optional frame-level properties (cornerRadius, clipsContent, opacity).
      if (p.cornerRadius != null) frame.cornerRadius = p.cornerRadius;
      if (p.clipsContent != null) frame.clipsContent = p.clipsContent;
      if (p.opacity != null) frame.opacity = p.opacity;
      await applyAutoLayout(frame, p);
      (parent as any).appendChild(frame);
      // Sizing-within-parent (FILL/HUG) — only valid once parented to an auto-layout
      // frame, hence after appendChild. Surfaced as a clear error if the parent isn't auto-layout.
      try {
        applyLayoutSizing(frame, p);
      } catch (e) {
        throw new Error(`create_frame: layoutSizing requires an auto-layout parent: ${String(e)}`);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: frame.id, name: frame.name, type: frame.type, bounds: getBounds(frame) },
      };
    }

    case "create_rectangle": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const rect = figma.createRectangle();
      rect.resize(p.width || 100, p.height || 100);
      rect.x = p.x != null ? p.x : 0;
      rect.y = p.y != null ? p.y : 0;
      if (p.name) rect.name = p.name;
      if (p.fillColor) rect.fills = [makeSolidPaint(p.fillColor)];
      // Uniform corner radius (shorthand) — applied first so per-corner overrides win.
      if (p.cornerRadius != null) rect.cornerRadius = p.cornerRadius;
      // Per-corner radii — applied after the uniform shorthand so they take precedence
      // when only some corners need a distinct radius.
      if (p.topLeftRadius != null) rect.topLeftRadius = p.topLeftRadius;
      if (p.topRightRadius != null) rect.topRightRadius = p.topRightRadius;
      if (p.bottomLeftRadius != null) rect.bottomLeftRadius = p.bottomLeftRadius;
      if (p.bottomRightRadius != null) rect.bottomRightRadius = p.bottomRightRadius;
      (parent as any).appendChild(rect);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
      };
    }

    case "create_ellipse": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const ellipse = figma.createEllipse();
      ellipse.resize(p.width || 100, p.height || 100);
      ellipse.x = p.x != null ? p.x : 0;
      ellipse.y = p.y != null ? p.y : 0;
      if (p.name) ellipse.name = p.name;
      if (p.fillColor) ellipse.fills = [makeSolidPaint(p.fillColor)];
      (parent as any).appendChild(ellipse);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: ellipse.id, name: ellipse.name, type: ellipse.type, bounds: getBounds(ellipse) },
      };
    }

    case "create_text": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const fontFamily = p.fontFamily || "Inter";
      const fontStyle = p.fontStyle || "Regular";
      await figma.loadFontAsync({ family: fontFamily, style: fontStyle });
      const textNode = figma.createText();
      textNode.fontName = { family: fontFamily, style: fontStyle };
      if (p.fontSize != null) textNode.fontSize = Number(p.fontSize);
      textNode.characters = p.text || "";
      textNode.x = p.x != null ? p.x : 0;
      textNode.y = p.y != null ? p.y : 0;
      if (p.name) textNode.name = p.name;
      if (p.fillColor) textNode.fills = [makeSolidPaint(p.fillColor)];
      // Optional styling at creation (alignment / auto-resize / spacing / case / decoration).
      // The chosen font is already loaded above; applyTextStyleProps only reloads on a font change.
      await applyTextStyleProps(textNode, p);
      (parent as any).appendChild(textNode);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: textNode.id, name: textNode.name, type: textNode.type, bounds: getBounds(textNode) },
      };
    }

    case "import_image": {
      const p = request.params || {};
      if (!p.imageData) throw new Error("imageData (base64) is required");
      // Validate scaleMode against the Figma ImagePaint union before use — the API
      // silently accepts an invalid string and produces a broken paint.
      const VALID_SCALE_MODES = ["FILL", "FIT", "CROP", "TILE"];
      const scaleMode: string = p.scaleMode || "FILL";
      if (!VALID_SCALE_MODES.includes(scaleMode)) {
        throw new Error(`Invalid scaleMode "${scaleMode}". Must be one of: ${VALID_SCALE_MODES.join(", ")}`);
      }
      const parent = await getParentNode(p.parentId);
      const bytes = base64ToBytes(p.imageData);
      const image = figma.createImage(bytes);
      const rect = figma.createRectangle();
      rect.resize(p.width || 200, p.height || 200);
      rect.x = p.x != null ? p.x : 0;
      rect.y = p.y != null ? p.y : 0;
      if (p.name) rect.name = p.name;
      rect.fills = [{ type: "IMAGE", imageHash: image.hash, scaleMode: scaleMode as ImagePaint["scaleMode"] }];
      (parent as any).appendChild(rect);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
      };
    }

    case "create_component": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type === "COMPONENT" || node.type === "COMPONENT_SET") {
        throw new Error(`Node ${nodeId} is already a ${node.type}`);
      }
      if (node.type === "INSTANCE") {
        throw new Error(`Node ${nodeId} is an INSTANCE — detach it first, or convert its main component`);
      }

      // Native in-place conversion: preserves ALL properties, bound variables (tokens),
      // effects, strokes, rotation, constraints, and children — and works on frames,
      // groups, and shapes alike. A hand-rolled createComponent()+copy silently drops
      // everything not explicitly copied (notably boundVariables → broken token bindings).
      let component: any;
      try {
        component = figma.createComponentFromNode(node);
      } catch (e: any) {
        throw new Error(`Cannot convert ${node.type} ${nodeId} to a component: ${e?.message ?? e}`);
      }
      if (p.name) component.name = p.name;

      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: component.id, name: component.name, type: component.type, bounds: getBounds(component) },
      };
    }

    case "create_section": {
      const p = request.params || {};
      // Resolve the target parent (throws if parentId is provided but invalid or not a container).
      const parent = await getParentNode(p.parentId);
      const section = figma.createSection();
      if (p.name) section.name = p.name;
      if (p.x != null) section.x = p.x;
      if (p.y != null) section.y = p.y;
      if (p.width != null || p.height != null) {
        section.resizeWithoutConstraints(p.width || section.width, p.height || section.height);
      }
      (parent as any).appendChild(section);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: section.id, name: section.name, type: section.type, bounds: getBounds(section) },
      };
    }

    default:
      return null;
  }
};
