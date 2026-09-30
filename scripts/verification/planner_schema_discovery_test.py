"""Negative controls for false-green discovery and candidate substitution."""
import json
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

import planner_schema_discovery as discovery


class DiscoveryControls(unittest.TestCase):
    def test_missing_or_duplicate_discovered_test(self):
        for output in ("ok runtime [no tests to run]", discovery.TESTS[0],
                       "\n".join((*discovery.TESTS, discovery.TESTS[0]))):
            with self.subTest(output=output), self.assertRaises(ValueError):
                discovery.check_discovery(output)

    def test_no_tests_executed(self):
        output = json.dumps({"Package": discovery.PACKAGE, "Action": "pass"})
        with self.assertRaises(ValueError):
            discovery.check_execution(output)

    def test_skip_or_fail_is_not_success(self):
        for action in ("skip", "fail"):
            with self.subTest(action=action), self.assertRaises(ValueError):
                discovery.check_execution(json.dumps({"Package": discovery.PACKAGE,
                                                       "Test": discovery.TESTS[0], "Action": action}))

    def test_wrong_package_is_not_success(self):
        with self.assertRaises(ValueError):
            discovery.check_execution(json.dumps({"Package": "wrong/pkg/runtime", "Action": "pass"}))

    def test_nonzero_command_with_green_output(self):
        result = subprocess.CompletedProcess([], 1, "PASS", "compiler failure")
        with patch.object(subprocess, "run", return_value=result), self.assertRaisesRegex(ValueError, "exited 1"):
            discovery.command(["go", "test"], {})

    def test_candidate_substitution(self):
        root = Path("/tmp/expected-candidate")
        package = {"ImportPath": discovery.PACKAGE, "Dir": str(root / "pkg/runtime"),
                   "Module": {"Path": discovery.MODULE, "Main": True, "Dir": str(root)}}
        discovery.check_package(package, root)
        for field, value in (("Dir", "/tmp/another-candidate"), ("Main", False),
                             ("Path", "other/module"), ("Replace", {"Dir": str(root)})):
            with self.subTest(field=field), self.assertRaises(ValueError):
                modified = {**package, "Module": {**package["Module"], field: value}}
                discovery.check_package(modified, root)

    def test_fixture_digest_tampering(self):
        original = Path.read_bytes

        def changed(path):
            data = original(path)
            return data + b" " if path.name == "empty-array.json" else data

        with patch.object(Path, "read_bytes", changed), self.assertRaisesRegex(ValueError, "digest mismatch"):
            discovery.check_fixtures()

    def test_invocation_from_another_root(self):
        with patch.object(Path, "cwd", return_value=Path("/tmp/wrong-candidate")), self.assertRaisesRegex(ValueError, "candidate repository root"):
            discovery.main()


if __name__ == "__main__":
    unittest.main()
