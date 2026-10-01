import unittest
from exhausted_task_reconciliation import validate, MODES


class ReconciliationVerifierTests(unittest.TestCase):
    required = {"pkg/manager": ["TestCorrectionNativeQueueSuccess"]}

    def events(self):
        pkg = "github.com/openexec/openexec/pkg/manager"
        test = "TestCorrectionNativeQueueSuccess"
        return [{"Package": pkg, "Test": test, "Action": "run"},
                {"Package": pkg, "Test": test, "Action": "output", "Output": 'RELOADED_CORRECTION {"task":{"attempt_count":3,"max_attempts":3,"metadata":{"task_correction":{"consumed":true}}}}\n' },
                {"Package": pkg, "Test": test, "Action": "pass"},
                {"Package": pkg, "Action": "pass"}]

    def test_rejects_unstructured_evidence(self):
        events = self.events()
        events[1]["Output"] = "RELOADED_CORRECTION persisted\n"
        with self.assertRaises(ValueError):
            validate(events, self.required)

    def test_requires_every_subcase(self):
        required = {"pkg/manager": ["TestCorrectionNativeQueueSuccess", "TestCorrectionNativeQueueSuccess/required"]}
        with self.assertRaises(ValueError):
            validate(self.events(), required)
        self.assertIn("TestCorrectionExecutionControls/cancel_after_admission", MODES["acceptance"]["pkg/manager"])
        self.assertIn("TestCorrectionExhaustionControls/no_authority", MODES["exhaustion-controls"]["pkg/manager"])

    def test_requires_named_execution(self):
        self.assertTrue(validate(self.events(), self.required))
        for events in ([], self.events()[1:], self.events()[:-1], self.events()[2:]):
            with self.assertRaises(ValueError):
                validate(events, self.required)


    def test_output_fragments_are_reassembled(self):
        events = self.events()
        events[1]["Output"] = 'RELOADED_CORRECTION {"task":{"attempt_count":3,'
        events.insert(2, dict(events[1], Output='"max_attempts":3,"metadata":{"task_correction":{"consumed":true}}}}\n'))
        self.assertEqual(len(validate(events, self.required)), 1)
        del events[2]
        with self.assertRaises(ValueError):
            validate(events, self.required)

    def test_rejects_skips_failures_and_duplicates(self):
        for action in ("skip", "fail", "run"):
            events = self.events()
            events.append(dict(events[0], Action=action))
            with self.assertRaises(ValueError):
                validate(events, self.required)
        with self.assertRaises(ValueError):
            validate(self.events(), {})

class ProofControls(unittest.TestCase):
    def test_coverage_refuses_empty_boundary_and_missing_records(self):
        import tempfile
        from pathlib import Path
        from exhausted_task_proof import require_coverage
        from retention_unit_coverage import evaluate
        for covered, total in ((0, 0), (9, 10), (89, 100)):
            with self.assertRaises(ValueError):
                require_coverage(dict(functions=[{}], covered=covered, statements=total))
        require_coverage(dict(functions=[{}], covered=91, statements=100))
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / 'coverage.out'
            fn = dict(path='example.go', name='logic', change='test', start=dict(Line=1, Column=1), end=dict(Line=9, Column=1))
            blocks = {'example.go': [(2, 1, 3, 1, 2)]}
            for text in ('', 'mode: count\n', 'mode: count\nexample.go:2.1,3.1 1 1\n'):
                profile.write_text(text)
                with self.assertRaises(ValueError):
                    evaluate([fn], blocks, profile)
            profile.write_text('mode: count\n')
            with self.assertRaises(ValueError):
                evaluate([], {}, profile)
            with self.assertRaises(ValueError):
                evaluate([fn], {'example.go': []}, profile)

    def test_mutation_rejects_compile_failure_wrong_error_and_survival(self):
        import json
        from subprocess import CompletedProcess
        from exhausted_task_proof import REGRESSION, REFUSAL, validate_mutation, replacement
        def result(code, message, action='fail'):
            events = [dict(Test=REGRESSION, Action='run'), dict(Test=REGRESSION, Action='output', Output='    regression_test.go:42: ' + message + '\n'),
                      dict(Test=REGRESSION, Action=action)]
            return CompletedProcess([], code, '\n'.join(map(json.dumps, events)), '')
        validate_mutation(result(1, REFUSAL))
        for broken in (result(0, REFUSAL), result(1, 'different failure'), result(1, REFUSAL, 'skip'),
                       CompletedProcess([], 1, '', 'compile failed')):
            with self.assertRaises(ValueError):
                validate_mutation(broken)
        with self.assertRaises(ValueError):
            replacement('integration absent')

    def test_overlay_cleanup_on_failure_and_timeout(self):
        from pathlib import Path
        from unittest.mock import patch
        import subprocess
        import exhausted_task_proof as proof
        before = set(proof.ROOT.glob('.exhausted-mutation-*'))
        source = (proof.ROOT / 'pkg/manager/task_queue.go').read_bytes()
        for failure in (ValueError('failure'), subprocess.TimeoutExpired('go', 120)):
            with patch.object(proof.subprocess, 'run', side_effect=failure), self.assertRaises(type(failure)):
                proof.removal_sensitive()
            self.assertEqual(set(proof.ROOT.glob('.exhausted-mutation-*')), before)
            self.assertEqual((proof.ROOT / 'pkg/manager/task_queue.go').read_bytes(), source)

    def test_inventory_cannot_omit_new_or_original_functions(self):
        import copy
        import json
        import tempfile
        from pathlib import Path
        from unittest.mock import patch
        import exhausted_task_proof as proof
        original = json.loads(proof.INVENTORY.read_text())
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            manifest = temp / 'inventory.json'
            new = original['functions'][-1]
            for damage in ('empty', 'original', 'changed', 'duplicate', 'missing_source'):
                data = copy.deepcopy(original)
                if damage == 'empty':
                    data['functions'] = []
                elif damage == 'original':
                    data['functions'].pop(0)
                elif damage == 'changed':
                    data['functions'].pop()
                elif damage == 'duplicate':
                    data['functions'].append(data['functions'][0])
                manifest.write_text(json.dumps(data))
                changed = [dict(path=new['path'], name=new['symbol'])]
                with patch.object(proof, 'INVENTORY', manifest), patch.object(proof.coverage, 'scope', return_value=changed), patch.object(proof.coverage, 'run', return_value='[]'), self.assertRaises(ValueError):
                    proof.inventory(temp / 'helper', temp)


if __name__ == "__main__":
    unittest.main()
