# Multi-Agent Orchestration with figma-mcp-express

Deep reference for safely driving this MCP with multiple parallel agents.
Read `SKILL.md § Workflow 5` first; this document is the full spec it points to.

---

## 1. What the server gives you — the 4 facts

| Fact | Mechanism | What it means for you |
|---|---|---|
| **Per-channel writes AND live reads serialize** | Each `connEntry` has a buffered semaphore (cap 1, FIFO). Every call — read or write — holds the sem for its entire lifetime. | No corruption on a single channel. But no real parallelism either. Concurrent agents on the same channel queue behind each other. |
| **Queue-wait does NOT count against the inactivity timeout** | The inactivity timer starts AFTER sem acquisition (after the request is dispatched to the plugin). | Queued calls will complete; they are not killed by waiting. A slow preceding call does not time out the next one. |
| **Cross-channel calls run in true parallel** | Each channel is its own `connEntry` with its own sem. No global lock. | Targeting a different Figma file = pass a different `channel: "auto-N"` = full parallelism, zero serialization. |
| **Read singleflight + C4 read-cache (3 s TTL)** | Simultaneous identical reads collapse onto one plugin round-trip (singleflight). Near-simultaneous ones hit the in-process LRU cache (TTL default 3 s). Any write on a channel invalidates ALL cached reads for that channel immediately (generation bump + map clear). | Cache hits never touch the plugin — those reads are fully parallel. Write → instant cache purge → next read is live. |

**Drop-on-disconnect.** When the WebSocket drops, all in-flight calls on that channel resolve immediately with `"connection closed: plugin disconnected"` (not a hang). The plugin auto-reconnects.

---

## 2. What the server does NOT give you — semantic conflicts

Serialization prevents **data corruption** (no torn writes). It does not prevent **semantic conflicts** between agents that each do logically correct but mutually destructive operations.

| Conflict mode | How it happens | Example |
|---|---|---|
| **Lost-update** | Agent A reads node → Agent B writes same node → Agent A writes (overwrites B's change). Classic read-modify-write race. | Agent A reads `itemSpacing`, adds 8, writes back. Agent B does the same concurrently. One update is lost. |
| **Interleave / ordering** | Agents write to the same parent at the same time. Auto-layout reflows on every child append, so ordering is non-deterministic. | Two agents both `create_frame` children inside the same wrapper. Result order and reflow are unpredictable. |
| **Delete-while-edit** | Agent A holds a node ID; Agent B deletes that node. A's subsequent write returns "node not found". | A is configuring a component instance while B deletes a stale draft; IDs collide. |
| **Auto-layout reflow** | A write to one node reflows its ancestors AND siblings. | Agent A resizes a card; Agent B is simultaneously reading that card's sibling's bounds. B gets a snapshot mid-reflow. |

**The cache is coherence, not a lock.** Write-invalidation means an agent reading after a write gets a live result — no stale data. But it does not serialize the logical *decision* of whether to create or mutate something.

---

## 3. The safe pattern

### 3a. Partition by disjoint regions

One agent per screen / section / subtree. Non-overlapping write sets = zero semantic conflicts by construction.

```
Screen A  → Agent 1 (writes only under nodeId=<frameA>)
Screen B  → Agent 2 (writes only under nodeId=<frameB>)
Screen C  → Agent 3 (writes only under nodeId=<frameC>)
```

Every agent prompt must state its scope: `"You own frame <X> only. Do not read or write outside it."`

### 3b. Coordinator creates shared resources ONCE upfront (sequential)

Before fan-out, the orchestrator creates every resource that more than one agent might reference or depend on:

- Page scaffold (top-level wrapper frames, section shells)
- Shared nav, header, sidebar
- Imported library variables / spacing tokens
- Any node whose ID will be passed as `parentId` to agents

**Do this sequentially in the coordinator, then hand node IDs to agents.** Agents create only their own content. This eliminates the existence-ambiguity problem: agents never need to ask "does this frame exist yet?" because the coordinator guarantees it does.

```
Coordinator (sequential):
  1. create_frame "Page Wrapper"      → wrapperId
  2. create_frame "Nav"               → navId
  3. batch op import_variable_by_key for spacing  → spacingVarId
  4. Hand (wrapperId, navId, spacingVarId) to agents via prompt

Agents (parallel):
  Agent 1 → create_frame "Section A", parentId=wrapperId
  Agent 2 → create_frame "Section B", parentId=wrapperId
  Agent 3 → create_frame "Section C", parentId=wrapperId
```

### 3c. Fan out live calls freely — the server queues them, safely and faster

Concurrent live calls on one channel are **safe** (the per-channel queue + singleflight serialize them, no drops/races/corruption) **and faster**: N agents firing N calls pipeline them into the queue, so the plugin runs them back-to-back instead of idling for an LLM round-trip between each sequential call — and that round-trip usually dominates, so the speedup is real. Execution stays serial per channel; for *overlapping* execution use separate channels.

```
SEQUENTIAL (one agent): emit -> wait full round-trip -> emit -> wait -> ...  (LLM gap between each)
PIPELINED (N agents):   all calls hit the queue at once -> plugin runs them back-to-back   <- faster
TRULY PARALLEL:         separate channels (each open file = its own plugin thread)
```

Reads that agents need before their work should be done by the coordinator and passed as structured data, or written to disk (`.figma-mcp-cache/` or project cache files) for agents to `grep`/`jq` locally.

### 3d. Verify-before-create / verify-before-mutate at decision points

A cache **miss** means "not in cache." It does NOT mean "the node does not exist in Figma."

**Never conclude "X is absent" or issue a create-if-absent from a cache miss alone.**

When your logic branches on existence (e.g., "create the wrapper only if it doesn't exist"), do ONE bounded live read at that decision point:

```
search_nodes(nodeId=<pageId>, types=["FRAME"], query="My Wrapper", limit=5)
```

If found → use the existing ID. If not found → create. Then hand the ID to agents.

This is cheap (one call) and rare if the coordinator-shared-once pattern is followed correctly. If you've partitioned and created shared resources upfront, agents almost never need this check.

### 3e. No lock

Locking introduces its own failure surface: stale locks, deadlocks, contention, and lock-holder crashes. The patterns above (partition + coordinator-shared-once) eliminate the *need* for a lock. A shared mutex across agents adds complexity without safety benefit.

If you find yourself reaching for a lock, it means two agents are competing on the same resource — that is a design problem. Fix the partition, not the locking.

### 3f. Multi-file = separate channels for true parallelism

```
list_channels → auto-1 (Library), auto-2 (Product App)

Agent 1: get_local_components(channel:"auto-1", origin:"grace")  → runs in parallel with Agent 2
Agent 2: batch(channel:"auto-2", origin:"theo", ops:[create_frame...])  → truly parallel, own sem
```

Pass `channel: "auto-N"` explicitly on every call. Missing `channel:` defaults to whichever file is active — wrong in a multi-file session.
Pass `origin` on every `batch` call as a top-level argument next to `channel`, never
inside an individual op. Inner ops do not carry agent identity; the outer batch does.

---

## 4. Failure modes

| Symptom | Cause | Recovery |
|---|---|---|
| Call returns `"connection closed: plugin disconnected"` | WebSocket drop during in-flight call | Plugin auto-reconnects. For **reads**: retry freely (idempotent). For **writes**: call `get_node` first to verify whether the effect was applied, THEN retry if not — never blind-retry a non-idempotent write. |
| Import (`import_component_by_key`) is slow; calls queue behind it | Malformed/truncated/node-id keys fail fast, but valid-looking unpublished/wrong-library keys or missing COMPONENT_SET type hints can still wait for the plugin import timeout | Don't loop-retry. Validate the key (`get_local_components`/`fetch_library_catalog`) before retrying, pass `assetType:"COMPONENT_SET"` for sets, or do other work (clone_node, set_text) meanwhile. Calls behind it complete once it clears. |
| Call fast-rejected with `ErrImportInFlight` or `ErrChannelStalled` | Another op is wedged on that channel — a prior import still running (`ErrImportInFlight`), or any op stuck with no progress (`ErrChannelStalled`). Your call wasn't queued behind it. | Not your call's fault. Do OTHER work — a different `channel`, or non-conflicting ops — or retry after a short pause. Never tight-loop retry. The wedged op clears on its own; the channel frees once it does. |
| Agent gets "node not found" mid-task | Another agent deleted the node, or ID was from a stale cache snapshot | Coordinator re-queries the live ID and re-dispatches. Scope partition prevents recurrence. |
| Reads return stale data after an external Figma Desktop edit | 3 s TTL window on the read-cache | Wait up to 3 s, or issue any write on the channel (instant invalidation). For must-be-live reads, the staleness window is ≤ 3 s. |
| Two agents create the same named frame | No coordinator-shared-once; both agents raced existence check | Design fix: coordinator creates the frame upfront, passes the ID to both agents. |

---

## 5. Quick decision table

| Question | Answer |
|---|---|
| Should agents use a lock for shared resources? | Almost never — partition + coordinator-shared-once removes the need. A lock = design smell. |
| Can agents read from the on-disk/project cache in parallel? | Yes — no plugin involved, full parallelism. |
| Can agents issue concurrent live reads on the same channel? | Yes — safe AND faster: firing them in parallel pipelines them into the queue (the plugin runs them back-to-back, no LLM round-trip between each). Execution is serial per channel; for *overlapping* execution use separate channels. |
| Can agents issue concurrent live reads on DIFFERENT channels? | Yes — truly parallel. |
| Should I verify-before-create on every call? | Only at shared-resource creation decisions where existence is ambiguous. Rare if the coordinator created shared resources upfront. |
| Can I retry a write after a connection drop? | Only after verifying the write's effect was NOT applied (get_node first). Blind retry risks double-apply. |
| Can I retry after `ErrImportInFlight` / `ErrChannelStalled`? | No — an op is wedged on that channel. Do other work (or a different channel) until it clears; don't tight-loop. |

---

## 6. Prompt template for parallel agents

Include this block in every parallel subagent prompt:

```
You are one of N agents running in parallel on the same Figma file.
Your write scope is: <frameId / region description>.
Do NOT read or write outside your scope.
Do NOT issue live plugin reads — use the cache data provided below.
Do NOT use git stash, git reset --hard, git checkout --, or git clean.
channel: "auto-N"  ← pass this on every tool call.
origin: "<rosterName>"  ← pass this on every batch AND read call so the plugin's
                          Watch-agent panel attributes your edits/reads.
                          (manual `status` + `task` go via the set_presence tool, set
                          by the orchestrator at dispatch — not per-builder — see § 7.)

Shared resources already created by coordinator:
  wrapperId = <id>
  spacingVar = <variableId>
  ...
```

---

## 7. Agent presence — the `origin` label

A live "who is working where" view for humans watching the file. When the plugin's
**Watch agent** toggle is on, each labeled write lights up the canvas and a per-agent
panel (avatar + last action + status). Shipped in **2.3.0** — available on the
published/production server (`npx figma-mcp-express`, port 1994) and the plugin.

> **Hard rule:** `origin` is not a free-form string. Use exactly the origin assigned to you; never substitute, infer, or rotate it. Do not pick a random roster enum. Valid origins are `grace`, `theo`, `sunho`,
> `zoe`, `taewon`, `emma`, `alex`, `rick`, `wolfgang`. The Watch-agent identity key
> is `sessionId+origin`, so concurrent agents in one session must use distinct
> origins. The orchestrator's own origin is `wolfgang`.

### Why a label, not auto-detection

The transport cannot tell agents apart on its own: parallel subagents in one Claude
Code session share **one** MCP process (one WebSocket), and process identity is
unstable across restarts. So the acting agent must self-identify. `origin` is an
**enum** (`grace`, `theo`, `sunho`, `zoe`, `taewon`, `emma`, `alex`, `rick`, `wolfgang`)
so each label maps deterministically to a name/color/avatar. It flows verbatim to the plugin (the
bridge strips only `channel`); unknown/empty values are dropped server-side and the
plugin fails safe.

### Orchestrator convention

Do not use `sunho` for the orchestrator; `sunho` is just another worker roster name.
Assign each worker one distinct origin at dispatch and bake it into the prompt. The
worker must keep that origin on every read/write/batch call. Do not reuse one `origin` across concurrent agents in the same session; it collapses follow/attribution into one row.

```
Orchestrator/self -> origin: "wolfgang"
Agent 1 -> owns frame A -> origin: "grace"
Agent 2 -> owns frame B -> origin: "theo"
Agent 3 -> owns frame C -> origin: "zoe"
```

Each worker then stamps its assigned origin on every batch call, e.g.
`batch(channel:"auto-1", origin:"grace", ops:[create_frame…])`.

### What presence does and does NOT do

- **Panel (primary):** shows every active agent at once — avatar, name, last action,
  relative time — with a `[→]` jump button. This is how you "follow many."
- **Canvas (secondary):** selects the **union** of active agents' recent nodes
  **without scrolling** — a single Figma viewport physically cannot chase N agents,
  so it never auto-follows; the panel's `[→]` is the only camera move (one agent at a
  time, on demand).
- It is **display only** — `origin` changes nothing about execution, serialization,
  or the op result. Unlabeled calls keep the legacy single-agent follow (select +
  scroll). The panel tracks **one latest-activity entry per origin** (not an event log):
  a row decays active → idle → away on a timer and auto-removes after ~150 s of silence;
  an origin that acts again after removal replays the entrance animation.

### Per-agent status — auto-derived vs LLM-set

Each row also shows *what* the agent is doing. Statuses come from two tiers:

| Tier | Cost | Statuses | How it's set |
|---|---|---|---|
| **Auto** | **0 LLM tokens** | `building` (Building…/Styling…/Moving…/Resizing…/Removing…), `importing` (⤵ Importing…), `screenshotting` (📸 Capturing…), `scanning` (🔍 Looking around…), `theming` (🎨 Theming…), `error`, `idle` (30–60 s), `away` (>60 s), `joined` (entrance anim) | Plugin/server derive it from the op the agent already sends — no extra call. `building`/`importing`/`theming` from write op type, `screenshotting` from `save_screenshots`, `scanning` from any `get_`/`scan_`/`search_`/`list_`/`fetch_` read. |
| **Auto (server)** | **0 LLM tokens** | `queued` (Queued · #N) | The **server** pushes the per-channel serial-slot waiting list to the plugin as an unsolicited `presence_queue` WS frame. The agent is by definition not yet running, so only the server (which owns the FIFO) can report it. |
| **LLM-set** | tiny | `thinking`, `waiting_review`, `reviewing`, `approved` (reviewer PASS), `escalated` (asset missing → STOP), `done` | The orchestrator/reviewer calls **`set_presence`** at **workflow transitions only**, never per op (see below). |

Auto statuses decay on a timer: active (≤30 s) → `idle` (30–60 s) → `away` (>60 s) →
auto-removed (>150 s quiet). LLM-set statuses are **sticky** — no TTL, never auto-removed
— so overwrite a stale `reviewing`/`done` explicitly (or re-announce on the next ping).

### The orchestrator stamps non-editing statuses on behalf of an origin

A `queued` or `waiting_review` agent **cannot self-report** — it's blocked in the
serial slot or has already returned its result to the orchestrator. So the LLM-set
transitions are stamped via `set_presence` **by the orchestrator on behalf of a named
`origin`**, not by the agent itself. The orchestrator knows the workflow shape (who's
thinking, who's up for review, who passed), so it owns those pings — and likewise
stamps each agent's `task` at dispatch. Per-op auto statuses still come from the acting
agent's own operational calls (which carry `origin`).

### Status + task pings — use the dedicated `set_presence` tool

To announce a workflow transition or declare an agent's task **without mutating the
canvas**, call **`set_presence`** — the dedicated presence tool. It performs no Figma
op; it records `{origin, status, task}` for the panel and returns `{ok}`:

```
set_presence(origin:"theo", status:"reviewing")                       ← phase transition
set_presence(origin:"grace", task:"redesigning the dashboard sidebar") ← declare task
set_presence(origin:"grace", task:"...", status:"thinking")            ← both at once
```

Use it for the manual statuses (`thinking`, `waiting_review`, `reviewing`, `approved`,
`escalated`, `done`) and for `task`. Manual `status` is **not accepted on `batch`** — send
it via `set_presence`; operational tools (incl. `batch`) carry only the required `origin`.
In plain terms: status is optional in the schema, not optional in the workflow. Do not skip `set_presence` because `status` is optional. Actively call `set_presence` at dispatch and workflow transitions, not on every operation.

Minimum cadence:

```
set_presence(origin:"grace", task:"registration form", status:"thinking")       ← dispatch
set_presence(origin:"grace", status:"waiting_review")                           ← worker returned
set_presence(origin:"wolfgang", task:"reviewing registration form", status:"reviewing") ← orchestrator review
set_presence(origin:"grace", status:"approved")                                 ← worker passed
set_presence(origin:"grace", status:"done")                                     ← section complete
```

### `task` — the agent's one-sentence narration

`task` is a free-form, sticky one-sentence description of what the agent is working on
("redesigning the dashboard sidebar"). The plugin remembers the last value per agent, so
send it **once at dispatch** and it persists — it shows as the MAIN line of the agent's
row, above the auto activity line. Reliability: have the **orchestrator** stamp each
agent's `task` right after spawning it (the orchestrator knows the task and is a single,
reliable caller); the worker need not call `set_presence` itself. An agent may re-send
`task` to refine it as it moves through sub-steps (best-effort).

### Two orchestrators on one file never collide (`sessionId`, automatic)

The leader server is shared across all Claude sessions, so two uncoordinated
orchestrators that both dispatch a "grace" would have collided on one panel row. The
server now stamps a per-process `sessionId` automatically (zero LLM burden), and the
plugin keys presence by **`(sessionId, origin)`** — same name from two sessions = two
distinct rows that never clobber. The displayed NAME stays truthful; the two are told
apart by a per-`(sessionId, origin)` avatar (distinct face) and a per-session accent
colour (shown only when ≥2 sessions are live). You don't pass `sessionId` — it's
injected for you. So you may freely reuse roster names across separate orchestrators.

### `origin` works on reads, writes, and batch

`origin` is not batch-only. Pass the assigned `origin` on reads
(`get_`/`scan_`/`search_`/`list_`/`fetch_`), writes, and outer `batch` calls so the
panel can attribute both exploration and mutation. For `batch`, `origin` is a
top-level argument next to `channel`; do not put it inside individual ops.
