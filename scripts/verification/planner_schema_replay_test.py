import json
import unittest

from planner_schema_replay import discovery, verify_events, MODULE


class ReplayVerifierTests(unittest.TestCase):
    def events(self):
        return [
            dict(Package=MODULE + "pkg/runtime", Action="run", Test="TestJourney"),
            dict(Package=MODULE + "pkg/runtime", Action="run", Test="TestJourney/approved"),
            dict(Package=MODULE + "pkg/runtime", Action="pass", Test="TestJourney/approved"),
            dict(Package=MODULE + "pkg/runtime", Action="pass", Test="TestJourney"),
            dict(Package=MODULE + "pkg/runtime", Action="pass"),
        ]

    def verify(self, events):
        verify_events("\n".join(json.dumps(e) for e in events),
                      "pkg/runtime", {"TestJourney": ["approved"]})

    def test_complete(self):
        discovery("TestJourney\nok", ["TestJourney"])
        self.verify(self.events())

    def test_discovery_missing_or_duplicate(self):
        for output in ("ok", "TestJourney\nTestJourney"):
            with self.assertRaises(ValueError):
                discovery(output, ["TestJourney"])

    def test_missing_each_event(self):
        for i in range(len(self.events())):
            events = self.events()
            del events[i]
            with self.assertRaises(ValueError):
                self.verify(events)

    def test_skip_fail_duplicate_and_wrong_package(self):
        for action in ("skip", "fail", "pass", "run"):
            events = self.events()
            events.append(dict(Package=MODULE + "pkg/runtime", Action=action, Test="TestJourney"))
            with self.assertRaises(ValueError):
                self.verify(events)
        events = self.events()
        events[0]["Package"] = MODULE + "pkg/other"
        with self.assertRaises(ValueError):
            self.verify(events)

    def test_undeclared_and_invalid_json(self):
        events = self.events()
        events[1]["Test"] = "TestJourney/unexpected"
        with self.assertRaises(ValueError):
            self.verify(events)
        with self.assertRaises(ValueError):
            verify_events("invalid", "pkg/runtime", {"TestJourney": []})


if __name__ == "__main__":
    unittest.main()
