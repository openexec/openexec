"""Composition controls: no empty success, dropped failures or stale proof."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import retention_story as story
from test_retention_mutations import transcript


class StoryControls(unittest.TestCase):
    def test_required_scenarios(self):
        events = '\n'.join(json.dumps(e) for e in [
            dict(Action='pass', Package=story.MODULE+'pkg/manager', Test='TestA'),
            dict(Action='pass', Package=story.MODULE+'pkg/manager')])
        required = ['pkg/manager:TestA']
        self.assertEqual(story.check_events(events, required), required)
        for invalid, manifest in [('', required), (events, []), (events, required*2),
                                  (events, required+['pkg/manager:TestA/omitted']),
                                  (events.replace('pass', 'skip', 1), required),
                                  (events.replace('pass', 'fail', 1), required),
                                  (events+'\n'+events, required),
                                  (events.splitlines()[0], required)]:
            with self.subTest(invalid=invalid, manifest=manifest), self.assertRaises(ValueError):
                story.check_events(invalid, manifest)

    def test_mutation_report_requires_assertions_and_all_branches(self):
        m = story.mutations
        report = {'candidate': m.classify(transcript({}), '', 0, {})}
        for branch, (_, _, assertions) in m.MUTATIONS.items():
            report[branch] = m.classify(transcript(assertions), '', 1, assertions)
        story.check_mutations(report)
        for branch in report:
            invalid = dict(report)
            del invalid[branch]
            with self.assertRaises(ValueError):
                story.check_mutations(invalid)
        report['Execute']['assertions'] = {}
        with self.assertRaises(ValueError):
            story.check_mutations(report)

    def test_subprocess_failure_propagates_for_every_case(self):
        manifest = json.loads(story.MANIFEST.read_text())
        with tempfile.TemporaryDirectory() as directory:
            for case in story.CASES:
                with self.subTest(case=case), patch.object(story.subprocess, 'run', side_effect=subprocess.CalledProcessError(17, ['fixture'])):
                    with self.assertRaises(subprocess.CalledProcessError) as error:
                        story.run_case(case, Path(directory), manifest)
                    self.assertEqual(error.exception.returncode, 17)

    def test_aggregate_stops_without_success_report_on_any_failure(self):
        for failed_case in story.CASES:
            with self.subTest(case=failed_case), tempfile.TemporaryDirectory() as directory:
                calls = []
                def run(case, output, manifest):
                    calls.append(case)
                    if case == failed_case:
                        raise subprocess.CalledProcessError(19, ['fixture'])
                    return {'status': 'passed'}
                with patch('sys.argv', ['retention_story.py', '--case', 'retention-story']), patch.object(story.tempfile, 'mkdtemp', return_value=directory), patch.object(story, 'run_case', side_effect=run):
                    with self.assertRaises(subprocess.CalledProcessError):
                        story.main()
                self.assertEqual(calls, list(story.CASES[:story.CASES.index(failed_case)+1]))
                self.assertFalse((Path(directory) / 'result.json').exists())

    def test_successful_process_without_report_is_not_proof(self):
        manifest = json.loads(story.MANIFEST.read_text())
        with tempfile.TemporaryDirectory() as directory:
            for case in story.CASES:
                with self.subTest(case=case), patch.object(story.subprocess, 'run'):
                    with self.assertRaises((ValueError, FileNotFoundError)):
                        story.run_case(case, Path(directory), manifest)


if __name__ == '__main__':
    unittest.main()
