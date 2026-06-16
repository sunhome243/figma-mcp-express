#!/usr/bin/env python3
"""
PreToolUse — nudges (at most ONCE per session) to load the /figma-mcp-express
skill before using its MCP tools. Warn-not-block (systemMessage; the model complies).

The nudge fires once and then stays quiet: after warning, the hook records a
per-session flag so later tool calls in the same session are silent — even when
the skill is never loaded (e.g. while developing this repo). The skill's on-load
touch also writes this flag; when CLAUDE_SESSION_ID is set both sides derive the
same path and loading the skill first suppresses even the one nudge. Without it
the keys differ, but the hook's own self-write still caps the nudge at once.
"""
import json, sys, os, re

data = json.load(sys.stdin)
tool_name = data.get("tool_name", "")

if not tool_name.startswith("mcp__figma-mcp-express__"):
    print(json.dumps({"continue": True})); sys.exit(0)

# Stable per-session id. The hook payload's session_id is consistent across the
# whole session; os.getppid() is NOT (a fresh hook process per call gave a new
# pid each time, so the flag path never matched and the nudge repeated on every
# call). Fall back to the env session id, then a constant, but never to getppid().
session_id = (
    data.get("session_id")
    or os.environ.get("CLAUDE_SESSION_ID")
    or os.environ.get("CODEX_SESSION_ID")
    or "default"
)
# Sanitize before interpolating into a path — drop anything outside [A-Za-z0-9_-]
# so a malformed/hostile session_id can't escape /tmp (path-traversal hardening).
session_id = re.sub(r"[^A-Za-z0-9_-]", "", session_id) or "default"
flag_path = f"/tmp/fme-skill-loaded-{session_id}"

if os.path.exists(flag_path):
    print(json.dumps({"continue": True})); sys.exit(0)

# Record the flag now so this nudge fires at most once per session.
try:
    open(flag_path, "w").close()
except OSError:
    pass

print(json.dumps({
    "continue": True,
    "systemMessage": (
        "⚠️  Load the /figma-mcp-express skill for correct tool use "
        "(setup checklist, read/write workflows, batch patterns, probe-first "
        "discipline, error handling): Skill({skill: 'figma-mcp-express'}). "
        "This reminder shows once per session."
    )
}))
