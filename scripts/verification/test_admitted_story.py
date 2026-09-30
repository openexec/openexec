"""The aggregate defers Console adoption until merge and refuses weak adoption reports."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import admitted_story as story


class AdoptionTests(unittest.TestCase):
    def proof(self):
        return dict(candidate_revision='candidate', status='passed', console_revision='c' * 40,
                    openexec_dependency='dependency', source_sha256={'source': 'a' * 64},
                    command=['actual-adapter-verifier'], exit_code=0, unresolved_work=[],
                    scenarios={case: dict.fromkeys(('exact_argv', 'workdir', 'exit_2', 'private_reference',
                               'database_reopen', 'repair_reload', 'redacted_public_output', 'diagnostic_tail'), True)
                               for case in story.ADAPTER_CASES}, coverage=dict(covered=91, statements=100))

    def test_report_contract(self):
        proof = self.proof()
        self.assertEqual(story.check_adoption(proof, 'candidate'), proof)
        with self.assertRaises(ValueError):
            story.check_adoption(None, 'candidate')
        for field in proof:
            bad = copy.deepcopy(proof)
            del bad[field]
            with self.subTest(field=field), self.assertRaises(ValueError):
                story.check_adoption(bad, 'candidate')

    def test_incomplete_stale_or_weak_proof(self):
        edits = [dict(candidate_revision='old'), dict(status='incomplete'), dict(exit_code=1),
                 dict(unresolved_work=['capture']), dict(scenarios={}), dict(scenarios=[]),
                 dict(console_revision='invalid'), dict(source_sha256={'source': 'invalid'}),
                 dict(command='true'), dict(coverage=[]),
                 dict(coverage=dict(covered=90, statements=100)),
                 dict(coverage=dict(covered=101, statements=100))]
        for edit in edits:
            with self.subTest(edit=edit), self.assertRaises(ValueError):
                story.check_adoption(self.proof() | edit, 'candidate')
        for case in story.ADAPTER_CASES:
            for field in self.proof()['scenarios'][case]:
                bad = self.proof()
                bad['scenarios'][case][field] = False
                with self.subTest(case=case, field=field), self.assertRaises(ValueError):
                    story.check_adoption(bad, 'candidate')

    def run_story(self, output, proof=None, returncode=0):
        with patch.object(story.subprocess, 'check_output', return_value='candidate'), \
             patch.object(story.subprocess, 'run') as run, \
             patch.object(story, 'adapter_journey', return_value=['fixture']):
            run.return_value.returncode = returncode
            return story.execute(output, proof)

    def test_fresh_local_success_defers_adoption_without_certifying_it(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            (output / 'result.json').write_text('{"status":"failed"}')
            result = self.run_story(output)
            self.assertEqual(result['status'], 'passed')
            self.assertEqual(set(result['local_checks']), set(story.CASES))
            self.assertEqual(json.loads((output / 'result.json').read_text()), result)
            self.assertEqual(result['adapter_adoption']['status'], 'deferred')
            self.assertIn('after this OpenExec change merges', result['adapter_adoption']['follow_up'])

    def test_deferred_adoption_cannot_hide_failed_local_case(self):
        with tempfile.TemporaryDirectory() as temp:
            self.assertEqual(self.run_story(Path(temp), returncode=1)['status'], 'failed')

    def test_supplied_weak_adoption_report_still_fails(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            proof = output / 'external.json'
            proof.write_text(json.dumps(self.proof() | dict(unresolved_work=['capture'])))
            result = self.run_story(output, proof)
            self.assertEqual(result['status'], 'failed')
            self.assertEqual(result['adapter_adoption']['status'], 'incomplete')

    def test_external_report_cannot_hide_failed_local_case(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            proof = output / 'external.json'
            proof.write_text(json.dumps(self.proof()))
            with patch.object(story.subprocess, 'check_output', return_value='candidate'), \
                 patch.object(story.subprocess, 'run') as run, \
                 patch.object(story, 'adapter_journey', return_value=['fixture']):
                run.return_value.returncode = 1
                self.assertEqual(story.execute(output, proof)['status'], 'failed')
