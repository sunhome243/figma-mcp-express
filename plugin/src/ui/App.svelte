<script lang="ts">
  import { onMount } from "svelte";
  import { nextReconnectDelay } from "../status";

  let connected = false;
  let fileName = "—";
  let fileKey = "";
  let pageName = "—";
  let selectionCount = 0;
  let channel = ""; // routing id for this file — shown so the user can tell Claude
  let activeRequests = new Set<string>();
  $: isWorking = activeRequests.size > 0;

  // Configurable server address.
  // Persisted via figma.clientStorage (through plugin core) because localStorage
  // is unavailable inside Figma's data: URL sandbox.
  let serverHost = "127.0.0.1";
  let serverPort = "1994";

  let showSettings = false;
  let editHost = serverHost;
  let editPort = serverPort;
  let minimized = false;

  const FULL_W = 320, FULL_H = 245;
  const PILL_W = 210, PILL_H = 36;

  // Tag every UI→plugin message with our pluginId (the manifest id). Figma routes
  // pluginMessage to the plugin code only when pluginId matches, so another plugin
  // or a navigated iframe cannot intercept these messages (which carry the WS
  // host/port config). See developers.figma.com/docs/plugins/creating-ui.
  const PLUGIN_ID = "figma-mcp-express";
  function postToPlugin(message: unknown) {
    parent.postMessage({ pluginMessage: message, pluginId: PLUGIN_ID }, "*");
  }

  function toggleMinimize() {
    minimized = !minimized;
    postToPlugin({
      type: "resize",
      width: minimized ? PILL_W : FULL_W,
      height: minimized ? PILL_H : FULL_H,
    });
  }

  let socket: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectAttempt = 0; // grows while the server is down → exponential backoff
  let configLoaded = false;

  // Best-effort: when the plugin iframe is backgrounded (document.hidden), slow
  // reconnect attempts further so a down server doesn't spin a hidden tab. Falls
  // back to normal cadence where document.hidden isn't reliable.
  const BACKGROUND_RECONNECT_MS = 60000;
  function reconnectDelay(): number {
    const base = nextReconnectDelay(reconnectAttempt);
    const hidden =
      typeof document !== "undefined" && document.hidden === true;
    return hidden ? Math.max(base, BACKGROUND_RECONNECT_MS) : base;
  }

  function scheduleReconnect() {
    if (reconnectTimer !== null) return;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      reconnectAttempt++;
      connect();
    }, reconnectDelay());
  }

  // Tell the server which file this channel is, so list_channels can show it.
  function sendRegister(ws: WebSocket) {
    if (ws.readyState !== WebSocket.OPEN) return;
    ws.send(
      JSON.stringify({ type: "__register__", data: { fileName, fileKey, pageName } }),
    );
  }

  function connect() {
    // Detach the old handler before closing so its onclose doesn't fire
    // after we've already assigned a new socket, which would null out the
    // new reference and silently break the connection.
    if (socket) {
      socket.onclose = null;
      socket.close();
    }
    const ws = new WebSocket(
      `ws://${serverHost}:${serverPort}/ws?channel=${encodeURIComponent(channel)}`,
    );
    socket = ws;

    ws.onopen = () => {
      connected = true;
      reconnectAttempt = 0; // reset backoff on a successful connect
      sendRegister(ws);
      postToPlugin({ type: "ui-ready" });
    };

    ws.onclose = () => {
      if (socket !== ws) return; // stale handler — a newer connect() already took over
      connected = false;
      socket = null;
      activeRequests.clear();
      activeRequests = activeRequests;
      scheduleReconnect();
    };

    ws.onerror = () => {
      connected = false;
    };

    ws.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.requestId) {
          activeRequests.add(payload.requestId);
          activeRequests = activeRequests;
        }
        postToPlugin({ type: "server-request", payload });
      } catch {
        // ignore malformed frames
      }
    };
  }

  function handleMessage(event: MessageEvent) {
    const msg = event.data?.pluginMessage;
    if (!msg) return;

    if (msg.type === "ws_config") {
      serverHost = msg.host ?? "127.0.0.1";
      serverPort = msg.port ?? "1994";
      channel = msg.channel ?? channel;
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
      return;
    }

    if (msg.type === "plugin-status") {
      fileName = msg.payload.fileName;
      fileKey = msg.payload.fileKey ?? "";
      pageName = msg.payload.pageName ?? "—";
      selectionCount = msg.payload.selectionCount;
      // File metadata may arrive after the socket opened — refresh the server's
      // record for this channel so list_channels shows the right file.
      if (socket) sendRegister(socket);
      return;
    }

    if ("requestId" in msg) {
      if (msg.type !== "progress_update") {
        activeRequests.delete(msg.requestId);
        activeRequests = activeRequests;
      }
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify(msg));
      }
    }
  }

  function openSettings() {
    editHost = serverHost;
    editPort = serverPort;
    showSettings = true;
  }

  function applySettings() {
    serverHost = editHost.trim() || "127.0.0.1";
    const p = parseInt(editPort, 10);
    serverPort = p > 0 && p <= 65535 ? String(p) : "1994";
    // Persist via plugin core (figma.clientStorage), since localStorage is
    // unavailable in Figma's data: URL environment.
    postToPlugin({ type: "save_ws_config", host: serverHost, port: serverPort });
    showSettings = false;
    // Cancel any pending reconnect and reconnect immediately with the new address.
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    reconnectAttempt = 0; // user-initiated reconnect → fresh backoff schedule
    connect();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Enter") applySettings();
    if (event.key === "Escape") showSettings = false;
  }

  onMount(() => {
    window.addEventListener("message", handleMessage);

    // Request stored config from plugin core (responds with ws_config message).
    // connect() is called once we receive the response.
    postToPlugin({ type: "get_ws_config" });

    // Fallback: if the plugin core doesn't respond within 500 ms (e.g. during
    // dev / hot-reload without a running core), connect with defaults.
    const fallback = setTimeout(() => {
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
    }, 500);

    return () => {
      clearTimeout(fallback);
      window.removeEventListener("message", handleMessage);
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      if (socket) socket.close();
    };
  });
</script>

{#if minimized}
  <!-- ── Pill mode ── -->
  <button class="pill" class:connected class:disconnected={!connected} on:click={toggleMinimize} title="Click to expand">
    <span class="dot"></span>
    <span>{connected ? "Connected" : "Disconnected"}</span>
    {#if channel}<span class="pill-ch">#{channel}</span>{/if}
    <svg width="9" height="9" viewBox="0 0 9 9" fill="none" style="opacity:.45;margin-left:2px">
      <path d="M1 8L8 1M5 1h3v3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>
  </button>
{:else}
<div class="app">

  <!-- Header: status + minimize -->
  <header>
    <div class="status-pill" class:connected class:disconnected={!connected}>
      <span class="dot"></span>
      <span>{connected ? "Connected" : "Disconnected"}</span>
      {#if isWorking}<div class="spinner" style="margin-left:4px"></div>{/if}
    </div>
    <button class="min-btn" on:click={toggleMinimize} title="Minimize to pill">
      <svg width="10" height="2" viewBox="0 0 10 2" fill="none">
        <rect width="10" height="2" rx="1" fill="currentColor"/>
      </svg>
    </button>
  </header>

  <!-- File + page -->
  <main>
    <div class="field">
      <span class="field-label">File</span>
      <span class="field-value" title={fileName}>{fileName}</span>
    </div>
    <div class="field">
      <span class="field-label">Page</span>
      <span class="field-value muted" title={pageName}>{pageName}</span>
    </div>
  </main>

  <!-- Footer -->
  <footer>
    <div class="footer-meta">
      <a class="author" href="https://github.com/sunhome243/figma-mcp-express" target="_blank">
        <img src="https://avatars.githubusercontent.com/sunhome243?v=4" alt="avatar" />
        sunhome243
      </a>
      {#if showSettings}
        <div class="settings-panel">
          <input class="addr-input" bind:value={editHost} placeholder="127.0.0.1" on:keydown={handleKeydown} />
          <span class="sep">:</span>
          <input class="port-input" bind:value={editPort} placeholder="1994" on:keydown={handleKeydown} />
          <button class="confirm-btn" on:click={applySettings}>✓</button>
          <button class="dismiss-btn" on:click={() => showSettings = false}>✕</button>
        </div>
      {:else}
        <button class="settings-btn" on:click={openSettings} title="Server settings">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor"><path d="M8 4.754a3.246 3.246 0 1 0 0 6.492 3.246 3.246 0 0 0 0-6.492zM5.754 8a2.246 2.246 0 1 1 4.492 0 2.246 2.246 0 0 1-4.492 0z"/><path d="M9.796 1.343c-.527-1.79-3.065-1.79-3.592 0l-.094.319a.873.873 0 0 1-1.255.52l-.292-.16c-1.64-.892-3.433.902-2.54 2.541l.159.292a.873.873 0 0 1-.52 1.255l-.319.094c-1.79.527-1.79 3.065 0 3.592l.319.094a.873.873 0 0 1 .52 1.255l-.16.292c-.892 1.64.901 3.434 2.541 2.54l.292-.159a.873.873 0 0 1 1.255.52l.094.319c.527 1.79 3.065 1.79 3.592 0l.094-.319a.873.873 0 0 1 1.255-.52l.292.16c1.64.893 3.434-.902 2.54-2.541l-.159-.292a.873.873 0 0 1 .52-1.255l.319-.094c1.79-.527 1.79-3.065 0-3.592l-.319-.094a.873.873 0 0 1-.52-1.255l.16-.292c.893-1.64-.902-3.433-2.541-2.54l-.292.159a.873.873 0 0 1-1.255-.52l-.094-.319zm-2.633.283c.246-.835 1.428-.835 1.674 0l.094.319a1.873 1.873 0 0 0 2.693 1.115l.291-.16c.764-.415 1.6.42 1.184 1.185l-.159.292a1.873 1.873 0 0 0 1.116 2.692l.318.094c.835.246.835 1.428 0 1.674l-.319.094a1.873 1.873 0 0 0-1.115 2.693l.16.291c.415.764-.42 1.6-1.185 1.184l-.291-.159a1.873 1.873 0 0 0-2.693 1.116l-.094.318c-.246.835-1.428.835-1.674 0l-.094-.319a1.873 1.873 0 0 0-2.692-1.115l-.292.16c-.764.415-1.6-.42-1.184-1.185l.159-.291A1.873 1.873 0 0 0 1.945 8.93l-.319-.094c-.835-.246-.835-1.428 0-1.674l.319-.094A1.873 1.873 0 0 0 3.06 4.377l-.16-.292c-.415-.764.42-1.6 1.185-1.184l.292.159a1.873 1.873 0 0 0 2.692-1.115l.094-.319z"/></svg>
        </button>
      {/if}
    </div>
    <div class="footer-actions">
      <a class="action-btn" href="https://github.com/sunhome243/figma-mcp-express/issues/new?labels=bug" target="_blank">Found a bug</a>
      <a class="action-btn" href="https://github.com/sunhome243/figma-mcp-express/issues/new?labels=enhancement&title=Feature+request%3A+" target="_blank">I have a suggestion</a>
    </div>
  </footer>

</div>
{/if}

<style>
  :global(*) { box-sizing: border-box; margin: 0; padding: 0; }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    font-size: 12px;
    background: #fff;
    color: #1e1e1e;
    height: 100vh;
    -webkit-font-smoothing: antialiased;
  }

  /* ── pill mode ── */
  .pill {
    position: fixed;
    inset: 0;
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    background: #fff;
    border: none;
    border-bottom: 1.5px solid #f0f0f0;
    font-family: inherit;
    font-size: 11px;
    font-weight: 500;
    color: #aaa;
    cursor: pointer;
    transition: background 0.15s;
    padding: 0 12px;
  }

  .pill.connected { color: #0d8f54; }
  .pill:hover { background: #fafafa; }

  .pill-ch {
    font-size: 10px;
    opacity: 0.5;
    font-family: ui-monospace, monospace;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
    flex-shrink: 0;
  }

  /* ── full app ── */
  .app {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  /* ── header ── */
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-bottom: 1px solid #f2f2f2;
    gap: 8px;
  }

  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    font-weight: 500;
    padding: 5px 11px 5px 9px;
    border-radius: 100px;
    background: #f5f5f5;
    color: #bbb;
    border: 1px solid #ebebeb;
    transition: background 0.2s, color 0.2s, border-color 0.2s;
  }

  .status-pill.connected {
    background: #edfaf4;
    color: #0d8f54;
    border-color: #bdecd3;
  }

  .spinner {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    border: 1.5px solid currentColor;
    border-top-color: transparent;
    opacity: 0.5;
    animation: spin 0.75s linear infinite;
    flex-shrink: 0;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

  /* minimize button — clearly visible */
  .min-btn {
    width: 28px;
    height: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #f5f5f5;
    border: 1px solid #e8e8e8;
    border-radius: 7px;
    color: #999;
    cursor: pointer;
    flex-shrink: 0;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
  }

  .min-btn:hover {
    background: #ebebeb;
    border-color: #d8d8d8;
    color: #444;
  }

  /* ── main ── */
  main {
    padding: 16px 14px 12px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .field-label {
    font-size: 10px;
    font-weight: 600;
    color: #c8c8c8;
    text-transform: uppercase;
    letter-spacing: 0.07em;
  }

  .field-value {
    font-size: 13px;
    font-weight: 500;
    color: #1e1e1e;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    line-height: 1.35;
  }

  .field-value.muted {
    font-weight: 400;
    color: #777;
    font-size: 12px;
  }

  /* ── footer ── */
  footer {
    border-top: 1px solid #f2f2f2;
    padding: 10px 14px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .footer-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .author {
    display: flex;
    align-items: center;
    gap: 7px;
    text-decoration: none;
    color: #888;
    font-size: 12px;
    font-weight: 500;
    transition: color 0.15s;
  }

  .author:hover { color: #333; }

  .author img {
    width: 20px;
    height: 20px;
    border-radius: 50%;
  }

  /* gear settings button */
  .settings-btn {
    background: none;
    border: none;
    color: #ccc;
    cursor: pointer;
    padding: 4px;
    border-radius: 5px;
    display: flex;
    align-items: center;
    transition: color 0.15s, background 0.15s;
  }

  .settings-btn:hover { color: #888; background: #f5f5f5; }

  .settings-panel {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 1;
    justify-content: flex-end;
  }

  .addr-input {
    width: 74px;
    background: #f7f7f7;
    border: 1px solid #e0e0e0;
    border-radius: 5px;
    color: #1e1e1e;
    font-family: inherit;
    font-size: 11px;
    padding: 3px 6px;
    outline: none;
    transition: border-color 0.15s;
  }

  .addr-input:focus { border-color: #18a0fb; }

  .port-input {
    width: 40px;
    background: #f7f7f7;
    border: 1px solid #e0e0e0;
    border-radius: 5px;
    color: #1e1e1e;
    font-family: inherit;
    font-size: 11px;
    padding: 3px 6px;
    outline: none;
    transition: border-color 0.15s;
  }

  .port-input:focus { border-color: #18a0fb; }

  .sep { color: #ccc; font-size: 12px; }

  .confirm-btn, .dismiss-btn {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 13px;
    padding: 3px 6px;
    border-radius: 4px;
    transition: background 0.15s;
  }

  .confirm-btn { color: #0d8f54; }
  .confirm-btn:hover { background: #edfaf4; }
  .dismiss-btn { color: #bbb; }
  .dismiss-btn:hover { background: #f5f5f5; }

  /* ── action buttons ── */
  .footer-actions {
    display: flex;
    gap: 8px;
  }

  .action-btn {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    text-decoration: none;
    font-size: 11px;
    font-weight: 500;
    color: #777;
    background: #f7f7f7;
    border: 1px solid #ebebeb;
    border-radius: 7px;
    padding: 7px 10px;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
    text-align: center;
    line-height: 1.3;
  }

  .action-btn:hover {
    background: #f0f0f0;
    border-color: #ddd;
    color: #333;
  }
</style>
