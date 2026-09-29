"""Exercise the verifier against isolated documentation/source mutations."""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from discovery import verify

ROOT = Path(__file__).resolve().parents[2]


class DiscoveryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        # Only files referenced by discovery; no runtime ledger or git mutations.
        import re
        body = (ROOT / "docs/ARCHITECTURE.md").read_text()
        paths = {"docs/ARCHITECTURE.md", "NOTES.md"}
        paths.update(re.findall(r"^\| `([^`]+\.go)` \|", body, re.M))
        paths.update(str(Path("docs") / p) for p in re.findall(r"\]\(([^)]+)\)", body))
        for path in paths:
            target = self.root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / path, target)

    def change(self, old, new):
        path = self.root / "docs/ARCHITECTURE.md"
        body = path.read_text()
        self.assertIn(old, body)
        path.write_text(body.replace(old, new))

    def test_valid_reloaded_document(self):
        self.assertEqual(verify(self.root)[0], [])

    def test_missing_source(self):
        (self.root / "internal/blueprint/engine.go").unlink()
        self.assertIn("missing source", "\n".join(verify(self.root)[0]))

    def test_missing_declaration(self):
        (self.root / "internal/blueprint/engine.go").write_text("package blueprint\n")
        self.assertIn("missing declaration", "\n".join(verify(self.root)[0]))

    def test_omitted_source(self):
        self.change('| `internal/blueprint/engine.go` | `ExecuteStage` |', '| omitted | `ExecuteStage` |')
        self.assertIn("required source references omitted", verify(self.root)[0])

    def test_mapping_wrong_owner(self):
        self.change('| US-008 |', '| US-009 |')
        self.assertIn("wrong owner: REQ-001", "\n".join(verify(self.root)[0]))

    def test_duplicate_mapping(self):
        self.change('## Accepted requirement mapping', '| REQ-001 | duplicate | US-008 |\n\n## Accepted requirement mapping')
        self.assertIn("duplicate mapping", "\n".join(verify(self.root)[0]))

    def test_shared_helper(self):
        self.change('No sibling test edits | retention-unit-coverage', '`scripts/verification/retention-unit-coverage.sh` | retention-unit-coverage')
        self.assertIn("shared independent", "\n".join(verify(self.root)[0]))

    def test_broken_link(self):
        self.change('(LIGHT_MODE.md)', '(missing.md)')
        self.assertIn("broken documentation link", "\n".join(verify(self.root)[0]))

    def test_dispatcher_refuses_unknown_and_malformed_cases(self):
        script = ROOT / "scripts/verify-retained-verification-evidence.sh"
        for args in ([], ['--case'], ['--case', 'unknown'],
                     ['--case', 'discovery', 'extra']):
            with self.subTest(args=args):
                result = subprocess.run(['bash', str(script), *args], cwd=self.root,
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 2, result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()
