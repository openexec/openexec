"""Fail-closed controls for the identity verifier's reused block evaluator."""
import tempfile
from pathlib import Path
import unittest

from retention_unit_coverage import evaluate


class IdentityCoverageTests(unittest.TestCase):
    def test_missing_coverage_block_refuses(self):
        function = dict(path='internal/planner/remap.go', name='RemapPlanIDs',
                        start={'Line': 1, 'Column': 1}, end={'Line': 5, 'Column': 1}, change='test')
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / 'coverage.out'
            profile.write_text('mode: count\n')
            with self.assertRaisesRegex(ValueError, 'missing/mismatched instrumentation'):
                evaluate([function], {'internal/planner/remap.go': [(2, 1, 3, 1, 1)]}, profile)

    def test_empty_instrumentation_refuses(self):
        function = dict(path='internal/planner/remap.go', name='RemapPlanIDs',
                        start={'Line': 1, 'Column': 1}, end={'Line': 5, 'Column': 1}, change='test')
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / 'coverage.out'
            profile.write_text('mode: count\n')
            with self.assertRaisesRegex(ValueError, 'empty instrumentation'):
                evaluate([function], {'internal/planner/remap.go': []}, profile)


if __name__ == '__main__':
    unittest.main()
