"""Fail closed on child failures, dirty/moved candidates and stale evidence."""
import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import goal_validation as goal


class GoalValidationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)

    def run_step(self, exit_code=0, identities=None):
        with patch.object(goal, 'identity', side_effect=identities or ['a' * 40, 'a' * 40]), \
             patch.object(goal, 'handoff'), patch.object(goal.subprocess, 'run', return_value=subprocess.CompletedProcess([], exit_code)) as run, \
             contextlib.redirect_stdout(io.StringIO()):
            code = goal.step(self.directory)
        return code, run

    def test_complete_round_trip_inventory_without_owner_decision(self):
        for index, (_, command) in enumerate(goal.CHECKS):
            code, run = self.run_step()
            self.assertEqual(code, 0 if index == len(goal.CHECKS) - 1 else 3)
            self.assertEqual(run.call_args.args[0], command)
        report = json.loads((self.directory / 'results.json').read_text())
        self.assertEqual(len(report['results']), len(goal.CHECKS))
        code, run = self.run_step()
        self.assertEqual(code, 0)
        run.assert_not_called()
        self.assertFalse(any('owner-acceptance' in command for _, command in goal.CHECKS))
        self.assertEqual([n for n, _ in goal.CHECKS], ['architecture', 'engine', 'external-consumer',
                                                       'make-test', 'make-compat-test', 'make-type-check'])

    def test_each_failed_child_is_saved_and_blocks_pass_and_resume(self):
        for index in range(len(goal.CHECKS)):
            (self.directory / 'results.json').unlink(missing_ok=True)
            for _ in range(index):
                self.run_step()
            code, _ = self.run_step(exit_code=17)
            self.assertEqual(code, 1)
            self.assertEqual(json.loads((self.directory / 'results.json').read_text())['results'][-1]['exit'], 17)
            with self.assertRaises(ValueError):
                self.run_step()

    def test_moved_candidate_and_stale_report_refused(self):
        with self.assertRaises(ValueError):
            self.run_step(identities=['a' * 40, 'b' * 40])
        self.assertFalse((self.directory / 'results.json').exists())
        self.run_step()
        with self.assertRaises(ValueError):
            self.run_step(identities=['b' * 40])

    def test_dirty_candidate_refused(self):
        with patch.object(goal.subprocess, 'check_output', side_effect=['a' * 40, b' M source']), self.assertRaises(ValueError):
            goal.identity()

    def test_missing_handoff_and_terms_refused(self):
        with patch.object(goal, 'ROOT', self.directory):
            with self.assertRaises(OSError):
                goal.handoff()
            (self.directory / 'docs').mkdir()
            (self.directory / 'docs/runtime-verification-handoff.md').write_text('incomplete')
            with self.assertRaises(ValueError):
                goal.handoff()
