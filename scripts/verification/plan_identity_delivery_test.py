"""Repository-local record fixtures and failure branches, including CLI round trips."""
import copy
from datetime import datetime, timezone, timedelta
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import plan_identity_delivery as verifier


class DeliveryEvidenceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix='identity-delivery-')
        cls.root = Path(cls.temp.name)
        cls.record = verifier.read(verifier.ROOT / verifier.RECORD)
        paths = {verifier.MANIFEST, verifier.RECORD, *verifier.VERIFIER_FILES}
        paths.update(line.split('  ', 1)[1] for line in
                     (verifier.ROOT / verifier.MANIFEST).read_text().splitlines())
        paths.update(c['log'] for c in cls.record['checks'].values())
        for name in paths:
            target = cls.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(verifier.ROOT / name, target)
        subprocess.run(['git', 'init', '-q', str(cls.root)], check=True)
        subprocess.run(['git', 'add', '.'], cwd=cls.root, check=True)

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def setUp(self):
        self.value = copy.deepcopy(self.record)

    def refuse(self, message, value=None, **kwargs):
        with self.assertRaisesRegex(ValueError, message):
            verifier.validate(self.root, value or self.value, **kwargs)

    def rewrite_log(self, name, change):
        check = self.value['checks'][name]
        path = self.root / check['log']
        original = path.read_bytes()
        self.addCleanup(path.write_bytes, original)
        path.write_text(change(path.read_text()))
        check['sha256'] = verifier.sha(path)

    def test_repository_fixtures(self):
        fixtures = verifier.ROOT / 'scripts/verification/fixtures/plan-identity-delivery'
        for name, error in [('complete', None), ('incomplete', 'disposition'), ('stale', 'stale')]:
            with self.subTest(name=name):
                value = dict(self.record, **verifier.read(fixtures / (name + '.json')))
                if error:
                    self.refuse(error, value)
                else:
                    result = verifier.validate(self.root, value)
                    self.assertEqual(result, dict(passed=True, preparation='ready', d2='pending', goal_complete=False))

    def test_every_required_check(self):
        for name in verifier.CHECKS:
            value = copy.deepcopy(self.record)
            del value['checks'][name]
            self.refuse('absent required checks', value)

    def test_every_mutation_failure(self):
        for name in verifier.MUTATIONS:
            with self.subTest(name=name):
                path = self.root / self.value['checks']['mutations']['log']
                original = path.read_bytes()
                try:
                    self.rewrite_log('mutations', lambda s: '\n'.join(
                        line for line in s.splitlines() if not line.startswith('PASS mutation: ' + name + ' compiled;')))
                    self.refuse('expected mutation')
                finally:
                    path.write_bytes(original)

    def test_failed_or_boolean_exit_code(self):
        for exit_code in (1, False, '0'):
            self.value['checks']['targeted']['exit_code'] = exit_code
            self.refuse('contradictory check')

    def test_false_pass(self):
        self.value['checks']['targeted']['passed'] = False
        self.refuse('contradictory check')

    def test_wrong_command(self):
        self.value['checks']['targeted']['command'] = 'true'
        self.refuse('wrong check command')

    def test_wrong_candidate(self):
        self.value['branch'] = 'main'
        self.refuse('candidate provenance')

    def test_missing_repair_evidence(self):
        self.value['findings'][0]['evidence'] = []
        self.refuse('missing repair evidence')

    def test_stale_source(self):
        path = self.root / 'pkg/manager/planner.go'
        original = path.read_bytes()
        self.addCleanup(path.write_bytes, original)
        path.write_bytes(original + b'\n// stale\n')
        self.refuse('stale content digest')

    def test_omitted_source(self):
        path = self.root / verifier.MANIFEST
        original = path.read_bytes()
        self.addCleanup(path.write_bytes, original)
        path.write_text('\n'.join(line for line in path.read_text().splitlines()
                                  if not line.endswith('pkg/manager/planner.go')) + '\n')
        self.value['content_sha256'] = verifier.sha(path)
        self.refuse('omits protected')

    def test_stale_verifier(self):
        name = 'scripts/verification/plan_identity_delivery.py'
        path = self.root / name
        original = path.read_bytes()
        self.addCleanup(path.write_bytes, original)
        path.write_bytes(original + b'\n# altered\n')
        self.refuse('stale verifier content')

    def test_stale_result_binding(self):
        self.value['checks']['targeted']['content_sha256'] = '0' * 64
        self.refuse('stale check content')

    def test_tampered_log(self):
        self.value['checks']['targeted']['sha256'] = '0' * 64
        self.refuse('stale verification log')

    def test_coverage_at_or_below_threshold(self):
        for count in (9, 8):
            with self.subTest(covered=count):
                path = self.root / self.value['checks']['coverage']['log']
                original = path.read_bytes()
                try:
                    self.rewrite_log('coverage', lambda s: s.replace('15/15 (100.00%)', f'{count}/10 ({count * 10:.2f}%)', 1))
                    self.refuse('exceed 90%')
                finally:
                    path.write_bytes(original)

    def test_missing_coverage_function(self):
        self.rewrite_log('coverage', lambda s: '\n'.join(line for line in s.splitlines()
                         if not line.startswith('internal/planner/remap.go:RemapPlanIDs:')))
        self.refuse('missing required function')

    def test_contradictory_coverage(self):
        self.value['coverage'][next(iter(self.value['coverage']))] = [1, 1]
        self.refuse('contradictory stored coverage')

    def test_missing_test_execution(self):
        self.rewrite_log('targeted', lambda s: s.replace('PASS pkg/manager:TestReviewedIdentityLifecycle\n', ''))
        self.refuse('missing mandatory test')

    def test_contradictory_log(self):
        self.rewrite_log('targeted', lambda s: s + '\nFAIL hidden failure\n')
        self.refuse('contradictory log')

    def test_path_escape(self):
        self.value['checks']['targeted']['log'] = '/tmp/log'
        self.refuse('nonlocal path')

    def test_contradictory_goal(self):
        self.value['goal_complete'] = True
        self.refuse('contradictory Goal')

    def test_merge_claim_requires_coordinator(self):
        self.value.update(d2='merged', goal_complete=True)
        self.refuse('coordinator-supplied')

    def test_goal_complete_requires_coordinator(self):
        self.refuse('coordinator-supplied', goal_complete=True)

    def receipt(self):
        return dict(issuer='agent-console', feature=verifier.FEATURE, pull_request=verifier.PR,
                    observed_at=datetime.now(timezone.utc).isoformat(), status='merged',
                    candidate_revision='a' * 40, candidate_files_sha256='b' * 64,
                    console_revision='c' * 40, merge_revision='d' * 40,
                    default_branch_revision='e' * 40, repository='openexec', default_branch='main',
                    owner_decision_ref='console:decision', independent_review_ref='console:review',
                    canonical_gate_ref='console:gate', merge_ref=verifier.PR,
                    candidate_in_default_branch=True, exact_candidate_reviewed=True,
                    exact_candidate_approved=True, canonical_gates_passed=True, unresolved_findings=[],
                    default_branch_merger='coordinator recorded actor')

    def test_receipt_contract(self):
        # Deterministic coordinator fixture, not an assertion that this PR merged.
        import delivery
        receipt = self.receipt()
        real_run = subprocess.check_output
        with patch.object(verifier, 'ROOT', self.root), \
             patch.object(verifier.subprocess, 'check_output') as run, \
             patch.object(delivery, 'candidate_files', return_value='b' * 64):
            run.side_effect = lambda args, **kw: 'a' * 40 if args == ['git', 'rev-parse', 'HEAD'] else real_run(args, **kw)
            result = verifier.validate(self.root, self.value, receipt)
            self.assertTrue(result['goal_complete'])
            self.assertEqual(result['d2'], 'verified_external_merge')
            for field, value, error in (
                ('candidate_revision', 'f' * 40, 'stale Console'),
                ('candidate_files_sha256', 'f' * 64, 'stale Console'),
                ('default_branch_merger', '', 'merger'),
                ('candidate_in_default_branch', False, 'unproven'),
                ('exact_candidate_approved', False, 'unproven'),
                ('canonical_gates_passed', False, 'unproven'),
                ('unresolved_findings', ['remaining'], 'unresolved'),
                ('issuer', 'local-agent', 'coordinator'),
                ('pull_request', 'another PR', 'provenance'),
                ('merge_revision', '', 'provenance'),
                ('observed_at', (datetime.now(timezone.utc) - timedelta(days=2)).isoformat(), 'stale coordinator'),
            ):
                with self.subTest(field=field):
                    self.refuse(error, merge=dict(receipt, **{field: value}))

    def test_cli_persisted_records_and_d2_refusal(self):
        with tempfile.TemporaryDirectory(prefix='.identity-delivery-fixture-', dir=verifier.ROOT) as directory:
            path = Path(directory) / 'record.json'
            for patch_value, expected in [({}, 0), ({'findings': []}, 1), ({'content_sha256': '0' * 64}, 1)]:
                path.write_text(json.dumps(dict(self.record, **patch_value)))
                run = subprocess.run(['bash', 'scripts/verify-plan-identity-delivery-evidence.sh',
                                      '--record', str(path.relative_to(verifier.ROOT))], cwd=verifier.ROOT,
                                     capture_output=True, text=True)
                self.assertEqual(run.returncode, expected, run.stdout + run.stderr)
                self.assertEqual(json.loads(run.stdout)['passed'], expected == 0)
            run = subprocess.run(['bash', 'scripts/verify-plan-identity-delivery-evidence.sh', '--goal-complete'],
                                 cwd=verifier.ROOT, capture_output=True, text=True)
            self.assertEqual(run.returncode, 1, run.stdout)
            self.assertEqual(json.loads(run.stdout)['preparation'], 'ready')
            self.assertFalse(json.loads(run.stdout)['goal_complete'])

    def test_duplicate_json_key(self):
        with self.assertRaisesRegex(ValueError, 'duplicate JSON'):
            json.loads('{"d2":"pending","d2":"merged"}', object_pairs_hook=verifier.pairs)


if __name__ == '__main__':
    unittest.main()
