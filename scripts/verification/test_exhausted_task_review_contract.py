import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import exhausted_task_review_contract as contract


class ReviewContractTests(unittest.TestCase):
    def setUp(self):
        self.scope = json.loads(contract.SCOPE.read_text())
        self.cases = json.loads(contract.CASES.read_text())

    def test_complete_definition_and_each_omitted_case(self):
        contract.validate(self.scope, self.cases)
        for case in self.cases['cases']:
            with self.subTest(case=case['id']):
                broken = copy.deepcopy(self.cases)
                broken['cases'].remove(case)
                with self.assertRaises(ValueError):
                    contract.validate(self.scope, broken)

    def test_policy_cannot_drop_missing_or_cli_or_add_exclusions(self):
        for key, value in [('missing_coverage', 'ignore'), ('exclusions', ['hard_function']),
                           ('packages', ['pkg/manager']), ('minimum_percent_exclusive', 80),
                           ('changed_function_roots', ['pkg/'])]:
            with self.subTest(key=key):
                broken = dict(self.scope, **{key: value})
                with self.assertRaises(ValueError):
                    contract.validate(broken, self.cases)

    def test_test_exit_zero_is_not_case_evidence(self):
        case = self.cases['cases'][0]
        package = contract.MODULE + case['package']
        events = [{'Action': 'pass', 'Package': package}]
        with self.assertRaises(ValueError):
            contract.check_events(events, [case])
        events += [{'Action': action, 'Package': package, 'Test': case['test']} for action in ['run', 'pass']]
        contract.check_events(events, [case])
        for action in ['fail', 'skip']:
            with self.assertRaises(ValueError):
                contract.check_events(events + [{'Action': action, 'Package': package, 'Test': case['test']}], [case])

    def test_missing_coverage_keeps_full_denominator(self):
        with tempfile.TemporaryDirectory() as directory:
            profile, filled = Path(directory)/'in', Path(directory)/'out'
            profile.write_text('mode: count\n')
            blocks = {'internal/cli/new.go': [(2, 1, 4, 2, 5)]}
            contract.fill_missing_blocks(blocks, profile, filled)
            fn = {'path': 'internal/cli/new.go', 'name': 'newCommand', 'change': 'added',
                  'start': {'Line': 1, 'Column': 1}, 'end': {'Line': 5, 'Column': 1}}
            result = contract.coverage.evaluate([fn], blocks, filled)
            self.assertEqual((result['covered'], result['statements'], result['passed']), (0, 5, False))
            self.assertEqual(profile.read_text(), 'mode: count\n')

    def test_each_finding_requires_disposition_and_reason(self):
        for finding in self.cases['findings']:
            broken = copy.deepcopy(self.cases)
            broken['findings'][int(finding['id'][1:])-1]['evidence'] = ''
            with self.assertRaises(ValueError):
                contract.validate(self.scope, broken)

    def test_new_cli_and_shared_functions_cannot_escape_scope(self):
        rows = json.loads((contract.ROOT / self.scope['inventory']).read_text())['functions']
        added = [{'path': 'internal/cli/task_correct.go', 'name': 'newTaskCorrectCmd'},
                 {'path': 'internal/cli/root.go', 'name': 'registerCommands'}]
        declarations = {}
        for row in rows:
            declarations.setdefault(row['path'], []).append({'name': row['symbol']})
        for fn in added:
            declarations.setdefault(fn['path'], []).append({'name': fn['name']})
        declarations.setdefault('pkg/manager/task_boundary.go', []).append({'name': 'Error'})
        with patch.object(contract.coverage, 'scope', return_value=added), patch.object(
                contract.coverage, 'run', side_effect=lambda helper, path: json.dumps(declarations[path])):
            result = contract.resolve_scope(self.scope, Path('/unused/helper'), Path('/unused'))
        keys = {(fn['path'], fn['name']) for fn in result}
        for fn in added:
            self.assertIn((fn['path'], fn['name']), keys)
        self.assertIn(('pkg/manager/task_boundary.go', 'Error'), keys)


if __name__ == '__main__':
    unittest.main()
