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


if __name__ == "__main__":
    unittest.main()
