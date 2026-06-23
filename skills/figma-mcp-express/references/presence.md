# Watch-Agent Presence

Use this when multiple agents or a reviewer need visible attribution in the Figma
plugin's Watch-agent panel. This is display metadata only; it does not change
execution, serialization, routing, or op results.

**Quick navigation:** [Origin roster](#origin-roster) · [Orchestrator convention](#orchestrator-convention) · [What the panel shows](#what-the-panel-shows) · [Status and task](#status-and-task) · [Where origin is accepted](#where-origin-is-accepted)

---

## Origin roster

Hard rule: `origin` is not a free-form string. Use exactly the origin assigned to you. Do not pick a random roster enum. Valid origins are `wolfgang`, `grace`, `theo`, `sunho`, `zoe`, `taewon`, `emma`, `alex`, `rick`.

The Watch-agent identity key is `sessionId+origin`, so concurrent agents in one
session must use distinct origins. Separate orchestrator sessions can reuse roster
names because the server injects a different `sessionId`.

## Orchestrator convention

The orchestrator's own origin is `wolfgang`. Do not use `sunho` for the orchestrator;
`sunho` is just another worker roster name.

Assign each worker one distinct origin at dispatch and bake it into the prompt. Do not reuse one `origin` across concurrent agents in the same session.

```
Orchestrator/self -> origin: "wolfgang"
Agent 1 -> owns frame A -> origin: "grace"
Agent 2 -> owns frame B -> origin: "theo"
Agent 3 -> owns frame C -> origin: "zoe"
```

Example outer batch stamp:

```
batch(channel:"auto-2", origin:"theo", ops:[create_frame...])
```

Pass `origin` on every `batch` call as a top-level argument next to `channel`; never
inside individual ops.

## What the panel shows

- Panel: one current row per `(sessionId, origin)` with avatar, name, last action,
  status/task, relative time, and a jump button.
- Canvas: union-selects active agents' recent nodes without auto-scrolling. The jump
  button is the only camera move.
- Decay: active -> idle -> away -> removed after quiet time. A later action recreates
  the row.
- Unlabeled calls keep legacy single-agent follow behavior and are not attributed to
  the panel row.

## Status and task

Statuses come from two paths:

| Path | Statuses | Setter |
|---|---|---|
| Auto | `building`, `importing`, `screenshotting`, `scanning`, `theming`, `error`, `idle`, `away`, `joined` | Derived from the op/tool already sent. |
| Server auto | `queued` | Server reports the per-channel FIFO wait list. |
| LLM-set | `thinking`, `waiting_review`, `reviewing`, `approved`, `escalated`, `done` | Orchestrator/reviewer calls `set_presence`. |

In plain terms: status is optional in the schema, not optional in the workflow. Do not skip `set_presence` because `status` is optional. Actively call `set_presence` at dispatch and workflow transitions, not on every operation.

`task` is a sticky one-sentence narration shown as the row's main line. The
orchestrator should set it at dispatch because it knows each worker's assignment.

Minimum cadence:

```
set_presence(origin:"grace", task:"registration form", status:"thinking")
set_presence(origin:"grace", status:"waiting_review")
set_presence(origin:"wolfgang", task:"reviewing registration form", status:"reviewing")
set_presence(origin:"grace", status:"approved")
set_presence(origin:"grace", status:"done")
```

Manual `status` and `task` go through `set_presence`, not `batch`. Operational tools
carry only `origin` and `channel`.

## Where origin is accepted

`origin` works on plugin reads, writes, and batch:

- plugin-facing reads: `get_`, `scan_`, `search_`, `list_`
- writes and outer `batch`
- not REST/local meta tools such as `fetch_library_catalog`

Use exactly the assigned origin on every plugin-facing call from that agent so reads
and writes are attributed consistently.
