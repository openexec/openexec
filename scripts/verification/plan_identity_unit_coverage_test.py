"""Fail-closed coverage and mutation-result controls."""
import json
import subprocess
import unittest
from unittest.mock import patch

from reviewed_plan_identity import check_execution, check_threshold
from plan_identity_discovery import changed_functions
from plan_identity_mutations import require_assertion


class UnitCoverageControls(unittest.TestCase):
    def test_zero_tests_and_missing_required_test(self):
        for events in [[], [{'Action': 'pass', 'Package': 'pkg/manager'}]]:
            with self.assertRaisesRegex(ValueError, 'missing lifecycle'):
                check_execution(events, ['pkg/manager:TestIdentity'])
        with self.assertRaisesRegex(ValueError, 'empty or duplicate'):
            check_execution([], [])

    def test_skips_refused_even_with_pass(self):
        events = [dict(Action='pass', Package='pkg/manager', Test='TestIdentity'),
                  dict(Action='skip', Test='TestIdentity/subtest')]
        with self.assertRaisesRegex(ValueError, 'skipped/failed'):
            check_execution(events, ['pkg/manager:TestIdentity'])

    def test_threshold_is_strict_and_per_function(self):
        for covered in [0, 89, 90]:
            with self.assertRaisesRegex(ValueError, 'exceed 90%'):
                check_threshold([dict(covered=1000, statements=1000),
                                 dict(covered=covered, statements=100)])
        check_threshold([dict(covered=91, statements=100)])
        for functions in [[], [dict(covered=0, statements=0)]]:
            with self.assertRaisesRegex(ValueError, 'exceed 90%'):
                check_threshold(functions)

    def test_omitted_changed_identity_logic(self):
        # Simulate a newly added production helper, absent from the fixed inventory.
        def run(*args):
            if args[:2] == ('git', 'diff'):
                return 'internal/release/new_identity.go\n'
            if args[:2] == ('git', 'ls-files'):
                return ''
            return json.dumps([dict(name='newIdentity', source='func newIdentity() {}')])
        with patch('plan_identity_discovery.run', side_effect=run), patch(
                'plan_identity_discovery.subprocess.run',
                return_value=subprocess.CompletedProcess([], 128, '', 'not in baseline')):
            with self.assertRaisesRegex(ValueError, 'changed function omitted'):
                changed_functions(dict(baseline='base', functions=[]), 'inventory')

    def test_mutation_must_fail_expected_assertion(self):
        def result(events, code=1):
            return subprocess.CompletedProcess([], code, '\n'.join(json.dumps(e) for e in events), '')
        wanted = [dict(Action='output', Test='TestIdentity', Output='identity moved'),
                  dict(Action='fail', Test='TestIdentity')]
        require_assertion(result(wanted), 'TestIdentity', 'identity moved')
        for events, code in [(wanted, 0), (wanted, 2), ([], 1),
                             ([dict(Action='fail', Test='TestOther')], 1),
                             (wanted + [dict(Action='fail', Test='TestOther')], 1),
                             (wanted + [dict(Action='fail', Test='TestIdentity/unrelated')], 1),
                             (wanted + [dict(Action='skip', Test='TestIdentity/skipped')], 1),
                             ([dict(Action='output', Output='[build failed]')], 1),
                             ([dict(Action='output', Test='TestIdentity', Output='unrelated error'),
                               dict(Action='fail', Test='TestIdentity')], 1)]:
            with self.assertRaises(ValueError):
                require_assertion(result(events, code), 'TestIdentity', 'identity moved')


if __name__ == '__main__':
    unittest.main()
