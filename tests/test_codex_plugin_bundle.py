import json
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]
SOURCE_ENTRIES = (
    Path(".codex-plugin/plugin.json"),
    Path(".claude-plugin/plugin.json"),
    Path(".mcp.json"),
    Path("hooks"),
    Path("skills"),
)
BUNDLE = Path("plugins/figma-mcp-express")


def is_ignored(path):
    return (
        "__pycache__" in path.parts
        or path.name.endswith(".pyc")
        or path.name == ".DS_Store"
    )


def files_under(entry):
    root = REPO / entry
    if root.is_file():
        return [entry]
    return sorted(
        path.relative_to(REPO)
        for path in root.rglob("*")
        if path.is_file() and not is_ignored(path.relative_to(REPO))
    )


def entries_under(root):
    entries = []
    stack = [root]
    while stack:
        current = stack.pop()
        for child in current.iterdir():
            entries.append(child)
            if child.is_dir() and not child.is_symlink():
                stack.append(child)
    return sorted(entries)


class MarketplacePluginBundleTest(unittest.TestCase):
    def test_marketplaces_point_at_the_self_contained_bundle(self):
        codex_marketplace = json.loads(
            (REPO / ".agents/plugins/marketplace.json").read_text(encoding="utf-8")
        )
        claude_marketplace = json.loads(
            (REPO / ".claude-plugin/marketplace.json").read_text(encoding="utf-8")
        )

        self.assertEqual(
            codex_marketplace["plugins"][0]["source"]["path"],
            f"./{BUNDLE}",
        )
        self.assertEqual(
            claude_marketplace["plugins"][0]["source"],
            f"./{BUNDLE}",
        )

    def test_marketplace_plugin_bundle_is_self_contained(self):
        bundle_root = REPO / BUNDLE
        self.assertTrue(bundle_root.is_dir(), f"missing bundle root: {BUNDLE}")

        symlinks = [
            path.relative_to(REPO)
            for path in entries_under(bundle_root)
            if path.is_symlink()
        ]
        self.assertEqual(symlinks, [])

        for entry in SOURCE_ENTRIES:
            bundled_entry = bundle_root / entry
            self.assertTrue(bundled_entry.exists(), f"missing bundled {entry}")
            self.assertFalse(bundled_entry.is_symlink(), f"{BUNDLE / entry} is a symlink")

        self.assertFalse(
            (bundle_root / ".claude-plugin/marketplace.json").exists(),
            "Claude marketplace catalog should stay at the marketplace root, not inside the plugin bundle",
        )

    def test_marketplace_plugin_bundle_matches_source_files(self):
        for entry in SOURCE_ENTRIES:
            for source_rel in files_under(entry):
                source = REPO / source_rel
                bundled = REPO / BUNDLE / source_rel
                self.assertTrue(bundled.is_file(), f"missing bundled file: {BUNDLE / source_rel}")
                self.assertFalse(bundled.is_symlink(), f"bundled file is a symlink: {BUNDLE / source_rel}")
                self.assertEqual(
                    bundled.read_bytes(),
                    source.read_bytes(),
                    f"bundled file is stale: {BUNDLE / source_rel}",
                )


if __name__ == "__main__":
    unittest.main()
