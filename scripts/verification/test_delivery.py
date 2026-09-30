"""Delivery composition refuses dropped failures, absent and skipped journeys."""
import json
import copy
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import delivery


class DeliveryTests(unittest.TestCase):
    def setUp(self):
        mock = patch.object(delivery, 'candidate_files', return_value='b' * 64)
        mock.start()
        self.addCleanup(mock.stop)

    def test_merge_receipt_requires_each_obligation_and_exact_candidate(self):
        report = dict(status='merged', candidate_revision='a' * 40,
                      candidate_files_sha256='b' * 64, console_revision='c' * 40,
                      merge_revision='d' * 40, default_branch_revision='e' * 40,
                      repository='openexec', default_branch='main', pull_request='pr:1',
                      owner_decision_ref='decision:1', independent_review_ref='review:1',
                      canonical_gate_ref='gate:1', merge_ref='merge:1',
                      candidate_in_default_branch=True, exact_candidate_reviewed=True,
                      exact_candidate_approved=True, canonical_gates_passed=True,
                      unresolved_findings=[])
        self.assertEqual(delivery.check_merge(report, 'a' * 40, 'b' * 64), report)
        for field in report:
            with self.subTest(field=field):
                bad = copy.deepcopy(report)
                del bad[field]
                with self.assertRaises(ValueError):
                    delivery.check_merge(bad, 'a' * 40, 'b' * 64)
        for field, value in [('candidate_revision', 'f' * 40),
                             ('candidate_files_sha256', 'f' * 64),
                             ('repository', 'other'), ('unresolved_findings', ['open']),
                             ('canonical_gates_passed', 1)]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                delivery.check_merge(dict(report, **{field: value}), 'a' * 40, 'b' * 64)

    def test_goal_complete_refuses_absent_merge_even_when_local_checks_pass(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(delivery, 'run_case', return_value=dict(passed=True)), patch.object(delivery.subprocess, 'check_output', return_value='fixture'):
            result = delivery.execute('goal-complete', Path(tmp))
            self.assertFalse(result['passed'])
            self.assertEqual(result['d2'], 'externally_pending')
            self.assertIn('D2 incomplete', result['error'])

    def test_adapter_report_is_forwarded_and_failure_preserved(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp)
            evidence = output / 'adoption.json'
            evidence.write_text('{}')
            with patch.object(delivery, 'command_result', return_value=dict(passed=False, exit_code=1)) as run:
                result = delivery.run_case('admitted-all', output, evidence)
                self.assertEqual(run.call_args.args[0][-2:], ['--adapter-evidence', str(evidence)])
                self.assertFalse(result['passed'])

    def test_candidate_changed_during_checks_refuses_success(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(delivery, 'run_case', return_value=dict(passed=True)), patch.object(delivery.subprocess, 'check_output', return_value='fixture'), patch.object(delivery, 'candidate_files', side_effect=['before', 'after']):
            result = delivery.execute('delivery-ready', Path(tmp))
            self.assertFalse(result['passed'])
            self.assertIn('changed', result['error'])

    def test_modes_require_every_member_and_preserve_failure(self):
        for mode in ('full', 'delivery-ready'):
            required = delivery.TECHNICAL if mode == 'delivery-ready' else (*delivery.TECHNICAL, *delivery.GATES)
            for failed in required:
                with self.subTest(mode=mode, failed=failed), tempfile.TemporaryDirectory() as tmp:
                    def run(case, output):
                        return dict(passed=case != failed, exit_code=23 if case == failed else 0)
                    with patch.object(delivery, 'run_case', side_effect=run), patch.object(delivery.subprocess, 'check_output', return_value='fixture'):
                        result = delivery.execute(mode, Path(tmp))
                    self.assertFalse(result['passed'])
                    self.assertEqual(set(result['cases']), set(required))
                    self.assertEqual(result['cases'][failed]['exit_code'], 23)
                    self.assertEqual(json.loads((Path(tmp) / 'result.json').read_text()), result)
                    self.assertEqual(result['d2'], 'externally_pending')

    def test_both_modes_can_pass_without_certifying_d2(self):
        for mode in ('full', 'delivery-ready'):
            with tempfile.TemporaryDirectory() as tmp, patch.object(delivery, 'run_case', return_value=dict(passed=True)), patch.object(delivery.subprocess, 'check_output', return_value='fixture'):
                result = delivery.execute(mode, Path(tmp))
                self.assertTrue(result['passed'])
                self.assertEqual(result['d2'], 'externally_pending')

    def test_retention_command_failure_keeps_exact_code(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(delivery.retention, 'run_case', side_effect=subprocess.CalledProcessError(17, ['go', 'test'])):
            result = delivery.run_case('retained-result', Path(tmp))
            self.assertFalse(result['passed'])
            self.assertEqual(result['exit_code'], 17)
            self.assertEqual(result['command'], ['go', 'test'])

    def test_native_missing_skipped_failed_and_duplicate_events(self):
        package = 'github.com/openexec/openexec/pkg/manager'
        events = [dict(Action='pass', Package=package, Test=s.split(':')[1]) for s in delivery.JOURNEYS]
        events.append(dict(Action='pass', Package=package))
        good = '\n'.join(map(json.dumps, events))
        bad = ['', '\n'.join(good.splitlines()[1:]), good.replace('pass', 'skip', 1), good.replace('pass', 'fail', 1), good+'\n'+good]
        for transcript in [good, *bad]:
            with tempfile.TemporaryDirectory() as tmp:
                def run(command, log):
                    log.write_text(transcript)
                    return dict(passed=True, exit_code=0)
                with patch.object(delivery, 'command_result', side_effect=run):
                    result = delivery.run_case('native-journey', Path(tmp))
                self.assertEqual(result['passed'], transcript == good)

    def test_gate_failure_not_converted_to_pass(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(delivery.subprocess, 'run') as run:
            run.return_value.returncode = 2
            result = delivery.run_case('check', Path(tmp))
            self.assertFalse(result['passed'])
            self.assertEqual(result['exit_code'], 2)
            self.assertEqual(result['command'], ['make', 'check'])


if __name__ == '__main__':
    unittest.main()
