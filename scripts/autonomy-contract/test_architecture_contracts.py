"""Exercise the shell entry point with persisted, isolated checkout fixtures."""
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class ArchitectureContractsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.doc = self.root / 'docs/ARCHITECTURE.md'
        content = (ROOT / 'docs/ARCHITECTURE.md').read_text()
        paths = {'docs/ARCHITECTURE.md',
                 'scripts/autonomy-contract/verify-runtime-evidence.sh',
                 'scripts/autonomy-contract/architecture_contracts.py'}
        paths.update(re.findall(r'\]\(\.\./([^#)]+)#', content))
        for path in paths:
            target = self.root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / path, target)

    def run_case(self, case='architecture-contracts'):
        return subprocess.run(
            ['bash', '-euo', 'pipefail', str(self.root / 'scripts/autonomy-contract/verify-runtime-evidence.sh'),
             '--case', case], cwd='/tmp', text=True, capture_output=True)

    def test_valid_document_round_trip(self):
        result = self.run_case()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('discovery only', result.stdout)

    def test_missing_document(self):
        self.doc.unlink()
        self.assertNotEqual(self.run_case().returncode, 0)

    def test_missing_source(self):
        (self.root / 'pkg/execution/execution.go').unlink()
        self.assertNotEqual(self.run_case().returncode, 0)

    def test_unresolved_symbol(self):
        self.doc.write_text(self.doc.read_text().replace('#Request)', '#NonexistentDeclaration)'))
        self.assertIn('unresolved declaration', self.run_case().stderr)

    def test_required_section(self):
        self.doc.write_text(self.doc.read_text().replace('## Binding validation', '## Removed'))
        self.assertNotEqual(self.run_case().returncode, 0)

    def test_removed_required_reference(self):
        self.doc.write_text(re.sub(r'\]\(\.\./pkg/execution/execution.go#\w+\)', ']', self.doc.read_text()))
        self.assertIn('missing required source', self.run_case().stderr)

    def test_missing_restored_resource_refused(self):
        source = self.root / 'pkg/runtime/execution.go'
        source.unlink()
        self.assertIn('missing restored resource', self.run_case().stderr)

    def test_checker_failure_propagates(self):
        (self.root / 'scripts/autonomy-contract/architecture_contracts.py').write_text(
            'raise SystemExit(23)\n')
        self.assertEqual(self.run_case().returncode, 23)

    def test_unsupported_case(self):
        self.assertEqual(self.run_case('unsupported-case').returncode, 2)


if __name__ == '__main__':
    unittest.main()
