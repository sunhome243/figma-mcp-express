// Plugin core — entry point, UI bootstrap, and request dispatch.

import { handleReadRequest } from "./read-handlers";
import { handleWriteRequest } from "./write-handlers";
import { handleBatchRequest } from "./batch";
import { statusEquals, type PluginStatus } from "./status";
import {
  activeAgents,
  collectAffectedNodeIds,
  derivePresence,
  focusNodes,
  highlightNodes,
  highlightUnion,
  isExpired,
  isHighlightableRequest,
  mergeQueued,
  presenceKey,
  scrollToNodes,
  unionActiveNodeIds,
  type PresenceEvent,
} from "./presence";

// Per-session channel id — a short routing token so the server can multiplex
// multiple files. Stable while this plugin instance is open; a different open
// file runs a different plugin instance and gets a different channel, so they
// coexist on the same server port without the connect/disconnect flap.
// This plugin-supplied id is the routing key of record — the server only
// auto-assigns an `auto-N` id when a client connects without one.
const channel = Math.random().toString(36).slice(2, 8);

// Presence ("watch the agent") toggle — when on, a completed write/batch selects
// + scrolls to the affected nodes so a human sees the agent's edits land live.
// Persisted per plugin id via clientStorage; hydrated on boot below.
let presenceEnabled = false;

// Multi-agent presence — the LATEST activity per agent (NOT an event log), keyed by
// presenceKey(sessionId, origin) so the same roster name from two sessions stays two
// rows: one row per agent, always current. Drives the panel's per-agent list and the
// canvas union-highlight. An AUTO status decays (active → idle → away) and is
// self-pruned once quiet past the remove window (see the sweep below); sticky LLM-set
// statuses persist. Reset when presence is off.
const presenceByKey = new Map<string, PresenceEvent>();

// Server-reported queue — origins currently waiting in the server's per-channel
// queue (forwarded from the server frame via a `presence_queue` UI message).
// Folded into the roster by mergeQueued so waiting agents show as "Queued · #N".
let queuedOrigins: string[] = [];

// "Follow" mode — the (sessionId, origin) composite key of the agent the viewport
// tracks (camera-only; the union selection is unchanged). null = no follow. Keyed by
// the composite, NOT origin, so following one "grace" doesn't track another session's.
let followKey: string | null = null;

// emitPresence posts the merged roster (decayed activity + server queue) to the
// UI. The single place a `presence_update` is emitted from a live event.
const emitPresence = () => {
  // Best-effort: a postMessage can throw if the UI was torn down mid-reload. Never
  // let presence bookkeeping surface an uncaught rejection from the async onmessage
  // handler — it must never affect the op reply.
  try {
    const now = Date.now();
    const agents = mergeQueued(
      activeAgents([...presenceByKey.values()], now),
      queuedOrigins,
      now,
    );
    figma.ui.postMessage({ type: "presence_update", agents });
  } catch (e) {
    console.warn("[presence] emit failed:", e);
  }
};

// Periodic sweep, only while presence is on: self-prune agents quiet past the remove
// window (AUTO statuses only — sticky LLM-set ones persist), then re-emit so idle→away
// decay updates live even with no new op. Pruned agents drop from the roster, so a
// later return re-triggers the join animation (a genuine re-join, vs a still-shown
// away agent that just wakes). Cheap: the map is bounded by the roster size.
const PRESENCE_SWEEP_MS = 5000;
let presenceSweep: ReturnType<typeof setInterval> | null = null;
const startPresenceSweep = () => {
  if (presenceSweep !== null) return;
  presenceSweep = setInterval(() => {
    const now = Date.now();
    let changed = false;
    for (const [key, e] of presenceByKey) {
      if (isExpired(e.status, e.ts, now)) {
        presenceByKey.delete(key);
        changed = true;
      }
    }
    if (changed || presenceByKey.size > 0 || queuedOrigins.length > 0) emitPresence();
  }, PRESENCE_SWEEP_MS);
};
const stopPresenceSweep = () => {
  if (presenceSweep !== null) {
    clearInterval(presenceSweep);
    presenceSweep = null;
  }
};

// Idle-stutter guard: `selectionchange`/`currentpagechange` fire 10-100×/sec
// during a pan/drag storm, and each raw send is 4 synchronous Figma reads + an
// IPC postMessage on the single-threaded plugin loop — starving Figma's
// renderer. We collapse that storm two ways:
//   1. Trailing debounce — a burst becomes ONE send after the canvas settles.
//   2. Diff-before-send — the send is skipped entirely when nothing the server
//      cares about (file/page/selection-count) actually changed.
const STATUS_DEBOUNCE_MS = 300;
let lastSent: PluginStatus | null = null;
let statusTimer: ReturnType<typeof setTimeout> | null = null;

const readStatus = (): PluginStatus => ({
  fileName: figma.root.name,
  fileKey: figma.fileKey ?? "",
  pageName: figma.currentPage.name,
  selectionCount: figma.currentPage.selection.length,
});

// Read → diff → (maybe) post. The single place that actually emits a status.
const flushStatus = () => {
  const next = readStatus();
  if (statusEquals(lastSent, next)) return;
  lastSent = next;
  figma.ui.postMessage({ type: "plugin-status", payload: next });
};

// Trailing-debounced status send for high-frequency listeners (selection/page).
const sendStatus = () => {
  if (statusTimer !== null) clearTimeout(statusTimer);
  statusTimer = setTimeout(() => {
    statusTimer = null;
    flushStatus();
  }, STATUS_DEBOUNCE_MS);
};

const handleRequest = async (request: any) => {
  try {
    // set_presence is a presence-only command — NO Figma mutation. The panel update
    // (status/task) happens in the server-request presence block below; here we only
    // acknowledge so the round-trip completes. Kept out of the read/write/batch chain
    // so opStatus never auto-flavors it as "Building…".
    if (request.type === "set_presence") {
      return { type: "set_presence", requestId: request.requestId, data: { ok: true } };
    }
    // Per-op perf toggle (global mutable Figma flag). Reset on EVERY request so a
    // prior op's `true` never leaks into a later op that omitted it (single-thread
    // + serial slot = no race). Omitted → false = current/non-breaking semantics.
    // batch handles its own inner ops (see batch.ts); this covers standalone ops.
    figma.skipInvisibleInstanceChildren =
      request.params?.skipInvisibleInstanceChildren === true;
    const result =
      (await handleBatchRequest(request)) ??
      (await handleReadRequest(request)) ??
      (await handleWriteRequest(request));
    if (result === null)
      throw new Error(`Unknown request type: ${request.type}`);
    return result;
  } catch (error) {
    return {
      type: request.type,
      requestId: request.requestId,
      error: error instanceof Error ? error.message : String(error),
    };
  }
};

figma.showUI(__html__, { width: 320, height: 245 });
// Initial + ui-ready sends are immediate (not debounced) so the panel and the
// server registration populate without a 300ms lag; the debounce only guards
// the high-frequency selection/page listeners below.
flushStatus();

// Hydrate the presence toggle from clientStorage (best-effort; defaults to off).
// get_presence awaits this so a persisted ON state can't be reported as OFF when
// the UI's mount-time query races the async read.
const presenceHydrated = figma.clientStorage
  .getAsync("presence_enabled")
  .then((v) => {
    presenceEnabled = v === true;
    if (presenceEnabled) startPresenceSweep(); // persisted ON → live decay/prune
  })
  .catch((e) => {
    console.warn("[presence] hydrate failed:", e);
  });

figma.on("selectionchange", () => {
  sendStatus();
});

figma.on("currentpagechange", () => {
  sendStatus();
});

figma.ui.onmessage = async (message) => {
  if (message.type === "ui-ready") {
    // Force a re-send on reconnect even if the status is unchanged, so the
    // server re-registers this channel after a socket drop.
    lastSent = null;
    flushStatus();
    return;
  }
  if (message.type === "get_ws_config") {
    const config = await figma.clientStorage.getAsync("ws_config");
    figma.ui.postMessage({
      type: "ws_config",
      host: config?.host ?? "127.0.0.1",
      port: config?.port ?? __DEFAULT_PORT__,
      channel,
    });
    return;
  }
  if (message.type === "resize") {
    figma.ui.resize(message.width, message.height);
    return;
  }
  if (message.type === "get_presence") {
    await presenceHydrated; // ensure clientStorage hydration finished first
    figma.ui.postMessage({ type: "presence_state", enabled: presenceEnabled });
    return;
  }
  if (message.type === "set_presence") {
    presenceEnabled = message.enabled === true;
    await figma.clientStorage.setAsync("presence_enabled", presenceEnabled);
    if (presenceEnabled) {
      startPresenceSweep();
    } else {
      // Turning it off clears the roster + follow so the panel resets immediately.
      stopPresenceSweep();
      presenceByKey.clear();
      queuedOrigins = [];
      followKey = null;
      figma.ui.postMessage({ type: "presence_update", agents: [] });
      figma.ui.postMessage({ type: "follow_state", key: null });
    }
    return;
  }
  if (message.type === "presence_queue") {
    queuedOrigins = Array.isArray(message.origins)
      ? message.origins.filter((o: unknown) => typeof o === "string")
      : [];
    if (presenceEnabled) emitPresence();
    return;
  }
  if (message.type === "jump_to_agent") {
    // Panel "jump" click: move the viewport to one agent's most recent nodes. Match
    // by the (sessionId, origin) composite so a shared roster name can't mis-target.
    const target = presenceKey(
      typeof message.sessionId === "string" ? message.sessionId : "",
      typeof message.origin === "string" ? message.origin : "",
    );
    const agent = activeAgents([...presenceByKey.values()], Date.now()).find(
      (a) => presenceKey(a.sessionId, a.origin) === target,
    );
    if (agent && agent.nodeIds.length) {
      try {
        await focusNodes(agent.nodeIds);
      } catch (e) {
        console.warn("[presence] jump failed:", e);
      }
    }
    return;
  }
  if (message.type === "set_follow") {
    // Toggle "follow this agent": clicking the followed agent again clears it. Identity
    // is the (sessionId, origin) composite key, so two same-name agents are distinct.
    const target =
      typeof message.origin === "string"
        ? presenceKey(typeof message.sessionId === "string" ? message.sessionId : "", message.origin)
        : null;
    followKey = followKey === target ? null : target;
    figma.ui.postMessage({ type: "follow_state", key: followKey });
    // Jump to the followed agent's latest work right away (select + scroll).
    if (followKey) {
      const agent = activeAgents([...presenceByKey.values()], Date.now()).find(
        (a) => presenceKey(a.sessionId, a.origin) === followKey,
      );
      if (agent && agent.nodeIds.length) {
        try {
          await focusNodes(agent.nodeIds);
        } catch (e) {
          console.warn("[presence] follow jump failed:", e);
        }
      }
    }
    return;
  }
  if (message.type === "save_ws_config") {
    await figma.clientStorage.setAsync("ws_config", {
      host: message.host,
      port: message.port,
    });
    return;
  }
  if (message.type === "server-request") {
    const response = await handleRequest(message.payload);
    try {
      figma.ui.postMessage(response);
    } catch (err) {
      figma.ui.postMessage({
        type: response.type,
        requestId: response.requestId,
        error: err instanceof Error ? err.message : String(err),
      });
    }
    // Presence — strictly best-effort AFTER the response is posted, so it can never
    // delay or break the op's reply. LABELED ops (carrying `origin`) record presence
    // for ANY op (incl. non-mutating reads, so scanning/screenshotting/theming/error
    // surface); highlighting stays gated to mutating ops that touched nodes.
    if (presenceEnabled && response) {
      const type = message.payload?.type;
      const params = message.payload?.params;
      const origin = typeof params?.origin === "string" ? params.origin : "";
      // sessionId identifies the orchestrator process. Presence is keyed by
      // (sessionId, origin) so the same roster name from two sessions never clobbers.
      // Absent (old server) → "" → one shared default bucket (graceful degradation).
      const sessionId = typeof params?.sessionId === "string" ? params.sessionId : "";
      const explicitStatus =
        typeof params?.status === "string" ? params.status : "";
      const isPresencePing = type === "set_presence";
      // task is the sticky one-sentence narration. Present this call → update it;
      // absent → undefined, so we carry the prior value forward (sticky) below.
      const task = typeof params?.task === "string" ? params.task : undefined;
      // An explicit status ping (or set_presence) is STATUS-ONLY: ignore any node ids
      // its carrier op returned, so it carries forward the agent's prior nodeIds and
      // never hijacks the selection / triggers a highlight.
      const ids =
        response.error || explicitStatus || isPresencePing
          ? []
          : collectAffectedNodeIds(response);

      if (origin) {
        const key = presenceKey(sessionId, origin);
        const prev = presenceByKey.get(key);

        // Status/label/task decision is the PURE, unit-tested derivePresence (see
        // presence.test.ts). main.ts only does the I/O around it.
        const { status, label, task: nextTask } = derivePresence(prev, {
          type,
          explicitStatus,
          hasError: !!response.error,
          isPresencePing,
          task,
        });

        // Update this (sessionId, origin)'s single latest-activity entry. Carry
        // forward the prior nodeIds on a status-only ping (empty ids) so jump/follow
        // still works. NOTE: a task-only set_presence ping refreshes `ts`, which
        // resets the auto-decay clock — intentional: an agent narrating its task is
        // clearly still alive, so it should not decay to idle mid-narration.
        const nodeIds = ids.length ? ids : (prev?.nodeIds ?? []);
        presenceByKey.set(key, {
          origin,
          sessionId,
          nodeIds,
          status,
          label,
          ts: Date.now(),
          task: nextTask,
        });

        // This op COMPLETED → the agent is no longer waiting on the serial slot, so
        // clear it from the server-reported queue LOCALLY. Defense-in-depth behind the
        // server's queue broadcast: that clear-broadcast is best-effort and TryLock-
        // dropped when the op's own write holds the socket (and the server now retries
        // it — see broadcastQueue), but clearing on activity here is reliable and
        // independent of any dropped frame, so an actively-working agent can never stay
        // pinned to "queued".
        const wasQueued = queuedOrigins.includes(origin);
        if (wasQueued) queuedOrigins = queuedOrigins.filter((o) => o !== origin);

        // Emit only on a MEANINGFUL change — a status transition, an explicit ping, a
        // task change, a queue clear, or a mutating op. Repeated same-status reads
        // (scanning) just refresh the entry; the periodic sweep re-emits so they don't
        // flood the UI.
        const meaningful =
          !prev ||
          prev.status !== status ||
          wasQueued ||
          !!explicitStatus ||
          isPresencePing ||
          wasQueued ||
          (task !== undefined && task !== prev?.task) ||
          (ids.length > 0 && isHighlightableRequest(type));
        if (meaningful) emitPresence();

        // Highlight only for mutating ops that actually touched nodes.
        if (ids.length && isHighlightableRequest(type)) {
          try {
            const agents = activeAgents([...presenceByKey.values()], Date.now());
            await highlightUnion(unionActiveNodeIds(agents));
            if (followKey && key === followKey) await scrollToNodes(ids);
          } catch (e) {
            console.warn("[presence] highlight failed:", e);
          }
        }
      } else if (!response.error && isHighlightableRequest(type) && ids.length) {
        // Unlabeled mutating op → legacy single-agent follow (select + scroll).
        try {
          await highlightNodes(ids);
        } catch (e) {
          console.warn("[presence] highlight failed:", e);
        }
      }
    }
  }
};
