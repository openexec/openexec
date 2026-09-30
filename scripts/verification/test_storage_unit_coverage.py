"""Fail-closed controls for the US-014 storage coverage verifier."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import storage_unit_coverage as gate
import test_retention_unit_coverage as controls


class StorageProfileTests(controls.CoverageGateTests):
    # Exercise the exact shared evaluator used by the storage gate, including
    # strict >90%, partial bodies, malformed profiles and skipped test events.
    def test_nonexistent_profile_refused(self):
        with self.assertRaises(FileNotFoundError):
            gate.shared.evaluate([self.function], self.blocks, self.profile)

    def test_missing_entire_file_refused(self):
        functions = [self.function, dict(self.function, path="pkg/other.go")]
        blocks = dict(self.blocks, **{"pkg/other.go": self.blocks["pkg/sample.go"]})
        with self.assertRaises(ValueError):
            self.evaluate("mode: count\n" + gate.shared.MODULE + "pkg/sample.go:1.10,2.2 9 1\n"
                          + gate.shared.MODULE + "pkg/sample.go:3.2,4.2 1 1\n", functions, blocks)


class StorageScopeTests(unittest.TestCase):
    def test_whole_file_inventory_includes_new_functions(self):
        functions = [dict(name=name, source="body", start={}, end={}) for name in ("Read", "NewHelper")]
        with patch.object(gate.shared, "run", return_value=json.dumps(functions)):
            selected = gate.inventory(Path("helper"), {"sources": {"storage.go": "*"}})
        self.assertEqual([f["name"] for f in selected], ["Read", "NewHelper"])

    def test_absent_duplicate_or_empty_scope_refused(self):
        with patch.object(gate.shared, "run", return_value=json.dumps([dict(name="Read", source="body")])):
            for sources in ({}, {"storage.go": ["Missing"]}, {"storage.go": ["Read", "Read"]}):
                with self.subTest(sources=sources), self.assertRaises(ValueError):
                    gate.inventory(Path("helper"), {"sources": sources})

    def test_failed_run_removes_stale_success(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            for name in ("result.json", "scope.json", "coverage.out", "tests.jsonl"):
                (output / name).write_text("stale success")
            with patch("sys.argv", ["storage_unit_coverage.py", "--output", temp]), \
                    patch.object(gate, "MANIFEST", output / "missing-manifest"):
                with self.assertRaises(FileNotFoundError):
                    gate.main()
            self.assertFalse(any(output.iterdir()))


if __name__ == "__main__":
    unittest.main()
