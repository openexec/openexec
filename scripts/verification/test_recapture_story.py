"""Aggregate controls: omitted proof must never turn into success."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import recapture_story as story
import test_recapture_boundaries as boundary_tests


class RecaptureStoryTests(unittest.TestCase):
    def setUp(self):
        self.manifest = json.loads(story.MANIFEST.read_text())

    def test_manifest(self):
        story.check_manifest(self.manifest)
        for case in story.CASES:
            with self.subTest(case=case):
                broken = copy.deepcopy(self.manifest)
                del broken[case]
                with self.assertRaises(ValueError):
                    story.check_manifest(broken)
                broken = copy.deepcopy(self.manifest)
                broken[case].pop(0)
                with self.assertRaises(ValueError):
                    story.check_manifest(broken)
                broken = copy.deepcopy(self.manifest)
                broken[case].append(broken[case][0])
                with self.assertRaises(ValueError):
                    story.check_manifest(broken)

    def test_failed_helper_preserved_and_remaining_cases_execute(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp)
            responses = [dict(passed=(i != 1), exit_code=7 if i == 1 else 0)
                         for i in range(4)]
            with patch.object(story, 'run_case', side_effect=responses) as run:
                result = story.execute(story.CASES, output, self.manifest)
            self.assertFalse(result['passed'])
            self.assertEqual(run.call_count, 4)
            self.assertEqual(json.loads((output / 'result.json').read_text()), result)
            self.assertEqual(result['cases'][story.CASES[1]]['exit_code'], 7)

    def test_missing_report_and_nonzero_exit(self):
        with tempfile.TemporaryDirectory() as tmp:
            for code in (0, 17):
                with patch.object(story.subprocess, 'run') as run:
                    run.return_value.returncode = code
                    result = story.run_case(story.CASES[0], Path(tmp), self.manifest[story.CASES[0]])
                self.assertFalse(result['passed'])
                self.assertEqual(result['exit_code'], code)
            with patch.object(story.subprocess, 'run') as run, patch.object(
                    story, 'check_report', side_effect=story.subprocess.CalledProcessError(2, 'go tool cover')):
                run.return_value.returncode = 0
                result = story.run_case(story.CASES[2], Path(tmp), self.manifest[story.CASES[2]])
            self.assertFalse(result['passed'])
            self.assertIn('go tool cover', result['error'])

    def test_boundary_report_must_retain_snapshots(self):
        events = '\n'.join(map(json.dumps, boundary_tests.RecaptureBoundaryProofTests().events()))
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            (directory / 'tests.jsonl').write_text(events)
            report = dict(passed=True, **story.boundaries.check_proof(events))
            (directory / 'result.json').write_text(json.dumps(report))
            story.check_report(story.CASES[1], directory, self.manifest[story.CASES[1]])
            del report['persisted_states']
            (directory / 'result.json').write_text(json.dumps(report))
            with self.assertRaises(ValueError):
                story.check_report(story.CASES[1], directory, self.manifest[story.CASES[1]])

    def test_incomplete_coverage_scope(self):
        required = self.manifest[story.CASES[2]]
        events = [dict(Action='pass', Package='github.com/openexec/openexec/' + s.split(':')[0],
                       Test=s.split(':')[1]) for s in required]
        events.extend(dict(Action='pass', Package='github.com/openexec/openexec/' + p)
                      for p in story.coverage.PACKAGES)
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            (directory / 'result.json').write_text('{"passed": true, "percent": 100}')
            (directory / 'scope.json').write_text('[]')
            (directory / 'tests.jsonl').write_text('\n'.join(map(json.dumps, events)))
            with self.assertRaisesRegex(ValueError, 'scope'):
                story.check_report(story.CASES[2], directory, required)

    def test_skipped_protected_format(self):
        events = [dict(Action='pass', Package=story.compatibility.PACKAGE, Test=s)
                  for s in story.compatibility.REQUIRED]
        events.append(dict(Action='pass', Package=story.compatibility.PACKAGE))
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            (directory / 'result.json').write_text('{"passed": true}')
            for action in ('skip', 'fail'):
                events[0]['Action'] = action
                (directory / 'tests.jsonl').write_text('\n'.join(map(json.dumps, events)))
                with self.assertRaises(ValueError):
                    story.check_report(story.CASES[3], directory, self.manifest[story.CASES[3]])


if __name__ == '__main__':
    unittest.main()
