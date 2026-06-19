// Batch — execute many ops (writes and reads) in ONE plugin round-trip.
//
// The discrete tools are the typed vocabulary; `batch` is an additive
// sequencing layer that reuses their existing handlers (read AND write).
// It covers these shapes uniformly:
//   • single-type bulk   — set_fills ×20                        (no refs)
//   • cross-type chains   — create_frame → append → set_fills    using $N.id refs
//   • read chains         — search_nodes → get_node $0.nodes.0.id (read→read)
//   • verify-after-write  — create_frame → set_fills $0.id → get_node $0.id
//
// No eval: each op is a structured {type, nodeIds, params} dispatched to the
// same handler the single tool would use. Inter-op data flows via $N.field refs
// resolved at runtime against earlier ops' results.
//
// Reads inside a batch bypass the Go bridge's read singleflight/dedup (the sem
// is held for the whole batch) and are always LIVE — so use batch reads for
// dependency chains and write→read verification, NOT as a cache bypass for
// bulk catalog reads (get_local_components etc. have REST + cache + singleflight).

import { handleWriteRequest } from "./write-handlers";
import { handleReadRequest } from "./read-handlers";
import { withTimeout } from "./timeout";

// Per-op timeout for a batch dispatch (issue #31). A hung Figma API call inside
// an op would otherwise hold the channel's serial slot until the SERVER's
// inactivity ceiling force-drains it, wedging every other agent on the channel.
//
// CRITICAL: the timeout must mirror the server's OWN per-op-type ceiling, or it
// regresses legitimate slow work. The Go bridge gives a `batch` request the
// GENEROUS read ceiling (FIGMA_MCP_READ_TIMEOUT, default 600s) precisely so a
// batch containing a big read (scan_text_nodes, get_design_context on a large
// file) doesn't time out — see resolveRequestTimeout/isHeavyReadOrBatch in
// internal/bridge.go. So a flat sub-600s cap would FALSE-KILL a legitimate slow
// read in a batch. Instead we split by op type, mirroring that server logic:
//   - heavy reads → heavyReadTimeoutMs (600s, the generous read ceiling): the
//     server allows this much, so we must too — no plugin-side false-trip.
//   - everything else (writes, cheap reads) → opTimeoutMs (120s, the base
//     ceiling): a write legitimately completes in well under a second, so a
//     hung write is capped at 120s instead of the batch's 600s — the real #31
//     win (the issue is hung *writes* / imports, not slow reads).
// Mutable config (not const) so tests dial it down; production never reassigns.
// Defaults assume the server's default timeouts; if FIGMA_MCP_READ_TIMEOUT is
// raised, raise heavyReadTimeoutMs to match. Library imports keep their own
// tighter 15s guard (withImportTimeout) inside the import handlers.
export const batchConfig = { opTimeoutMs: 120_000, heavyReadTimeoutMs: 600_000 };

// Mirrors internal/bridge.go isHeavyReadOrBatch (minus "batch" itself): reads
// whose payloads can be legitimately large/slow and so earn the generous ceiling.
const HEAVY_READ_OPS = new Set([
  "get_node",
  "get_nodes_info",
  "get_design_context",
  "get_document",
  "scan_nodes_by_types",
  "scan_text_nodes",
  "search_nodes",
  "get_local_components",
]);

// opTimeoutMs picks the per-op ceiling by type, mirroring the server so the
// plugin never kills an op the server itself would have allowed to run.
function opTimeoutMs(type: string): number {
  return HEAVY_READ_OPS.has(type)
    ? batchConfig.heavyReadTimeoutMs
    : batchConfig.opTimeoutMs;
}

// A ref is a string of the exact form "$<opIndex>.<dotted.path>", e.g. "$0.id"
// or "$2.bounds.width". REF_BODY is the single source of that grammar — REF
// matches a whole string (so ordinary strings containing "$" are left untouched);
// the same body is reused to cheaply detect whether a batch uses refs at all.
const REF_BODY = String.raw`\$(\d+)\.([\w.]+)`;

// Named-binding ref grammar — matches a string value that is EXACTLY a named
// binding ref with optional dotted path, e.g. "$item", "$item.id", "$index".
// Whole-string anchored only (so "$item costs $5" is left untouched); numeric
// heads are excluded so "$0.id" stays an op-index ref for resolveRefs.
// GOTCHA: a loop-var projection like "$item.children[*].id" is NOT matched here
// ([\w.]+ won't consume "[*]") — it reaches resolveRefs as a literal and silently
// fails. A future extension should detect "bound head + value contains [*]" and throw.
const NAMED_BINDING = /^\$([A-Za-z_]\w*)(?:\.([\w.]+))?$/;

// MAP_ITER_CAP: maximum number of items a single map op may iterate.
// Exceeding this is an EXPLICIT ERROR (never silent truncation) — the error
// names both the actual count and the cap so callers understand what happened.
const MAP_ITER_CAP = 500;
const REF = new RegExp(`^${REF_BODY}$`);
const PUBLISHED_KEY = /^[0-9a-f]{40}$/;
const BARE_NODE_ID = /^[0-9]+:[0-9]+$/;

// A projection ref carries exactly one `[*]` wildcard segment:
//   $N.<pre>[*]          → the elements of the array at <pre> (mapped as-is)
//   $N.<pre>[*].<post>   → <post> projected out of every element
// <pre> and <post> are ordinary dotted paths (index segments allowed). It maps
// over an earlier op's array → an ARRAY, the fan-in that feeds a bulk setter.
const PROJECTION = new RegExp(String.raw`^\$(\d+)\.([\w.]+)\[\*\](?:\.([\w.]+))?$`);

// A string is "projection-shaped" if it looks like a $N ref carrying a `[*]`.
// Validity (exactly one `[*]`, well-formed paths) is enforced in resolveProjection
// so a malformed multi-`[*]` ref throws a clear error instead of slipping through
// as an untouched literal.
function isProjectionRef(value: any): boolean {
  return typeof value === "string" && /^\$\d+\..*\[\*\]/.test(value);
}

function getPath(obj: any, path: string): any {
  return path.split(".").reduce((o, k) => (o == null ? undefined : o[k]), obj);
}

// resolveProjection evaluates one `$N.<pre>[*].<post>` ref to an array. It rejects
// more than one `[*]` (2-D projection is out of scope), a missing pre-array, and a
// non-array target — each with a message that names the offending ref.
function resolveProjection(value: string, results: any[]): any[] {
  if ((value.match(/\[\*\]/g) || []).length > 1) {
    throw new Error(`ref ${value}: exactly one [*] wildcard is allowed`);
  }
  const m = value.match(PROJECTION);
  if (!m) {
    throw new Error(`ref ${value}: malformed projection ($N.<path>[*].<subpath>)`);
  }
  const idx = Number(m[1]);
  const pre = m[2];
  const post = m[3]; // undefined for the bare `[*]` (project elements themselves)
  if (idx >= results.length) {
    throw new Error(
      `ref ${value} points to op #${idx}, which has not run yet ` +
        `(only ${results.length} op(s) completed before this one) — ` +
        `$N.field may reference earlier ops only`,
    );
  }
  const r = results[idx];
  if (!r || r.error || r.data == null) {
    throw new Error(`ref ${value} points to op #${idx}, which produced no data`);
  }
  const arr = getPath(r.data, pre);
  if (arr === undefined) {
    throw new Error(`ref ${value}: op #${idx} result has no field "${pre}"`);
  }
  if (!Array.isArray(arr)) {
    throw new Error(`ref ${value}: field "${pre}" is not an array`);
  }
  return post === undefined ? arr : arr.map((el) => getPath(el, post));
}

// resolveRefs walks value (string | array | object) and replaces every $N.path
// ref with the resolved value from results[N].data. results holds ONLY ops that
// already ran, so a forward/self ref (N >= results.length) is rejected. Nested
// structure is preserved — this is a structural walk, never a string replace.
export function resolveRefs(value: any, results: any[]): any {
  if (typeof value === "string") {
    // Projection refs ($N.path[*]…) resolve to an array. Checked before the
    // single-value REF (whose anchored match can't consume the `[*]`). As an
    // object value or a standalone string the array is returned as-is; the array
    // branch below spreads it in place (flattening) when it is an element.
    if (isProjectionRef(value)) return resolveProjection(value, results);
    const m = value.match(REF);
    if (!m) return value;
    const idx = Number(m[1]);
    if (idx >= results.length) {
      throw new Error(
        `ref ${value} points to op #${idx}, which has not run yet ` +
          `(only ${results.length} op(s) completed before this one) — ` +
          `$N.field may reference earlier ops only`,
      );
    }
    const r = results[idx];
    if (!r || r.error || r.data == null) {
      throw new Error(`ref ${value} points to op #${idx}, which produced no data`);
    }
    const resolved = getPath(r.data, m[2]);
    if (resolved === undefined) {
      throw new Error(`ref ${value}: op #${idx} result has no field "${m[2]}"`);
    }
    return resolved;
  }
  if (Array.isArray(value)) {
    // A projection ref AS AN ARRAY ELEMENT spreads in place (flatMap), so
    // nodeIds:["$0.nodes[*].id"] becomes the flat [id0,id1,…] not [[…]]. The rule
    // is positional, not count-based: any element that is a projection ref is
    // spread; every other element resolves to a single value.
    const out: any[] = [];
    for (const v of value) {
      if (isProjectionRef(v)) out.push(...resolveProjection(v, results));
      else out.push(resolveRefs(v, results));
    }
    return out;
  }
  if (value && typeof value === "object") {
    const out: any = {};
    for (const k of Object.keys(value)) out[k] = resolveRefs(value[k], results);
    return out;
  }
  return value;
}

// substituteBindings walks value (string | array | object) and replaces ONLY
// named-binding refs (e.g. $item, $item.foo.bar, $index) with values from
// `bindings`. It runs BEFORE resolveRefs, leaving $N op-index refs untouched.
//
// Resolution rules:
//   - Whole-string anchored match only (^\$name(\.path)?$).
//   - Non-numeric head distinguishes $item from $0 (digits → not a named binding).
//   - Unbound named ref (head not in bindings) → LOUD THROW naming bound keys.
//   - $index resolves to the iteration number (scalar, in bindings as "index").
//   - $item resolves to the element; $item.foo.bar projects off the element.
//   - A non-anchored string ("$item costs $5") is left untouched — known collision
//     cost: named refs must occupy the whole string value, not a substring.
//
// Returns a new value; never mutates input.
export function substituteBindings(value: any, bindings: Record<string, any>): any {
  if (typeof value === "string") {
    const m = value.match(NAMED_BINDING);
    if (!m) return value;
    const head = m[1];
    const path = m[2]; // may be undefined
    if (!(head in bindings)) {
      const bound = Object.keys(bindings).join(", ");
      throw new Error(`unknown binding $${head} (bound: ${bound})`);
    }
    const base = bindings[head];
    if (path === undefined) return base;
    const resolved = getPath(base, path);
    if (resolved === undefined) {
      throw new Error(`binding $${head}.${path}: no field "${path}" on value`);
    }
    return resolved;
  }
  if (Array.isArray(value)) {
    return value.map((v) => substituteBindings(v, bindings));
  }
  if (value && typeof value === "object") {
    const out: any = {};
    for (const k of Object.keys(value)) out[k] = substituteBindings(value[k], bindings);
    return out;
  }
  return value;
}

function validatePublishedImportKey(kind: "component" | "style", key: any): string | null {
  if (typeof key !== "string" || key === "") return "key is required";
  if (PUBLISHED_KEY.test(key)) return null;
  if (BARE_NODE_ID.test(key)) {
    return `that's a node id, not a published ${kind} key`;
  }
  if (key.length < 40) return `${kind} key looks truncated (got ${key.length} chars, expected 40)`;
  return `malformed ${kind} key; expected 40-char hex`;
}

function validateVariableImportKey(key: any): string | null {
  if (typeof key !== "string" || key === "") return "key is required";
  if (BARE_NODE_ID.test(key)) {
    return "that's a node id, not a published variable key";
  }
  return null;
}

function validateComponentAssetType(assetType: any): string | null {
  if (assetType == null || assetType === "") return null;
  if (assetType === "COMPONENT" || assetType === "COMPONENT_SET") return null;
  return "assetType must be COMPONENT or COMPONENT_SET";
}

function validateResolvedImportOp(type: string, params: any): void {
  switch (type) {
    case "import_component_by_key": {
      const keyError = validatePublishedImportKey("component", params?.key);
      if (keyError) throw new Error(keyError);
      const assetTypeError = validateComponentAssetType(params?.assetType);
      if (assetTypeError) throw new Error(assetTypeError);
      return;
    }
    case "import_style_by_key": {
      const keyError = validatePublishedImportKey("style", params?.key);
      if (keyError) throw new Error(keyError);
      return;
    }
    case "import_variable_by_key": {
      const keyError = validateVariableImportKey(params?.key);
      if (keyError) throw new Error(keyError);
      return;
    }
  }
}

function requireNodeIds(type: string, nodeIds: any): void {
  if (!Array.isArray(nodeIds) || nodeIds.length === 0) {
    throw new Error(`${type}: nodeIds is required`);
  }
}

function hasOwn(obj: any, key: string): boolean {
  return obj != null && Object.prototype.hasOwnProperty.call(obj, key);
}

function requireColorOrPaints(type: string, params: any): void {
  if (Array.isArray(params?.paints)) return;
  if (typeof params?.color === "string" && params.color !== "") return;
  throw new Error(`${type}: color or paints is required`);
}

function validateMode(type: string, params: any): void {
  if (params?.mode == null || params.mode === "") return;
  if (params.mode !== "replace" && params.mode !== "append") {
    throw new Error(`${type}: mode must be 'replace' or 'append'`);
  }
}

function requireNumberRange(type: string, effect: any, key: string, min: number, max: number): void {
  if (effect[key] == null) return;
  if (typeof effect[key] !== "number" || !Number.isFinite(effect[key]) || effect[key] < min || effect[key] > max) {
    throw new Error(`${type}: ${key} must be between ${min} and ${max}`);
  }
}

function requireNumber(type: string, effect: any, key: string): void {
  if (effect[key] == null) return;
  if (typeof effect[key] !== "number" || !Number.isFinite(effect[key])) {
    throw new Error(`${type}: ${key} must be a number`);
  }
}

function requireNumberMin(type: string, effect: any, key: string, min: number): void {
  if (effect[key] == null) return;
  if (typeof effect[key] !== "number" || !Number.isFinite(effect[key]) || effect[key] < min) {
    throw new Error(`${type}: ${key} must be >= ${min}`);
  }
}

function validateVector(type: string, effect: any, key: string, bounded: boolean): void {
  if (effect[key] == null) return;
  const vector = effect[key];
  if (typeof vector !== "object" || Array.isArray(vector)) {
    throw new Error(`${type}: ${key} must be an object with numeric x and y`);
  }
  for (const axis of ["x", "y"] as const) {
    if (typeof vector[axis] !== "number" || !Number.isFinite(vector[axis])) {
      throw new Error(`${type}: ${key}.${axis} must be a number`);
    }
    if (bounded) {
      if (vector[axis] < 0 || vector[axis] > 1) {
        throw new Error(`${type}: ${key}.${axis} must be between 0 and 1`);
      }
    } else if (vector[axis] <= 0) {
      throw new Error(`${type}: ${key}.${axis} must be > 0`);
    }
  }
}

function validateAdvancedEffect(type: string, effect: any, index: number): void {
  const prefix = `${type}: effects[${index}]`;
  switch (effect.type) {
    case "DROP_SHADOW":
    case "INNER_SHADOW":
      requireNumberRange(prefix, effect, "opacity", 0, 1);
      requireNumberMin(prefix, effect, "radius", 0);
      requireNumberMin(prefix, effect, "spread", 0);
      requireNumber(prefix, effect, "offsetX");
      requireNumber(prefix, effect, "offsetY");
      return;
    case "LAYER_BLUR":
    case "BACKGROUND_BLUR":
      if (effect.blurType != null && effect.blurType !== "NORMAL" && effect.blurType !== "PROGRESSIVE") {
        throw new Error(`${prefix}: blurType must be NORMAL or PROGRESSIVE`);
      }
      requireNumberMin(prefix, effect, "radius", 0);
      requireNumberMin(prefix, effect, "startRadius", 0);
      validateVector(prefix, effect, "startOffset", true);
      validateVector(prefix, effect, "endOffset", true);
      return;
    case "GLASS":
      requireNumberRange(prefix, effect, "lightIntensity", 0, 1);
      requireNumberRange(prefix, effect, "refraction", 0, 1);
      requireNumberRange(prefix, effect, "dispersion", 0, 1);
      requireNumberMin(prefix, effect, "depth", 1);
      requireNumberMin(prefix, effect, "radius", 0);
      requireNumber(prefix, effect, "lightAngle");
      return;
    case "NOISE":
      if (effect.noiseType != null && !["MONOTONE", "DUOTONE", "MULTITONE"].includes(effect.noiseType)) {
        throw new Error(`${prefix}: noiseType must be MONOTONE, DUOTONE, or MULTITONE`);
      }
      requireNumberRange(prefix, effect, "opacity", 0, 1);
      requireNumberRange(prefix, effect, "density", 0, 1);
      requireNumberMin(prefix, effect, "noiseSize", 0);
      validateVector(prefix, effect, "noiseSizeVector", false);
      return;
    case "TEXTURE":
      requireNumberMin(prefix, effect, "noiseSize", 0);
      requireNumberMin(prefix, effect, "radius", 0);
      validateVector(prefix, effect, "noiseSizeVector", false);
      if (effect.clipToShape != null && typeof effect.clipToShape !== "boolean") {
        throw new Error(`${prefix}: clipToShape must be a boolean`);
      }
      return;
  }
}

function validateResolvedEffects(params: any): void {
  if (!Array.isArray(params?.effects)) throw new Error("set_effects: effects array is required");
  const valid = new Set([
    "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR",
    "GLASS", "NOISE", "TEXTURE",
  ]);
  params.effects.forEach((effect: any, index: number) => {
    if (!effect || typeof effect !== "object") {
      throw new Error(`set_effects: effects[${index}] must be an object`);
    }
    if (!valid.has(effect.type)) {
      throw new Error(
        `set_effects: effects[${index}].type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, BACKGROUND_BLUR, GLASS, NOISE, or TEXTURE`,
      );
    }
    validateAdvancedEffect("set_effects", effect, index);
  });
}

function validateResolvedOp(type: string, nodeIds: any, params: any): void {
  validateResolvedImportOp(type, params);
  switch (type) {
    case "set_fills":
      requireNodeIds(type, nodeIds);
      requireColorOrPaints(type, params);
      validateMode(type, params);
      return;
    case "set_strokes":
      requireNodeIds(type, nodeIds);
      requireColorOrPaints(type, params);
      validateMode(type, params);
      return;
    case "set_corner_radius":
      requireNodeIds(type, nodeIds);
      if (
        !hasOwn(params, "cornerRadius") &&
        !hasOwn(params, "topLeftRadius") &&
        !hasOwn(params, "topRightRadius") &&
        !hasOwn(params, "bottomLeftRadius") &&
        !hasOwn(params, "bottomRightRadius")
      ) {
        throw new Error(
          "set_corner_radius: at least one of cornerRadius, topLeftRadius, topRightRadius, bottomLeftRadius, or bottomRightRadius is required",
        );
      }
      return;
    case "set_constraints":
      requireNodeIds(type, nodeIds);
      if (!hasOwn(params, "horizontal") && !hasOwn(params, "vertical")) {
        throw new Error("set_constraints: at least one of horizontal or vertical is required");
      }
      return;
    case "set_effects":
      requireNodeIds(type, nodeIds);
      validateResolvedEffects(params);
      return;
    case "delete_variable":
      if (!params?.variableId && !params?.collectionId) {
        throw new Error("delete_variable: variableId or collectionId is required");
      }
      return;
    case "delete_page":
      if (!params?.pageId && !params?.pageName) {
        throw new Error("delete_page: pageId or pageName is required");
      }
      return;
    case "rename_page":
      if (!params?.pageId && !params?.pageName) {
        throw new Error("rename_page: pageId or pageName is required");
      }
      if (!params?.newName) throw new Error("rename_page: newName is required");
      return;
    case "update_paint_style":
      if (!params?.styleId) throw new Error("update_paint_style: styleId is required");
      if (
        !hasOwn(params, "name") &&
        !hasOwn(params, "color") &&
        !hasOwn(params, "paints") &&
        !hasOwn(params, "description")
      ) {
        throw new Error("update_paint_style: at least one of name, color, paints, or description is required");
      }
      return;
  }
}

// executeOp is a small extracted helper that resolves refs on a single concrete op
// (after any binding substitution has already happened) and dispatches it to the
// correct handler. Returns { i, type, data } on success; throws on failure.
// Extracted so both the outer loop and the map inner loop use the same path.
async function executeOp(
  op: any,
  i: number,
  results: any[],
  requestId: string,
): Promise<{ i: number; type: string; data: any }> {
  if (!op || typeof op.type !== "string") {
    throw new Error("each op requires a string `type`");
  }
  const nodeIds = resolveRefs(op.nodeIds ?? [], results);
  const params = resolveRefs(op.params ?? {}, results);
  validateResolvedOp(op.type, nodeIds, params);
  // Reset the per-op perf flag for EVERY op so a prior op's `true` never leaks into a
  // later op that omitted it (a batch dispatches inner ops itself, so it re-applies here).
  figma.skipInvisibleInstanceChildren =
    params?.skipInvisibleInstanceChildren === true;
  const opReq = { type: op.type, requestId, nodeIds, params };
  // Guard the dispatch with a timeout so a hung Figma API call can't wedge the
  // channel until the server ceiling fires (issue #31). A timeout throws like any
  // other op failure, so the outer loop's continueOnError policy still applies.
  const dispatch = (async () =>
    (await handleReadRequest(opReq)) ?? (await handleWriteRequest(opReq)))();
  const out = await withTimeout(
    dispatch,
    `batch op "${op.type}" (#${i})`,
    opTimeoutMs(op.type),
    "the Figma API call hung; re-scope the op or run a heavy read as a standalone call",
  );
  if (out === null) throw new Error(`unknown op type: ${op.type}`);
  if (out.error) throw new Error(out.error);
  return { i, type: op.type, data: out.data };
}

// runMap implements the map control-flow op. It is NOT dispatched to read/write
// handlers — the batch loop intercepts it and calls runMap directly.
//
// Shape:
//   { type:"map", over:<arrayRef|literal>, as:<bindingName>, do:<opTemplate> }
//
// over:  resolved via resolveRefs (handles $N.path[*] projection and plain arrays).
// as:    names the loop variable (e.g. "item") → exposes $item / $item.path.
// do:    op template — substituteBindings runs per-iteration (producing a concrete
//        op), then resolveRefs handles any outer $N refs still present.
//
// Returns { results, okCount, failCount } — same shape as handleBatchRequest's
// data, so a downstream op can project $M.results[*].data.<field> through the
// EXISTING projection resolver with no new machinery.
//
// Safety:
//   - over must resolve to an array (clear error if not).
//   - Iteration cap: MAP_ITER_CAP. Exceeding it throws naming count + cap.
//   - continueOnError controls inner-loop stop policy (same semantics as outer).
//   - Throttled progress_update every 10 items or 500ms so the Go bridge inactivity
//     timer doesn't fire on a long map (the outer loop ticks once per map op, but
//     without inner ticks a 200-item map is silent mid-run).
async function runMap(
  op: any,
  outerResults: any[],
  requestId: string,
  continueOnError: boolean,
): Promise<{ results: any[]; okCount: number; failCount: number }> {
  // Resolve the 'over' array using the existing resolver (handles projection + scalar)
  const rawOver = resolveRefs(op.over, outerResults);
  if (!Array.isArray(rawOver)) {
    throw new Error(
      `map op: 'over' must resolve to an array, got ${typeof rawOver}`,
    );
  }
  if (rawOver.length > MAP_ITER_CAP) {
    throw new Error(
      `map op: 'over' has ${rawOver.length} items, exceeding the cap of ${MAP_ITER_CAP}; ` +
        `reduce the list or split into multiple map ops`,
    );
  }

  const bindingName: string = op.as ?? "item";
  const doTemplate = op.do;
  if (!doTemplate || typeof doTemplate !== "object") {
    throw new Error("map op: 'do' must be an op object");
  }

  const innerResults: any[] = [];
  let okCount = 0;
  let failCount = 0;
  let lastProgressMs = Date.now();
  const PROGRESS_INTERVAL_MS = 500;
  const PROGRESS_EVERY_N = 10;

  for (let idx = 0; idx < rawOver.length; idx++) {
    const element = rawOver[idx];
    let res: any;
    try {
      // Step 1: substitute named bindings ($item, $index) in the do-template
      const bindings: Record<string, any> = { [bindingName]: element, index: idx };
      const concreteOp = substituteBindings(doTemplate, bindings);
      // Step 2: dispatch via executeOp — it owns $N ref resolution via resolveRefs
      // internally. Passing concreteOp directly avoids calling resolveRefs twice.
      const result = await executeOp(concreteOp, idx, outerResults, requestId);
      res = { i: idx, type: concreteOp.type, data: result.data };
      okCount++;
    } catch (e) {
      res = { i: idx, type: doTemplate?.type, error: e instanceof Error ? e.message : String(e) };
      failCount++;
      if (!continueOnError) {
        innerResults.push(res);
        throw new Error(
          `map iteration ${idx} failed: ${res.error}`,
        );
      }
    }
    innerResults.push(res);

    // Throttled progress tick — resets the Go bridge inactivity timer inside the
    // map loop so a 200-item map doesn't go silent for >120s.
    const now = Date.now();
    if (idx % PROGRESS_EVERY_N === 0 || now - lastProgressMs >= PROGRESS_INTERVAL_MS) {
      figma.ui.postMessage({
        type: "progress_update",
        requestId,
        progress: Math.min(98, Math.round(((idx + 1) / rawOver.length) * 98)),
        message: `map: item ${idx + 1}/${rawOver.length}`,
      });
      lastProgressMs = now;
    }
  }

  return { results: innerResults, okCount, failCount };
}

// handleBatchRequest runs request.params.ops sequentially in one round-trip.
// Stop policy (deliberate): if any op carries a $N ref the batch is a dependent
// chain → stop on first error (downstream refs would break). With no refs it is
// independent bulk → continue and report every op, so one stale id does not
// abort an otherwise-good batch. `params.continueOnError` overrides either way.
// Returns null for non-batch requests so the dispatcher can fall through.
export async function handleBatchRequest(request: any): Promise<any> {
  if (request.type !== "batch") return null;
  const requestId = request.requestId;
  const ops = request.params?.ops;
  if (!Array.isArray(ops)) {
    return { type: "batch", requestId, error: "batch requires params.ops (array)" };
  }

  const hasRefs = new RegExp(REF_BODY).test(JSON.stringify(ops));
  const continueOnError =
    typeof request.params?.continueOnError === "boolean"
      ? request.params.continueOnError
      : !hasRefs;

  const results: any[] = [];
  let okCount = 0;
  let failCount = 0;
  let failedAt = -1;

  for (let i = 0; i < ops.length; i++) {
    const op = ops[i];
    let res: any;
    try {
      if (!op || typeof op.type !== "string") {
        throw new Error("each op requires a string `type`");
      }
      if (op.type === "map") {
        // map is a control-flow op interpreted here — NOT dispatched to handlers.
        // The per-op skipInvisibleInstanceChildren flag is NOT set for map itself
        // (it has no nodeIds); executeOp sets it for each inner op instead.
        const mapData = await runMap(op, results, requestId, continueOnError);
        res = { i, type: "map", data: mapData };
        okCount++;
      } else {
        // Standard op — executeOp resolves refs, sets the per-op perf flag, and dispatches.
        const result = await executeOp(op, i, results, requestId);
        res = { i, type: result.type, data: result.data };
        okCount++;
      }
    } catch (e) {
      res = { i, type: op?.type, error: e instanceof Error ? e.message : String(e) };
      failCount++;
      if (failedAt < 0) failedAt = i;
    }
    results.push(res);

    // One progress tick per op resets the Go-bridge per-request timeout, so a
    // long batch never trips the 120s ceiling mid-run.
    figma.ui.postMessage({
      type: "progress_update",
      requestId,
      progress: Math.min(99, Math.round(((i + 1) / ops.length) * 99)),
      message: `batch: op ${i + 1}/${ops.length} (${op?.type})`,
    });

    if (res.error && !continueOnError) break;
  }

  return {
    type: "batch",
    requestId,
    data: { results, okCount, failCount, failedAt },
  };
}
