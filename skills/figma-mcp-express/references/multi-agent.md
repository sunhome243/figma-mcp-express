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

**Drop-on-disconnect.** When the WebSocket drops, all in-flight calls on that channel resolve immediately with `"connection closed: plugin disconnected"` (not a hang). The plugin auto-reconnects with exponential backoff.

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
  3. import_variable_by_key(spacing)  → spacingVarId
  4. Hand (wrapperId, navId, spacingVarId) to agents via prompt

Agents (parallel):
  Agent 1 → create_frame "Section A", parentId=wrapperId
  Agent 2 → create_frame "Section B", parentId=wrapperId
  Agent 3 → create_frame "Section C", parentId=wrapperId
```

### 3c. One write-coordinator funnels live calls per channel; agents read cache in parallel

**Never fan out concurrent live reads on one channel.** The single-threaded plugin serializes them anyway — "parallel" live reads on one channel are actually sequential, queued, and they jam import threads.

```
WRONG:
  Agent 1: get_node(channel="auto-1", ...)   ┐
  Agent 2: get_node(channel="auto-1", ...)   │ false parallelism
  Agent 3: get_node(channel="auto-1", ...)   ┘

RIGHT:
  Coordinator: get_node(channel="auto-1", ...)  → writes result to shared cache / disk
  Agent 1, 2, 3: grep the on-disk cache (no plugin call)
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

Agent 1: get_local_components(channel="auto-1")  → runs in parallel with Agent 2
Agent 2: create_frame(channel="auto-2", ...)     → truly parallel, own sem
```

Pass `channel: "auto-N"` explicitly on every call. Missing `channel:` defaults to whichever file is active — wrong in a multi-file session.

---

## 4. Failure modes

| Symptom | Cause | Recovery |
|---|---|---|
| Call returns `"connection closed: plugin disconnected"` | WebSocket drop during in-flight call | Plugin auto-reconnects. For **reads**: retry freely (idempotent). For **writes**: call `get_node` first to verify whether the effect was applied, THEN retry if not — never blind-retry a non-idempotent write. |
| Import (`import_component_by_key`) hangs; subsequent calls queue | Prior failed import jammed the single thread; `importInFlight` marker still set | Do NOT retry the import in a loop — each retry re-poisons the thread. Do other work (clone_node, set_text, set_fills, save_screenshots) until the in-flight marker clears (plugin response or timeout expiry). |
| Agent gets "node not found" mid-task | Another agent deleted the node, or ID was from a stale cache snapshot | Coordinator re-queries the live ID and re-dispatches. Scope partition prevents recurrence. |
| Reads return stale data after an external Figma Desktop edit | 3 s TTL window on the read-cache | Wait up to 3 s, or issue any write on the channel (instant invalidation). For must-be-live reads, the staleness window is ≤ 3 s. |
| Two agents create the same named frame | No coordinator-shared-once; both agents raced existence check | Design fix: coordinator creates the frame upfront, passes the ID to both agents. |

---

## 5. Quick decision table

| Question | Answer |
|---|---|
| Should agents use a lock for shared resources? | Almost never — partition + coordinator-shared-once removes the need. A lock = design smell. |
| Can agents read from the on-disk/project cache in parallel? | Yes — no plugin involved, full parallelism. |
| Can agents issue concurrent live reads on the same channel? | Technically yes (calls queue, all complete), but it is false parallelism — they serialize. Route live reads through the coordinator, deliver results as data. |
| Can agents issue concurrent live reads on DIFFERENT channels? | Yes — truly parallel. |
| Should I verify-before-create on every call? | Only at shared-resource creation decisions where existence is ambiguous. Rare if the coordinator created shared resources upfront. |
| Can I retry a write after a connection drop? | Only after verifying the write's effect was NOT applied (get_node first). Blind retry risks double-apply. |
| Can I retry an import after "import in flight"? | No. Do non-import work until the thread clears. |

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

Shared resources already created by coordinator:
  wrapperId = <id>
  spacingVar = <variableId>
  ...
```
