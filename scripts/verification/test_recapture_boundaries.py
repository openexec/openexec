"""Negative controls for incomplete or stale boundary proof acceptance."""
import json
import unittest

import recapture_boundaries as boundaries


class RecaptureBoundaryProofTests(unittest.TestCase):
    def events(self):
        package = 'github.com/openexec/openexec/pkg/manager'
        events = []
        for test in boundaries.REQUIRED:
            for checkpoint in sorted(boundaries.CHECKPOINTS.get(test, ())):
                state = dict(checkpoint=checkpoint, task={'id': 'A'}, settings={'id': 'Settings'})
                events.append(dict(Action='output', Package=package, Test=test,
                                   Output=boundaries.MARKER + json.dumps(state)))
            events.append(dict(Action='pass', Package=package, Test=test))
        events.append(dict(Action='pass', Package=package))
        return events

    def check(self, events):
        return boundaries.check_proof('\n'.join(map(json.dumps, events)))

    def test_complete(self):
        proof = self.check(self.events())
        self.assertEqual(len(proof['persisted_states']), 8)

    def test_missing_completion(self):
        events = self.events()
        events.pop(-2)
        with self.assertRaises(ValueError):
            self.check(events)

    def test_skip(self):
        events = self.events()
        events[-2]['Action'] = 'skip'
        with self.assertRaises(ValueError):
            self.check(events)

    def test_missing_state(self):
        events = self.events()
        events.pop(1)
        with self.assertRaises(ValueError):
            self.check(events)

    def test_duplicate_state(self):
        events = self.events()
        events.append(events[1])
        with self.assertRaises(ValueError):
            self.check(events)

    def test_empty_snapshot(self):
        events = self.events()
        events[1]['Output'] = boundaries.MARKER + json.dumps(
            dict(checkpoint='terminal', task={}, settings={}))
        with self.assertRaises(ValueError):
            self.check(events)

    def test_duplicate_completion(self):
        events = self.events()
        events.append(events[-2])
        with self.assertRaises(ValueError):
            self.check(events)


if __name__ == '__main__':
    unittest.main()
