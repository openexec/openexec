"""Negative controls for the recovery verifier's shared fail-closed accounting."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import planner_schema_recovery as recovery


class RecoveryVerifierTests(unittest.TestCase):
    def test_statement_threshold_and_missing_instrumentation(self):
        function = dict(path='example.go', name='recover', change='modified',
                        start=dict(Line=1, Column=1), end=dict(Line=10, Column=1))
        blocks = {'example.go': [(2, 1, 3, 1, 9), (4, 1, 5, 1, 1)]}
        with tempfile.TemporaryDirectory() as directory, patch.object(recovery.coverage, 'run', return_value='baseline'):
            profile = Path(directory) / 'coverage.out'
            profile.write_text('mode: count\nexample.go:2.1,3.1 9 1\nexample.go:4.1,5.1 1 0\n')
            self.assertFalse(recovery.coverage.evaluate([function], blocks, profile)['passed'])
            profile.write_text('mode: count\nexample.go:2.1,3.1 9 1\nexample.go:4.1,5.1 1 1\n')
            self.assertTrue(recovery.coverage.evaluate([function], blocks, profile)['passed'])
            profile.write_text('mode: count\nexample.go:2.1,3.1 9 1\n')
            with self.assertRaisesRegex(ValueError, 'missing/mismatched instrumentation'):
                recovery.coverage.evaluate([function], blocks, profile)
            profile.write_text('mode: count\nexample.go:2.1,3.1 9 0\nexample.go:4.1,5.1 1 0\n')
            self.assertEqual(recovery.coverage.evaluate([function], blocks, profile)['covered'], 0)

    def test_missing_and_skipped_tests_fail(self):
        package = recovery.coverage.MODULE + 'internal/planner'
        with patch.object(recovery.coverage, 'PACKAGES', ['internal/planner']):
            complete = [dict(Action='pass', Package=package), dict(Action='pass', Package=package, Test='TestSchema')]
            required = ['internal/planner:TestSchema']
            recovery.coverage.check_tests('\n'.join(map(json.dumps, complete)), required)
            with self.assertRaisesRegex(ValueError, 'missing required'):
                recovery.coverage.check_tests(json.dumps(complete[0]), required)
            complete[1]['Action'] = 'skip'
            with self.assertRaisesRegex(ValueError, 'skip'):
                recovery.coverage.check_tests('\n'.join(map(json.dumps, complete)), required)

    def test_changed_function_cannot_be_omitted(self):
        with tempfile.TemporaryDirectory() as directory:
            helper = Path(directory) / 'inventory'
            recovery.coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
            manifest = json.loads(recovery.MANIFEST.read_text())
            manifest['functions'].remove('pkg/manager/planner_replay.go:(*Manager).replayReviewedPlan')
            with self.assertRaisesRegex(ValueError, 'scope mismatch'):
                recovery.inventory(helper, Path(directory) / 'old.go', manifest)


if __name__ == '__main__':
    unittest.main()
