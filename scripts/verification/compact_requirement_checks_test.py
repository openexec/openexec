"""Negative controls for scope arithmetic and assertion-specific mutations."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import compact_requirement_evidence as evidence
import compact_requirement_coverage as coverage
import compact_requirement_mutations as mutations
import compact_requirement_story as story
from retention_unit_coverage import evaluate


class CoverageTests(unittest.TestCase):
    def test_missing_function(self):
        with patch.object(coverage, 'run', return_value='[]'):
            with self.assertRaisesRegex(ValueError, 'missing/duplicate function'):
                coverage.inventory(Path('helper'), {'functions': [{'path': 'x.go', 'symbol': 'missing'}]})

    def test_statement_weighted_strict_threshold_and_missing_blocks(self):
        functions = [dict(path='x.go', name='f', change='scope', start={'Line': 1, 'Column': 1}, end={'Line': 5, 'Column': 1})]
        blocks = {'x.go': [(1, 1, 2, 1, 9), (3, 1, 4, 1, 1)]}
        with tempfile.TemporaryDirectory() as directory, patch('retention_unit_coverage.run', return_value='baseline'):
            profile = Path(directory) / 'cover'
            profile.write_text('mode: count\nx.go:1.1,2.1 9 1\nx.go:3.1,4.1 1 0\n')
            result = evaluate(functions, blocks, profile)
            self.assertEqual((result['covered'], result['statements']), (9, 10))
            self.assertFalse(result['passed'])
            profile.write_text('mode: count\nx.go:1.1,2.1 9 1\nx.go:3.1,4.1 1 1\n')
            self.assertTrue(evaluate(functions, blocks, profile)['passed'])
            profile.write_text('mode: count\nx.go:1.1,2.1 9 1\n')
            with self.assertRaisesRegex(ValueError, 'missing/mismatched'):
                evaluate(functions, blocks, profile)
            with self.assertRaisesRegex(ValueError, 'empty'):
                evaluate([], {}, profile)


class MutationTests(unittest.TestCase):
    def test_only_intended_assertion_counts(self):
        event = {'Action': 'output', 'Test': mutations.TEST, 'Output': 'expected reason'}
        failure = {'Action': 'fail', 'Test': mutations.TEST}
        def result(events, code=1):
            return subprocess.CompletedProcess([], code, '\n'.join(json.dumps(e) for e in events), '')
        self.assertTrue(mutations.intended_failure(result([event, failure]), 'expected reason'))
        for value in [result([event, failure], 0), result([failure]), result([event]),
                      result([event, failure, {'Action': 'fail', 'Test': 'TestUnrelated'}])]:
            self.assertFalse(mutations.intended_failure(value, 'expected reason'))

    def test_failed_command_restores_exact_contents(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'internal/planner/prompt.go'
            source.parent.mkdir(parents=True)
            original = (mutations.ROOT / 'internal/planner/prompt.go').read_bytes()
            source.write_bytes(original)
            with patch.object(mutations, 'ROOT', root), patch.object(mutations, 'OUTPUT', root / 'out'), \
                    patch.object(mutations.subprocess, 'run', return_value=subprocess.CompletedProcess([], 2, '', 'compile failure')), \
                    patch.object(mutations.signal, 'signal'):
                with self.assertRaisesRegex(ValueError, 'did not fail intended assertion'):
                    mutations.main()
            self.assertEqual(source.read_bytes(), original)
            self.assertFalse((root / 'out/mutations.json').exists())


class StoryTests(unittest.TestCase):
    def test_exact_commands_and_failed_receipt(self):
        self.assertEqual(len(story.commands()), 8)
        self.assertEqual(story.commands()[4], 'make test')
        with tempfile.TemporaryDirectory() as directory, patch.object(evidence, 'ROOT', Path(directory)), \
                patch.object(evidence, 'source_digest', return_value='digest'), \
                patch.object(story, 'commands', return_value=['exit 7']), \
                patch('sys.argv', ['story']):
            self.assertEqual(story.main(), 1)
            receipt = json.loads((Path(directory) / '.openexec/compact-requirement-checks/story-1.json').read_text())
            self.assertEqual(receipt['exit_code'], 7)
            self.assertEqual(receipt['status'], 'finished')


class RepairTests(unittest.TestCase):
    def test_repair_refuses_stale_missing_and_incomplete_proof(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / '.openexec/compact-requirement-checks'
            output.mkdir(parents=True)
            prompt = root / 'internal/planner/prompt.go'
            prompt.parent.mkdir(parents=True)
            prompt.write_text('source')
            measured = dict(source_sha256='digest', covered=91, statements=100, functions=[{}])
            controls = dict(source_sha256='digest', restored=True,
                restored_sha256=hashlib.sha256(prompt.read_bytes()).hexdigest(),
                mutations=[dict(mutation=m, exit_code=1) for m in ('output-key', 'identity-rule')])
            result = dict(compact_output_declares_requirement_id=True, functions=1)
            def write():
                (output / 'coverage.json').write_text(json.dumps(measured))
                (output / 'mutations.json').write_text(json.dumps(controls))
            with patch.object(evidence, 'ROOT', root), patch.object(evidence, 'source_digest', return_value='digest'):
                with self.assertRaises(FileNotFoundError): evidence.repair(result.copy())
                write()
                self.assertTrue(evidence.repair(result.copy())['repair_verified'])
                for obj, key, value, error in [(measured, 'source_sha256', 'old', 'stale'),
                        (measured, 'covered', 90, 'insufficient'),
                        (measured, 'statements', 0, 'insufficient'),
                        (measured, 'functions', [], 'missing measured'),
                        (controls, 'restored', False, 'not restored'),
                        (controls, 'mutations', [], 'missing mutations')]:
                    old = obj[key]
                    obj[key] = value
                    write()
                    with self.assertRaisesRegex(ValueError, error): evidence.repair(result.copy())
                    obj[key] = old
                write()
                result['compact_output_declares_requirement_id'] = False
                with self.assertRaisesRegex(ValueError, 'output field absent'): evidence.repair(result)


if __name__ == '__main__':
    unittest.main()
