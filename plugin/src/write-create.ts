import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, base64ToBytes, applyAutoLayout, applyTextStyleProps, applyLayoutSizing } from "./write-helpers";

const isFontName = (font: unknown): font is FontName => {
  return !!font && typeof (font as any).family === "string" && typeof (font as any).style === "string";
};

const tableCellFontKey = (font: FontName) => `${font.family}\0${font.style}`;

const loadFontsForTableCells = async (table: TableNode, cells: any[], numRows: number, numColumns: number) => {
  const loaded = new Set<string>();
  for (let r = 0; r < cells.length && r < numRows; r++) {
    const row = cells[r];
    if (!Array.isArray(row)) continue;
    for (let c = 0; c < row.length && c < numColumns; c++) {
      if (row[c] == null) continue;
      const fontName = table.cellAt(r, c).text.fontName;
      if (!isFontName(fontName)) {
        throw new Error(`create_table: cell ${r},${c} has mixed fontName and cannot be edited safely`);
      }
      const key = tableCellFontKey(fontName);
      if (!loaded.has(key)) {
        await figma.loadFontAsync(fontName);
        loaded.add(key);
      }
    }
  }
};

const validPaintScaleModes = ["FILL", "FIT", "CROP", "TILE"];
const textPathSourceTypes = ["VECTOR", "RECTANGLE", "ELLIPSE", "POLYGON", "STAR", "LINE"];

const validatePaintScaleMode = (scaleMode: string) => {
  if (!validPaintScaleModes.includes(scaleMode)) {
    throw new Error(`Invalid scaleMode "${scaleMode}". Must be one of: ${validPaintScaleModes.join(", ")}`);
  }
};

const isValidPageDividerName = (name: string) => /^(?:\*+|-+| +|\u2013+|\u2014+)$/.test(name);

const finiteNumberField = (field: string, value: unknown): number => {
  const numberValue = Number(value);
  if (!Number.isFinite(numberValue)) {
    throw new Error(`${field} must be a finite number`);
  }
  return numberValue;
};

const validateMediaTransform = (field: "imageTransform" | "videoTransform", value: unknown): Transform => {
  if (!Array.isArray(value) || value.length !== 2) {
    throw new Error(`${field} must be a 2x3 numeric matrix`);
  }
  return value.map((row, rowIndex) => {
    if (!Array.isArray(row) || row.length !== 3) {
      throw new Error(`${field} must be a 2x3 numeric matrix`);
    }
    return row.map((cell, columnIndex) => finiteNumberField(`${field}[${rowIndex}][${columnIndex}]`, cell));
  }) as Transform;
};

const applyMediaPaintFields = (paint: any, p: any, transformField: "imageTransform" | "videoTransform") => {
  const next: any = {};
  if (p.rotation != null) next.rotation = finiteNumberField("rotation", p.rotation);
  if (p.scalingFactor != null) {
    const scalingFactor = finiteNumberField("scalingFactor", p.scalingFactor);
    if (scalingFactor <= 0) throw new Error("scalingFactor must be positive");
    next.scalingFactor = scalingFactor;
  }
  if (p[transformField] != null) next[transformField] = validateMediaTransform(transformField, p[transformField]);
  const FILTER_KEYS = ["exposure", "contrast", "saturation", "temperature", "tint", "highlights", "shadows"];
  const filters: any = {};
  for (const k of FILTER_KEYS) {
    if (p[k] != null) {
      const filterValue = finiteNumberField(k, p[k]);
      if (filterValue < -1 || filterValue > 1) {
        throw new Error(`${k} must be between -1 and 1`);
      }
      filters[k] = filterValue;
    }
  }
  Object.assign(paint, next);
  if (Object.keys(filters).length > 0) paint.filters = filters;
};
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
      if (!p.imageData && !p.imageUrl) throw new Error("imageData (base64) or imageUrl is required");
      // Validate scaleMode against the Figma ImagePaint union before use — the API
      // silently accepts an invalid string and produces a broken paint.
      const scaleMode: string = p.scaleMode || "FILL";
      validatePaintScaleMode(scaleMode);
      const parent = await getParentNode(p.parentId);
      let image: Image;
      if (p.imageUrl) {
        if (typeof figma.createImageAsync !== "function") {
          throw new Error("createImageAsync is unavailable in this Figma host");
        }
        image = await figma.createImageAsync(String(p.imageUrl));
      } else {
        image = figma.createImage(base64ToBytes(p.imageData));
      }
      const rect = figma.createRectangle();
      rect.resize(p.width || 200, p.height || 200);
      rect.x = p.x != null ? p.x : 0;
      rect.y = p.y != null ? p.y : 0;
      if (p.name) rect.name = p.name;
      const paint: any = { type: "IMAGE", imageHash: image.hash, scaleMode };
      // Optional ImagePaint fields — scaleMode-gated by Figma (rotation: FILL/FIT/TILE,
      // imageTransform: CROP, scalingFactor: TILE). We pass through; Figma enforces the rules.
      // ImageFilters are built only from explicitly-provided fields so we never
      // send unintended zeros (every filter defaults to 0).
      applyMediaPaintFields(paint, p, "imageTransform");
      rect.fills = [paint as ImagePaint];
      (parent as any).appendChild(rect);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
      };
    }

    case "create_video": {
      const p = request.params || {};
      if (!p.videoData) throw new Error("videoData (base64) is required");
      if (typeof figma.createVideoAsync !== "function") {
        throw new Error("createVideoAsync is unavailable in this Figma host");
      }
      const scaleMode: string = p.scaleMode || "FILL";
      validatePaintScaleMode(scaleMode);
      const parent = await getParentNode(p.parentId);
      const video = await figma.createVideoAsync(base64ToBytes(p.videoData));
      const rect = figma.createRectangle();
      rect.resize(p.width || 200, p.height || 200);
      rect.x = p.x != null ? p.x : 0;
      rect.y = p.y != null ? p.y : 0;
      if (p.name) rect.name = p.name;
      const paint: any = { type: "VIDEO", videoHash: video.hash, scaleMode };
      applyMediaPaintFields(paint, p, "videoTransform");
      rect.fills = [paint as VideoPaint];
      (parent as any).appendChild(rect);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
      };
    }

    case "create_gif": {
      const p = request.params || {};
      if (!p.imageHash) throw new Error("imageHash is required");
      if (typeof figma.createGif !== "function") {
        throw new Error("createGif is unavailable in this Figma host (FigJam-only API)");
      }
      const parent = await getParentNode(p.parentId);
      const media = figma.createGif(String(p.imageHash));
      media.x = p.x != null ? p.x : 0;
      media.y = p.y != null ? p.y : 0;
      if (p.name) media.name = p.name;
      if (p.width != null || p.height != null) {
        media.resize(p.width || media.width, p.height || media.height);
      }
      (parent as any).appendChild(media);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: media.id, name: media.name, type: media.type, bounds: getBounds(media) },
      };
    }

    case "create_link_preview": {
      const p = request.params || {};
      if (!p.url) throw new Error("url is required");
      if (typeof figma.createLinkPreviewAsync !== "function") {
        throw new Error("createLinkPreviewAsync is unavailable in this Figma host (FigJam-only API)");
      }
      const parent = await getParentNode(p.parentId);
      const node = await figma.createLinkPreviewAsync(String(p.url));
      node.x = p.x != null ? p.x : 0;
      node.y = p.y != null ? p.y : 0;
      if (p.name) node.name = p.name;
      (parent as any).appendChild(node);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name, type: node.type, bounds: getBounds(node) },
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

    case "create_vector": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const vector = figma.createVector();
      vector.resize(p.width || 100, p.height || 100);
      vector.x = p.x != null ? p.x : 0;
      vector.y = p.y != null ? p.y : 0;
      if (p.name) vector.name = p.name;
      if (Array.isArray(p.vectorPaths)) vector.vectorPaths = p.vectorPaths;
      if (p.fillColor) vector.fills = [makeSolidPaint(p.fillColor)];
      (parent as any).appendChild(vector);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: vector.id, name: vector.name, type: vector.type, bounds: getBounds(vector) },
      };
    }

    case "create_slice": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const slice = figma.createSlice();
      slice.resize(p.width || 100, p.height || 100);
      slice.x = p.x != null ? p.x : 0;
      slice.y = p.y != null ? p.y : 0;
      if (p.name) slice.name = p.name;
      (parent as any).appendChild(slice);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: slice.id, name: slice.name, type: slice.type, bounds: getBounds(slice) },
      };
    }

    case "create_page_divider": {
      const p = request.params || {};
      if (typeof figma.createPageDivider !== "function") {
        throw new Error("createPageDivider is unavailable in this Figma host");
      }
      if (p.name != null && !isValidPageDividerName(String(p.name))) {
        throw new Error("name must be all asterisks, hyphens, spaces, en dashes, or em dashes");
      }
      const divider = figma.createPageDivider(p.name != null ? String(p.name) : undefined);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: divider.id, name: divider.name, type: divider.type },
      };
    }

    case "create_text_path": {
      const p = request.params || {};
      if (!p.nodeId) throw new Error("nodeId is required");
      if (typeof figma.createTextPath !== "function") {
        throw new Error("createTextPath is unavailable in this Figma host");
      }
      const node = await figma.getNodeByIdAsync(String(p.nodeId)) as any;
      if (!node) throw new Error(`Node not found: ${p.nodeId}`);
      if (!textPathSourceTypes.includes(node.type)) {
        throw new Error(`Node ${p.nodeId} is not a vector-like node (${textPathSourceTypes.join(", ")})`);
      }
      const startSegment = Number(p.startSegment ?? 0);
      const startPosition = Number(p.startPosition ?? 0);
      if (!Number.isInteger(startSegment) || startSegment < 0) throw new Error("startSegment must be a non-negative integer");
      if (!Number.isFinite(startPosition) || startPosition < 0 || startPosition > 1) {
        throw new Error("startPosition must be a number between 0 and 1");
      }
      const textPath = figma.createTextPath(node, startSegment, startPosition);
      if (p.name) textPath.name = p.name;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: textPath.id, name: textPath.name, type: textPath.type, bounds: getBounds(textPath) },
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

    case "create_line": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const line = figma.createLine();
      const length = p.length != null ? Number(p.length) : 100;
      // A LineNode is always 0-height; length is set via the width dimension.
      line.resize(length, 0);
      line.x = p.x != null ? p.x : 0;
      line.y = p.y != null ? p.y : 0;
      if (p.name) line.name = p.name;
      // Lines render nothing without a stroke — default to a visible 1px black stroke.
      line.strokes = [makeSolidPaint(p.strokeColor || "#000000")];
      line.strokeWeight = p.strokeWeight != null ? Number(p.strokeWeight) : 1;
      if (p.strokeCap != null) line.strokeCap = p.strokeCap;
      if (p.rotation != null) line.rotation = Number(p.rotation);
      (parent as any).appendChild(line);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: line.id, name: line.name, type: line.type, bounds: getBounds(line) },
      };
    }

    case "create_polygon": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const polygon = figma.createPolygon();
      polygon.resize(p.width || 100, p.height || 100);
      polygon.x = p.x != null ? p.x : 0;
      polygon.y = p.y != null ? p.y : 0;
      if (p.name) polygon.name = p.name;
      // pointCount must be >= 3 (triangle). Figma clamps lower values but we guard for clarity.
      if (p.pointCount != null) polygon.pointCount = Math.max(3, Math.round(Number(p.pointCount)));
      if (p.fillColor) polygon.fills = [makeSolidPaint(p.fillColor)];
      (parent as any).appendChild(polygon);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: polygon.id, name: polygon.name, type: polygon.type, bounds: getBounds(polygon) },
      };
    }

    case "create_star": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const star = figma.createStar();
      star.resize(p.width || 100, p.height || 100);
      star.x = p.x != null ? p.x : 0;
      star.y = p.y != null ? p.y : 0;
      if (p.name) star.name = p.name;
      if (p.pointCount != null) star.pointCount = Math.max(3, Math.round(Number(p.pointCount)));
      // innerRadius is a 0–1 ratio of the outer radius; clamp to the valid range.
      if (p.innerRadius != null) star.innerRadius = Math.min(1, Math.max(0, Number(p.innerRadius)));
      if (p.fillColor) star.fills = [makeSolidPaint(p.fillColor)];
      (parent as any).appendChild(star);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: star.id, name: star.name, type: star.type, bounds: getBounds(star) },
      };
    }

    case "import_svg": {
      const p = request.params || {};
      if (!p.svg || typeof p.svg !== "string") throw new Error("svg (raw SVG markup string) is required");
      const parent = await getParentNode(p.parentId);
      let frame: FrameNode;
      try {
        frame = figma.createNodeFromSvg(p.svg);
      } catch (e: any) {
        throw new Error(`Invalid SVG markup: ${e?.message ?? e}`);
      }
      frame.x = p.x != null ? p.x : 0;
      frame.y = p.y != null ? p.y : 0;
      if (p.name) frame.name = p.name;
      (parent as any).appendChild(frame);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: frame.id, name: frame.name, type: frame.type, bounds: getBounds(frame) },
      };
    }

    case "create_table": {
      const p = request.params || {};
      const numRows = p.numRows != null ? Math.max(1, Math.round(Number(p.numRows))) : 2;
      const numColumns = p.numColumns != null ? Math.max(1, Math.round(Number(p.numColumns))) : 2;
      const parent = await getParentNode(p.parentId);
      let table: TableNode;
      try {
        table = figma.createTable(numRows, numColumns);
      } catch (e: any) {
        throw new Error(`createTable failed (tables may be unavailable in this editor): ${e?.message ?? e}`);
      }
      table.x = p.x != null ? p.x : 0;
      table.y = p.y != null ? p.y : 0;
      if (p.name) table.name = p.name;
      // Optional cell text: a 2D array [rowIndex][colIndex] of strings. The cell's text
      // sublayer font must be loaded before assigning characters.
      if (Array.isArray(p.cells)) {
        await loadFontsForTableCells(table, p.cells, numRows, numColumns);
        for (let r = 0; r < p.cells.length && r < numRows; r++) {
          const row = p.cells[r];
          if (!Array.isArray(row)) continue;
          for (let c = 0; c < row.length && c < numColumns; c++) {
            if (row[c] != null) table.cellAt(r, c).text.characters = String(row[c]);
          }
        }
      }
      (parent as any).appendChild(table);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: table.id, name: table.name, type: table.type, numRows: table.numRows, numColumns: table.numColumns, bounds: getBounds(table) },
      };
    }

    default:
      return null;
  }
};
