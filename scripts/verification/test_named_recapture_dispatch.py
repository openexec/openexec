"""Run the retained task's public verification command, including falsifiers."""
import json
from pathlib import Path
import subprocess
import unittest


class NamedRecaptureDispatchTests(unittest.TestCase):
    def test_retained_task_command(self):
        root = Path(__file__).resolve().parents[2]
        result = subprocess.run(
            ['bash', str(root / 'scripts/verify-verification-repair.sh'),
             '--case', 'legacy-incident'],
            cwd='/tmp', capture_output=True, text=True, timeout=180)
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(set(report),
                         {'baseline', 'no_phase_inference', 'no_named_fallback'})
        self.assertEqual(report['baseline']['result'], 'passed')
        for gate in ('lint', 'test'):
            for phase in ('empty', 'matching'):
                self.assertIn(f'TestNamedRecaptureIncident/{gate}/{phase}',
                              report['baseline']['completions'])
        for mutation in ('no_phase_inference', 'no_named_fallback'):
            self.assertEqual(report[mutation]['result'],
                             'rejected at unresolved/needs_review with zero executions')
