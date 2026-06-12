#!/usr/bin/env python3
"""
PreToolUse — warns if /figma-mcp-express skill not yet loaded this session.
Approach: warn-not-block (systemMessage; model will comply).
"""
import json, sys, os

data = json.load(sys.stdin)
tool_name = data.get("tool_name", "")

if not tool_name.startswith("mcp__figma-mcp-express__"):
    print(json.dumps({"continue": True})); sys.exit(0)

# File-based session flag — set by the skill on load
session_id = os.environ.get("CLAUDE_SESSION_ID", os.environ.get("CODEX_SESSION_ID", str(os.getppid())))
flag_path = f"/tmp/fme-skill-loaded-{session_id}"

if os.path.exists(flag_path):
    print(json.dumps({"continue": True})); sys.exit(0)

print(json.dumps({
    "continue": True,
    "systemMessage": (
        "⚠️  MANDATORY: Load the /figma-mcp-express skill before this tool call. "
        "It contains the setup checklist, read/write workflows, batch patterns, "
        "probe-first discipline, and error handling for correct tool use. "
        "Call Skill({skill: 'figma-mcp-express'}) first."
    )
}))
