"""Exercise aggregate discovery through the real CLI in isolated disk fixtures."""
import subprocess
import unittest

import test_architecture_traceability as traceability


class ArchitectureTest(traceability.ArchitectureTraceabilityTest):
    def run_case(self, case='architecture'):
        return super().run_case(case)

    def test_valid_document_round_trip(self):
        for _ in range(2):
            result = self.run_case()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('architecture-contracts: PASS', result.stdout)
            self.assertIn('architecture-traceability: PASS', result.stdout)
            self.assertIn('architecture: PASS', result.stdout)

    def test_each_checked_command_failure_stops_aggregate(self):
        directory = self.root / 'scripts/autonomy-contract'
        for checker, code in [('architecture_contracts.py', 23),
                              ('architecture_traceability.py', 24)]:
            with self.subTest(checker=checker):
                path = directory / checker
                original = path.read_text()
                try:
                    path.write_text(f'raise SystemExit({code})\n')
                    result = self.run_case()
                    self.assertEqual(result.returncode, code, result.stderr)
                    self.assertNotIn('architecture: PASS', result.stdout)
                    if code == 23:
                        self.assertNotIn('architecture-traceability:', result.stdout)
                finally:
                    path.write_text(original)

    def test_missing_checker_refused(self):
        (self.root / 'scripts/autonomy-contract/architecture_traceability.py').unlink()
        result = self.run_case()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('architecture: PASS', result.stdout)

    def test_invalid_arguments_refused(self):
        script = self.root / 'scripts/autonomy-contract/verify-runtime-evidence.sh'
        for args in [[], ['--case'], ['architecture'], ['--wrong', 'architecture'],
                     ['--case', 'architecture', 'extra']]:
            with self.subTest(args=args):
                result = subprocess.run(['bash', str(script), *args], cwd='/tmp',
                                        text=True, capture_output=True)
                self.assertEqual(result.returncode, 2)
                self.assertNotIn('PASS', result.stdout)


if __name__ == '__main__':
    unittest.main()
