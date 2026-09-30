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
                                 'repair'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)


if __name__ == '__main__':
    unittest.main()
