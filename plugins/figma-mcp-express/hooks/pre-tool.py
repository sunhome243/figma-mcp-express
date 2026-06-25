#!/usr/bin/env python3
"""
PreToolUse — requires the figma-mcp-express skill before any Figma MCP tool call,
then validates presence arguments just before the tool runs.

The root failure mode is using Figma MCP tools without loading the bundled skill.
If the skill has not written its per-session marker, every figma-mcp-express tool
call is denied with instructions to load the skill and retry. Once loaded, this
hook catches high-confidence mistakes that prose cannot: missing/unknown origin,
nested batch presence params, and operational `status` or `task` usage outside
`set_presence`.
"""
import json
import os
import re
import sys
import tempfile

ROSTER = ("wolfgang", "grace", "theo", "sunho", "zoe", "taewon", "emma", "alex", "rick")
ROSTER_SET = set(ROSTER)
STATUSES = {"thinking", "waiting_review", "reviewing", "approved", "escalated", "done"}
ORIGIN_EXEMPT_TOOLS = {
    "list_channels",
    "search_batch_ops",
    "get_batch_op_spec",
    "fetch_library_catalog",
}
PRESENCE_KEYS = {"origin", "status", "task", "channel"}
SERVER_PREFIX = "figma-mcp-express"


def emit(payload, code=0):
    print(json.dumps(payload, ensure_ascii=False))
    sys.exit(code)


def allow():
    emit({"continue": True})


def deny(message):
    emit(
        {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": message,
            },
        }
    )


def allow_with_context(context):
    emit(
        {
            "continue": True,
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "additionalContext": context,
            },
        }
    )


def sanitize_session_id(sid):
    return re.sub(r"[^A-Za-z0-9_-]", "", str(sid)) or "default"


def session_id_candidates(data):
    candidates = []
    for raw in (
        data.get("session_id"),
        os.environ.get("CLAUDE_CODE_SESSION_ID"),
        os.environ.get("CLAUDE_SESSION_ID"),
        os.environ.get("CODEX_SESSION_ID"),
        "default",
    ):
        if not raw:
            continue
        sid = sanitize_session_id(raw)
        if sid not in candidates:
            candidates.append(sid)
    return candidates or ["default"]


def session_id(data):
    return session_id_candidates(data)[0]


def flag_path(kind, sid):
    return os.path.join(tempfile.gettempdir(), f"fme-{kind}-{sid}")


def skill_flag_path(sid):
    return os.path.join(tempfile.gettempdir(), f"fme-skill-loaded-{sid}")


def mark_once(path):
    if os.path.exists(path):
        return False
    try:
        with open(path, "w", encoding="utf-8"):
            pass
    except OSError:
        return False
    return True


def skill_loaded(data):
    return any(os.path.exists(skill_flag_path(sid)) for sid in session_id_candidates(data))


def fme_tool_name(tool_name):
    parts = str(tool_name or "").split("__", 2)
    if len(parts) != 3 or parts[0] != "mcp":
        return ""
    server_name, tool = parts[1], parts[2]
    if not server_name.startswith(SERVER_PREFIX):
        return ""
    return tool


def tool_input(data):
    value = data.get("tool_input")
    if isinstance(value, dict):
        return value
    # Defensive aliases for host/version differences.
    for key in ("input", "arguments", "args"):
        value = data.get(key)
        if isinstance(value, dict):
            return value
    return {}


def batch_ops(args):
    ops = args.get("ops")
    return ops if isinstance(ops, list) else []


def nested_presence_violations(ops):
    violations = []
    for i, op in enumerate(ops):
        if not isinstance(op, dict):
            continue
        for key in sorted(PRESENCE_KEYS.intersection(op.keys())):
            violations.append(f"ops[{i}].{key}")
        params = op.get("params")
        if isinstance(params, dict):
            for key in sorted(PRESENCE_KEYS.intersection(params.keys())):
                violations.append(f"ops[{i}].params.{key}")
    return violations


def skill_required_message(tool):
    return (
        f"Blocked figma-mcp-express `{tool}` because the figma-mcp-express skill "
        "has not been loaded in this session. Load/use the `figma-mcp-express` "
        "skill first, then retry the exact MCP call. The skill writes the "
        "per-session marker that allows Figma MCP tools to run."
    )


def grace_warning(sid, tool):
    path = flag_path("grace-origin-warning", sid)
    if not mark_once(path):
        return ""
    return (
        f"figma-mcp-express call `{tool}` is using origin:\"grace\". "
        "That is valid only if this worker was explicitly assigned grace. "
        "If you are the orchestrator, retry with origin:\"wolfgang\"; if you are a "
        "worker, use your assigned origin and keep it stable."
    )


def validate(tool, args):
    if tool in ORIGIN_EXEMPT_TOOLS:
        return None

    origin = args.get("origin")
    if not isinstance(origin, str) or not origin:
        return (
            f"Blocked figma-mcp-express `{tool}`: missing required top-level "
            "`origin`. Use origin:\"wolfgang\" for the orchestrator/self, or the "
            "worker origin explicitly assigned in the prompt. Do not rely on the "
            "schema enum's first value."
        )
    if origin not in ROSTER_SET:
        return (
            f"Blocked figma-mcp-express `{tool}`: unknown origin {origin!r}. "
            f"Valid origins are: {', '.join(ROSTER)}."
        )

    if tool == "set_presence" and "status" in args:
        status = args.get("status")
        if not isinstance(status, str) or status not in STATUSES:
            return (
                f"Blocked figma-mcp-express `set_presence`: unknown status {status!r}. "
                "Valid sticky statuses are: thinking, waiting_review, reviewing, "
                "approved, escalated, done."
            )

    if tool != "set_presence":
        forbidden = [key for key in ("status", "task") if key in args]
        if forbidden:
            return (
                f"Blocked figma-mcp-express `{tool}`: {', '.join(forbidden)} "
                "belongs on `set_presence`, not operational tools. Operational "
                "tools carry only top-level `origin` (and optional `channel`)."
            )

    if tool == "batch":
        violations = nested_presence_violations(batch_ops(args))
        if violations:
            return (
                "Blocked figma-mcp-express `batch`: presence/routing params are "
                f"nested inside ops ({', '.join(violations)}). Put `origin` and "
                "`channel` on the outer batch call only; use `set_presence` for "
                "status/task."
            )

    return None


def main():
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        allow()

    tool_name = data.get("tool_name", "")
    tool = fme_tool_name(tool_name)
    if not tool:
        allow()

    sid = session_id(data)
    if not skill_loaded(data):
        deny(skill_required_message(tool))

    args = tool_input(data)
    problem = validate(tool, args)
    if problem:
        deny(problem)

    messages = []
    if args.get("origin") == "grace":
        warning = grace_warning(sid, tool)
        if warning:
            messages.append(warning)
    if messages:
        allow_with_context("\n\n".join(messages))
    allow()


if __name__ == "__main__":
    main()
