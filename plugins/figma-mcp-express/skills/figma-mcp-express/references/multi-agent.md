# Multi-Agent Orchestration

Use after `SKILL.md`; this expands the Reference Router entry for parallel
agents, channels, and shared-resource handoff. For Watch-agent labels, `origin`,
status, and task, read `presence.md`.

**Quick navigation:** [Facts](#facts) · [Safe pattern](#safe-pattern) · [Failure modes](#failure-modes) · [Prompt block](#prompt-block)

---

## Facts

| Fact | Impact |
|---|---|
| One channel serializes live calls | Same-file calls are safe but not overlapping. |
| Different channels run in parallel | Open multiple files/channels for true overlap. |
| Identical reads singleflight/cache briefly | Cheap for parallel reads; any write clears that channel cache. |
| Serialization is not a semantic lock | Two agents can still make conflicting plans from stale assumptions. |

Semantic conflicts to avoid: two agents appending to the same parent, editing after
another agent deletes a node, creating the same shared wrapper twice, or writing
layout from a snapshot taken before another reflow.

## Safe pattern

| Step | Rule |
|---|---|
| Partition | Assign one screen, section, or subtree per worker. |
| Create shared resources once | Orchestrator creates/imports wrappers, nav, variables, components, and passes concrete IDs. |
| Fan out | Workers write only inside assigned scope. Cross-channel calls can overlap; same-channel calls queue. |
| Verify at decision points | If creation depends on absence, do one bounded live read, then create once. |
| Avoid locks | If a lock seems needed, repartition or move shared work back to the orchestrator. |

Multi-file example:

```text
list_channels -> auto-1 (Library), auto-2 (Product App)

Agent 1: get_local_components(channel:"auto-1", origin:"grace")
Agent 2: batch(channel:"auto-2", origin:"theo", ops:[create_frame...])
```

Pass `channel` explicitly on every file-specific call. Missing `channel` falls back
to the active file and is unsafe in a multi-file session. Pass `origin` on every
plugin-facing read/write and on the outer `batch` call; see `presence.md`.

## Failure modes

| Symptom | Recovery |
|---|---|
| `connection closed: plugin disconnected` | Retry reads freely. For writes, first read the target and retry only if the effect did not apply. |
| Slow import queues work | Validate key/catalog/type hint; continue elsewhere instead of loop-retrying. |
| `ErrImportInFlight` / `ErrChannelStalled` | Do other work or another channel; retry later. |
| `node not found` mid-task | Coordinator re-queries the live ID and redispatches with tighter scope. |
| Duplicate named frame | Coordinator should create it upfront and pass the ID. |

## Prompt block

```text
You are one of N agents running in parallel on the same Figma file.
Your write scope is: <frameId / region description>.
Do NOT read or write outside your scope.
Do NOT issue live plugin reads unless explicitly told to.
Use the cache/context data provided below.
Do NOT use git stash, git reset --hard, git checkout --, or git clean.

channel: "auto-N"  <- pass this on every file-specific tool call.
origin: "<rosterName>"  <- pass this on every plugin-facing read/write and outer batch call.

Shared resources already created by coordinator:
  wrapperId = <id>
  spacingVar = <variableId>
```
