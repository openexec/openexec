"""Reject unrelated failures as proof of the old-directory regression."""
import json
import subprocess
import unittest

import private_storage as gate


class NegativeControlTests(unittest.TestCase):
    def result(self, markers=gate.EXPOSURE, omitted=None, action='fail', code=1):
        events = []
        for name in gate.JOURNEYS - {omitted}:
            events.extend([{'Test': name, 'Action': 'output', 'Output': '\n'.join(markers)},
                           {'Test': name, 'Action': action}])
        return subprocess.CompletedProcess([], code, '\n'.join(map(json.dumps, events)))

    def test_all_journeys_must_detect_source_and_staging(self):
        result = gate.negative_result(self.result())
        self.assertEqual(result['status'], 'passed')
        self.assertEqual(len(result['source_and_staging_detected']), 6)

    def test_compile_or_path_only_failure_is_not_proof(self):
        for markers in ((), ('unprotected evidence path',)):
            self.assertEqual(gate.negative_result(self.result(markers))['status'], 'failed')

    def test_each_visibility_and_staging_assertion_is_required(self):
        for marker in gate.EXPOSURE:
            with self.subTest(marker=marker):
                result = self.result(tuple(m for m in gate.EXPOSURE if m != marker))
                self.assertEqual(gate.negative_result(result)['status'], 'failed')

    def test_missing_skipped_or_successful_control_is_not_proof(self):
        for result in (self.result(omitted=next(iter(gate.JOURNEYS))),
                       self.result(action='skip'), self.result(code=0)):
            self.assertEqual(gate.negative_result(result)['status'], 'failed')


if __name__ == '__main__':
    unittest.main()
