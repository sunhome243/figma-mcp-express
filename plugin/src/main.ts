// Plugin core — entry point, UI bootstrap, and request dispatch.

import { handleReadRequest } from "./read-handlers";
import { handleWriteRequest } from "./write-handlers";
import { handleBatchRequest } from "./batch";
import { statusEquals, type PluginStatus } from "./status";

// Per-session channel id — a short routing token so the server can multiplex
// multiple files. Stable while this plugin instance is open; a different open
// file runs a different plugin instance and gets a different channel, so they
// coexist on the same server port without the connect/disconnect flap.
// This plugin-supplied id is the routing key of record — the server only
// auto-assigns an `auto-N` id when a client connects without one.
const channel = Math.random().toString(36).slice(2, 8);

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
      port: config?.port ?? "1994",
      channel,
    });
    return;
  }
  if (message.type === "resize") {
    figma.ui.resize(message.width, message.height);
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
  }
};
