import json
import os
import subprocess
import sys
import tempfile
import unittest
import uuid
from pathlib import Path


HOOK = Path(__file__).with_name("pre-tool.py")


class PreToolHookTest(unittest.TestCase):
    def run_hook(self, payload, skill_loaded=True):
        sid = payload.setdefault("session_id", f"test-{uuid.uuid4().hex}")
        self.addCleanup(lambda: self.cleanup_flags(sid))
        if skill_loaded:
            Path(self.flag_path("skill-loaded", sid)).touch()
        proc = subprocess.run(
            [sys.executable, str(HOOK)],
            input=json.dumps(payload),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        try:
            data = json.loads(proc.stdout or "{}")
        except json.JSONDecodeError as exc:
            self.fail(f"invalid JSON stdout: {proc.stdout!r}; stderr={proc.stderr!r}; {exc}")
        return proc.returncode, data

    @staticmethod
    def safe_session_id(session_id):
        return "".join(ch for ch in session_id if ch.isalnum() or ch in "_-") or "default"

    @classmethod
    def flag_path(cls, kind, session_id):
        return os.path.join(tempfile.gettempdir(), f"fme-{kind}-{cls.safe_session_id(session_id)}")

    @classmethod
    def cleanup_flags(cls, session_id):
        for kind in ("skill-loaded", "grace-origin-warning"):
            try:
                os.remove(cls.flag_path(kind, session_id))
            except FileNotFoundError:
                pass

    def test_ignores_non_figma_mcp_tools(self):
        code, out = self.run_hook({"tool_name": "Bash", "tool_input": {"command": "true"}})
        self.assertEqual(code, 0)
        self.assertEqual(out, {"continue": True})

    def test_blocks_figma_mcp_until_skill_loaded(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__get_metadata",
            "tool_input": {"origin": "wolfgang"},
        }, skill_loaded=False)
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("skill has not been loaded", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_blocks_origin_exempt_tools_until_skill_loaded(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__list_channels",
            "tool_input": {},
        }, skill_loaded=False)
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("skill has not been loaded", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_blocks_missing_origin_on_plugin_facing_tool(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__get_metadata",
            "tool_input": {},
        })
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("missing required top-level `origin`", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_allows_origin_exempt_tools_without_origin(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__list_channels",
            "tool_input": {},
        })
        self.assertEqual(code, 0)
        self.assertTrue(out["continue"])

    def test_accepts_dev_server_with_valid_origin(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express-dev__set_reactions",
            "tool_input": {"origin": "theo", "nodeId": "1:2", "reactions": []},
        })
        self.assertEqual(code, 0)
        self.assertTrue(out["continue"])

    def test_blocks_unknown_origin(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__get_node",
            "tool_input": {"origin": "random-agent", "nodeId": "1:2"},
        })
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("unknown origin", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_blocks_operational_status(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__batch",
            "tool_input": {
                "origin": "wolfgang",
                "status": "reviewing",
                "ops": [{"type": "get_metadata"}],
            },
        })
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("belongs on `set_presence`", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_allows_set_presence_status_and_task(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__set_presence",
            "tool_input": {
                "origin": "wolfgang",
                "status": "reviewing",
                "task": "reviewing prototype wiring",
            },
        })
        self.assertEqual(code, 0)
        self.assertTrue(out["continue"])

    def test_blocks_unknown_set_presence_status(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__set_presence",
            "tool_input": {"origin": "wolfgang", "status": "blocked"},
        })
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("unknown status", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_blocks_nested_batch_presence_params(self):
        code, out = self.run_hook({
            "tool_name": "mcp__figma-mcp-express__batch",
            "tool_input": {
                "origin": "wolfgang",
                "channel": "auto-1",
                "ops": [
                    {
                        "type": "set_reactions",
                        "nodeIds": ["1:2"],
                        "params": {"origin": "grace", "reactions": []},
                    }
                ],
            },
        })
        self.assertEqual(code, 0)
        self.assertEqual(out["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertIn("ops[0].params.origin", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_warns_once_for_grace_without_blocking(self):
        sid = f"test-{uuid.uuid4().hex}"
        payload = {
            "session_id": sid,
            "tool_name": "mcp__figma-mcp-express__get_metadata",
            "tool_input": {"origin": "grace"},
        }
        code, out = self.run_hook(dict(payload))
        self.assertEqual(code, 0)
        self.assertIn("origin:\"grace\"", out["hookSpecificOutput"].get("additionalContext", ""))

        code, out = self.run_hook(dict(payload))
        self.assertEqual(code, 0)
        self.assertNotIn("origin:\"grace\"", out.get("hookSpecificOutput", {}).get("additionalContext", ""))


if __name__ == "__main__":
    unittest.main()
