"""Fail-closed controls for the executable discovery verifier."""
import copy
import json
import subprocess
import unittest
from unittest.mock import patch

import compact_requirement_evidence as verifier


class DiscoveryTests(unittest.TestCase):
    def setUp(self):
        self.data = json.loads(verifier.read_local(verifier.MANIFEST))
        self.evidence = verifier.read_local(verifier.EVIDENCE)

    def test_discovery_from_another_working_directory(self):
        result = subprocess.run(['bash', str(verifier.ROOT / 'scripts/verify-compact-requirement-evidence.sh'),
                                 'discovery'], cwd='/', capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        observed = json.loads(result.stdout)
        self.assertGreater(observed['functions'], 0)
        self.assertFalse(observed['repair_verified'])
        self.assertFalse(observed['delivery_verified'])

    def test_manifest_refusals(self):
        cases = []
        data = copy.deepcopy(self.data)
        data['functions'] = []
        cases.append((data, 'narrowed coverage scope'))
        data = copy.deepcopy(self.data)
        data['functions'].append(data['functions'][0])
        cases.append((data, 'duplicate scope'))
        data = copy.deepcopy(self.data)
        data['functions'][0]['symbol'] = 'Story.Missing'
        cases.append((data, 'absent executable function'))
        data = copy.deepcopy(self.data)
        data['functions'][0]['path'] = '../outside.go'
        cases.append((data, 'nonlocal source'))
        data = copy.deepcopy(self.data)
        data['functions'][0]['reason'] = ''
        cases.append((data, 'missing inclusion reason'))
        data = copy.deepcopy(self.data)
        data['contract'] = 'done when tests pass'
        cases.append((data, 'completion contract changed'))
        data = copy.deepcopy(self.data)
        data['repair_verification'] = 'true'
        cases.append((data, 'repair verification changed'))
        data = copy.deepcopy(self.data)
        data['fixtures'][0]['symbol'] = 'TestMissingFixture'
        cases.append((data, 'absent fixture'))
        for data, error in cases:
            with self.subTest(error=error), self.assertRaisesRegex(ValueError, error):
                verifier.validate(data, self.evidence)

    def test_missing_decision_and_false_delivery_refused(self):
        for before, after, error in [
                ('**Refinement —', '**Other —', 'disposition'),
                ('## G-007 completion and verification', '## Removed', 'evidence section'),
                ('D2 status: **unverified**', 'D2 status: **verified**', 'cannot certify delivery')]:
            with self.subTest(error=error), self.assertRaisesRegex(ValueError, error):
                verifier.validate(self.data, self.evidence.replace(before, after))

    def test_new_helper_cannot_escape_scope(self):
        read = verifier.read_local
        def changed(path):
            source = read(path)
            if path == 'internal/planner/planner.go':
                source += '\nfunc newCompactHelper() {}\n'
            return source
        with patch.object(verifier, 'read_local', side_effect=changed):
            with self.assertRaisesRegex(ValueError, 'new helper omitted from scope'):
                verifier.validate(self.data, self.evidence)

    def test_unknown_mode_cannot_pass(self):
        result = subprocess.run(['bash', str(verifier.ROOT / 'scripts/verify-compact-requirement-evidence.sh'),
                                 'unknown'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)


class DeliveryTests(unittest.TestCase):
    def setUp(self):
        self.evidence = verifier.read_local(verifier.EVIDENCE)

    def test_delivery_cli_preserves_unverified_obligations(self):
        result = subprocess.run(['bash', str(verifier.ROOT / 'scripts/verify-compact-requirement-evidence.sh'),
                                 'delivery'], cwd='/', capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        observed = json.loads(result.stdout)
        self.assertEqual(observed['mode'], 'delivery')
        self.assertEqual(observed['d2_status'], 'unverified')
        self.assertTrue(observed['assessment_validated'])
        self.assertFalse(observed['delivery_verified'])
        self.assertFalse(observed['repair_verified'])
        self.assertIn('cannot establish an unobserved merge', observed['validation_scope'])

    def test_delivery_refuses_missing_or_contradictory_obligations(self):
        changes = [
            ('D2 status: **unverified**.', 'D2 status: **verified**.'),
            ('D2 status: **unverified**.', 'D2 status: **unverified**.\nD2 status: **unverified**.'),
            ('unverified; retained', 'completed'),
            ('distinct from D2; incomplete full-story verification', 'passed'),
            ('Agent Console owns delivery', 'OpenExec owns delivery'),
            ('Agent Console owns the later agent-console parent retry', 'OpenExec owns the later agent-console parent retry'),
            ('https://github.com/openexec/openexec/pull/75', 'replacement PR'),
            ('after the task queue finishes', 'immediately'),
            ('fresh parent-run evidence', 'historical notes'),
            ('Historical notes are supporting context, not delivery receipts', 'Historical notes prove delivery'),
            ('not an OpenExec serving revision, merge receipt or deployment proof', 'OpenExec delivery proof'),
            ('Ancestry and a PR number in a commit subject do not prove a default-branch merge', 'Ancestry proves a merge'),
            ('Structural evidence validation cannot establish\nan unobserved merge', 'Structural validation proves a merge'),
            ('compact-requirement-results.json', 'missing.json'),
            ('65d800426c2ef16ee1a3e0d8224a3161f4982828', 'missing-commit'),
        ]
        row = next(line for line in self.evidence.splitlines() if line.startswith('| D2 delivery |'))
        changes.extend([(row, ''), (row, row + '\n' + row)])
        for before, after in changes:
            with self.subTest(before=before):
                self.assertIn(before, self.evidence)
                with self.assertRaises(ValueError):
                    verifier.delivery({}, self.evidence.replace(before, after))

    def test_delivery_requires_readable_local_references_and_consistent_history(self):
        read = verifier.read_local
        for mode in ('absent', 'changed'):
            def altered(path):
                if path == 'docs/verification/delivery-result.json':
                    if mode == 'absent':
                        raise FileNotFoundError(path)
                    data = json.loads(read(path))
                    data['goal_complete_negative']['exit_code'] = 0
                    return json.dumps(data)
                return read(path)
            with self.subTest(mode=mode), patch.object(verifier, 'read_local', side_effect=altered):
                with self.assertRaises((ValueError, FileNotFoundError)):
                    verifier.delivery({}, self.evidence)


if __name__ == '__main__':
    unittest.main()
