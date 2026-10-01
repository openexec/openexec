"""Negative controls for discovery, separate from later runtime mutations."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from plan_identity_discovery import ROOT, MANIFEST, DOCUMENT, run, validate


class DiscoveryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix='identity-discovery-test-')
        cls.helper = Path(cls.temp.name) / 'inventory'
        run('go', 'build', '-o', str(cls.helper), './scripts/verification/retentioncoverage')
        cls.manifest = json.loads((ROOT / MANIFEST).read_text())

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def test_missing_and_external_fixture(self):
        for path in ['missing.json', '/etc/passwd', '../retained.json']:
            m = copy.deepcopy(self.manifest)
            m['fixtures'][0] = path
            with self.assertRaisesRegex(ValueError, 'missing or nonlocal'):
                validate(ROOT, m, self.helper)

    def test_fixture_substitution(self):
        m = copy.deepcopy(self.manifest)
        m['fixtures'][0] = m['fixtures'][1]
        with self.assertRaisesRegex(ValueError, 'fixture semantics'):
            validate(ROOT, m, self.helper)
        m = copy.deepcopy(self.manifest)
        m['identities'][0] = 'US-010'
        with self.assertRaisesRegex(ValueError, 'literal fixture identities'):
            validate(ROOT, m, self.helper)

    def test_native_and_sql_inventory_required(self):
        for entry in ['pkg/manager/planner.go:(*Manager).importBoundPlan',
                      'internal/release/reviewed_plan_import.go:(*Manager).importReviewedPlan',
                      'internal/release/sqlite_store.go:(*SQLiteStore).getStoryInternal']:
            m = copy.deepcopy(self.manifest)
            m['functions'].remove(entry)
            with self.assertRaisesRegex(ValueError, 'inventory'):
                validate(ROOT, m, self.helper)

    def test_threshold_cannot_be_weakened(self):
        for key, value in [('operator', '>='), ('percent', 89), ('scope', 'edited lines')]:
            m = copy.deepcopy(self.manifest)
            m['coverage'][key] = value
            with self.assertRaisesRegex(ValueError, 'exceed 90%'):
                validate(ROOT, m, self.helper)

    def test_source_blocks_required(self):
        m = copy.deepcopy(self.manifest)
        m['blocks'] = []
        with self.assertRaisesRegex(ValueError, 'block omitted'):
            validate(ROOT, m, self.helper)

    def test_missing_case_evidence(self):
        original = Path.read_text
        def read(path, *args, **kwargs):
            content = original(path, *args, **kwargs)
            if path == ROOT / DOCUMENT:
                return content.replace('## Schema-supported SQL NULL\n', '')
            return content
        with patch.object(Path, 'read_text', read):
            with self.assertRaisesRegex(ValueError, 'case evidence'):
                validate(ROOT, self.manifest, self.helper)


if __name__ == '__main__':
    unittest.main()
