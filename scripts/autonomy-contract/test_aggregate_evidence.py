import contextlib
import io
import subprocess
import unittest
from unittest.mock import patch

from aggregate_evidence import AGGREGATES, ROOT, SCRIPT, leaves, verify


class AggregateTests(unittest.TestCase):
    def test_complete_story_inventory(self):
        self.assertEqual(set(leaves('story-evidence')), {
            'behavioral-reproduction', 'runtime-boundary', 'native-recovery-tracer',
            'exit-1-slice', 'recovery-matrix', 'compatibility-refusal',
            'implementation-unit-coverage', 'harness-tests',
        })
        for case in AGGREGATES:
            self.assertEqual(len(leaves(case)), len(set(leaves(case))))

    def test_aggregate_runs_every_child(self):
        for case in AGGREGATES:
            with self.subTest(case=case), \
                 patch('aggregate_evidence.subprocess.check_output', side_effect=['revision', b'', b'', 'revision']), \
                 patch('aggregate_evidence.subprocess.run') as run, \
                 contextlib.redirect_stdout(io.StringIO()) as output:
                verify(case)
            self.assertEqual(len(run.call_args_list), len(leaves(case)))
            for call, leaf in zip(run.call_args_list, leaves(case)):
                self.assertTrue(call.kwargs['check'])
                self.assertEqual(call.kwargs['cwd'], ROOT)
                if leaf == 'harness-tests':
                    self.assertIn('unittest', call.args[0])
                else:
                    self.assertEqual(call.args[0], ['bash', '-euo', 'pipefail', str(SCRIPT), '--case', leaf])
            self.assertIn(case + ': PASS candidate revision', output.getvalue())

    def test_every_failed_child_blocks_aggregate(self):
        for case in AGGREGATES:
            for index, child in enumerate(leaves(case)):
                with self.subTest(case=case, child=child), \
                     patch('aggregate_evidence.subprocess.check_output', side_effect=['revision', b'']), \
                     patch('aggregate_evidence.subprocess.run', side_effect=[None] * index + [subprocess.CalledProcessError(17, child)]) as run, \
                     contextlib.redirect_stdout(io.StringIO()) as output, \
                     self.assertRaises(subprocess.CalledProcessError) as error:
                    verify(case)
                self.assertEqual(error.exception.returncode, 17)
                self.assertEqual(run.call_count, index + 1)
                self.assertNotIn(case + ': PASS', output.getvalue())

    def test_uncommitted_or_changed_candidate_is_blocking(self):
        for metadata in (['revision', b' M source.go'],
                         ['revision', b'', b'?? source.go', 'revision'],
                         ['revision', b'', b'', 'other-revision']):
            with self.subTest(metadata=metadata), \
                 patch('aggregate_evidence.subprocess.check_output', side_effect=metadata), \
                 patch('aggregate_evidence.subprocess.run'), \
                 contextlib.redirect_stdout(io.StringIO()) as output, self.assertRaises(ValueError):
                verify('native-recovery')
            self.assertNotIn('native-recovery: PASS', output.getvalue())

    def test_shell_rejects_unsupported_and_malformed_cases(self):
        for args in ([], ['--case'], ['--case', 'unsupported'],
                     ['--wrong', 'story-evidence'], ['--case', 'story-evidence', 'extra']):
            result = subprocess.run(['bash', str(SCRIPT)] + args, text=True, capture_output=True)
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            self.assertNotIn('PASS', result.stdout)

    def test_shell_preserves_child_command_failure(self):
        # A real shell dispatch with a controlled failing python executable.
        import tempfile
        import os
        from pathlib import Path
        with tempfile.TemporaryDirectory() as temporary:
            python = Path(temporary) / 'python3'
            python.write_text('#!/bin/sh\nexit 17\n')
            python.chmod(0o755)
            env = dict(os.environ, PATH=temporary + os.pathsep + os.environ['PATH'])
            for case in (*AGGREGATES, 'runtime-boundary', 'implementation-unit-coverage'):
                result = subprocess.run(['bash', str(SCRIPT), '--case', case], env=env,
                                        text=True, capture_output=True)
                self.assertEqual(result.returncode, 17)
                self.assertNotIn('PASS', result.stdout)


if __name__ == '__main__':
    unittest.main()
