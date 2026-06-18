// Serializers — shared read/write helpers for converting Figma node data to JSON.

export const isMixed = (value: any) => typeof value === "symbol";

// Round floating-point pixel values to 2 decimal places.
// Figma sometimes returns values like 123.99999999999999 instead of 124.
const pixelRound = (v: number) => Math.round(v * 100) / 100;

export const toHex = (color: any) => {
  const clamp = (v: any) => Math.min(255, Math.max(0, Math.round(v * 255)));
  const [r, g, b] = [clamp(color.r), clamp(color.g), clamp(color.b)];
  return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
};

export const serializePaints = (paints: any) => {
  if (isMixed(paints)) return "mixed";

  if (!paints || !Array.isArray(paints)) return undefined;

  const result = paints
    .filter((paint: any) => paint.type !== undefined)
    .map((paint: any) => {
      if (paint.type === "SOLID" && "color" in paint) {
        const hex = toHex(paint.color);
        const opacity = paint.opacity != null ? paint.opacity : 1;
        const value =
          opacity === 1
            ? hex
            : hex +
              Math.round(opacity * 255)
                .toString(16)
                .padStart(2, "0");
        // Surface a color variable binding so token-binding is verifiable on reads
        // (issue #27). Unbound fills stay bare hex strings; a bound fill becomes
        // {color, variableId} — a raw hex and a bound token are no longer
        // byte-identical. Resolve the token name via get_variable_defs if needed.
        const variableId = paint.boundVariables?.color?.id;
        if (typeof variableId === "string") {
          return { color: value, variableId };
        }
        return value;
      }
      // Non-SOLID: emit at least {type}. For IMAGE also emit scaleMode+imageHash.
      if (paint.type === "IMAGE") {
        const entry: any = { type: "IMAGE" };
        if (paint.scaleMode !== undefined) entry.scaleMode = paint.scaleMode;
        if (paint.imageHash !== undefined) entry.imageHash = paint.imageHash;
        return entry;
      }
      // GRADIENT and any other non-SOLID type: just the type discriminant
      return { type: paint.type };
    });

  return result.length > 0 ? result : undefined;
};

export const getBounds = (node: any) => {
  if ("x" in node && "y" in node && "width" in node && "height" in node) {
    return {
      x: pixelRound(node.x),
      y: pixelRound(node.y),
      width: pixelRound(node.width),
      height: pixelRound(node.height),
    };
  }

  return undefined;
};

// Per-read memo for getStyleByIdAsync. When N nodes share the same applied
// style, this collapses N sequential plugin round-trips to 1 (7R-2). Threaded
// from the read handler so it lives for ONE read only — never module-global, or
// a renamed style would serve a stale name on the next read. Caches undefined
// too (a miss shouldn't re-query). The default `new Map()` keeps standalone
// callers byte-identical.
export type StyleCache = Map<string, string | undefined>;

const resolveStyleName = async (
  id: string,
  cache: StyleCache,
): Promise<string | undefined> => {
  if (cache.has(id)) return cache.get(id);
  const style = await figma.getStyleByIdAsync(id);
  const name = style ? style.name : undefined;
  cache.set(id, name);
  return name;
};

export const serializeStyles = async (
  node: any,
  styleCache: StyleCache = new Map(),
) => {
  const styles: any = {};

  if ("fills" in node) {
    // Prefer named style over raw fill values when a style is applied.
    if (node.fillStyleId && typeof node.fillStyleId === "string") {
      const name = await resolveStyleName(node.fillStyleId, styleCache);
      if (name) styles.fillStyle = name;
    }
    const fills = serializePaints(node.fills);
    if (fills !== undefined) styles.fills = fills;
  }

  if ("strokes" in node) {
    if (node.strokeStyleId && typeof node.strokeStyleId === "string") {
      const name = await resolveStyleName(node.strokeStyleId, styleCache);
      if (name) styles.strokeStyle = name;
    }
    const strokes = serializePaints(node.strokes);
    if (strokes !== undefined) styles.strokes = strokes;
    // Emit strokeWeight + strokeAlign only when there are actual strokes —
    // these fields are meaningless (and misleading) on stroke-less nodes.
    if (strokes !== undefined && strokes !== "mixed") {
      if ("strokeWeight" in node && node.strokeWeight !== undefined) {
        styles.strokeWeight = isMixed(node.strokeWeight) ? "mixed" : node.strokeWeight;
      }
      if ("strokeAlign" in node && node.strokeAlign !== undefined) {
        styles.strokeAlign = node.strokeAlign;
      }
    }
  }

  if ("cornerRadius" in node) {
    const cr = isMixed(node.cornerRadius) ? "mixed" : node.cornerRadius;
    if (cr !== 0) styles.cornerRadius = cr;
  }

  if ("paddingLeft" in node) {
    styles.padding = {
      top: node.paddingTop,
      right: node.paddingRight,
      bottom: node.paddingBottom,
      left: node.paddingLeft,
    };
  }

  // Emit effects only when the array is non-empty — an empty array is the default
  // and adds noise to every node's serialized output.
  if ("effects" in node && Array.isArray(node.effects) && node.effects.length > 0) {
    styles.effects = node.effects;
  }

  return styles;
};

export const serializeLineHeight = (lineHeight: any) => {
  if (isMixed(lineHeight)) return "mixed";

  if (!lineHeight || lineHeight.unit === "AUTO") return undefined;

  return { value: lineHeight.value, unit: lineHeight.unit };
};

export const serializeLetterSpacing = (letterSpacing: any) => {
  if (isMixed(letterSpacing)) return "mixed";

  if (!letterSpacing || letterSpacing.value === 0) return undefined;

  return { value: letterSpacing.value, unit: letterSpacing.unit };
};

export const serializeText = async (
  node: any,
  base: any,
  styleCache: StyleCache = new Map(),
) => {
  let fontFamily: any;
  let fontStyle: any;

  if (typeof node.fontName === "symbol") {
    fontFamily = "mixed";
    fontStyle = "mixed";
  } else if (node.fontName) {
    fontFamily = node.fontName.family;
    fontStyle = node.fontName.style;
  }

  const textStyleName =
    node.textStyleId && typeof node.textStyleId === "string"
      ? await resolveStyleName(node.textStyleId, styleCache)
      : undefined;

  return Object.assign({}, base, {
    characters: node.characters,
    styles: Object.assign({}, base.styles, {
      ...(textStyleName ? { textStyle: textStyleName } : {}),
      fontSize: isMixed(node.fontSize) ? "mixed" : node.fontSize,
      fontFamily,
      fontStyle,
      fontWeight: isMixed(node.fontWeight) ? "mixed" : node.fontWeight,
      textDecoration: isMixed(node.textDecoration)
        ? "mixed"
        : node.textDecoration !== "NONE"
          ? node.textDecoration
          : undefined,
      lineHeight: serializeLineHeight(node.lineHeight),
      letterSpacing: serializeLetterSpacing(node.letterSpacing),
      textAlignHorizontal: isMixed(node.textAlignHorizontal)
        ? "mixed"
        : node.textAlignHorizontal,
    }),
  });
};

// Per-read caches threaded through one serializeNode traversal: style names and
// main-component refs shared by many nodes resolve once, not once per node
// (7R-2). Created fresh per top-level call; defaults keep output byte-identical.
export interface SerializeCaches {
  styles: StyleCache;
  components: ComponentCache;
}

const makeCaches = (): SerializeCaches => ({
  styles: new Map(),
  components: new Map(),
});

// Options for a depth-bounded / enriched single-pass serialization. All optional;
// omitting `opts` entirely keeps every existing caller byte-identical.
//   maxDepth     — stop recursing past this depth. The node AT the cutoff is still
//                  fully serialized, but its children are dropped for a
//                  `{ childCount }` summary — the exact truncation shape the
//                  read-document wrappers (serializeNodeWithDepth / serializeWithDepth)
//                  produced before this became a single walk. Default Infinity.
//   enrich       — optional async post-process applied to EVERY serialized node
//                  AFTER its children/childCount are built, so any keys it appends
//                  land after `children` (get_design_context codegen depends on
//                  this exact order — see its key-order golden).
//   currentDepth — internal recursion counter; top-level callers pass 0 (or omit).
export interface SerializeOptions {
  maxDepth?: number;
  enrich?: (node: any, serialized: any) => any | Promise<any>;
  currentDepth?: number;
}

export const serializeNode = async (
  node: any,
  caches: SerializeCaches = makeCaches(),
  // Optional per-node heartbeat. serializeNode recurses over the whole subtree in
  // one call (unbounded Promise.all over children), so a full-page walk (get_document)
  // has no natural place to yield. Passing a tick (e.g. makeProgress) lets the walk
  // periodically yield the JS thread and post a progress_update that resets the
  // Go-bridge inactivity timer. Omitted by depth-bounded callers that tick themselves.
  onVisit?: (total?: number) => void | Promise<void>,
  opts?: SerializeOptions,
): Promise<any> => {
  if (onVisit) await onVisit();
  const maxDepth = opts?.maxDepth ?? Infinity;
  const currentDepth = opts?.currentDepth ?? 0;
  const enrich = opts?.enrich;
  // Apply the optional per-node enrichment last so its keys append AFTER children.
  const finish = async (value: any) => (enrich ? await enrich(node, value) : value);

  const styles = await serializeStyles(node, caches.styles);
  let base: any = {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: getBounds(node),
    styles,
  };
  // Emit opacity only when it differs from the default (1) and visible only when
  // false — omitting the defaults keeps payloads compact and diff-friendly.
  if ("opacity" in node && node.opacity !== 1) base.opacity = node.opacity;
  if ("visible" in node && node.visible === false) base.visible = false;

  if (node.type === "TEXT") return finish(await serializeText(node, base, caches.styles));
  // INSTANCE: resolve the main component name + key inline so the node read is
  // self-contained — the model knows which component it is and can re-import by key
  // without a follow-up round-trip (the read→dereference→read chain is the single-
  // thread killer this avoids).
  if (node.type === "INSTANCE") {
    const ref = await serializeComponentRef(node, caches.components);
    if (ref) {
      // Split the master node id out to a top-level `mainComponentId` (issue #29
      // recovery path) so `mainComponent` stays exactly {key,name,remote}.
      const { id: mainComponentId, ...mainComponent } = ref;
      base = Object.assign({}, base, { mainComponent });
      if (mainComponentId) base.mainComponentId = mainComponentId;
    }
    // Surface the variant/property selection (e.g. {Type: "icon"}) so a reader can tell which
    // variant this instance is — the loss of this was the root cause of icon-cells flattening
    // to generic cells on rebuild. Flattened to bare values to match the dedupeComponents path
    // (read-document.ts) so consumers see ONE consistent componentProperties shape.
    if (node.componentProperties && typeof node.componentProperties === "object") {
      const props: Record<string, any> = {};
      for (const [key, prop] of Object.entries(node.componentProperties)) {
        props[key] = (prop as any)?.value;
      }
      if (Object.keys(props).length > 0) {
        base = Object.assign({}, base, { componentProperties: props });
      }
    }
  }
  if ("children" in node) {
    // At the depth cap: serialize THIS node fully but drop its children for a
    // { childCount } summary — byte-identical to the old wrapper truncation.
    if (currentDepth >= maxDepth) {
      return finish(Object.assign({}, base, { childCount: node.children.length }));
    }
    const children = await Promise.all(
      node.children.map((child: any) =>
        serializeNode(child, caches, onVisit, {
          maxDepth,
          enrich,
          currentDepth: currentDepth + 1,
        }),
      ),
    );
    return finish(Object.assign({}, base, { children }));
  }
  return finish(base);
};

// deduplicateStyles does a two-pass walk over a serialized node tree.
// First pass: count how many times each fills/strokes array value appears.
// Second pass: replace values that appear more than once with a short ref key.
// Returns the rewritten tree and a globalVars.styles map (or undefined if nothing was deduped).
export const deduplicateStyles = (tree: any): { tree: any; globalVars: Record<string, any> | undefined } => {
  // Pass 1: count occurrences of each serialized fill/stroke value
  const counts = new Map<string, number>();
  const countWalk = (node: any) => {
    if (!node || typeof node !== "object") return;
    const s = node.styles;
    if (s) {
      if (Array.isArray(s.fills)) counts.set(JSON.stringify(s.fills), (counts.get(JSON.stringify(s.fills)) ?? 0) + 1);
      if (Array.isArray(s.strokes)) counts.set(JSON.stringify(s.strokes), (counts.get(JSON.stringify(s.strokes)) ?? 0) + 1);
    }
    if (Array.isArray(node.children)) node.children.forEach(countWalk);
  };
  countWalk(tree);

  // Build ref map for values that appear more than once
  let counter = 0;
  const keyToRef = new Map<string, string>();
  const refs: Record<string, any> = {};
  for (const [key, count] of counts) {
    if (count > 1) {
      const ref = `s${++counter}`;
      keyToRef.set(key, ref);
      refs[ref] = JSON.parse(key);
    }
  }
  if (keyToRef.size === 0) return { tree, globalVars: undefined };

  // Pass 2: replace repeated values with ref keys, MUTATING IN PLACE (6B-1).
  // `tree` is the freshly-built throwaway from serializeNode — not aliased to any
  // caller state — so in-place assignment is safe and eliminates the 2× peak the
  // old spread-rebuild ({...node, ...}) created. Assigning s.fills/s.strokes
  // overwrites the existing key in its existing position, so key order (and thus
  // the serialized JSON bytes) is preserved.
  const replaceWalk = (node: any): void => {
    if (!node || typeof node !== "object") return;
    const s = node.styles;
    if (s) {
      if (Array.isArray(s.fills)) {
        const ref = keyToRef.get(JSON.stringify(s.fills));
        if (ref) s.fills = ref;
      }
      if (Array.isArray(s.strokes)) {
        const ref = keyToRef.get(JSON.stringify(s.strokes));
        if (ref) s.strokes = ref;
      }
    }
    if (Array.isArray(node.children)) node.children.forEach(replaceWalk);
  };
  replaceWalk(tree);

  return { tree, globalVars: { styles: refs } };
};

// serializeAutoLayout extracts auto-layout properties for codegen detail.
// Returns undefined for non-auto-layout nodes (no layoutMode, or layoutMode === "NONE").
// Omits any sub-field that is undefined on the node.
export const serializeAutoLayout = (node: any) => {
  if (!("layoutMode" in node) || node.layoutMode === "NONE") return undefined;

  const al: any = { layoutMode: node.layoutMode };
  const passThrough = [
    "primaryAxisAlignItems",
    "counterAxisAlignItems",
    "itemSpacing",
    "counterAxisSpacing",
    "layoutWrap",
    "layoutSizingHorizontal",
    "layoutSizingVertical",
  ];
  for (const field of passThrough) {
    if (node[field] !== undefined) al[field] = node[field];
  }

  if (
    node.paddingTop !== undefined ||
    node.paddingRight !== undefined ||
    node.paddingBottom !== undefined ||
    node.paddingLeft !== undefined
  ) {
    al.padding = {
      top: node.paddingTop,
      right: node.paddingRight,
      bottom: node.paddingBottom,
      left: node.paddingLeft,
    };
  }

  return al;
};

// Text typography fields bind as VariableAlias[] (per-character-range), not scalar.
// We resolve element [0] (the uniform binding) — see plugin typings VariableBindableTextField.
const TEXT_BINDABLE_FIELDS = new Set([
  "fontFamily",
  "fontSize",
  "fontStyle",
  "fontWeight",
  "letterSpacing",
  "lineHeight",
  "paragraphSpacing",
  "paragraphIndent",
]);

// serializeCodegenTokens resolves design-variable (token) names bound to a node.
// Walks node.boundVariables: scalar alias fields (paddingLeft, cornerRadius, …) and
// the array-valued text-typography fields (fontSize, lineHeight, …, resolved from [0]).
// Other array fields (fills/strokes/effects/layoutGrids) are handled at paint/effect
// level. Resolution is cached per id and degrades gracefully on throw/miss.
// Returns undefined when nothing resolves.
// Per-read memo for getVariableByIdAsync, keyed by variable id. Extracted so the
// codegen serialization walk AND the prewarm pre-pass resolve a token id through
// the EXACT same code path → identical cached value (the byte-identity guarantee
// for prewarm rests on this). Caches misses too; degrades to undefined on throw.
export const resolveVariableName = async (
  id: string,
  cache: Map<string, string | undefined>,
): Promise<string | undefined> => {
  if (!id) return undefined;
  if (cache.has(id)) return cache.get(id);
  let name: string | undefined;
  try {
    const variable = await figma.variables.getVariableByIdAsync(id);
    name = variable?.name ?? undefined;
  } catch {
    name = undefined;
  }
  cache.set(id, name);
  return name;
};

export const serializeCodegenTokens = async (
  node: any,
  cache: Map<string, string | undefined> = new Map(),
) => {
  const tokens: Record<string, string> = {};

  const resolve = (id: string) => resolveVariableName(id, cache);

  // Node-level bound variables: scalar aliases + text-typography arrays.
  const bound = node.boundVariables;
  if (bound && typeof bound === "object") {
    for (const [field, value] of Object.entries(bound)) {
      let id: unknown;
      if (TEXT_BINDABLE_FIELDS.has(field) && Array.isArray(value)) {
        id = (value[0] as any)?.id; // uniform text binding
      } else if (Array.isArray(value)) {
        continue; // fills/strokes/effects/etc — resolved at paint/effect level
      } else {
        id = (value as any)?.id;
      }
      if (typeof id !== "string") continue;
      const name = await resolve(id);
      if (name) tokens[field] = name;
    }
  }

  // Paint-level color bindings on fills and strokes.
  for (const paintField of ["fills", "strokes"]) {
    const paints = node[paintField];
    if (!Array.isArray(paints)) continue;
    for (let i = 0; i < paints.length; i++) {
      const colorId = paints[i]?.boundVariables?.color?.id;
      if (typeof colorId !== "string") continue;
      const name = await resolve(colorId);
      if (name) tokens[`${paintField}.${i}.color`] = name;
    }
  }

  // Effect-level bindings (shadow color/radius/spread/offsetX/offsetY).
  const effects = node.effects;
  if (Array.isArray(effects)) {
    for (let i = 0; i < effects.length; i++) {
      const eb = effects[i]?.boundVariables;
      if (!eb || typeof eb !== "object") continue;
      for (const [field, alias] of Object.entries(eb)) {
        const id = (alias as any)?.id;
        if (typeof id !== "string") continue;
        const name = await resolve(id);
        if (name) tokens[`effects.${i}.${field}`] = name;
      }
    }
  }

  return Object.keys(tokens).length > 0 ? tokens : undefined;
};

// Per-read memo for getMainComponentAsync, keyed by INSTANCE node id (7R-2).
// Under documentAccess:"dynamic-page" the main component is only knowable after
// the await, so two DISTINCT instances can't share a pre-call key — this
// collapses re-resolution of the SAME instance within one read (e.g.
// get_design_context resolves an instance both in serializeNode and again in
// enrichForCodegen). Narrower than the style memo by construction.
export type ComponentCache = Map<
  string,
  { key: string; name: string; remote: boolean; id?: string } | undefined
>;

// serializeComponentRef returns the published main-component key + name for an
// INSTANCE node, used for Code-Connect mapping. Returns undefined for non-INSTANCE
// nodes or when no main component resolves. The `id` (master node id) is carried
// for the issue-#29 recovery path; surfacing sites that need a {key,name,remote}-
// only shape (mainComponent / componentRef) strip it back out.
export const serializeComponentRef = async (
  node: any,
  cache: ComponentCache = new Map(),
) => {
  if (node.type !== "INSTANCE" || typeof node.getMainComponentAsync !== "function") {
    return undefined;
  }
  if (cache.has(node.id)) return cache.get(node.id);
  let mc: any;
  try {
    mc = await node.getMainComponentAsync();
  } catch {
    cache.set(node.id, undefined);
    return undefined;
  }
  // Include the remote flag so callers can distinguish library components from local ones —
  // omitting it caused false "component not found" gaps in the relibrary pipeline.
  const ref = mc
    ? { key: mc.key, name: mc.name, remote: mc.remote === true, id: mc.id }
    : undefined;
  cache.set(node.id, ref);
  return ref;
};

// ── Prewarm: parallelize the per-read async lookups ──────────────────────────
// serializeNode resolves styles / main-components / (codegen) token names INLINE
// during the recursive walk, so on a cache MISS each distinct id is fetched
// serially — await blocks the next node. prewarmReadCaches does ONE cheap
// structural pass collecting the unique ids, then resolves them ALL with one
// Promise.all into the SAME caches the walk uses, so the walk then hits the cache
// for every id and runs effectively synchronously (the async I/O overlaps).
//
// Byte-identity guarantee: prewarm only POPULATES the caches, using the exact same
// resolver functions the inline walk uses (resolveStyleName / serializeComponentRef
// / resolveVariableName). The walk always checks the cache first, so a prewarmed
// value is identical to what inline resolution would produce, and a collection MISS
// simply falls back to correct inline resolution. Over- or under-collecting can
// only change how much is parallelized, never the output.

// Mirror serializeCodegenTokens's id sources (scalar + text[0] bound variables,
// paint-level fill/stroke color bindings, effect bindings). Imperfection is safe.
const collectCodegenVariableIds = (node: any, out: Set<string>): void => {
  const bound = node.boundVariables;
  if (bound && typeof bound === "object") {
    for (const [field, value] of Object.entries(bound)) {
      let id: unknown;
      if (TEXT_BINDABLE_FIELDS.has(field) && Array.isArray(value)) id = (value[0] as any)?.id;
      else if (Array.isArray(value)) continue;
      else id = (value as any)?.id;
      if (typeof id === "string") out.add(id);
    }
  }
  for (const paintField of ["fills", "strokes"]) {
    const paints = node[paintField];
    if (!Array.isArray(paints)) continue;
    for (const p of paints) {
      const colorId = p?.boundVariables?.color?.id;
      if (typeof colorId === "string") out.add(colorId);
    }
  }
  const effects = node.effects;
  if (Array.isArray(effects)) {
    for (const e of effects) {
      const eb = e?.boundVariables;
      if (!eb || typeof eb !== "object") continue;
      for (const alias of Object.values(eb)) {
        const id = (alias as any)?.id;
        if (typeof id === "string") out.add(id);
      }
    }
  }
};

export const prewarmReadCaches = async (
  roots: any[],
  caches: SerializeCaches,
  opts?: { maxDepth?: number; tokenCache?: Map<string, string | undefined> },
): Promise<void> => {
  const maxDepth = opts?.maxDepth ?? Infinity;
  const tokenCache = opts?.tokenCache;
  const styleIds = new Set<string>();
  const instances: any[] = [];
  const variableIds = new Set<string>();

  const collect = (node: any, depth: number): void => {
    if (typeof node.fillStyleId === "string" && node.fillStyleId) styleIds.add(node.fillStyleId);
    if (typeof node.strokeStyleId === "string" && node.strokeStyleId) styleIds.add(node.strokeStyleId);
    if (typeof node.textStyleId === "string" && node.textStyleId) styleIds.add(node.textStyleId);
    if (node.type === "INSTANCE" && typeof node.getMainComponentAsync === "function") instances.push(node);
    if (tokenCache) collectCodegenVariableIds(node, variableIds);
    // Mirror serializeNode's visitation: the node AT the cap is still serialized
    // (its own ids count) but its children are not visited.
    if (depth < maxDepth && "children" in node && Array.isArray(node.children)) {
      for (const c of node.children) collect(c, depth + 1);
    }
  };
  for (const r of roots) if (r) collect(r, 0);

  await Promise.all([
    ...Array.from(styleIds).map((id) => resolveStyleName(id, caches.styles)),
    ...instances.map((n) => serializeComponentRef(n, caches.components)),
    ...(tokenCache ? Array.from(variableIds).map((id) => resolveVariableName(id, tokenCache)) : []),
  ]);
};

export const serializeVariableValue = (value: any) => {
  if (typeof value !== "object" || value === null) return value;

  if ("type" in value && value.type === "VARIABLE_ALIAS") {
    return { type: "VARIABLE_ALIAS", id: value.id };
  }

  if ("r" in value && "g" in value && "b" in value) {
    return {
      type: "COLOR",
      r: value.r,
      g: value.g,
      b: value.b,
      a: "a" in value ? value.a : 1,
    };
  }

  return value;
};
