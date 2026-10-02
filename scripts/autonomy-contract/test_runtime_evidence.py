import contextlib
import io
import json
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch
from runtime_evidence import BOUNDARY, COMPATIBILITY, LOCAL_CASES, RECOVERY, REFUSALS, PREFIX, main, run_tests, validate_events

class InventoryTests(unittest.TestCase):
    def event(self, action, name='required'):
        return {'Action': action, 'Test': name}

    def test_pass(self):
        validate_events([self.event('run'), self.event('pass')], ['required'])

    def test_reject_incomplete_evidence(self):
        for events in [[], [self.event('pass')], [self.event('run')],
                       [self.event('run'), self.event('skip')],
                       [self.event('run'), self.event('fail')],
                       [self.event('run', 'other'), self.event('pass', 'other')],
                       [self.event('run'), self.event('pass'), {'Action': 'fail'}]]:
            with self.subTest(events=events), self.assertRaises(ValueError):
                validate_events(events, ['required'])

    def test_baseline_requires_actual_failure(self):
        validate_events([self.event('run'), self.event('fail')], ['required'], 'required')
        for events in [[], [self.event('run'), self.event('pass')], [{'Action':'fail'}]]:
            with self.subTest(events=events), self.assertRaises(ValueError):
                validate_events(events, ['required'], 'required')

class CommandTests(unittest.TestCase):
    def events(self, names=('required',)):
        return ([{'Action': action, 'Test': name, 'Package': PREFIX + 'pkg/manager'}
                 for name in names for action in ('run', 'pass')]
                + [{'Action': 'pass', 'Package': PREFIX + 'pkg/manager'}])

    def run_fixture(self, events, code=0, names=('required',), raw=None):
        output = raw if raw is not None else '\n'.join(json.dumps(e) for e in events)
        result = subprocess.CompletedProcess([], code, output, '')
        with patch('runtime_evidence.subprocess.run', return_value=result), contextlib.redirect_stdout(io.StringIO()):
            return run_tests(Path('.'), 'pkg/manager', list(names))

    def test_complete_command(self):
        self.run_fixture(self.events())

    def test_fail_closed_command(self):
        good = self.events()
        cases = [([], 0, None), (good, 1, None), (good, -9, None),
                 (good[:-1], 0, None), (good + [good[-1]], 0, None),
                 (good, 0, 'not json'), ([], 0, 'null'),
                 ([dict(e, Package='wrong') for e in good], 0, None),
                 ([dict(e, Action='skip') if e['Action'] == 'pass' else e for e in good], 0, None),
                 ([dict(e, Action='fail') if e['Action'] == 'pass' else e for e in good], 0, None)]
        for events, code, raw in cases:
            with self.subTest(events=events, code=code, raw=raw), self.assertRaises(ValueError):
                self.run_fixture(events, code, raw=raw)

    def test_matrix_requires_every_named_branch(self):
        parent = 'TestPersistedRecoveryMatrix'
        branches = [f'{parent}/exit_{code}/restart_{restart}'
                    for code in (0, 1, 125) for restart in ('false', 'true')]
        self.run_fixture(self.events([parent] + branches), names=[parent])
        for missing in branches:
            with self.subTest(missing=missing), self.assertRaises(ValueError):
                self.run_fixture(self.events([parent] + [n for n in branches if n != missing]), names=[parent])

    def test_compatibility_requires_named_branches(self):
        inventories = {
            'TestLocalCommandFailureCompatibility': LOCAL_CASES,
            'TestPersistedTerminalRefusalsDoNotRepair': REFUSALS,
            'TestCompatibility_ExistingProjects_StatusCLI': ['current_openexec_project', 'legacy_uaos_project'],
            'TestCompatibility_LegacyTasksJSONFallback': ['initial', 'reloaded'],
        }
        for parent, children in inventories.items():
            names = [parent] + [parent + '/' + child for child in children]
            self.run_fixture(self.events(names), names=[parent])
            for missing in names[1:]:
                with self.subTest(missing=missing), self.assertRaises(ValueError):
                    self.run_fixture(self.events([n for n in names if n != missing]), names=[parent])

    def test_compatibility_runs_behavior_without_coverage_or_baseline(self):
        with patch('sys.argv', ['runtime_evidence.py', '--case', 'compatibility-refusal']), \
             patch('runtime_evidence.run_tests') as run, \
             patch('runtime_evidence.baseline') as baseline, \
             contextlib.redirect_stdout(io.StringIO()):
            main()
        self.assertEqual([(c.args[1], c.args[2]) for c in run.call_args_list],
                         list(BOUNDARY.items()) + list(RECOVERY.items()) + list(COMPATIBILITY.items()))
        baseline.assert_not_called()

    def test_timeout_is_failure(self):
        with patch('runtime_evidence.subprocess.run', side_effect=subprocess.TimeoutExpired('go', 120)):
            with self.assertRaises(subprocess.TimeoutExpired):
                run_tests(Path('.'), 'pkg/manager', ['required'])

    def test_behavioral_cases_never_print_pass_after_failure(self):
        for case in ('recovery-matrix', 'compatibility-refusal'):
            output = io.StringIO()
            with self.subTest(case=case), \
                 patch('sys.argv', ['runtime_evidence.py', '--case', case]), \
                 patch('runtime_evidence.run_tests', side_effect=ValueError('refused')), \
                 contextlib.redirect_stdout(output), self.assertRaises(ValueError):
                main()
            self.assertNotIn('PASS', output.getvalue())


if __name__ == '__main__':
    unittest.main()
