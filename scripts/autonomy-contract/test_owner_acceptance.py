"""Private disk fixtures test the consumer, never supply actual acceptance."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from owner_acceptance import verify

ROOT = Path(__file__).resolve().parents[2]


class OwnerAcceptanceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='owner-decision-fixture-')
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / 'fixture.json'
        self.candidate = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
        self.record = dict(candidate=self.candidate, pull_request='fixture:pr:1', actor='fixture:owner',
                           goal='G-006', task='T-US-010-002', decision='accept-merge', decision_ref='fixture:decision')
        self.env = {k: v for k, v in os.environ.items() if not k.startswith('OPENEXEC_OWNER_')}
        self.env.update(OPENEXEC_OWNER_PR='fixture:pr:1', OPENEXEC_OWNER_ID='fixture:owner',
            OPENEXEC_OWNER_DECISION_LOADER=json.dumps([sys.executable, '-c',
                'import pathlib,sys; print(pathlib.Path(sys.argv[1]).read_text())', str(self.path)]))

    def run_case(self):
        return subprocess.run(['bash', str(ROOT / 'scripts/autonomy-contract/verify-runtime-evidence.sh'),
                               '--case', 'owner-acceptance'], cwd=ROOT, env=self.env, text=True, capture_output=True)

    def test_valid_fixture_round_trip(self):
        self.path.write_text(json.dumps(self.record))
        before = self.path.read_bytes()
        for _ in range(2):
            result = self.run_case()
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.path.read_bytes(), before)

    def test_refuses_each_mismatch(self):
        for field in self.record:
            with self.subTest(field=field):
                record = self.record | {field: '' if field == 'decision_ref' else 'wrong'}
                self.path.write_text(json.dumps(record))
                result = self.run_case()
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('PASS', result.stdout)

    def test_absent_malformed_and_self_authorized(self):
        for record in (None, [], {}, {'authorized': True}, self.record | {'actor': 'worker', 'authorized': True}):
            self.path.write_text(json.dumps(record))
            self.assertNotEqual(self.run_case().returncode, 0)
        self.path.unlink()
        self.assertNotEqual(self.run_case().returncode, 0)
        self.path.write_text('not JSON')
        self.assertNotEqual(self.run_case().returncode, 0)

    def test_unconfigured_loader_and_context_refused(self):
        self.env.pop('OPENEXEC_OWNER_DECISION_LOADER')
        self.assertNotEqual(self.run_case().returncode, 0)
        for candidate, pr, owner in [('', 'pr', 'owner'), (self.candidate, '', 'owner'), (self.candidate, 'pr', '')]:
            with self.assertRaises(ValueError):
                verify(candidate, pr, owner, lambda: self.record)
        with self.assertRaises(ValueError):
            verify(self.candidate, 'pr', 'owner', None)

    def test_authentication_failure_cannot_be_overridden(self):
        self.path.write_text(json.dumps(self.record | {'authorized': True}))
        self.env['OPENEXEC_OWNER_DECISION_LOADER'] = json.dumps([sys.executable, '-c', 'raise SystemExit(19)'])
        self.assertNotEqual(self.run_case().returncode, 0)
