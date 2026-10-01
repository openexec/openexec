"""Persisted fixture controls for the preparation contract."""
import copy
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

import exhausted_task_delivery as delivery


class DeliveryTests(unittest.TestCase):
    def test_persisted_package_and_refusals(self):
        record = delivery.read(delivery.ROOT / delivery.RECORD)
        delivery.validate(delivery.ROOT, record)
        fixtures = delivery.read(delivery.ROOT / 'scripts/verification/fixtures/exhausted-task-delivery/refusals.json')
        for fixture in fixtures:
            with self.subTest(fixture['name']):
                changed = copy.deepcopy(record)
                parent = changed
                for key in fixture['path'][:-1]:
                    parent = parent[key]
                key = fixture['path'][-1]
                if fixture.get('delete'):
                    del parent[key]
                else:
                    parent[key] = fixture['value']
                # Write/re-read every fixture through the real CLI in an excluded
                # evidence directory, so failure is semantic, not source drift.
                with tempfile.NamedTemporaryFile(mode='w', suffix='.json', dir=delivery.ROOT / delivery.RESULTS) as output:
                    json.dump(changed, output)
                    output.flush()
                    result = subprocess.run(['scripts/verify-exhausted-task-delivery-evidence.sh', '--phase',
                                             'preparation', '--record', str(Path(output.name).relative_to(delivery.ROOT))],
                                            cwd=delivery.ROOT, text=True, capture_output=True)
                    self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                    self.assertIn(fixture['error'], result.stderr)

    def test_preparation_does_not_require_merge_but_delivery_does(self):
        for phase, expected, marker in (
            ('preparation', 0, 'D2 pending'),
            ('delivery', 1, 'actual coordinator merge evidence required'),
        ):
            result = subprocess.run(['bash', 'scripts/verify-exhausted-task-delivery-evidence.sh',
                                     '--phase', phase], cwd=delivery.ROOT, text=True, capture_output=True)
            self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
            self.assertIn(marker, result.stdout + result.stderr)

    def test_accepted_task_command_and_separate_merge_command(self):
        record = delivery.read(delivery.ROOT / delivery.RECORD)
        plan = delivery.read(delivery.ROOT / record['handoff']['accepted_plan']['path'])
        command = plan['stories'][-1]['tasks'][-1]['verification_script']
        result = subprocess.run(['bash', '-c', command], cwd=delivery.ROOT,
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('D2 pending', result.stdout)
        result = subprocess.run(['python3', 'scripts/verify-exhausted-task-records.py',
                                 '--record', 'delivery', '--require-provenance',
                                 '--require-coordinator-merge-evidence'], cwd=delivery.ROOT,
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn('actual coordinator merge evidence required', result.stderr)

    def test_native_boundary_refusals(self):
        record = delivery.read(delivery.ROOT / delivery.RECORD)
        original = delivery.read(delivery.ROOT / record['handoff']['accepted_plan']['path'])
        for change, marker in (
            ('dependency', 'external native dependency'),
            ('gate', 'native verification waits for delivery'),
            ('hitl', 'duplicate native HITL boundary'),
            ('effect', 'native verification waits for delivery'),
            ('task', 'unexpected native delivery task'),
        ):
            with self.subTest(change):
                plan = copy.deepcopy(original)
                task = plan['stories'][-1]['tasks'][-1]
                if change == 'dependency':
                    task['depends_on'].append('external-merge')
                elif change == 'gate':
                    task['verification_script'] += ' --require-coordinator-merge-evidence'
                elif change == 'hitl':
                    task['mode'] = 'hitl'
                elif change == 'effect':
                    task['verification_script'] += '\ngh pr merge 80'
                else:
                    task['id'] = 'T-PUBLISH'
                changed = copy.deepcopy(record)
                with tempfile.NamedTemporaryFile(mode='w', suffix='.json',
                                                 dir=delivery.ROOT / delivery.RESULTS) as output:
                    json.dump(plan, output)
                    output.flush()
                    path = Path(output.name)
                    changed['handoff']['accepted_plan'] = {
                        'path': str(path.relative_to(delivery.ROOT)),
                        'sha256': delivery.sha(path.read_bytes())}
                    with self.assertRaisesRegex(ValueError, marker):
                        delivery.validate_handoff(delivery.ROOT, changed)

    def test_merge_export_uses_actual_git_ancestry_and_retained_pr(self):
        # Synthetic exports stay in the temporary fixture, never in real evidence.
        from datetime import datetime, timezone
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            def git(*args):
                return delivery.git(root, *args)
            git('init', '-q')
            git('remote', 'add', 'origin', 'https://github.com/openexec/openexec.git')
            source = root / 'source'
            source.write_text('base')
            git('add', '.')
            git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                'commit', '-qm', 'base')
            base = git('rev-parse', 'HEAD')
            source.write_text('candidate')
            git('add', '.')
            git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                'commit', '-qm', 'candidate')
            head = git('rev-parse', 'HEAD')
            git('update-ref', 'refs/remotes/origin/main', head)
            git('symbolic-ref', 'refs/remotes/origin/HEAD', 'refs/remotes/origin/main')
            record = {'source_sha256': delivery.content(root)[1],
                      'handoff': {'pull_request': 'https://github.com/openexec/openexec/pull/80'}}
            receipt = dict(status='merged', issuer='agent-console', feature=delivery.BRANCH.split('/')[1],
                           candidate_revision=head, candidate_files_sha256=record['source_sha256'],
                           console_revision='a'*40, merge_revision=head, default_branch_revision=head,
                           repository='openexec', default_branch='main', pull_request=record['handoff']['pull_request'],
                           owner_decision_ref='fixture-owner', independent_review_ref='fixture-review',
                           canonical_gate_ref='fixture-gate', merge_ref='fixture-merge',
                           candidate_in_default_branch=True, exact_candidate_reviewed=True,
                           exact_candidate_approved=True, canonical_gates_passed=True,
                           unresolved_findings=[], integration='ancestry',
                           observed_at=datetime.now(timezone.utc).isoformat())
            delivery.validate_merge(root, record, receipt)
            for field in receipt:
                broken = dict(receipt)
                del broken[field]
                with self.subTest(missing=field), self.assertRaises((ValueError, KeyError)):
                    delivery.validate_merge(root, record, broken)
            for field, value, marker in (
                ('pull_request', 'https://github.com/openexec/openexec/pull/81', 'wrong merge pull request'),
                ('repository', 'agent-console', 'wrong merge repository'),
                ('default_branch', 'other', 'wrong default branch'),
                ('merge_revision', base, 'candidate absent from merge'),
            ):
                with self.subTest(field=field), self.assertRaisesRegex(ValueError, marker):
                    delivery.validate_merge(root, record, dict(receipt, **{field: value}))
            source.write_text('dirty')
            with self.assertRaisesRegex(ValueError, 'uncommitted candidate'):
                delivery.validate_merge(root, record, receipt)

    def test_notes_cannot_be_coordinator_evidence(self):
        record = delivery.read(delivery.ROOT / delivery.RECORD)
        with self.assertRaisesRegex(ValueError, 'Console merge evidence'):
            delivery.validate_merge(delivery.ROOT, record, {'status': 'pending',
                'console_revision': '0f5ce914', 'checkout_note': 'merged'})

    def test_manifest_includes_uncommitted_bytes_modes_and_deletions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(['git', 'init', '-q', directory], check=True)
            source = root / 'implementation.go'
            source.write_text('first')
            subprocess.run(['git', 'add', '.'], cwd=root, check=True)
            first = delivery.content(root)
            source.write_text('unstaged')
            self.assertNotEqual(first, delivery.content(root))
            source.write_text('first')
            source.chmod(0o755)
            self.assertNotEqual(first, delivery.content(root))
            (root / 'new_test.go').write_text('untracked')
            self.assertEqual(len(delivery.content(root)[0]), 2)
            source.unlink()
            self.assertEqual([r['path'] for r in delivery.content(root)[0]], ['new_test.go'])

    def test_nonlocal_reference_refused(self):
        with self.assertRaisesRegex(ValueError, 'nonlocal'):
            delivery.local(delivery.ROOT, '../outside')


if __name__ == '__main__':
    unittest.main()
