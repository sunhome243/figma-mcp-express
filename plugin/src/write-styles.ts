import { makeSolidPaint, hexToRgb, bulkApply } from "./write-helpers";

// Native advanced effects (Figma 2025): GLASS, NOISE, TEXTURE. These have no shadow/blur
// geometry — we pass them through to node.effects / style.effects with sane defaults so a
// caller can request REAL frosted glass, grain, or texture instead of faking it with a
// background-blur + translucent-fill recipe. Field shapes follow @figma/plugin-typings.
const buildGlassEffect = (e: any): Effect => ({
  type: "GLASS",
  lightIntensity: Number(e.lightIntensity ?? 0.5), // 0–1, specular highlight brightness
  lightAngle: Number(e.lightAngle ?? 130),         // degrees, highlight direction
  refraction: Number(e.refraction ?? 0.3),         // 0–1, edge distortion
  depth: Number(e.depth ?? 10),                    // >= 1, how far the curved edge reaches in
  dispersion: Number(e.dispersion ?? 0.1),         // 0–1, chromatic split at edges
  radius: Number(e.radius ?? 12),                  // frost (background blur) radius
  visible: e.visible ?? true,
} as unknown as Effect);

const buildTextureEffect = (e: any): Effect => ({
  type: "TEXTURE",
  noiseSize: Number(e.noiseSize ?? 1),
  ...(e.noiseSizeVector != null ? { noiseSizeVector: e.noiseSizeVector } : {}),
  radius: Number(e.radius ?? 4),
  clipToShape: e.clipToShape ?? true,
  visible: e.visible ?? true,
} as unknown as Effect);

const buildNoiseEffect = (e: any): Effect => {
  const noiseType = e.noiseType || "MONOTONE"; // MONOTONE | DUOTONE | MULTITONE
  const { r, g, b, a } = hexToRgb(e.color || "#000000");
  const base: any = {
    type: "NOISE",
    noiseType,
    color: { r, g, b, a },
    blendMode: (e.blendMode || "NORMAL") as BlendMode,
    noiseSize: Number(e.noiseSize ?? 1),
    density: Number(e.density ?? 0.5),
    visible: e.visible ?? true,
  };
  if (e.noiseSizeVector != null) {
    base.noiseSizeVector = e.noiseSizeVector;
  }
  if (noiseType === "DUOTONE") {
    const s = hexToRgb(e.secondaryColor || "#FFFFFF");
    base.secondaryColor = { r: s.r, g: s.g, b: s.b, a: s.a };
  } else if (noiseType === "MULTITONE") {
    base.opacity = Number(e.opacity ?? 0.5);
  }
  return base as unknown as Effect;
};

// Returns a built advanced effect for GLASS/NOISE/TEXTURE, or null for the classic
// shadow/blur types (which the callers build inline).
const buildAdvancedEffect = (e: any): Effect | null => {
  switch (e.type) {
    case "GLASS": return buildGlassEffect(e);
    case "TEXTURE": return buildTextureEffect(e);
    case "NOISE": return buildNoiseEffect(e);
    default: return null;
  }
};

// LAYER_BLUR / BACKGROUND_BLUR — uniform (NORMAL) or PROGRESSIVE (gradual: blur ramps from
// startRadius at startOffset to radius at endOffset; offsets are normalized 0–1 in object space,
// so a top→bottom gradual blur is startOffset {0.5,0} → endOffset {0.5,1}). Pass blurType:"PROGRESSIVE"
// to opt in; otherwise a single uniform blur is built.
const buildBlurEffect = (e: any, eType: "LAYER_BLUR" | "BACKGROUND_BLUR"): Effect => {
  if (e.blurType === "PROGRESSIVE") {
    return {
      type: eType,
      blurType: "PROGRESSIVE",
      radius: Number(e.radius ?? e.endRadius ?? 8),
      startRadius: Number(e.startRadius ?? 0),
      startOffset: e.startOffset ?? { x: 0.5, y: 0 },
      endOffset: e.endOffset ?? { x: 0.5, y: 1 },
      visible: e.visible ?? true,
    } as unknown as Effect;
  }
  return { type: eType, blurType: "NORMAL", radius: Number(e.radius ?? 4), visible: e.visible ?? true } as BlurEffect;
};

const styleTypeDispatch = <T>(
  styleType: string,
  handlers: Record<string, () => T>,
): T => {
  const handler = handlers[styleType];
  if (!handler) throw new Error("styleType must be PAINT, TEXT, EFFECT, or GRID");
  return handler();
};

const assertStyleMatchesType = (style: BaseStyle, styleType: string) => {
  if (style.type !== styleType) {
    throw new Error(`styleType ${styleType} does not match style ${style.id} type ${style.type}`);
  }
};

export const handleWriteStyleRequest = async (request: any) => {
  switch (request.type) {
    case "create_paint_style": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      // Accept paints[] array (gradient/image/solid passthrough) OR color shorthand.
      if (!p.paints && !p.color) throw new Error("color or paints is required");
      const existing = (await figma.getLocalPaintStylesAsync()).find(s => s.name === p.name);
      if (existing) {
        return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
      }
      const style = figma.createPaintStyle();
      style.name = p.name;
      // paints[] takes precedence over the color shorthand.
      style.paints = Array.isArray(p.paints) ? p.paints : [makeSolidPaint(p.color)];
      if (p.description) style.description = p.description;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: style.id, name: style.name },
      };
    }

    case "create_text_style": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      const existing = (await figma.getLocalTextStylesAsync()).find(s => s.name === p.name);
      if (existing) {
        return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
      }
      const family = p.fontFamily || "Inter";
      const fontStyle = p.fontStyle || "Regular";
      await figma.loadFontAsync({ family, style: fontStyle });
      const style = figma.createTextStyle();
      style.name = p.name;
      style.fontName = { family, style: fontStyle };
      if (p.fontSize != null) style.fontSize = Number(p.fontSize);
      if (p.description) style.description = p.description;
      if (p.textDecoration && p.textDecoration !== "NONE") {
        style.textDecoration = p.textDecoration;
      }
      if (p.lineHeightValue != null) {
        style.lineHeight = { value: Number(p.lineHeightValue), unit: p.lineHeightUnit || "PIXELS" };
      }
      if (p.letterSpacingValue != null) {
        style.letterSpacing = { value: Number(p.letterSpacingValue), unit: p.letterSpacingUnit || "PIXELS" };
      }
      // Apply optional typographic properties not covered by the base font/size fields.
      if (p.textCase != null) style.textCase = p.textCase;
      if (p.paragraphSpacing != null) style.paragraphSpacing = Number(p.paragraphSpacing);
      if (p.paragraphIndent != null) style.paragraphIndent = Number(p.paragraphIndent);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: style.id, name: style.name },
      };
    }

    case "create_effect_style": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      const existing = (await figma.getLocalEffectStylesAsync()).find(s => s.name === p.name);
      if (existing) {
        return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
      }
      // Effect builder — shared with set_effects to keep reconstruction logic in one place.
      const buildEffect = (e: any): Effect => {
        const eType = e.type || "DROP_SHADOW";
        if (eType === "LAYER_BLUR" || eType === "BACKGROUND_BLUR") {
          // Uniform or PROGRESSIVE (gradual) blur — see buildBlurEffect.
          return buildBlurEffect(e, eType);
        } else if (eType === "GLASS" || eType === "NOISE" || eType === "TEXTURE") {
          return buildAdvancedEffect({ ...e, type: eType })!;
        } else {
          // DROP_SHADOW or INNER_SHADOW
          const { r, g, b, a } = hexToRgb(e.color || "#000000");
          const alpha = e.opacity != null ? Number(e.opacity) : (a !== 1 ? a : 0.25);
          const shadow: any = {
            type: eType as "DROP_SHADOW" | "INNER_SHADOW",
            color: { r, g, b, a: alpha },
            offset: { x: Number(e.offsetX ?? 0), y: Number(e.offsetY ?? 4) },
            radius: Number(e.radius ?? 8),
            spread: Number(e.spread ?? 0),
            visible: e.visible ?? true,
            blendMode: (e.blendMode || "NORMAL") as BlendMode,
          };
          // showShadowBehindNode is DROP_SHADOW-only in the Figma API.
          if (eType === "DROP_SHADOW") shadow.showShadowBehindNode = e.showShadowBehindNode ?? false;
          return shadow as DropShadowEffect;
        }
      };
      // Accept an effects[] array for multi-effect styles; fall back to treating the top-level
      // params as a single-effect shorthand for callers that pass only one effect.
      let effects: Effect[];
      if (Array.isArray(p.effects) && p.effects.length > 0) {
        effects = p.effects.map(buildEffect);
      } else {
        effects = [buildEffect(p)];
      }
      const style = figma.createEffectStyle();
      style.name = p.name;
      style.effects = effects;
      if (p.description) style.description = p.description;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: style.id, name: style.name },
      };
    }

    case "create_grid_style": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      const existing = (await figma.getLocalGridStylesAsync()).find(s => s.name === p.name);
      if (existing) {
        return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
      }
      const pattern = p.pattern || "GRID";
      let grid: LayoutGrid;
      if (pattern === "COLUMNS" || pattern === "ROWS") {
        grid = {
          pattern,
          count: Number(p.count ?? 12),
          gutterSize: Number(p.gutterSize ?? 16),
          offset: Number(p.offset ?? 0),
          alignment: p.alignment || "STRETCH",
          visible: true,
        };
      } else {
        // GRID
        const { r, g, b, a } = hexToRgb(p.color || "#FF0000");
        grid = {
          pattern: "GRID",
          sectionSize: Number(p.sectionSize ?? 8),
          visible: true,
          color: { r, g, b, a: p.opacity != null ? Number(p.opacity) : (a !== 1 ? a : 0.1) },
        };
      }
      const style = figma.createGridStyle();
      style.name = p.name;
      style.layoutGrids = [grid];
      if (p.description) style.description = p.description;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: style.id, name: style.name },
      };
    }

    case "update_paint_style": {
      const p = request.params || {};
      if (!p.styleId) throw new Error("styleId is required");
      const style = await figma.getStyleByIdAsync(p.styleId);
      if (!style) throw new Error(`Style not found: ${p.styleId}`);
      if (style.type !== "PAINT") throw new Error(`Style ${p.styleId} is not a paint style`);
      if (p.name) style.name = p.name;
      // Accept a full paints[] array (gradient/image/solid passthrough) or a color shorthand;
      // paints[] takes precedence so callers can supply any paint type, not just SOLID.
      if (Array.isArray(p.paints)) {
        (style as PaintStyle).paints = p.paints;
      } else if (p.color) {
        (style as PaintStyle).paints = [makeSolidPaint(p.color)];
      }
      if (p.description != null) style.description = p.description;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: style.id, name: style.name },
      };
    }

    case "delete_style": {
      const p = request.params || {};
      if (!p.styleId) throw new Error("styleId is required");
      const style = await figma.getStyleByIdAsync(p.styleId);
      if (!style) throw new Error(`Style not found: ${p.styleId}`);
      style.remove();
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { styleId: p.styleId, deleted: true },
      };
    }

    case "reorder_local_style": {
      const p = request.params || {};
      if (!p.styleType) throw new Error("styleType is required");
      if (!p.styleId) throw new Error("styleId is required");
      const target = await figma.getStyleByIdAsync(p.styleId);
      if (!target) throw new Error(`Style not found: ${p.styleId}`);
      const reference = p.afterStyleId ? await figma.getStyleByIdAsync(p.afterStyleId) : null;
      if (p.afterStyleId && !reference) throw new Error(`Style not found: ${p.afterStyleId}`);
      assertStyleMatchesType(target, p.styleType);
      if (reference) assertStyleMatchesType(reference, p.styleType);
      styleTypeDispatch(p.styleType, {
        PAINT: () => figma.moveLocalPaintStyleAfter(target as PaintStyle, reference as PaintStyle | null),
        TEXT: () => figma.moveLocalTextStyleAfter(target as TextStyle, reference as TextStyle | null),
        EFFECT: () => figma.moveLocalEffectStyleAfter(target as EffectStyle, reference as EffectStyle | null),
        GRID: () => figma.moveLocalGridStyleAfter(target as GridStyle, reference as GridStyle | null),
      });
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { styleId: target.id, afterStyleId: reference ? reference.id : null, styleType: p.styleType },
      };
    }

    case "reorder_local_style_folder": {
      const p = request.params || {};
      if (!p.styleType) throw new Error("styleType is required");
      if (!p.folder) throw new Error("folder is required");
      const reference = p.afterFolder == null || p.afterFolder === "" ? null : String(p.afterFolder);
      styleTypeDispatch(p.styleType, {
        PAINT: () => figma.moveLocalPaintFolderAfter(String(p.folder), reference),
        TEXT: () => figma.moveLocalTextFolderAfter(String(p.folder), reference),
        EFFECT: () => figma.moveLocalEffectFolderAfter(String(p.folder), reference),
        GRID: () => figma.moveLocalGridFolderAfter(String(p.folder), reference),
      });
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { folder: String(p.folder), afterFolder: reference, styleType: p.styleType },
      };
    }

    case "apply_style_to_node": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      if (!p.styleId) throw new Error("styleId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      const style = await figma.getStyleByIdAsync(p.styleId);
      if (!style) throw new Error(`Style not found: ${p.styleId}`);
      const n = node as any;
      switch (style.type) {
        case "PAINT": {
          const target = p.target || "fill";
          if (target === "stroke") {
            if (!("strokeStyleId" in node)) throw new Error(`Node ${nodeId} does not support stroke styles`);
            await n.setStrokeStyleIdAsync(p.styleId);
          } else {
            if (!("fillStyleId" in node)) throw new Error(`Node ${nodeId} does not support fill styles`);
            await n.setFillStyleIdAsync(p.styleId);
          }
          break;
        }
        case "TEXT":
          if (!("textStyleId" in node)) throw new Error(`Node ${nodeId} does not support text styles`);
          await n.setTextStyleIdAsync(p.styleId);
          break;
        case "EFFECT":
          if (!("effectStyleId" in node)) throw new Error(`Node ${nodeId} does not support effect styles`);
          await n.setEffectStyleIdAsync(p.styleId);
          break;
        case "GRID":
          if (!("gridStyleId" in node)) throw new Error(`Node ${nodeId} does not support grid styles`);
          await n.setGridStyleIdAsync(p.styleId);
          break;
        default:
          throw new Error(`Unknown style type: ${(style as any).type}`);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: n.id, name: n.name, styleId: p.styleId, styleType: style.type },
      };
    }

    case "set_effects": {
      const p = request.params || {};
      if ((request.nodeIds || []).length === 0) throw new Error("nodeIds is required");
      if (!Array.isArray(p.effects)) throw new Error("effects array is required");
      // Build the effects array once (shared across all target nodes).
      const effects: Effect[] = p.effects.map((e: any) => {
        switch (e.type) {
          case "DROP_SHADOW": {
            // showShadowBehindNode is only present on DROP_SHADOW (not INNER_SHADOW) in the
            // Figma plugin API, so it must be set here rather than in a shared path.
            const { r, g, b } = hexToRgb(e.color || "#000000");
            return {
              type: "DROP_SHADOW" as const,
              color: { r, g, b, a: e.opacity != null ? Number(e.opacity) : 0.25 },
              offset: { x: Number(e.offsetX ?? 0), y: Number(e.offsetY ?? 4) },
              radius: Number(e.radius ?? 4),
              spread: Number(e.spread ?? 0),
              visible: e.visible ?? true,
              blendMode: (e.blendMode || "NORMAL") as BlendMode,
              showShadowBehindNode: e.showShadowBehindNode ?? false,
            } as DropShadowEffect;
          }
          case "INNER_SHADOW": {
            // Note: INNER_SHADOW does NOT have showShadowBehindNode in Figma typings.
            const { r, g, b } = hexToRgb(e.color || "#000000");
            return {
              type: "INNER_SHADOW" as const,
              color: { r, g, b, a: e.opacity != null ? Number(e.opacity) : 0.25 },
              offset: { x: Number(e.offsetX ?? 0), y: Number(e.offsetY ?? 4) },
              radius: Number(e.radius ?? 4),
              spread: Number(e.spread ?? 0),
              visible: e.visible ?? true,
              blendMode: (e.blendMode || "NORMAL") as BlendMode,
            } as InnerShadowEffect;
          }
          case "LAYER_BLUR":
          case "BACKGROUND_BLUR":
            // Uniform (NORMAL) or PROGRESSIVE (gradual) blur — see buildBlurEffect.
            return buildBlurEffect(e, e.type);
          case "GLASS":
          case "NOISE":
          case "TEXTURE":
            // Native advanced effects — passed through with defaults (see buildAdvancedEffect).
            return buildAdvancedEffect(e)!;
          default:
            throw new Error(`Unknown effect type: ${e.type}. Must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, BACKGROUND_BLUR, GLASS, NOISE, or TEXTURE`);
        }
      });
      // Use bulkApply so the same effects array is written to every nodeId in the request,
      // collecting per-node errors rather than aborting on the first failure.
      return bulkApply(request, (node, nid) => {
        if (!("effects" in node)) throw new Error(`Node ${nid} does not support effects`);
        node.effects = effects;
        return { effectCount: effects.length };
      });
    }

    case "bind_variable_to_node": {
      const p = request.params || {};
      // Shared lookup resolved ONCE (request-level throw): the same variable is
      // bound to the same field on every node. Per-node failures (field
      // unsupported on this node) are collected so a bulk bind doesn't abort.
      if ((request.nodeIds || []).length === 0) throw new Error("nodeIds is required");
      if (!p.variableId) throw new Error("variableId is required");
      if (!p.field) throw new Error("field is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);

      // Allowlist of every bindable node field and its expected variable type, derived from
      // Figma's `VariableBindableNodeField` union (plugin-api.d.ts). fillColor/strokeColor
      // are paint-binding special cases handled via setBoundVariableForPaint. Fields absent
      // from this map (cornerRadius, rotation, x, y) are NOT bindable — setBoundVariable
      // would throw on the real API, so we reject them up-front with a helpful message.
      const FIELD_EXPECTED_TYPE: Record<string, string> = {
        fillColor: "COLOR", strokeColor: "COLOR",
        visible: "BOOLEAN",
        characters: "STRING",
        width: "FLOAT", height: "FLOAT",
        minWidth: "FLOAT", maxWidth: "FLOAT", minHeight: "FLOAT", maxHeight: "FLOAT",
        itemSpacing: "FLOAT", counterAxisSpacing: "FLOAT",
        gridRowGap: "FLOAT", gridColumnGap: "FLOAT",
        paddingTop: "FLOAT", paddingRight: "FLOAT", paddingBottom: "FLOAT", paddingLeft: "FLOAT",
        topLeftRadius: "FLOAT", topRightRadius: "FLOAT",
        bottomLeftRadius: "FLOAT", bottomRightRadius: "FLOAT",
        strokeWeight: "FLOAT",
        strokeTopWeight: "FLOAT", strokeRightWeight: "FLOAT",
        strokeBottomWeight: "FLOAT", strokeLeftWeight: "FLOAT",
        opacity: "FLOAT",
      };
      // Request-level reject: a non-bindable field (cornerRadius/rotation/x/y/typo) would
      // throw at setBoundVariable on the real API — fail fast with a helpful message.
      if (!(p.field in FIELD_EXPECTED_TYPE)) {
        throw new Error(
          `Field '${p.field}' is not a bindable node field. Bindable: ${Object.keys(FIELD_EXPECTED_TYPE).join(", ")}. ` +
          `(For corner radius use topLeftRadius/topRightRadius/bottomLeftRadius/bottomRightRadius — 'cornerRadius' is not bindable.)`
        );
      }
      const expectedType = FIELD_EXPECTED_TYPE[p.field];

      return bulkApply(request, (node, nid) => {
        // Type-compatibility guard: reject a COLOR variable on a FLOAT field (etc.) before
        // the API call, which would otherwise accept it and produce a silent bad binding.
        if (expectedType && variable.resolvedType && variable.resolvedType !== expectedType) {
          throw new Error(
            `Variable type mismatch for field '${p.field}': expected ${expectedType}, got ${variable.resolvedType}`
          );
        }

        if (p.field === "fillColor") {
          if (!("fills" in node)) throw new Error(`Node ${nid} does not support fills`);
          const fills = [...(node.fills as Paint[])];
          const base = fills.length > 0 ? fills[0] : makeSolidPaint("#000000");
          // Guard: setBoundVariableForPaint only works on a SOLID base paint — gradient and
          // image paints have no "color" binding point and the API throws without this check.
          if (fills.length > 0 && (fills[0] as any).type !== "SOLID") {
            throw new Error(
              `Node ${nid} fills[0] is not SOLID (type=${(fills[0] as any).type}); setBoundVariableForPaint requires a SOLID base paint`
            );
          }
          const bound = figma.variables.setBoundVariableForPaint(base as SolidPaint, "color", variable);
          // Bind only paint index 0 and keep the rest: setBoundVariableForPaint replaces a
          // single SOLID paint, so collapsing the array would silently drop any overlay fills.
          node.fills = [bound, ...fills.slice(1)];
        } else if (p.field === "strokeColor") {
          if (!("strokes" in node)) throw new Error(`Node ${nid} does not support strokes`);
          const strokes = [...(node.strokes as Paint[])];
          const base = strokes.length > 0 ? strokes[0] : makeSolidPaint("#000000");
          // Same SOLID guard for strokes — setBoundVariableForPaint requires a SOLID base.
          if (strokes.length > 0 && (strokes[0] as any).type !== "SOLID") {
            throw new Error(
              `Node ${nid} strokes[0] is not SOLID (type=${(strokes[0] as any).type}); setBoundVariableForPaint requires a SOLID base paint`
            );
          }
          const bound = figma.variables.setBoundVariableForPaint(base as SolidPaint, "color", variable);
          // Bind only stroke index 0 and preserve the rest, same rationale as fills above.
          node.strokes = [bound, ...strokes.slice(1)];
        } else {
          if (!(p.field in node)) throw new Error(`Node ${nid} does not have field: ${p.field}`);
          node.setBoundVariable(p.field, variable);
        }
        return { name: node.name, variableId: p.variableId, field: p.field };
      });
    }

    case "bind_variable_to_effect": {
      const p = request.params || {};
      if (!p.effect || typeof p.effect !== "object") throw new Error("effect is required");
      if (!p.field) throw new Error("field is required");
      if (!p.variableId) throw new Error("variableId is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      const effect = figma.variables.setBoundVariableForEffect(p.effect as Effect, p.field, variable);
      return { type: request.type, requestId: request.requestId, data: { effect, field: p.field, variableId: variable.id } };
    }

    case "bind_variable_to_layout_grid": {
      const p = request.params || {};
      if (!p.layoutGrid || typeof p.layoutGrid !== "object") throw new Error("layoutGrid is required");
      if (!p.field) throw new Error("field is required");
      if (!p.variableId) throw new Error("variableId is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      const layoutGrid = figma.variables.setBoundVariableForLayoutGrid(p.layoutGrid as LayoutGrid, p.field, variable);
      return { type: request.type, requestId: request.requestId, data: { layoutGrid, field: p.field, variableId: variable.id } };
    }

    default:
      return null;
  }
};
