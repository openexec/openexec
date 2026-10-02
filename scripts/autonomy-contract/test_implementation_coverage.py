import os
import unittest

from implementation_coverage import enforce, inventory, measure, PREFIX, validate_unit_run


class ImplementationCoverageTests(unittest.TestCase):
    def test_unit_run_requires_package_and_named_test_verdicts(self):
        events = [{'Package': PREFIX + 'pkg/runtime', 'Action': a, 'Test': 'unit'}
                  for a in ('run', 'pass')]
        verdict = {'Package': PREFIX + 'pkg/runtime', 'Action': 'pass'}
        validate_unit_run(events + [verdict], 'pkg/runtime', ['unit'])
        for invalid in ([], events, [verdict], events + [verdict, verdict],
                        events + [dict(verdict, Action='fail')],
                        events + [dict(verdict, Package='wrong')],
                        events + [dict(events[0], Action='skip'), verdict],
                        [None]):
            with self.subTest(events=invalid), self.assertRaises(ValueError):
                validate_unit_run(invalid, 'pkg/runtime', ['unit'])
        with self.assertRaises(ValueError):
            validate_unit_run([verdict], 'pkg/runtime', [])
        # Baseline-only empty packages supply zero-count instrumented blocks.
        validate_unit_run([verdict], 'pkg/runtime', [], baseline_units=True)

    def test_strict_threshold(self):
        for covered in (0, 89, 90):
            with self.subTest(covered=covered), self.assertRaises(ValueError):
                enforce([{'name': 'unit', 'covered': covered, 'total': 100}])
        enforce([{'name': 'unit', 'covered': 91, 'total': 100}])
        with self.assertRaises(ValueError):
            enforce([])

    def test_high_aggregate_cannot_hide_low_function(self):
        with self.assertRaises(ValueError):
            enforce([{'name': 'large', 'covered': 1000, 'total': 1000},
                     {'name': 'small', 'covered': 9, 'total': 10}])

    def test_complete_body_includes_nested_closure_not_other_functions(self):
        function = {'file': 'pkg/example.go', 'name': 'changed', 'start': 10, 'end': 30}
        blocks = [f'{PREFIX}pkg/example.go:10.1,12.2 3 1',
                  f'{PREFIX}pkg/example.go:15.1,19.2 2 0',
                  f'{PREFIX}pkg/example.go:20.1,30.2 5 1',
                  f'{PREFIX}pkg/example.go:31.1,50.2 1000 1',
                  f'{PREFIX}unrelated.go:10.1,30.2 1000 1']
        row, = measure([function], blocks)
        self.assertEqual((row['covered'], row['total']), (8, 10))
        self.assertEqual(row['uncovered_lines'], [[15, 19]])

    def test_missing_or_malformed_profile_fails(self):
        function = {'file': 'pkg/example.go', 'name': 'changed', 'start': 10, 'end': 30}
        for blocks in ([], ['invalid'], [f'{PREFIX}other.go:10.1,30.2 1000 1']):
            with self.subTest(blocks=blocks), self.assertRaises(ValueError):
                measure([function], blocks)

    def test_ast_scope_includes_full_method_and_closure(self):
        env = os.environ.copy()
        env['GOCACHE'] = '/tmp/openexec-exit1-go-cache'
        source = '''package fixture
type T struct{}
func (t *T) Changed(x int) int {
 f := func() int {
  return x + 1
 }
 return f()
}
func Untouched() {}
'''
        functions = inventory({'pkg/example.go': source}, env)
        method = next(f for f in functions if f['name'] == 'T.Changed')
        self.assertEqual((method['start'], method['end']), (3, 8))
        self.assertIn('func (t *T) Changed(x int)', method['body'])
        self.assertIn('return x + 1', method['body'])
        self.assertNotIn('Untouched', method['body'])
        with self.assertRaises(RuntimeError):
            inventory({'broken.go': 'not go'}, env)


if __name__ == '__main__':
    unittest.main()
