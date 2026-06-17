// Plugin core — entry point, UI bootstrap, and request dispatch.

import { handleReadRequest } from "./read-handlers";
import { handleWriteRequest } from "./write-handlers";
import { handleBatchRequest } from "./batch";
import { statusEquals, type PluginStatus } from "./status";
import {
  actionLabel,
  activeAgents,
  appendEvent,
  collectAffectedNodeIds,
  focusNodes,
  highlightNodes,
  highlightUnion,
  isHighlightableRequest,
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

// Multi-agent presence log — a tiny ring buffer of recent labeled ops, keyed by
// the `origin` each agent stamps on its calls. Drives the panel's per-agent list
// and the canvas union-highlight. Reset when presence is turned off.
let presenceEvents: PresenceEvent[] = [];

// "Follow" mode — when set to an origin, the viewport tracks that agent's every
// subsequent op (camera-only; the union selection is unchanged). null = no follow.
let followOrigin: string | null = null;

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
    // Turning it off clears the roster + follow so the panel resets immediately.
    if (!presenceEnabled) {
      presenceEvents = [];
      followOrigin = null;
      figma.ui.postMessage({ type: "presence_update", agents: [] });
      figma.ui.postMessage({ type: "follow_state", origin: null });
    }
    return;
  }
  if (message.type === "jump_to_agent") {
    // Panel "jump" click: move the viewport to one agent's most recent nodes.
    const agent = activeAgents(presenceEvents, Date.now()).find(
      (a) => a.origin === message.origin,
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
    // Toggle "follow this agent": clicking the followed agent again clears it.
    const target = typeof message.origin === "string" ? message.origin : null;
    followOrigin = followOrigin === target ? null : target;
    figma.ui.postMessage({ type: "follow_state", origin: followOrigin });
    // Jump to the followed agent's latest work right away (select + scroll).
    if (followOrigin) {
      const agent = activeAgents(presenceEvents, Date.now()).find(
        (a) => a.origin === followOrigin,
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
    // Presence highlight — strictly best-effort AFTER the response is posted, so it
    // can never delay or break the op's reply. Only for successful, highlightable
    // (write/batch) ops; read verbs and errors are skipped.
    if (
      presenceEnabled &&
      response &&
      !response.error &&
      isHighlightableRequest(message.payload?.type)
    ) {
      try {
        const ids = collectAffectedNodeIds(response);
        const origin =
          typeof message.payload?.params?.origin === "string"
            ? message.payload.params.origin
            : "";
        if (origin && ids.length) {
          // Labeled op → record presence, refresh the panel, highlight the union
          // of active agents' nodes (no scroll — the viewport can't chase N agents).
          presenceEvents = appendEvent(presenceEvents, {
            origin,
            nodeIds: ids,
            action: actionLabel(message.payload?.type, ids.length),
            ts: Date.now(),
          });
          const agents = activeAgents(presenceEvents, Date.now());
          figma.ui.postMessage({ type: "presence_update", agents });
          await highlightUnion(unionActiveNodeIds(agents));
          // If following this agent, track its camera too (selection stays union).
          if (followOrigin && origin === followOrigin) {
            await scrollToNodes(ids);
          }
        } else if (ids.length) {
          // Unlabeled op → legacy single-agent follow (select + scroll).
          await highlightNodes(ids);
        }
      } catch (e) {
        console.warn("[presence] highlight failed:", e);
      }
    }
  }
};
