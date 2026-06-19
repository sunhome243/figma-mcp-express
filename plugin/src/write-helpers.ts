// Write helpers — utilities used exclusively by write handlers.

import { makeProgress } from "./progress";

// Tick cadence for bulk-write loops. Writes are heavier than read-walk visits, so
// tick more often than the 800-node read default — a progress_update every 50
// mutations keeps the Go-bridge inactivity timer alive on a large single-op array
// without meaningfully slowing the common small bulk apply.
export const WRITE_PROGRESS_EVERY = 50;

// bulkApply is the shared "all→all" loop for setters that should apply to EVERY
// id in request.nodeIds and report a STRUCTURED per-node result instead of
// aborting on the first failure. It is the plugin-side half of the bulk-apply
// primitive (a scan op fans its matched ids into one setter via "$0.nodes[*].id").
//
// Error split:
//   • request-level (empty nodeIds, missing required params, shared-lookup
//     failures) → the CALLER throws BEFORE calling bulkApply, as a single error.
//   • per-node (node not found, wrong type, this-op-can't-apply) → COLLECTED into
//     results[i].error so one bad id never aborts the rest.
//
// `apply(node, nodeId)` runs the mutation for one already-fetched node and returns
// the success fields for that entry (merged with { nodeId }); it may throw to
// record a per-node error. A single-id call is just a 1-element loop — identical
// Figma mutation, same { results:[…] } envelope the multi-node setters already use.
export const bulkApply = async (
  request: any,
  apply: (node: any, nodeId: string) => any | Promise<any>,
): Promise<any> => {
  const nodeIds: string[] = request.nodeIds || [];
  if (nodeIds.length === 0) throw new Error("nodeIds is required");
  const tick = makeProgress(request.requestId, request.type, WRITE_PROGRESS_EVERY);
  const results: any[] = [];
  // Prefetch every node in parallel before the mutation loop. getNodeByIdAsync
  // lazy-loads the node's page under documentAccess:"dynamic-page", so a serial
  // await-per-node paid that latency N times back-to-back; Promise.all overlaps
  // the loads (the pattern group_nodes already uses for its fetches). Mutations
  // stay SEQUENTIAL below so per-node error attribution, result order, and the
  // single trailing commitUndo are unchanged. (For nodes already on the loaded
  // page each fetch resolves on a microtask, so this is a no-op there — the win
  // is on cross-page / not-yet-loaded targets.)
  const fetched = (await Promise.all(
    nodeIds.map((nid) => figma.getNodeByIdAsync(nid)),
  )) as any[];
  for (let i = 0; i < nodeIds.length; i++) {
    const nid = nodeIds[i];
    const n = fetched[i];
    if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
    try {
      const fields = await apply(n, nid);
      results.push({ nodeId: nid, ...(fields || {}) });
    } catch (e) {
      results.push({ nodeId: nid, error: e instanceof Error ? e.message : String(e) });
    }
    await tick(nodeIds.length);
  }
  figma.commitUndo();
  return { type: request.type, requestId: request.requestId, data: { results } };
};

export const hexToRgb = (hex: string) => {
  const clean = hex.replace("#", "");
  return {
    r: parseInt(clean.slice(0, 2), 16) / 255,
    g: parseInt(clean.slice(2, 4), 16) / 255,
    b: parseInt(clean.slice(4, 6), 16) / 255,
    a: clean.length >= 8 ? parseInt(clean.slice(6, 8), 16) / 255 : 1,
  };
};

export const makeSolidPaint = (colorInput: any, opacityOverride?: number): SolidPaint => {
  const { r, g, b, a } = typeof colorInput === "string"
    ? hexToRgb(colorInput)
    : { r: colorInput.r, g: colorInput.g, b: colorInput.b, a: colorInput.a != null ? colorInput.a : 1 };
  const eff = opacityOverride != null ? opacityOverride : a;
  const paint: any = { type: "SOLID", color: { r, g, b } };
  if (eff !== 1) paint.opacity = eff;
  return paint;
};

export const getParentNode = async (parentId: string | undefined) => {
  if (!parentId) return figma.currentPage;
  const parent = await figma.getNodeByIdAsync(parentId);
  if (!parent) throw new Error(`Parent node not found: ${parentId}`);
  if (!("appendChild" in parent)) throw new Error(`Node ${parentId} cannot have children`);
  return parent as ChildrenMixin & BaseNode;
};

// Helper: resolve a variable by ID and bind it to a frame field.
// Silently skips when the variable cannot be found (no hard error — caller may pass an id
// that belongs to a future-creation flow; the plain-number fallback covers that case).
const bindSpacingVariable = async (frame: any, field: string, variableId: string) => {
  const variable = await figma.variables.getVariableByIdAsync(variableId);
  if (variable) frame.setBoundVariable(field, variable);
};

// applyAutoLayout is async so it can optionally bind spacing variables instead of writing
// raw integers. When a *VariableId param is present the variable is resolved via
// figma.variables.getVariableByIdAsync and applied with frame.setBoundVariable(); callers
// that pass only plain numbers follow the synchronous path and need not await.
//
// Optional variable params:
//   paddingTopVariableId / paddingRightVariableId / paddingBottomVariableId / paddingLeftVariableId
//   itemSpacingVariableId
//
// NaN guard: raw numeric fields are only written when Number(value) is finite, preventing
// a stray variableId string (e.g. "variableId:123") from silently coercing to NaN.
export const applyAutoLayout = async (frame: FrameNode, p: any): Promise<void> => {
  if (p.layoutMode != null) frame.layoutMode = p.layoutMode;
  // Raw padding/spacing — only write when finite to guard against stray non-numeric values.
  const n = (v: any) => { const x = Number(v); return Number.isFinite(x) ? x : null; };
  if (p.paddingTop != null) { const v = n(p.paddingTop); if (v !== null) frame.paddingTop = v; }
  if (p.paddingRight != null) { const v = n(p.paddingRight); if (v !== null) frame.paddingRight = v; }
  if (p.paddingBottom != null) { const v = n(p.paddingBottom); if (v !== null) frame.paddingBottom = v; }
  if (p.paddingLeft != null) { const v = n(p.paddingLeft); if (v !== null) frame.paddingLeft = v; }
  // itemSpacing is flex-only — GRID uses gridRowGap/gridColumnGap instead, and writing
  // itemSpacing in GRID mode is out-of-mode. Gate it (padding IS valid in GRID).
  if (p.itemSpacing != null && frame.layoutMode !== "GRID") { const v = n(p.itemSpacing); if (v !== null) frame.itemSpacing = v; }
  // Variable bindings (additive — only when *VariableId params are provided).
  if (p.paddingTopVariableId) await bindSpacingVariable(frame, "paddingTop", p.paddingTopVariableId);
  if (p.paddingRightVariableId) await bindSpacingVariable(frame, "paddingRight", p.paddingRightVariableId);
  if (p.paddingBottomVariableId) await bindSpacingVariable(frame, "paddingBottom", p.paddingBottomVariableId);
  if (p.paddingLeftVariableId) await bindSpacingVariable(frame, "paddingLeft", p.paddingLeftVariableId);
  if (p.itemSpacingVariableId && frame.layoutMode !== "GRID") await bindSpacingVariable(frame, "itemSpacing", p.itemSpacingVariableId);
  if (frame.layoutMode === "GRID") {
    // CSS-grid auto-layout: row/column counts + per-axis gaps replace itemSpacing.
    if (p.gridRowCount != null) { const v = n(p.gridRowCount); if (v !== null) frame.gridRowCount = v; }
    if (p.gridColumnCount != null) { const v = n(p.gridColumnCount); if (v !== null) frame.gridColumnCount = v; }
    if (p.gridRowGap != null) { const v = n(p.gridRowGap); if (v !== null) frame.gridRowGap = v; }
    if (p.gridColumnGap != null) { const v = n(p.gridColumnGap); if (v !== null) frame.gridColumnGap = v; }
    if (p.gridRowGapVariableId) await bindSpacingVariable(frame, "gridRowGap", p.gridRowGapVariableId);
    if (p.gridColumnGapVariableId) await bindSpacingVariable(frame, "gridColumnGap", p.gridColumnGapVariableId);
  } else if (frame.layoutMode !== "NONE") {
    if (p.primaryAxisAlignItems) frame.primaryAxisAlignItems = p.primaryAxisAlignItems;
    if (p.counterAxisAlignItems) frame.counterAxisAlignItems = p.counterAxisAlignItems;
    if (p.primaryAxisSizingMode) frame.primaryAxisSizingMode = p.primaryAxisSizingMode;
    if (p.counterAxisSizingMode) frame.counterAxisSizingMode = p.counterAxisSizingMode;
    if (p.layoutWrap) frame.layoutWrap = p.layoutWrap;
    if (p.counterAxisSpacing != null && frame.layoutWrap === "WRAP") {
      const v = n(p.counterAxisSpacing); if (v !== null) frame.counterAxisSpacing = v;
    }
    if (p.counterAxisSpacingVariableId && frame.layoutWrap === "WRAP") {
      await bindSpacingVariable(frame, "counterAxisSpacing", p.counterAxisSpacingVariableId);
    }
    if (p.counterAxisAlignContent && frame.layoutWrap === "WRAP") {
      frame.counterAxisAlignContent = p.counterAxisAlignContent;
    }
    // Per the Plugin API, strokesIncludedInLayout / itemReverseZIndex only apply to
    // HORIZONTAL/VERTICAL auto-layout (not GRID, not NONE) — scope them to this branch.
    if (p.strokesIncludedInLayout != null) frame.strokesIncludedInLayout = !!p.strokesIncludedInLayout;
    if (p.itemReverseZIndex != null) frame.itemReverseZIndex = !!p.itemReverseZIndex;
  }
  // Frame-level min/max constraints — valid on any frame (null clears the constraint).
  if (p.minWidth !== undefined) frame.minWidth = p.minWidth === null ? null : n(p.minWidth);
  if (p.maxWidth !== undefined) frame.maxWidth = p.maxWidth === null ? null : n(p.maxWidth);
  if (p.minHeight !== undefined) frame.minHeight = p.minHeight === null ? null : n(p.minHeight);
  if (p.maxHeight !== undefined) frame.maxHeight = p.maxHeight === null ? null : n(p.maxHeight);
  if (p.overflowDirection != null) frame.overflowDirection = p.overflowDirection;
};

// Text styling shared by set_text + create_text. Loads a NEW font only when
// fontFamily/fontStyle change; the caller must already have the node's current
// font loaded (set_text/create_text both do). All props are opt-in (null = skip).
export const applyTextStyleProps = async (node: any, p: any) => {
  // Link to a named text style FIRST (it sets a bundle of font/size/spacing), so any
  // explicit props below act as overrides on top of the style.
  if (p.textStyleId != null && typeof node.setTextStyleIdAsync === "function") {
    await node.setTextStyleIdAsync(p.textStyleId);
  }
  if (p.fontFamily != null || p.fontStyle != null) {
    const cur = typeof node.fontName === "symbol" ? { family: "Inter", style: "Regular" } : node.fontName;
    const family = p.fontFamily != null ? String(p.fontFamily) : cur.family;
    const style = p.fontStyle != null ? String(p.fontStyle) : cur.style;
    await figma.loadFontAsync({ family, style });
    node.fontName = { family, style };
  }
  if (p.fontSize != null) node.fontSize = Number(p.fontSize);
  if (p.textAlignHorizontal != null) node.textAlignHorizontal = p.textAlignHorizontal;
  if (p.textAlignVertical != null) node.textAlignVertical = p.textAlignVertical;
  if (p.textAutoResize != null) node.textAutoResize = p.textAutoResize;
  if (p.textCase != null) node.textCase = p.textCase;
  if (p.textDecoration != null) node.textDecoration = p.textDecoration;
  if (p.lineHeightUnit === "AUTO") node.lineHeight = { unit: "AUTO" };
  else if (p.lineHeightValue != null) node.lineHeight = { value: Number(p.lineHeightValue), unit: p.lineHeightUnit || "PIXELS" };
  if (p.letterSpacingValue != null) node.letterSpacing = { value: Number(p.letterSpacingValue), unit: p.letterSpacingUnit || "PIXELS" };
  // Paragraph / list rhythm and truncation (whole-node).
  if (p.paragraphIndent != null) node.paragraphIndent = Number(p.paragraphIndent);
  if (p.paragraphSpacing != null) node.paragraphSpacing = Number(p.paragraphSpacing);
  if (p.listSpacing != null) node.listSpacing = Number(p.listSpacing);
  if (p.hangingPunctuation != null) node.hangingPunctuation = !!p.hangingPunctuation;
  if (p.hangingList != null) node.hangingList = !!p.hangingList;
  if (p.leadingTrim != null) node.leadingTrim = p.leadingTrim;
  if (p.textTruncation != null) node.textTruncation = p.textTruncation;
  // maxLines only takes effect with textTruncation = "ENDING"; null restores unlimited.
  if (p.maxLines !== undefined) node.maxLines = p.maxLines === null ? null : Number(p.maxLines);
};

// Sizing-WITHIN-parent props (FILL/HUG/FIXED etc). Require an auto-layout parent —
// Figma throws "layoutSizing requires a parent" / "...auto-layout" otherwise, so the
// caller should wrap this in try/catch and surface a clear error. Positioning is set
// first because an ABSOLUTE-positioned node can't take FILL sizing.
export const applyLayoutSizing = (node: any, p: any) => {
  if (p.layoutPositioning != null) node.layoutPositioning = p.layoutPositioning;
  if (p.layoutAlign != null) node.layoutAlign = p.layoutAlign;
  if (p.layoutGrow != null) node.layoutGrow = Number(p.layoutGrow);
  if (p.layoutSizingHorizontal != null) node.layoutSizingHorizontal = p.layoutSizingHorizontal;
  if (p.layoutSizingVertical != null) node.layoutSizingVertical = p.layoutSizingVertical;
};

export const base64ToBytes = (b64: string) => {
  const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const lookup: Record<string, number> = {};
  for (let i = 0; i < chars.length; i++) lookup[chars[i]] = i;
  const padded = b64.replace(/[^A-Za-z0-9+/=]/g, "");
  const clean = padded.replace(/=/g, "");
  let outLen = Math.floor(padded.length * 3 / 4);
  if (padded.endsWith("==")) outLen -= 2;
  else if (padded.endsWith("=")) outLen -= 1;
  const bytes = new Uint8Array(outLen);
  let j = 0;
  for (let i = 0; i < clean.length; i += 4) {
    const a = lookup[clean[i]] || 0;
    const bv = lookup[clean[i + 1]] || 0;
    const c = lookup[clean[i + 2]] || 0;
    const d = lookup[clean[i + 3]] || 0;
    bytes[j++] = (a << 2) | (bv >> 4);
    if (j < outLen) bytes[j++] = ((bv & 15) << 4) | (c >> 2);
    if (j < outLen) bytes[j++] = ((c & 3) << 6) | d;
  }
  return bytes;
};
