<script lang="ts">
  import { onMount } from "svelte";
  import { nextReconnectDelay } from "../status";
  import { metaFor, initialOf, avatarFor } from "../presence-roster";
  import {
    PRESENCE_ACTIVE_WINDOW_MS,
    presenceKey,
    sessionAccents,
    type AgentActivity,
  } from "../presence";
  import Typewriter from "./Typewriter.svelte";
  import Marquee from "./Marquee.svelte";

  // Map a status to one of FOUR semantic colour GROUPS so colour actually carries
  // meaning (vs a 14-colour rainbow): working (green, breathing) · waiting (amber) ·
  // attention (red) · done/idle (grey). The STATUS colour is distinct from the AGENT
  // identity colour (--c, on the avatar ring / follow tag).
  function statusGroup(status: string): string {
    switch (status) {
      case "queued":
      case "waiting_review":
        return "grp-waiting";
      case "escalated":
      case "error":
        return "grp-attention";
      case "done":
      case "approved":
      case "idle":
      case "away":
        return "grp-done";
      default:
        return "grp-working"; // building/importing/screenshotting/scanning/theming/reviewing/thinking/joined
    }
  }

  // One agent's live activity, as computed by the plugin core (activeAgents()).
  // `active` from the core is a point-in-time snapshot; the UI re-derives it
  // against its own 1 Hz clock (see isActive) so rows dim on schedule even when
  // no new op arrives to push a fresh presence_update.

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
  // __DEFAULT_PORT__ (1994) is injected at build time so the plugin auto-connects
  // to the local figma-mcp-express server.
  let serverHost = "127.0.0.1";
  let serverPort = __DEFAULT_PORT__;

  let showSettings = false;
  let editHost = serverHost;
  let editPort = serverPort;
  let minimized = false;

  // Presence ("watch the agent") toggle — mirrors plugin-core state, hydrated via
  // get_presence on mount and persisted by the core on every change.
  let watchAgent = false;
  // Live per-agent activity (pushed by the core as presence_update), plus a
  // 1 Hz clock for relative timestamps and a set of origins whose avatar 404'd.
  let agents: AgentActivity[] = [];
  let now = Date.now();
  let avatarFailed = new Set<string>();
  // Composite key (sessionId|origin) of the agent the viewport follows, or null.
  let followKey: string | null = null;

  // akey is the identity a presence row is grouped by — (sessionId, origin). ALL
  // per-agent UI state (follow, pulse, join, avatar-failed) keys by this, never by
  // origin alone, so two same-name agents from different sessions don't share state.
  const akey = (a: AgentActivity): string => presenceKey(a.sessionId, a.origin);

  // ── Followed agent pinned to the TOP. Guard undefined: the followed agent may
  // not yet have a row (e.g. follow set before its first op). Pulse stays on the
  // RAW agents array so it fires on whoever just acted, not the pinned row.
  $: orderedAgents = (() => {
    const f = agents.find((a) => akey(a) === followKey);
    return f ? [f, ...agents.filter((a) => a !== f)] : agents;
  })();

  // Per-session accent hue (only when ≥2 sessions are present) so two agents that
  // share a roster name but come from different orchestrators read as distinct —
  // alongside their per-(sessionId,origin) avatar. Names stay truthful (metaFor).
  $: sessionAccentMap = sessionAccents(orderedAgents.map((a) => a.sessionId));

  // ── Join entrance: origins not yet `seen` get a one-shot entrance animation.
  // `seen` resets when Watch is toggled off so re-enabling re-animates the joins.
  let seen = new Set<string>();
  let joining = new Set<string>();
  const joinTimers = new Map<string, ReturnType<typeof setTimeout>>();
  function markJoins(list: AgentActivity[]) {
    const fresh = list.map(akey).filter((k) => !seen.has(k));
    if (fresh.length) {
      const nextJoining = new Set(joining);
      for (const k of fresh) {
        nextJoining.add(k);
        const prev = joinTimers.get(k);
        if (prev) clearTimeout(prev);
        joinTimers.set(
          k,
          setTimeout(() => {
            joining = new Set([...joining].filter((x) => x !== k));
            joinTimers.delete(k);
          }, 450),
        );
      }
      joining = nextJoining;
    }
    // `seen` tracks the (sessionId,origin) keys CURRENTLY present. An agent that
    // decayed/was pruned out of the roster drops from `seen`, so if it returns later
    // it counts as fresh and re-animates (a genuine re-join) — while a still-shown
    // away agent that merely wakes stays in `seen` and does NOT re-animate.
    seen = new Set(list.map(akey));
  }

  // Re-derive activity against the live clock so dimming tracks real time.
  const isActive = (lastTs: number, ref: number): boolean =>
    ref - lastTs <= PRESENCE_ACTIVE_WINDOW_MS;

  const FULL_W = 320;
  const PILL_W = 210, PILL_H = 36;
  const MIN_H = 120, HARD_MAX_H = 800;

  // The window FITS ITS CONTENT: measured live from the .app element
  // (bind:clientHeight) and pushed to figma.ui.resize. Empty state → compact (no
  // wasted margin); each agent row grows the window until the list hits LIST_MAX_H,
  // then the list scrolls. The cap is fixed (no manual resize handle).
  let contentH = 0;
  const LIST_MAX_H = 300; // px cap on the agent list before it scrolls
  let lastSentH = 0;

  function applyResize() {
    if (minimized) {
      postToPlugin({ type: "resize", width: PILL_W, height: PILL_H });
      lastSentH = 0; // force a fresh send when restored
      return;
    }
    if (contentH <= 0) return;
    const h = Math.max(MIN_H, Math.min(Math.ceil(contentH), HARD_MAX_H));
    if (h === lastSentH) return;
    lastSentH = h;
    postToPlugin({ type: "resize", width: FULL_W, height: h });
  }
  // Re-fit whenever measured content or minimized state changes.
  $: contentH, minimized, applyResize();

  $: liveCount = agents.filter((a) => isActive(a.lastTs, now)).length;

  // ── Activity pulse: flash the row of whichever agent just acted. agents[0] is
  // the most-recent actor (activeAgents sorts by lastTs desc), so each
  // presence_update pulses that row in the agent's colour. No core change needed.
  let pulse = new Set<string>();
  const pulseTimers = new Map<string, ReturnType<typeof setTimeout>>();
  // Last lastTs we saw per (sessionId,origin) key — so a pulse fires only when
  // activity genuinely advances, not on the periodic decay sweep's re-emit.
  let lastTsByKey = new Map<string, number>();
  function triggerPulse(key: string) {
    // Drop then re-add next frame so the CSS animation restarts on rapid repeats.
    pulse = new Set([...pulse].filter((k) => k !== key));
    requestAnimationFrame(() => {
      pulse = new Set(pulse).add(key);
      const prev = pulseTimers.get(key);
      if (prev) clearTimeout(prev);
      pulseTimers.set(
        key,
        setTimeout(() => {
          pulse = new Set([...pulse].filter((k) => k !== key));
          pulseTimers.delete(key);
        }, 650),
      );
    });
  }

  // Clicking a row toggles "follow" — the viewport then tracks that agent's ops
  // until it's clicked again (or another agent is followed). The core echoes the
  // resulting state back via follow_state. Carry sessionId so the followed identity
  // is the (sessionId, origin) pair, not just a (possibly shared) roster name.
  function toggleFollow(a: AgentActivity) {
    postToPlugin({ type: "set_follow", origin: a.origin, sessionId: a.sessionId });
  }

  function markAvatarFailed(key: string) {
    avatarFailed = new Set(avatarFailed).add(key); // new ref → Svelte reactivity
  }

  function relTime(ts: number, ref: number): string {
    const s = Math.max(0, Math.floor((ref - ts) / 1000));
    if (s < 3) return "just now";
    if (s < 60) return `${s}s ago`;
    return `${Math.floor(s / 60)}m ago`;
  }

  // Tag every UI→plugin message with our pluginId (the manifest id). Figma routes
  // pluginMessage to the plugin code only when pluginId matches, so another plugin
  // or a navigated iframe cannot intercept these messages (which carry the WS
  // host/port config). See developers.figma.com/docs/plugins/creating-ui.
  const PLUGIN_ID = __PLUGIN_ID__;
  function postToPlugin(message: unknown) {
    parent.postMessage({ pluginMessage: message, pluginId: PLUGIN_ID }, "*");
  }

  function toggleWatchAgent() {
    watchAgent = !watchAgent;
    postToPlugin({ type: "set_presence", enabled: watchAgent });
    if (!watchAgent) {
      agents = []; // clear immediately; core also resets its log
      followKey = null;
      pulse = new Set();
      seen = new Set(); // re-enabling should re-animate joins
      joining = new Set();
      lastTsByKey = new Map();
      avatarFailed = new Set(); // forget 404s so a recovered avatar re-renders
    }
    // The content-fit reactive resizes the window once the section mounts/unmounts.
  }

  function toggleMinimize() {
    minimized = !minimized; // the $: reactive re-fits (pill vs content height)
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
        // Unsolicited presence_queue frame (no requestId): route it to the plugin
        // core, which folds it into presence_update. Must branch BEFORE the
        // generic server-request path or it'd be mistaken for a request.
        if (payload?.type === "presence_queue") {
          postToPlugin({
            type: "presence_queue",
            origins: Array.isArray(payload.origins) ? payload.origins : [],
          });
          return;
        }
        if (payload?.requestId) {
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

    if (msg.type === "presence_state") {
      watchAgent = msg.enabled === true;
      // content-fit reactive handles the resize once the section renders
      return;
    }

    if (msg.type === "presence_update") {
      agents = Array.isArray(msg.agents) ? msg.agents : [];
      markJoins(agents); // one-shot entrance for any origin we haven't seen yet
      // Pulse a row ONLY on genuine new activity — i.e. its lastTs advanced since
      // we last saw it. The 5s decay sweep re-emits with UNCHANGED timestamps, so
      // it no longer makes the most-recent row flash while nothing is happening.
      // Queued rows are synthetic (lastTs = now each emit) → excluded explicitly.
      for (const a of agents) {
        if (a.status !== "queued" && a.lastTs > (lastTsByKey.get(akey(a)) ?? 0)) {
          triggerPulse(akey(a));
        }
      }
      lastTsByKey = new Map(agents.map((a) => [akey(a), a.lastTs]));
      return;
    }

    if (msg.type === "follow_state") {
      followKey = typeof msg.key === "string" ? msg.key : null;
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

    // 1 Hz tick so relative timestamps ("2s ago") stay live.
    const clock = setInterval(() => (now = Date.now()), 1000);

    // Request stored config from plugin core (responds with ws_config message).
    // connect() is called once we receive the response.
    postToPlugin({ type: "get_ws_config" });

    // Hydrate the presence toggle from plugin-core state.
    postToPlugin({ type: "get_presence" });

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
      clearInterval(clock);
      pulseTimers.forEach((t) => clearTimeout(t));
      joinTimers.forEach((t) => clearTimeout(t));
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
<div class="app" bind:clientHeight={contentH}>

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

    <!-- Presence toggle: follow the agent's edits live on canvas -->
    <button
      class="watch-row"
      class:on={watchAgent}
      on:click={toggleWatchAgent}
      title="Select + scroll to nodes the agent edits, live"
    >
      <span class="watch-label">👀 Watch agent</span>
      <span class="switch" class:on={watchAgent}><span class="knob"></span></span>
    </button>

    <!-- Per-agent presence: who is working where (one viewport can't follow N
         agents, so the canvas highlights the union and [→] jumps to one). -->
    {#if watchAgent}
      <div class="presence">
        <div class="presence-head">
          <span class="field-label">Agents</span>
          {#if liveCount > 0}
            <span class="live-badge"><span class="live-dot"></span>{liveCount} live</span>
          {/if}
        </div>
        {#if agents.length === 0}
          <div class="presence-empty"><span class="eyes">👀</span> waiting for an agent…</div>
        {:else}
          <div class="presence-list" style="max-height:{LIST_MAX_H}px">
            {#each orderedAgents as a (akey(a))}
              {@const meta = metaFor(a.origin)}
              {@const following = akey(a) === followKey}
              {@const active = isActive(a.lastTs, now)}
              {@const hue = sessionAccentMap.get(a.sessionId)}
              <button
                class="agent-row"
                class:idle={!active}
                class:following
                class:session-accented={hue != null}
                class:pulse={pulse.has(akey(a))}
                class:joining={joining.has(akey(a))}
                style="--c:{meta.color}{hue != null ? `; --sc:hsl(${hue} 70% 55%)` : ''}"
                on:click={() => toggleFollow(a)}
                title={following ? `Following ${meta.name} — click to stop` : `Follow ${meta.name}'s edits`}
              >
                <span class="avatar-wrap" class:crowned={meta.crown}>
                  {#if avatarFailed.has(akey(a))}
                    <span class="avatar avatar-mono">{initialOf(a.origin)}</span>
                  {:else}
                    <img
                      class="avatar"
                      src={avatarFor(a.sessionId, a.origin)}
                      alt={meta.name}
                      on:error={() => markAvatarFailed(akey(a))}
                    />
                  {/if}
                  {#if meta.crown}<span class="crown" title="Orchestrator">👑</span>{/if}
                </span>
                <span class="agent-text">
                  <span class="agent-name">{meta.name}</span>
                  {#if a.task}<Marquee text={a.task} cls="agent-task" title={a.task} />{/if}
                  <span class="agent-action"><Typewriter text={a.label} /> <span class="agent-time">· {relTime(a.lastTs, now)}</span></span>
                </span>
                <span class="agent-status">
                  {#if following}
                    <span class="follow-tag"><span class="fdot"></span>following</span>
                  {:else}
                    <span class="status-chip {statusGroup(a.status)}" title={a.status}></span>
                  {/if}
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
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
    /* height is content-driven: the plugin measures .app and resizes the window
       to fit, so the empty state stays compact (no leftover margin). */
    height: auto;
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
  /* height is intentionally auto (content-driven) — bind:clientHeight measures it
     and the plugin resizes the window to match. */
  .app {
    display: flex;
    flex-direction: column;
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

  /* ── watch-agent toggle ── */
  .watch-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    background: #f7f7f7;
    border: 1px solid #ebebeb;
    border-radius: 8px;
    padding: 8px 10px;
    cursor: pointer;
    font-family: inherit;
    transition: background 0.15s, border-color 0.15s;
  }

  .watch-row:hover { background: #f0f0f0; border-color: #ddd; }
  .watch-row.on { background: #edfaf4; border-color: #bdecd3; }

  .watch-label {
    font-size: 12px;
    font-weight: 500;
    color: #555;
  }

  .watch-row.on .watch-label { color: #0d8f54; }

  .switch {
    width: 30px;
    height: 18px;
    border-radius: 100px;
    background: #d8d8d8;
    flex-shrink: 0;
    position: relative;
    transition: background 0.15s;
  }

  .switch.on { background: #0d8f54; }

  .knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: #fff;
    transition: transform 0.15s;
  }

  .switch.on .knob { transform: translateX(12px); }

  /* ── presence: per-agent activity console ──
     Chrome stays quiet (monochrome); colour + motion live ONLY here. Each agent's
     identity colour is carried on the row as --c and drives the avatar ring, the
     live dot, the follow tag, and the activity pulse. */
  .presence {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-height: 0;
  }

  .presence-head {
    display: flex;
    align-items: center;
    gap: 7px;
  }

  .live-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 600;
    color: #0d8f54;
    letter-spacing: 0.02em;
  }

  .live-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: #22c55e;
    animation: breathe 1.6s ease-in-out infinite;
  }

  .presence-empty {
    font-size: 11px;
    color: #b0b0b0;
    padding: 12px 2px;
    text-align: center;
    letter-spacing: 0.01em;
  }

  .presence-empty .eyes {
    display: inline-block;
    animation: breathe 2s ease-in-out infinite;
  }

  .presence-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
    overflow-y: auto;
    /* max-height is set inline (LIST_MAX_H) — fixed cap, then the list scrolls. */
  }

  .agent-row {
    --c: #9a9a9a;
    position: relative;
    overflow: hidden;
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 9px;
    width: 100%;
    flex-shrink: 0;
    background: #fafafa;
    border: 1px solid #e8e8e8;
    border-radius: 10px;
    padding: 7px 10px 7px 8px;
    cursor: pointer;
    font-family: inherit;
    text-align: left;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
    transition: background 0.15s, border-color 0.15s, opacity 0.25s, box-shadow 0.15s;
  }

  /* Per-session accent: a coloured left edge so two agents sharing a roster name but
     from different sessions read as distinct (only applied when ≥2 sessions). */
  .agent-row.session-accented {
    border-left: 3px solid var(--sc);
    padding-left: 5px;
  }

  .agent-row:hover { background: #f3f3f3; border-color: #e2e2e2; }
  .agent-row.idle { opacity: 0.45; }

  .agent-row.following {
    background: #fff;
    border-color: var(--c);
    box-shadow: inset 0 0 0 1px var(--c);
    opacity: 1; /* never dim a followed agent */
  }

  /* Colour-wash pulse on each op — an overlay so it works over any row bg. */
  .agent-row::before {
    content: "";
    position: absolute;
    inset: 0;
    background: var(--c);
    opacity: 0;
    pointer-events: none;
    border-radius: inherit;
  }
  .agent-row.pulse::before { animation: agentPulse 0.65s ease-out; }
  @keyframes agentPulse {
    from { opacity: 0.24; }
    to { opacity: 0; }
  }

  .avatar-wrap {
    position: relative;
    flex-shrink: 0;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    padding: 2px;
    background: var(--c); /* the identity-colour ring */
  }

  /* Orchestrator (Wolfgang 👑) — thicker gold ring + crown badge above the avatar. */
  .avatar-wrap.crowned {
    padding: 2.5px;
    box-shadow: 0 0 0 1px var(--c);
  }
  .crown {
    position: absolute;
    top: -9px;
    left: 50%;
    transform: translateX(-50%) rotate(8deg);
    font-size: 11px;
    line-height: 1;
    pointer-events: none;
    filter: drop-shadow(0 1px 1px rgba(0, 0, 0, 0.25));
  }

  .avatar {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    background: #fff;
    display: block;
  }

  .avatar-mono {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--c);
    color: #fff;
    font-size: 11px;
    font-weight: 700;
  }

  .agent-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .agent-name {
    font-size: 12px;
    font-weight: 600;
    color: #1a1a1a;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Sticky task narration — rendered inside <Marquee> which owns overflow/truncation.
     :global() pierces the component boundary so the class applies to Marquee's inner spans. */
  :global(.agent-task) {
    font-size: 11px;
    color: #3d3d3d;
    font-weight: 450;
    line-height: 1.4;
  }

  /* Monospace activity line — a "live log" character without an external font. */
  .agent-action {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 10px;
    color: #9a9a9a;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    letter-spacing: -0.01em;
  }

  .agent-status {
    flex-shrink: 0;
    display: flex;
    align-items: center;
  }

  /* Status chip: a dot coloured by the agent's SEMANTIC status (distinct from the
     --c identity colour on the avatar ring/follow tag). Active-work statuses
     breathe; terminal/idle statuses are static. */
  .status-chip {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #d2d2d2;
    box-shadow: 0 0 0 0 transparent;
  }

  /* Four semantic groups — colour = meaning, not identity. */
  .grp-working   { background: #22c55e; animation: breathe 1.6s ease-in-out infinite; } /* green, breathing */
  .grp-waiting   { background: #f59e0b; } /* amber, static */
  .grp-attention { background: #ef4444; } /* red, static */
  .grp-done      { background: #c8c8c8; } /* grey, static */

  .follow-tag {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 700;
    color: var(--c);
    white-space: nowrap;
  }
  .follow-tag .fdot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--c);
  }

  @keyframes breathe {
    0%, 100% { opacity: 0.4; transform: scale(0.82); }
    50% { opacity: 1; transform: scale(1); }
  }

  .agent-time { color: #c4c4c4; }

  /* ── Join entrance: a subtle one-shot when an agent first appears. The row fades
     in, settles up + scales 0.96→1, and its avatar ring sweeps in. */
  .agent-row.joining { animation: agentJoin 0.44s ease-out both; }
  .agent-row.joining .avatar-wrap { animation: ringJoin 0.44s ease-out both; }
  @keyframes agentJoin {
    from { opacity: 0; transform: translateY(6px) scale(0.96); }
    to   { opacity: 1; transform: translateY(0) scale(1); }
  }
  @keyframes ringJoin {
    from { opacity: 0; transform: scale(0.7); }
    to   { opacity: 1; transform: scale(1); }
  }
  @media (prefers-reduced-motion: reduce) {
    .agent-row.joining,
    .agent-row.joining .avatar-wrap { animation: none; }
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
