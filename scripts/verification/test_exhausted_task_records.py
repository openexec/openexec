"""Tool controls use synthetic evidence; they do not certify the production repairs."""
import copy
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import exhausted_task_records as records


class RecordsTests(unittest.TestCase):
    def setUp(self):
        self.record = records.read(records.ROOT / records.RECORD)
        self.scope, self.cases = records.validate_records(records.ROOT, self.record)

    def test_every_checklist_field_case_and_falsifier_required(self):
        for finding, checklist in self.record['checklists'].items():
            for field in checklist:
                with self.subTest(finding=finding, field=field):
                    broken = copy.deepcopy(self.record)
                    del broken['checklists'][finding][field]
                    with self.assertRaises(ValueError):
                        records.validate_records(records.ROOT, broken)
            for field in ('root_cause', 'fix'):
                for ref in checklist[field]:
                    broken = copy.deepcopy(self.record)
                    broken['checklists'][finding][field].remove(ref)
                    with self.assertRaises(ValueError):
                        records.validate_records(records.ROOT, broken)
            for field in ('prove', 'falsify'):
                for case in checklist[field]:
                    broken = copy.deepcopy(self.record)
                    broken['checklists'][finding][field].remove(case)
                    with self.assertRaisesRegex(ValueError, 'cases or falsifiers'):
                        records.validate_records(records.ROOT, broken)
            broken = copy.deepcopy(self.record)
            del broken['checklists'][finding]
            with self.assertRaisesRegex(ValueError, 'four finding'):
                records.validate_records(records.ROOT, broken)

    def test_sources_dependencies_and_claims_refused(self):
        for mutate in (
            lambda r: r['dependencies'].clear(),
            lambda r: r['dependencies'][0].update(task='T-US-010-003'),
            lambda r: r['dependencies'][0]['consumes'].append('../other-repo/file'),
            lambda r: r['delivery'].update(D2='merged'),
            lambda r: r['delivery'].update(native_preparation='passed'),
            lambda r: r['delivery'].update(goal_complete=True),
            lambda r: r['checklists']['F1']['root_cause'][0].update(path='../other-repo/file'),
            lambda r: r['checklists']['F1']['fix'][0].update(symbol='absentDeclaration987'),
        ):
            broken = copy.deepcopy(self.record)
            mutate(broken)
            with self.assertRaises((ValueError, OSError)):
                records.validate_records(records.ROOT, broken)

    def test_all_upstream_cases_assertions_commands_and_scope_required(self):
        matrix = records.read(records.ROOT / records.MATRIX)
        original_read = records.read
        def validate(matrix=matrix, scope=self.scope):
            def read(path):
                if path == records.ROOT / records.MATRIX:
                    return matrix
                if path == records.ROOT / records.SCOPE:
                    return scope
                return original_read(path)
            with patch.object(records, 'read', side_effect=read):
                records.validate_records(records.ROOT, self.record)
        for case in matrix['cases']:
            broken = copy.deepcopy(matrix)
            broken['cases'].remove(case)
            with self.assertRaises(ValueError):
                validate(matrix=broken)
        for field, value in [('test', 'Test.*'), ('package', '../outside'), ('assertions', ['']),
                             ('negative_control', ''), ('finding', 'unknown'), ('implementation', 'verified')]:
            broken = copy.deepcopy(matrix)
            broken['cases'][0][field] = value
            with self.assertRaises(ValueError):
                validate(matrix=broken)
        broken = copy.deepcopy(matrix)
        broken['execution']['command_template'] = ['true']
        with self.assertRaisesRegex(ValueError, 'command template'):
            validate(matrix=broken)
        for field, value in [('exclusions', ['hard_function']), ('minimum_percent_exclusive', 80),
                             ('inventory', '../outside'), ('production_files', []), ('baseline', 'HEAD'),
                             ('changed_function_roots', ['pkg/']), ('missing_coverage', 'ignore')]:
            broken = copy.deepcopy(self.scope)
            broken[field] = value
            with self.assertRaises(ValueError):
                validate(scope=broken)

    def test_referenced_frozen_inventory_cannot_be_narrowed(self):
        path = records.ROOT / self.scope['inventory']
        inventory = records.read(path)
        original_read = records.read
        for rows in (inventory['functions'][:-1], inventory['functions'] + inventory['functions'][:1]):
            with patch.object(records, 'read', side_effect=lambda p: dict(inventory, functions=rows) if p == path else original_read(p)):
                with self.assertRaisesRegex(ValueError, 'inventory narrowed or duplicated'):
                    records.validate_records(records.ROOT, self.record)

    def test_commands_anchor_each_subtest_and_enable_race(self):
        for case in self.cases:
            command = records.case_command(case)
            self.assertEqual(command[-1], '/'.join('^'+s+'$' for s in case['test'].split('/')))
            self.assertIn('./' + case['package'], command)
            self.assertEqual('-race' in command, case['id'] == 'concurrency')

    def test_path_escape_absolute_symlink_and_duplicate_json(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'inside').write_text('safe')
            (root / 'escape').symlink_to('/etc/passwd')
            for path in ('../outside', '/etc/passwd', 'escape', 'a/../inside', './inside', 'missing', 'a\\b'):
                with self.subTest(path=path), self.assertRaises(ValueError):
                    records.local(root, path)
            (root / 'bad.json').write_text('{"D2":"outstanding","D2":"merged"}')
            with self.assertRaisesRegex(ValueError, 'duplicate'):
                records.read(root / 'bad.json')

    def test_actual_source_denominator_and_empty_profile(self):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            functions, blocks, statements = records.denominator(records.ROOT, self.scope, temp)
            self.assertGreater(len(functions), 0)
            self.assertGreater(statements, 0)
            measured = records.review.coverage.evaluate(functions, blocks, temp / 'filled.out')
            self.assertEqual(measured['statements'], statements)
            self.assertEqual(measured['covered'], 0)
            self.assertFalse(measured['passed'])

    def test_planning_and_native_queue_boundary_with_removal_controls(self):
        # Compile mutated source with overlays; never edit the retained candidate.
        mutants = [
            ('internal/planner/prompt.go', '` + DeliveryBoundaryRule + `', '',
             './internal/planner', '^TestDeliveryBoundarySharedAcrossPlanningPaths$',
             'planning path omits post-queue delivery boundary'),
            ('pkg/manager/task_queue.go', 'return retainedTaskBoundary(current)',
             'return fmt.Errorf("waiting for coordinator merge")',
             './pkg/manager', '^TestTaskQueuePreparationDoesNotWaitForOrClaimMerge$',
             'preparation waits for post-queue merge'),
            ('pkg/manager/planner_replay.go', 'Contract: s.Contract,', 'Contract: "D2 merged",',
             './pkg/manager', '^TestTaskQueuePreparationDoesNotWaitForOrClaimMerge$',
             'queue completion lost or satisfied outstanding D2'),
        ]
        original = {path: (records.ROOT / path).read_bytes() for path, *_ in mutants}
        positive = ['go', 'test', './internal/planner', './pkg/manager', '-count=1', '-timeout=60s',
                    '-run', '^Test(DeliveryBoundarySharedAcrossPlanningPaths|DeliveryContractSurvivesPlanningReviewAndRefinement|TaskQueuePreparationDoesNotWaitForOrClaimMerge)$']
        result = subprocess.run(positive, cwd=records.ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            for path, before, after, package, selector, assertion in mutants:
                with self.subTest(path=path):
                    source = original[path].decode()
                    self.assertIn(before, source)
                    overlay_source = temp / 'mutant.go'
                    overlay_source.write_text(source.replace(before, after))
                    overlay = temp / 'overlay.json'
                    overlay.write_text(json.dumps({'Replace': {str(records.ROOT / path): str(overlay_source)}}))
                    command = ['go', 'test', '-overlay', str(overlay), package, '-count=1', '-timeout=60s', '-run', selector]
                    result = subprocess.run(command, cwd=records.ROOT, capture_output=True, text=True)
                    self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                    self.assertIn('--- FAIL: ', result.stdout)
                    self.assertIn(assertion, result.stdout)
                    self.assertNotIn('[build failed]', result.stdout + result.stderr)
        for path, content in original.items():
            self.assertEqual((records.ROOT / path).read_bytes(), content, 'overlay changed original source')
        result = subprocess.run(positive, cwd=records.ROOT, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_persisted_cli_record_and_refusals(self):
        with tempfile.TemporaryDirectory(prefix='.records-test-', dir=records.ROOT) as directory:
            path = Path(directory) / 'record.json'
            command = ['python3', 'scripts/verify-exhausted-task-records.py', '--record', str(path.relative_to(records.ROOT))]
            def run(record, *extra):
                path.write_text(json.dumps(record))
                return subprocess.run(command + list(extra), cwd=records.ROOT, capture_output=True, text=True)
            result = run(self.record)
            self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
            self.assertEqual(json.loads(result.stdout)['D2'], 'outstanding')
            self.assertEqual(json.loads(result.stdout)['native_preparation'], 'unverified')
            broken = copy.deepcopy(self.record)
            del broken['checklists']['F1']['fix']
            result = run(broken)
            self.assertEqual(result.returncode, 1)
            self.assertIn('incomplete/unknown', result.stdout)
            for phase in ('preparation', 'delivery'):
                result = run(self.record, '--phase', phase)
                self.assertEqual(result.returncode, 1)
                self.assertIn('native preparation evidence required', result.stdout)


class NativeBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.evidence = self.root / records.EVIDENCE_DIR
        self.evidence.mkdir(parents=True)
        self.git('init', '-q', '-b', 'main')
        self.git('remote', 'add', 'origin', 'https://github.com/openexec/openexec.git')
        (self.root / 'native.go').write_text('package fixture\nfunc Native() { println("ok") }\n')
        self.git('add', 'native.go')
        self.commit('base')
        self.base = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', '-b', 'outcome/' + records.FEATURE)
        (self.root / 'native.go').write_text('package fixture\nfunc Native() { println("corrected") }\n')
        self.git('add', 'native.go')
        self.commit('candidate')
        self.head = self.git('rev-parse', 'HEAD')
        self.cases = records.read(records.ROOT / records.MATRIX)['cases']
        self.fn = [{'path': 'native.go', 'name': 'Native', 'change': 'fixture',
                    'start': {'Line': 1, 'Column': 1}, 'end': {'Line': 4, 'Column': 1}}]
        self.blocks = {'native.go': [(2, 1, 2, 20, 9), (3, 1, 3, 20, 1)]}
        event_rows = []
        for case in self.cases:
            event_rows += [dict(Action=a, Package=records.review.MODULE + case['package'], Test=case['test']) for a in ('run', 'pass')]
        event_rows += [dict(Action='pass', Package=records.review.MODULE + p) for p in {c['package'] for c in self.cases}]
        digest = records.source_digest(self.root)
        self.native = dict(schema_version=1, candidate_revision=self.head, source_sha256=digest,
                           events=self.artifact('events.jsonl', '\n'.join(json.dumps(e) for e in event_rows)),
                           profile=self.artifact('coverage.out', 'mode: count\n'+records.review.MODULE+'native.go:2.1,2.20 9 1\n'+records.review.MODULE+'native.go:3.1,3.20 1 1\n'),
                           falsifiers={}, checks={})
        for case in self.cases:
            proof = [dict(Action=a, Package=records.review.MODULE+case['package'], Test=case['test'],
                          Output=case['assertions'][0] if a == 'output' else '') for a in ('run', 'output', 'fail')]
            self.native['falsifiers'][case['id']] = dict(mutation=case['negative_control'], assertion=case['assertions'][0],
                events=self.artifact(case['id']+'.jsonl', '\n'.join(json.dumps(e) for e in proof)), source_restored_sha256=digest)
        for name in ('lint', 'test'):
            self.native['checks'][name] = dict(command=['run_declared_check', name], exit_code=0,
                evidence=self.artifact(name+'.json', json.dumps({'content':[{'text':name+' exited 0'}]})))
        self.native['scratch'] = dict(command=['bin/openexec', 'task', 'correct', 'A', '--decision-ref', 'fixture'], exit_code=0,
            evidence=self.artifact('scratch.json', json.dumps(dict(decision_ref='fixture', persisted_decision_ref='fixture',
                candidate_branch='outcome/'+records.FEATURE, candidate_sha256=digest,
                A=dict(status='done', attempt_count=3, max_attempts=3), Settings=dict(status='done'),
                original_receipt_sha256='1'*64, reopened_receipt_sha256='1'*64))))

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, text=True, stderr=subprocess.PIPE).strip()

    def commit(self, message):
        self.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', message)

    def artifact(self, name, data):
        path = self.evidence / name
        path.write_text(data)
        return dict(path=str(path.relative_to(self.root)), sha256=hashlib.sha256(data.encode()).hexdigest())

    def validate(self, report=None):
        return records.validate_native(self.root, self.native if report is None else report, self.cases, self.fn, self.blocks, self.evidence)

    def test_native_preparation_passes_with_d2_outstanding_and_round_trip(self):
        # Synthetic proof tests the verifier; this is never checked in as D1 evidence.
        path = self.evidence / 'native.json'
        path.write_text(json.dumps(self.native))
        result = self.validate(records.read(path))
        self.assertTrue(result['passed'])
        status = records.completion(self.root, result['passed'])
        self.assertEqual(status['D2'], 'outstanding')
        self.assertFalse(status['goal_complete'])
        with self.assertRaisesRegex(ValueError, 'coordinator merge evidence'):
            records.completion(self.root, True, demand_merge=True)
        with self.assertRaisesRegex(ValueError, 'native preparation'):
            records.completion(self.root, False)

    def test_incomplete_stale_failed_skipped_native_and_mutant_evidence(self):
        for field in self.native:
            broken = copy.deepcopy(self.native)
            del broken[field]
            with self.assertRaises(ValueError):
                self.validate(broken)
        for mutate in (
            lambda r: r.update(source_sha256='0'*64),
            lambda r: r.update(candidate_revision=self.base),
            lambda r: r['falsifiers'].pop(self.cases[0]['id']),
            lambda r: r['falsifiers'][self.cases[0]['id']].update(assertion='unrelated compile failure'),
            lambda r: r['falsifiers'][self.cases[0]['id']].update(source_restored_sha256='0'*64),
            lambda r: r['checks']['test'].update(exit_code=1),
            lambda r: r['events'].update(sha256='0'*64),
        ):
            broken = copy.deepcopy(self.native)
            mutate(broken)
            with self.assertRaises(ValueError):
                self.validate(broken)
        for action in ('skip', 'fail'):
            broken = copy.deepcopy(self.native)
            data = records.artifact(self.root, broken['events']) + '\n' + json.dumps({'Action': action})
            broken['events'] = self.artifact('bad-events.jsonl', data)
            with self.assertRaisesRegex(ValueError, 'failed/skipped'):
                self.validate(broken)
        broken = copy.deepcopy(self.native)
        broken['events'] = self.artifact('empty-events.jsonl', json.dumps({'Action':'pass','Package':'pkg/manager'}))
        with self.assertRaisesRegex(ValueError, 'missing native'):
            self.validate(broken)
        proof = broken['falsifiers'][self.cases[0]['id']]
        proof['events'] = self.artifact('compile.jsonl', json.dumps({'Action':'fail','Package':records.review.MODULE+'pkg/manager','Output':'build failed'}))
        broken['events'] = self.native['events']
        with self.assertRaisesRegex(ValueError, 'intended executed assertion'):
            self.validate(broken)

    def test_missing_profile_blocks_keep_denominator_and_exact_90_refuses(self):
        for text in ('mode: count\n', 'mode: count\n'+records.review.MODULE+'native.go:2.1,2.20 9 1\n',
                     'mode: set\n', 'mode: count\nbogus\n', 'mode: count\n../outside.go:1.1,2.2 1 1\n'):
            broken = copy.deepcopy(self.native)
            broken['profile'] = self.artifact('bad-profile.out', text)
            with self.assertRaises(ValueError):
                self.validate(broken)

    def merge_receipt(self):
        self.git('update-ref', 'refs/remotes/origin/main', self.head)
        self.git('symbolic-ref', 'refs/remotes/origin/HEAD', 'refs/remotes/origin/main')
        return dict(status='merged', issuer='agent-console', feature=records.FEATURE, observed_at=datetime.now(timezone.utc).isoformat(),
            candidate_revision=self.head, candidate_files_sha256=records.source_digest(self.root),
            console_revision='a'*40, merge_revision=self.head, default_branch_revision=self.head,
            repository='openexec', default_branch='main', pull_request=records.PR,
            owner_decision_ref='fixture-owner-decision', independent_review_ref='fixture-review',
            canonical_gate_ref='fixture-gate', merge_ref='fixture-merge', candidate_in_default_branch=True,
            exact_candidate_reviewed=True, exact_candidate_approved=True, canonical_gates_passed=True,
            unresolved_findings=[], integration='ancestry')

    def test_only_bound_coordinator_merge_can_satisfy_d2(self):
        receipt = self.merge_receipt()
        self.assertTrue(records.completion(self.root, True, receipt)['goal_complete'])
        for field in receipt:
            broken = copy.deepcopy(receipt)
            del broken[field]
            with self.subTest(missing=field), self.assertRaises((ValueError, KeyError)):
                records.completion(self.root, True, broken)
        for field, value in [('status','published'), ('repository','agent-console'), ('issuer','local-task'),
                             ('owner_decision_ref',''), ('candidate_files_sha256','0'*64),
                             ('canonical_gates_passed',False), ('merge_revision','b'*40), ('observed_at','2020-01-01T00:00:00Z'),
                             ('unresolved_findings',['F1']), ('default_branch','other')]:
            with self.subTest(field=field), self.assertRaises((ValueError, subprocess.SubprocessError)):
                records.completion(self.root, True, dict(receipt, **{field:value}))
        self.git('update-ref', 'refs/remotes/origin/main', self.base)
        with self.assertRaisesRegex(ValueError, 'stale default-branch'):
            records.completion(self.root, True, receipt)
        receipt.update(default_branch_revision=self.base)
        with self.assertRaisesRegex(ValueError, 'absent from default branch'):
            records.completion(self.root, True, receipt)

    def test_squash_tree_equivalence_and_different_tree_refusal(self):
        receipt = self.merge_receipt()
        tree = self.git('rev-parse', 'HEAD^{tree}')
        squash = self.git('-c','user.name=Fixture','-c','user.email=fixture@example.invalid',
                          'commit-tree', tree, '-p', self.base, '-m', 'squash fixture')
        self.git('update-ref', 'refs/remotes/origin/main', squash)
        receipt.update(merge_revision=squash, default_branch_revision=squash, integration='tree')
        self.assertEqual(records.completion(self.root, True, receipt)['D2'], 'verified_external_merge')
        receipt.update(merge_revision=self.base, default_branch_revision=squash)
        with self.assertRaisesRegex(ValueError, 'tree differs'):
            records.completion(self.root, True, receipt)


if __name__ == '__main__':
    unittest.main()
