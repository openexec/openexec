"""Negative controls for the retained-evidence coverage gate itself."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import retention_unit_coverage as gate


class CoverageGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.profile = Path(self.temp.name) / "coverage.out"
        self.function = dict(path="pkg/sample.go", name="Changed", change="modified",
                             start=dict(Line=1, Column=10), end=dict(Line=8, Column=2))
        self.blocks = {"pkg/sample.go": [(1, 10, 2, 2, 9), (3, 2, 4, 2, 1)]}

    def evaluate(self, text, functions=None, blocks=None):
        self.profile.write_text(text)
        with patch.object(gate, "run", return_value="baseline\n"):
            return gate.evaluate([self.function] if functions is None else functions,
                                 self.blocks if blocks is None else blocks, self.profile)

    def test_strict_threshold_and_statement_weighting(self):
        profile = "mode: count\n" + gate.MODULE + "pkg/sample.go:1.10,2.2 9 1\n" + gate.MODULE + "pkg/sample.go:3.2,4.2 1 0\n"
        self.assertFalse(self.evaluate(profile)["passed"])
        self.assertEqual(self.evaluate(profile)["percent"], 90)
        self.assertTrue(self.evaluate(profile.replace("1 0\n", "1 1\n"))["passed"])

    def test_missing_or_wrong_instrumentation_refused(self):
        for profile in ("", "mode: set\n", "mode: count\n", "mode: count\nmalformed\n",
                        "mode: count\n" + gate.MODULE + "pkg/sample.go:1.10,2.2 8 1\n"):
            with self.subTest(profile=profile), self.assertRaises(ValueError):
                self.evaluate(profile)

    def test_partial_body_and_duplicates_refused(self):
        line = gate.MODULE + "pkg/sample.go:1.10,2.2 9 1\n"
        for profile in ("mode: count\n" + line, "mode: count\n" + line + line.replace("9 1", "8 1")):
            with self.assertRaises(ValueError):
                self.evaluate(profile)

    def test_empty_scope_or_uninstrumented_function_refused(self):
        with self.assertRaises(ValueError):
            self.evaluate("mode: count\n", functions=[])
        with self.assertRaises(ValueError):
            self.evaluate("mode: count\n", blocks={"pkg/sample.go": []})

    def test_omitted_changed_functions_and_stale_baseline_refused(self):
        identity = "pkg/sample.go:Changed"
        manifest = dict(baseline=gate.BASELINE, functions=[identity])
        gate.check_scope([self.function], manifest)
        for bad in (dict(baseline=gate.BASELINE, functions=[]),
                    dict(baseline="HEAD", functions=[identity]),
                    dict(baseline=gate.BASELINE, functions=[identity, identity])):
            with self.assertRaises(ValueError):
                gate.check_scope([self.function], bad)
        with self.assertRaises(ValueError):
            gate.check_scope([], dict(baseline=gate.BASELINE, functions=[]))
        with self.assertRaises(ValueError):
            gate.check_scope([self.function, self.function], manifest)

    def test_combined_package_profiles_count_each_statement_once(self):
        profile = "mode: count\n" + gate.MODULE + "pkg/sample.go:1.10,2.2 9 1\n" + gate.MODULE + "pkg/sample.go:3.2,4.2 1 0\n"
        combined = profile + profile.split("\n", 1)[1].replace("1 0\n", "1 1\n")
        result = self.evaluate(combined)
        self.assertEqual((result["covered"], result["statements"]), (10, 10))

    def test_skipped_failed_and_absent_tests_refused(self):
        required = ["pkg/sample:TestRequired"]
        passes = [dict(Action="pass", Package=gate.MODULE + p) for p in gate.PACKAGES]
        passes.append(dict(Action="pass", Package=gate.MODULE + "pkg/sample", Test="TestRequired"))
        log = lambda events: "\n".join(json.dumps(e) for e in events)
        gate.check_tests(log(passes), required)
        for action in ("skip", "fail"):
            # A parent pass must not hide skipped or failed subtests.
            events = passes + [dict(Action=action, Package=gate.MODULE + "pkg/sample", Test="TestRequired/branch")]
            with self.assertRaises(ValueError):
                gate.check_tests(log(events), required)
        with self.assertRaises(ValueError):
            gate.check_tests(log(passes[:-1]), required)
        with self.assertRaises(ValueError):
            gate.check_tests(log(passes[1:]), required)


class ConsolidatedScopeTests(unittest.TestCase):
    def test_companion_scope_cannot_hide_unowned_or_missing_functions(self):
        manifest = json.loads(gate.MANIFEST.read_text())
        companion = json.loads((gate.ROOT / 'docs/verification/recapture-coverage-scope.json').read_text())
        identities = set(manifest['functions']) | set(companion['functions'])
        functions = [dict(path=s.split(':')[0], name=s.split(':')[1]) for s in identities]
        self.assertEqual(len(gate.slice_scope(functions, manifest, companion)), len(manifest['functions']))
        for bad in (functions[:-1], functions + [dict(path='pkg/new.go', name='unowned')]):
            with self.assertRaises(ValueError):
                gate.slice_scope(bad, manifest, companion)


if __name__ == "__main__":
    unittest.main()
