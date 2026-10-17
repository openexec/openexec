"""Run the task command against disk fixtures and refuse incomplete evidence."""
import shutil
import unittest

import test_architecture_contracts as contracts
from architecture_traceability import OBLIGATIONS, BOUNDARY


class ArchitectureTraceabilityTest(contracts.ArchitectureContractsTest):
    def setUp(self):
        super().setUp()
        shutil.copyfile(contracts.ROOT / 'scripts/autonomy-contract/architecture_traceability.py',
                        self.root / 'scripts/autonomy-contract/architecture_traceability.py')

    def run_case(self, case='architecture-traceability'):
        return super().run_case(case)

    def test_valid_document_round_trip(self):
        result = self.run_case()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('8 obligations', result.stdout)
        self.assertIn('documentation only', result.stdout)

    def test_each_obligation_is_required(self):
        original = self.doc.read_text()
        for name in OBLIGATIONS:
            with self.subTest(name=name):
                self.doc.write_text('\n'.join(line for line in original.splitlines()
                                              if not line.startswith(f'| {name} |')) + '\n')
                self.assertIn('obligation rows', self.run_case().stderr)

    def test_duplicate_obligation(self):
        text = self.doc.read_text()
        row = next(line for line in text.splitlines() if line.startswith('| D2 default'))
        self.doc.write_text(text.replace(row, row + '\n' + row))
        self.assertIn('duplicate obligation', self.run_case().stderr)

    def test_engine_cannot_own_console_obligation(self):
        self.doc.write_text(self.doc.read_text().replace(
            '| D1 downstream verification | Agent Console |',
            '| D1 downstream verification | OpenExec engine |'))
        self.assertIn('responsible party', self.run_case().stderr)

    def test_no_unsupported_completion(self):
        self.doc.write_text(self.doc.read_text().replace('| Pending:', '| Completed:'))
        self.assertIn('unsupported completion', self.run_case().stderr)

    def test_required_downstream_evidence(self):
        self.doc.write_text(self.doc.read_text().replace('real-HTTP', 'mock-only'))
        self.assertIn('missing real-HTTP', self.run_case().stderr)

    def test_retained_boundary_fields(self):
        original = self.doc.read_text()
        for key, value in BOUNDARY.items():
            with self.subTest(key=key):
                self.doc.write_text(original.replace(f'- {key}: {value}', f'- {key}: invented'))
                self.assertIn('retained boundary changed', self.run_case().stderr)

    def test_historical_claims_require_qualification(self):
        self.doc.write_text(self.doc.read_text().replace('historical, unverified', 'verified current'))
        self.assertIn('historical evidence', self.run_case().stderr)

    def test_traceability_checker_failure_propagates(self):
        (self.root / 'scripts/autonomy-contract/architecture_traceability.py').write_text(
            'raise SystemExit(24)\n')
        self.assertEqual(self.run_case().returncode, 24)


if __name__ == '__main__':
    unittest.main()
