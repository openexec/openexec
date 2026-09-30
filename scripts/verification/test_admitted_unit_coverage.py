"""Negative controls exercise the same evaluator as the admitted gate."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import admitted_unit_coverage as gate
import test_storage_unit_coverage as controls


class AdmittedProfileTests(controls.StorageProfileTests):
    pass


class AdmittedScopeTests(unittest.TestCase):
    def inventory(self, names, changed=(), declared=None):
        study = {'baseline': gate.BASELINE, 'scopes': {'US-012': {
            'functions': ['pkg/example.go:Original'], 'planned_functions': ['pkg/example.go:Attachment'],
            'minimum_statement_percent_exclusive': 90}}}
        if declared:
            study['scopes']['US-012'].update(declared)
        with tempfile.TemporaryDirectory() as temp:
            source = Path(temp) / 'study.json'
            source.write_text(json.dumps(study))
            functions = [dict(name=n, source='body') for n in names]
            with patch.object(gate, 'STUDY', source), patch.object(gate.shared, 'scope', return_value=list(changed)), \
                    patch.object(gate.shared, 'run', return_value=json.dumps(functions)):
                return gate.inventory(Path('helper'), {'functions': ['pkg/example.go:Original', 'pkg/example.go:Attachment']}, Path(temp)/'baseline.go')

    def test_declared_and_planned_bodies_required(self):
        for names in ([], ['Original'], ['Attachment']):
            with self.subTest(names=names), self.assertRaisesRegex(ValueError, 'missing scoped'):
                self.inventory(names)

    def test_added_helper_cannot_be_omitted(self):
        changed = [dict(path='pkg/example.go', name='NewHelper')]
        with self.assertRaisesRegex(ValueError, 'missing scoped'):
            self.inventory(['Original', 'Attachment'], changed)
        result = self.inventory(['Original', 'Attachment', 'NewHelper'], changed)
        self.assertEqual(len(result), 3)

    def test_duplicate_or_weakened_declaration(self):
        for declared in ({'functions': ['pkg/example.go:Original']*2},
                         {'minimum_statement_percent_exclusive': 89}):
            with self.assertRaises(ValueError):
                self.inventory(['Original', 'Attachment'], declared=declared)

    def test_unowned_source_refused(self):
        with self.assertRaisesRegex(ValueError, 'unowned'):
            self.inventory(['Original', 'Attachment'], [dict(path='pkg/new.go', name='Forgotten')])

    def test_study_omission_refused(self):
        with self.assertRaisesRegex(ValueError, 'scope omitted'):
            self.inventory(['Original', 'Attachment'], declared={'planned_functions': []})

    def test_stale_source_refused(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(gate, 'ROOT', Path(temp)):
            p = Path(temp)/'source.go'
            p.write_text('old')
            hashes = {'source.go': hashlib.sha256(p.read_bytes()).hexdigest()}
            gate.check_sources(hashes)
            p.write_text('new')
            with self.assertRaisesRegex(ValueError, 'source changed'):
                gate.check_sources(hashes)

    def test_failed_run_removes_stale_success(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            for name in ('result.json', 'scope.json', 'coverage.out', 'tests.jsonl'):
                (output/name).write_text('stale')
            with patch('sys.argv', ['gate', '--output', temp]), patch.object(gate, 'MANIFEST', output/'absent'):
                with self.assertRaises(FileNotFoundError):
                    gate.main()
            self.assertFalse(any(output.iterdir()))
