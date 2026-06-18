// Presence — "watch the agent work" live highlight (shipped 2.3.0).
//
// When enabled, after a write/batch op completes the plugin selects the affected
// node(s) and scrolls the viewport to them, so a human watching the file sees the
// agent's edits land in real time. This is the proven, non-destructive pattern
// (selection + scrollAndZoomIntoView — same as grab/cursor-talk-to-figma-mcp's
// set_focus/set_selections): zero canvas pollution, zero undo entries. Figma gives
// plugins no separate overlay/cursor layer, so a literal floating cursor would mean
// real nodes (layer/undo/sync churn) — deliberately out of scope (a true cursor
// layer is future work; see CHANGELOG 2.3.0 "future" note).
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
// acceptable edge case. Everything else (create_/set_/move_/resize_/import_/…) writes.
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

// ── Multi-agent presence ──────────────────────────────────────────────────────
// When an op carries an `origin` label (the acting agent's roster name), the
// plugin records it as a PresenceEvent. The panel shows WHO is working WHERE
// (avatar + last action), and the canvas highlights the UNION of active agents'
// recent nodes — without auto-scrolling (one viewport can't follow N agents).
// These functions are PURE (unit-tested); the Figma side effects live below.

// A PresenceEvent records ONE labeled op (or a status-only ping). `status` is the
// semantic state (drives the chip colour + decay), `label` is the display string
// already flavored at record time (e.g. "Building…", "📸 Capturing…").
export interface PresenceEvent {
  origin: string;
  // sessionId identifies the orchestrator process that sent this. Presence is keyed
  // by (sessionId, origin) so the same roster name from two sessions does NOT clobber.
  // Empty string = a pre-session-id (old server) event → one shared default bucket.
  sessionId: string;
  nodeIds: string[];
  status: string;
  label: string;
  ts: number;
  // task is the agent's sticky one-sentence narration (set via set_presence). The
  // plugin carries the last value forward across ops (main.ts), so it persists
  // between status pings. Undefined until the agent declares one.
  task?: string;
}

export interface AgentActivity {
  origin: string;
  sessionId: string;
  status: string; // EFFECTIVE status after decay / queued merge
  label: string; // EFFECTIVE display string
  lastTs: number;
  nodeIds: string[];
  active: boolean; // within the active window (used by the union highlight)
  queuePos?: number; // 1-based position when status === "queued"
  task?: string; // sticky one-sentence narration (set_presence)
}

// presenceKey is the identity a presence row is grouped by: (sessionId, origin).
// The "|" separator can't appear in a roster name or hex sessionId, so distinct
// pairs never alias. An empty sessionId folds all old-server events into one bucket.
export function presenceKey(sessionId: string, origin: string): string {
  return `${sessionId}|${origin}`;
}

// sessionAccents maps each DISTINCT non-empty sessionId to an evenly-spaced hue
// (0–359), assigned by SORTED id so different orchestrator sessions are separable
// at a glance. Even spacing (vs hashing a single id) guarantees maximum separation
// and never collides at 2–3 sessions. When 0 or 1 real session is present nobody
// needs disambiguation, so every id maps to null (no accent); the "" old-server
// bucket never gets an accent.
export function sessionAccents(sessionIds: string[]): Map<string, number | null> {
  const distinct = [...new Set(sessionIds.filter((s) => s !== ""))].sort();
  const m = new Map<string, number | null>();
  m.set("", null);
  if (distinct.length <= 1) {
    for (const s of distinct) m.set(s, null);
    return m;
  }
  distinct.forEach((sid, i) => m.set(sid, Math.round((360 * i) / distinct.length)));
  return m;
}

// Presence is stored as the LATEST activity per (sessionId, origin) — a Map keyed by
// presenceKey (see main.ts), not an event log: one row per agent, always current. An
// AUTO status decays on a timer — active (breathing, in the union highlight) within
// ACTIVE, then idle, then away — and is finally REMOVED once it has been quiet past
// REMOVE (so a finished agent disappears on its own instead of lingering).
export const PRESENCE_ACTIVE_WINDOW_MS = 30000; // ≤30s → active (breathing)
export const PRESENCE_AWAY_WINDOW_MS = 60000; // 30–60s → idle, >60s → away
export const PRESENCE_REMOVE_WINDOW_MS = 150000; // >150s quiet (AUTO only) → removed

// LLM-set statuses are STICKY — the orchestrator owns them, so they never decay
// or auto-remove on a timer (auto statuses like building/scanning do). See taxonomy.
const STICKY_STATUSES = new Set([
  "thinking",
  "waiting_review",
  "reviewing",
  "approved",
  "escalated",
  "done",
]);

export function isStickyStatus(status: string): boolean {
  return STICKY_STATUSES.has(status);
}

// isExpired reports whether an agent's last activity is stale enough to drop from
// the roster entirely. Only AUTO statuses expire; sticky (LLM-set) ones persist
// until the orchestrator changes them. The core sweep uses this to self-prune.
export function isExpired(
  status: string,
  ts: number,
  now: number,
  removeWindowMs: number = PRESENCE_REMOVE_WINDOW_MS,
): boolean {
  return !isStickyStatus(status) && now - ts > removeWindowMs;
}

// opStatus maps a request type to the AUTO status it implies (0 tokens — derived
// purely from the op the agent already sends). theming is checked before the
// generic import_ prefix because import_variable/import_style are token work.
export function opStatus(type: unknown): string {
  const t = typeof type === "string" ? type : "";
  if (
    t === "set_bound_variable" ||
    t === "create_variable" ||
    t.startsWith("import_variable") ||
    t.startsWith("import_style")
  )
    return "theming";
  if (t.startsWith("import_")) return "importing";
  if (t === "save_screenshots") return "screenshotting";
  if (
    t.startsWith("get_") ||
    t.startsWith("scan_") ||
    t.startsWith("search_") ||
    t.startsWith("list_") ||
    t.startsWith("fetch_")
  )
    return "scanning";
  return "building"; // create_/set_/move_/resize_/delete_/batch/…
}

// buildingVerb flavors the "building" status by op type so the panel reads with a
// bit of life ("Styling…", "Moving…") instead of a flat "Building…".
function buildingVerb(type: unknown): string {
  const t = typeof type === "string" ? type : "";
  if (t.startsWith("move_")) return "Moving…";
  if (t.startsWith("resize_")) return "Resizing…";
  if (t.startsWith("delete_") || t.startsWith("remove_")) return "Removing…";
  if (t.startsWith("set_")) return "Styling…";
  return "Building…"; // create_/clone_/batch/unknown
}

// statusLabel renders a status into the short display string the panel teletypes.
// ctx.opType flavors "building"; ctx.queuePos numbers a "queued" row.
export function statusLabel(
  status: string,
  ctx: { opType?: unknown; queuePos?: number } = {},
): string {
  switch (status) {
    case "building":
      return buildingVerb(ctx.opType);
    case "importing":
      return "⤵ Importing…";
    case "screenshotting":
      return "📸 Capturing…";
    case "scanning":
      return "🔍 Looking around…";
    case "theming":
      return "🎨 Theming…";
    case "queued":
      return ctx.queuePos ? `Queued · #${ctx.queuePos}` : "Queued";
    case "error":
      return "Hit an error";
    case "idle":
      return "Idle";
    case "away":
      return "💤 Away";
    case "joined":
      return "Joined the file";
    case "thinking":
      return "Thinking…";
    case "waiting_review":
      return "Waiting for review";
    case "reviewing":
      return "Reviewing…";
    case "approved":
      return "Approved ✓";
    case "escalated":
      return "🛑 Escalated";
    case "done":
      return "Done ✓";
    default:
      return status;
  }
}

// derivePresence computes the (status, label, task) to STORE for one op, given the
// agent's prior stored entry. PURE so it is unit-tested directly — main.ts only does
// the I/O around it (read params, key the map, emit, highlight). Precedence:
//   1. explicit LLM-set status (from set_presence / a status ping) wins
//   2. an errored response → "error"
//   3. a set_presence ping with no explicit status KEEPS the prior status — it must
//      NOT run opStatus, whose catch-all would falsely show "Building…" (none → "joined")
//   4. otherwise derive the auto status from the op type
// `task` is sticky: undefined (not sent this call) keeps the prior value.
export function derivePresence(
  prev: { status: string; label: string; task?: string } | undefined,
  input: {
    type: unknown;
    explicitStatus: string;
    hasError: boolean;
    isPresencePing: boolean;
    task?: string;
  },
): { status: string; label: string; task?: string } {
  let status: string;
  let label: string;
  if (input.explicitStatus) {
    status = input.explicitStatus;
    label = statusLabel(status);
  } else if (input.hasError) {
    status = "error";
    label = statusLabel("error");
  } else if (input.isPresencePing) {
    status = prev?.status ?? "joined";
    label = prev?.label ?? statusLabel(status);
  } else {
    status = opStatus(input.type);
    label = statusLabel(status, { opType: input.type });
  }
  const task = input.task !== undefined ? input.task : prev?.task;
  return { status, label, task };
}

// activeAgents collapses the event log into the latest activity per origin,
// most-recent first. AUTO statuses decay on a timer (active → idle → away);
// STICKY (LLM-set) statuses are preserved. The latest NON-EMPTY nodeIds per
// origin are carried forward so a status-only ping (empty nodeIds) doesn't break
// jump/follow for that agent.
export function activeAgents(
  events: PresenceEvent[],
  now: number,
  opts: { activeWindowMs?: number; awayWindowMs?: number } = {},
): AgentActivity[] {
  const activeWindowMs = opts.activeWindowMs ?? PRESENCE_ACTIVE_WINDOW_MS;
  const awayWindowMs = opts.awayWindowMs ?? PRESENCE_AWAY_WINDOW_MS;

  // Group by (sessionId, origin), NOT origin alone — the same roster name from two
  // sessions is two distinct agents and must not collapse onto one row.
  const latest = new Map<string, PresenceEvent>();
  const latestNodes = new Map<string, { ts: number; nodeIds: string[] }>();
  for (const e of events) {
    const key = presenceKey(e.sessionId, e.origin);
    const prev = latest.get(key);
    if (!prev || e.ts >= prev.ts) latest.set(key, e);
    if (e.nodeIds.length) {
      const pn = latestNodes.get(key);
      if (!pn || e.ts >= pn.ts) latestNodes.set(key, { ts: e.ts, nodeIds: e.nodeIds });
    }
  }

  return [...latest.values()]
    .sort((a, b) => b.ts - a.ts)
    .map((e) => {
      const age = now - e.ts;
      const active = age <= activeWindowMs;
      let status = e.status;
      let label = e.label;
      if (!isStickyStatus(e.status)) {
        // AUTO status decays once its op is stale.
        if (age > awayWindowMs) {
          status = "away";
          label = statusLabel("away");
        } else if (age > activeWindowMs) {
          status = "idle";
          label = statusLabel("idle");
        }
      }
      return {
        origin: e.origin,
        sessionId: e.sessionId,
        status,
        label,
        lastTs: e.ts,
        nodeIds: latestNodes.get(presenceKey(e.sessionId, e.origin))?.nodeIds ?? e.nodeIds,
        active,
        task: e.task,
      };
    });
}

// mergeQueued folds the server's currently-waiting origins into the activity
// list. An origin that is actively building keeps its building row; otherwise it
// is shown as "queued" (overriding a stale row, or added if it has none). queuePos
// is its 1-based slot in the server's waiting order.
export function mergeQueued(
  agents: AgentActivity[],
  queuedOrigins: string[],
  now: number,
): AgentActivity[] {
  const posOf = (o: string): number => queuedOrigins.indexOf(o) + 1;
  const queuedSet = new Set(queuedOrigins);

  const overridden = agents.map((a) =>
    queuedSet.has(a.origin) && !a.active
      ? {
          ...a,
          status: "queued",
          label: statusLabel("queued", { queuePos: posOf(a.origin) }),
          queuePos: posOf(a.origin),
        }
      : a,
  );

  const known = new Set(agents.map((a) => a.origin));
  const added: AgentActivity[] = queuedOrigins
    .filter((o) => !known.has(o))
    .map((o) => ({
      origin: o,
      // The server queue frame is still origin-only; a queued row with no prior
      // activity has no known sessionId yet (default bucket). Session-qualifying the
      // queue is a follow-up (Go PresenceQueueFrame must carry sessionId per entry).
      sessionId: "",
      status: "queued",
      label: statusLabel("queued", { queuePos: posOf(o) }),
      lastTs: now,
      nodeIds: [],
      active: false,
      queuePos: posOf(o),
    }));

  return [...overridden, ...added];
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
