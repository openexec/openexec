import json
import unittest

from recapture_compatibility import PACKAGE, REQUIRED, check_events


class CompatibilityVerifierTests(unittest.TestCase):
    def events(self):
        return [dict(Action='pass', Package=PACKAGE, Test=name) for name in sorted(REQUIRED)] + [dict(Action='pass', Package=PACKAGE)]

    def check(self, events):
        return check_events('\n'.join(json.dumps(event) for event in events))

    def test_complete(self):
        self.assertEqual(self.check(self.events()), sorted(REQUIRED))

    def test_missing_leaf_parent_or_package(self):
        for index in range(len(self.events())):
            events = self.events()
            del events[index]
            with self.assertRaises(ValueError):
                self.check(events)

    def test_skip_failure_duplicate_unexpected(self):
        for event in [dict(Action=action, Package=PACKAGE, Test=next(iter(REQUIRED))) for action in ('skip', 'fail', 'pass')] + [dict(Action='pass', Package=PACKAGE, Test='unexpected')]:
            with self.assertRaises(ValueError):
                self.check(self.events() + [event])

    def test_empty_malformed_foreign(self):
        with self.assertRaises(ValueError):
            check_events('')
        with self.assertRaises(ValueError):
            check_events('not json')
        with self.assertRaises(ValueError):
            self.check([dict(Action='pass', Package='foreign')])


if __name__ == '__main__':
    unittest.main()
