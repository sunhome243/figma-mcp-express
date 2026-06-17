// Presence — "watch the agent work" live highlight (PoC).
//
// When enabled, after a write/batch op completes the plugin selects the affected
// node(s) and scrolls the viewport to them, so a human watching the file sees the
// agent's edits land in real time. This is the proven, non-destructive pattern
// (selection + scrollAndZoomIntoView — same as grab/cursor-talk-to-figma-mcp's
// set_focus/set_selections): zero canvas pollution, zero undo entries. Figma gives
// plugins no separate overlay/cursor layer, so a literal floating cursor would mean
// real nodes (layer/undo/sync churn) — deliberately out of scope for this PoC.
//
// collectAffectedNodeIds + isHighlightableRequest are PURE (unit-tested in
// presence.test.ts). highlightNodes is the side-effectful Figma call.

// A Figma node id: "123:45", page "0:1", or an instance-sublayer id "I12:3;45:6".
// Permissive on purpose — highlightNodes resolves + filters to current-page scene
// nodes, so a stray non-resolving id is harmless.
const NODE_ID_RE = /^I?\d+:\d+(;\d+:\d+)*$/;

// Read tools conventionally use these verb prefixes; they return node ids too
// (e.g. get_node.data.id) but must NOT move the viewport. A `batch` is treated as
// highlightable (it almost always writes); a read-only batch scrolling once is an
// acceptable PoC edge. Everything else (create_/set_/move_/resize_/import_/…) writes.
const READ_PREFIXES = ["get_", "scan_", "search_", "list_", "export_", "fetch_", "save_"];

// isHighlightableRequest decides whether a completed request should trigger a
// highlight. True for `batch` and any non-read op type; false for read verbs.
export function isHighlightableRequest(type: unknown): boolean {
  if (typeof type !== "string" || type === "") return false;
  if (type === "batch") return true;
  return !READ_PREFIXES.some((p) => type.startsWith(p));
}

// collectAffectedNodeIds walks a handler/batch response and returns the de-duped
// node ids it touched. Shapes covered:
//   • create_* / set_text:  { data: { id } }
//   • bulk/move/resize:      { data: { results: [{ nodeId }, …] } }   (skips per-node errors)
//   • batch / map:           { data: { results: [{ i, type, data }, …] } }  (recurses .data)
// Only the `id` / `nodeId` keys are read (never variableId/styleId/parentId), and a
// per-entry `error` string excludes that entry's id (the op didn't affect that node).
export function collectAffectedNodeIds(result: unknown): string[] {
  const ids = new Set<string>();

  const add = (s: unknown): void => {
    if (typeof s === "string" && NODE_ID_RE.test(s)) ids.add(s);
  };

  const walk = (v: unknown): void => {
    if (v == null) return;
    if (Array.isArray(v)) {
      for (const x of v) walk(x);
      return;
    }
    if (typeof v !== "object") return;
    const obj = v as Record<string, unknown>;
    const errored = typeof obj.error === "string" && obj.error.length > 0;
    if (!errored) {
      add(obj.id);
      add(obj.nodeId);
    }
    // Recurse only into known containers so unrelated id-shaped values can't leak in.
    if (obj.data !== undefined) walk(obj.data);
    if (obj.results !== undefined) walk(obj.results);
  };

  walk(result);
  return [...ids];
}

// ── Multi-agent presence (PoC v2) ─────────────────────────────────────────────
// When an op carries an `origin` label (the acting agent's roster name), the
// plugin records it as a PresenceEvent. The panel shows WHO is working WHERE
// (avatar + last action), and the canvas highlights the UNION of active agents'
// recent nodes — without auto-scrolling (one viewport can't follow N agents).
// These functions are PURE (unit-tested); the Figma side effects live below.

export interface PresenceEvent {
  origin: string;
  nodeIds: string[];
  action: string;
  ts: number;
}

export interface AgentActivity {
  origin: string;
  action: string;
  lastTs: number;
  nodeIds: string[];
  active: boolean;
}

// "Show I'm working", not a full history — a tiny ring buffer is enough.
export const MAX_PRESENCE_EVENTS = 10;
// An agent counts as "active" if it acted within this window; older = dimmed.
export const PRESENCE_ACTIVE_WINDOW_MS = 15000;

// appendEvent returns a NEW array with `event` appended, capped to the last
// `cap` items (ring buffer). Immutable — never mutates its input.
export function appendEvent(
  events: PresenceEvent[],
  event: PresenceEvent,
  cap: number = MAX_PRESENCE_EVENTS,
): PresenceEvent[] {
  const next = [...events, event];
  return next.length > cap ? next.slice(next.length - cap) : next;
}

// actionLabel turns a request type + affected-node count into a short human
// phrase for the panel, e.g. "moved 2 nodes", "created 1 node".
export function actionLabel(type: unknown, nodeCount: number): string {
  const t = typeof type === "string" ? type : "";
  let verb = "edited";
  if (t.startsWith("create_") || t.startsWith("clone_")) verb = "created";
  else if (t.startsWith("move_")) verb = "moved";
  else if (t.startsWith("resize_")) verb = "resized";
  else if (t.startsWith("delete_") || t.startsWith("remove_")) verb = "deleted";
  else if (t.startsWith("import_")) verb = "imported";
  else if (t.startsWith("set_")) verb = "styled";
  const noun = nodeCount === 1 ? "node" : "nodes";
  return `${verb} ${nodeCount} ${noun}`;
}

// activeAgents collapses the event log into the latest activity per origin,
// most-recent first, flagging which are still "active" within the window.
export function activeAgents(
  events: PresenceEvent[],
  now: number,
  windowMs: number = PRESENCE_ACTIVE_WINDOW_MS,
): AgentActivity[] {
  const latest = new Map<string, PresenceEvent>();
  for (const e of events) {
    const prev = latest.get(e.origin);
    if (!prev || e.ts >= prev.ts) latest.set(e.origin, e);
  }
  return [...latest.values()]
    .sort((a, b) => b.ts - a.ts)
    .map((e) => ({
      origin: e.origin,
      action: e.action,
      lastTs: e.ts,
      nodeIds: e.nodeIds,
      active: now - e.ts <= windowMs,
    }));
}

// unionActiveNodeIds collects the de-duped recent node ids across currently
// active agents — the set the canvas highlights (no scroll).
export function unionActiveNodeIds(agents: AgentActivity[]): string[] {
  const ids = new Set<string>();
  for (const a of agents) {
    if (!a.active) continue;
    for (const id of a.nodeIds) ids.add(id);
  }
  return [...ids];
}

// Leading-edge throttle: each top-level request already yields ONE highlight call
// carrying all of its affected nodes, so this only de-bounces rapid back-to-back
// requests from thrashing the viewport.
const HIGHLIGHT_THROTTLE_MS = 150;
let lastHighlightAt = 0;

const pageOf = (node: BaseNode | null): PageNode | null => {
  let n: BaseNode | null = node;
  while (n && n.type !== "PAGE") n = n.parent;
  return n && n.type === "PAGE" ? (n as PageNode) : null;
};

// resolveOnPage turns a list of ids into the SceneNodes that exist on the
// CURRENT page (dropping removed/off-page/non-resolving ids — all harmless).
// pageOf returns null for the document and for a page node itself, so the
// `=== currentPage` test alone narrows to a scene node on the current page.
const resolveOnPage = async (ids: string[]): Promise<SceneNode[]> => {
  const resolved = await Promise.all(ids.map((id) => figma.getNodeByIdAsync(id)));
  return resolved.filter(
    (n): n is SceneNode => !!n && !n.removed && pageOf(n) === figma.currentPage,
  );
};

// highlightNodes selects the given nodes (current page only) and scrolls the
// viewport to fit them. Used for the single-agent (unlabeled) follow path.
// Best-effort: any failure is the caller's to swallow — it must never disturb
// the op's response flow.
export async function highlightNodes(ids: string[]): Promise<void> {
  if (!ids.length) return;
  const now = Date.now();
  if (now - lastHighlightAt < HIGHLIGHT_THROTTLE_MS) return;
  lastHighlightAt = now;

  const onPage = await resolveOnPage(ids);
  if (!onPage.length) return;
  figma.currentPage.selection = onPage;
  figma.viewport.scrollAndZoomIntoView(onPage);
}

// highlightUnion selects the union of active agents' recent nodes WITHOUT
// scrolling — the multi-agent path, where forcing the single viewport to chase
// whichever op ran last would just make the camera thrash. Throttled like
// highlightNodes (shares lastHighlightAt) since both are auto-highlight; under
// rapid back-to-back writes the canvas can briefly lag the panel (which updates
// unthrottled). Like the single-agent path, this takes over the user's selection
// while Watch agent is on — intended, and scoped to that toggle.
export async function highlightUnion(ids: string[]): Promise<void> {
  if (!ids.length) return;
  const now = Date.now();
  if (now - lastHighlightAt < HIGHLIGHT_THROTTLE_MS) return;
  lastHighlightAt = now;

  const onPage = await resolveOnPage(ids);
  if (!onPage.length) return;
  figma.currentPage.selection = onPage; // deliberately NO scrollAndZoomIntoView
}

// focusNodes is the explicit "jump to this agent" action from the panel: select
// + scroll to fit. NOT throttled — a user click should always move the camera.
export async function focusNodes(ids: string[]): Promise<void> {
  if (!ids.length) return;
  const onPage = await resolveOnPage(ids);
  if (!onPage.length) return;
  figma.currentPage.selection = onPage;
  figma.viewport.scrollAndZoomIntoView(onPage);
}

// scrollToNodes pans/zooms to fit the nodes WITHOUT changing selection — used by
// "follow this agent" so the union highlight (selection) is preserved while the
// camera tracks one agent's ongoing work. NOT throttled: every followed op moves
// the camera.
export async function scrollToNodes(ids: string[]): Promise<void> {
  if (!ids.length) return;
  const onPage = await resolveOnPage(ids);
  if (!onPage.length) return;
  figma.viewport.scrollAndZoomIntoView(onPage);
}
