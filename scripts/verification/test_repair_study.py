"""Saved-file negative controls for the stage's actual study command."""
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

from repair_study import DOC, JSON_FILES, ROOT, SOURCES, verify


class StudyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        paths = {'NOTES.md', 'docs/ARCHITECTURE.md', DOC,
                 'scripts/verify-verification-repair.sh',
                 'scripts/verification/repair_study.py', 'scripts/verification/discovery.py'}
        paths.update('docs/verification/' + f for f in JSON_FILES)
        paths.update(SOURCES)
        scopes = json.loads((ROOT / 'docs/verification/repair-coverage-scopes.json').read_text())
        paths.update(f.split(':')[0] for s in scopes['scopes'].values() for f in s['functions'])
        for doc in ('docs/ARCHITECTURE.md', DOC):
            text = (ROOT / doc).read_text()
            paths.update(re.findall(r'^\| `([^`]+\.go)` \|', text, re.M))
            paths.update(str(Path(doc).parent / p.split('#')[0])
                         for p in re.findall(r'\]\(([^)]+)\)', text) if '://' not in p)
        for path in paths:
            target = self.root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / path, target)

    def change(self, old, new, path=DOC):
        file = self.root / path
        text = file.read_text()
        self.assertIn(old, text)
        file.write_text(text.replace(old, new))

    def edit_json(self, name, edit):
        file = self.root / 'docs/verification' / name
        value = json.loads(file.read_text())
        edit(value)
        file.write_text(json.dumps(value, indent=2) + '\n')

    def refused(self, text):
        self.assertIn(text, '\n'.join(verify(self.root)[0]))

    def run_dispatch(self, *args):
        return subprocess.run(['bash', str(self.root / 'scripts/verify-verification-repair.sh'), *args],
                              cwd='/tmp', capture_output=True, text=True)

    def test_saved_study_end_to_end_and_missing_source(self):
        result = self.run_dispatch('--case', 'study')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('not certified', result.stdout)
        (self.root / 'internal/execution/evidence/capture.go').unlink()
        result = self.run_dispatch('--case', 'study')
        self.assertEqual(result.returncode, 1)
        self.assertIn('missing scoped source', result.stderr)

    def test_unknown_pending_and_malformed_dispatch(self):
        for args in ([], ['--case'], ['--other', 'study'], ['--case', 'unknown'],
                     ['--case', 'delivery-ready'], ['--case', 'study', 'extra']):
            with self.subTest(args=args):
                self.assertEqual(self.run_dispatch(*args).returncode, 2)

    def test_exact_clause_mutation(self):
        self.edit_json('repair-requirements.json', lambda v: v[0].update(exact_clause='weakened'))
        self.refused('changed exact accepted clause')

    def test_wrong_owner(self):
        self.edit_json('repair-requirements.json', lambda v: v[0].update(owner='US-013'))
        self.refused('wrong requirement owner')

    def test_duplicate_requirement(self):
        self.edit_json('repair-requirements.json', lambda v: v.append(v[0]))
        self.refused('missing/duplicate requirement')

    def test_contract_snapshot_tamper(self):
        self.edit_json('repair-study-contract.json', lambda v: v['goal'].update(description='different'))
        self.refused('accepted contract snapshot changed')

    def test_missing_checklist_item(self):
        self.change('- **Falsify:** replace new wrapper', '- **Omitted:** replace new wrapper')
        self.refused('missing/duplicate checklist: F2/Falsify')

    def test_missing_case(self):
        self.change('F1-empty-test', 'omitted-case')
        self.refused('missing/duplicate review case: F1-empty-test')

    def test_duplicate_finding(self):
        self.change('## Coverage contract', '## F3 — Private evidence enters source commits\n\n## Coverage contract')
        self.refused('missing/duplicate study section')

    def test_missing_scope(self):
        self.edit_json('repair-coverage-scopes.json', lambda v: v['scopes'].pop('US-014'))
        self.refused('missing scope')

    def test_hidden_denominator(self):
        self.edit_json('repair-coverage-scopes.json', lambda v: v['scopes']['US-012']['functions'].pop())
        self.refused('inventory/denominator changed')

    def test_lowered_coverage_threshold(self):
        self.edit_json('repair-coverage-scopes.json', lambda v: v['scopes']['US-013'].update(minimum_statement_percent_exclusive=89))
        self.refused('coverage threshold changed')

    def test_absent_declaration(self):
        (self.root / 'pkg/runtime/evidence.go').write_text('package runtime\n')
        self.refused('missing scoped declaration')

    def test_missing_provenance_and_invalid_json(self):
        file = self.root / 'docs/verification/repair-study-provenance.json'
        for content in ('{}', '{bad json', '{"feature":1,"feature":2}'):
            file.write_text(content)
            result = self.run_dispatch('--case', 'study')
            self.assertEqual(result.returncode, 1, result.stdout)
            self.assertIn('study failed:', result.stderr)

    def test_candidate_mismatch(self):
        self.edit_json('repair-study-provenance.json', lambda v: v['candidate'].update(revision='0' * 40))
        self.refused('invalid candidate provenance')

    def test_console_deployment_overclaim(self):
        self.edit_json('repair-study-provenance.json', lambda v: v['console'].update(deployment='verified'))
        self.refused('invalid Console evidence')

    def test_missing_dependency_evidence(self):
        self.edit_json('repair-study-provenance.json', lambda v: v['console']['files'].pop('go.sum'))
        self.refused('missing Console source/dependency evidence')

    def test_broken_documentation_link(self):
        self.change('(repair-requirements.json)', '(missing.json)')
        self.refused('broken documentation link')

    def test_notes_persistence(self):
        self.change('[US-010 / T-US-010-001]', '[removed]', 'NOTES.md')
        self.refused('missing current task memory')

    def test_lost_authority_limit(self):
        self.change('outside supplied', 'inside some')
        self.refused('missing evidence/authority limitation')


if __name__ == '__main__':
    unittest.main()
