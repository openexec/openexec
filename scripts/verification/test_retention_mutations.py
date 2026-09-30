"""Fail-closed controls for the mutation verifier, independent of coverage."""
import json
import unittest

import retention_mutations as verifier


def transcript(expected):
    endings = {name: "pass" for name in verifier.LEAVES | {verifier.ENGINE, verifier.INTEGRATED, ""}}
    events = []
    if expected:
        endings[""] = endings[verifier.ENGINE] = endings[verifier.INTEGRATED] = "fail"
    for name, message in expected.items():
        endings[name] = "fail"
        events.append(dict(Action="output", Test=name,
                           Output=f"    retention_journey_test.go:99: {message}\n"))
    events.extend(dict(Action=action, **({"Test": name} if name else {}))
                  for name, action in endings.items())
    return "\n".join(json.dumps(dict(Package=verifier.PACKAGE, **e)) for e in events)


class MutationControls(unittest.TestCase):
    def test_candidate_and_both_expected_mutations(self):
        verifier.classify(transcript({}), "", 0, {})
        for _, _, expected in verifier.MUTATIONS.values():
            verifier.classify(transcript(expected), "", 1, expected)

    def test_reject_invalid_proof(self):
        expected = verifier.MUTATIONS["Execute"][2]
        valid = transcript(expected)
        controls = {
            "compile error": (valid, "undefined: broken", 1),
            "wrong exit": (valid, "", 2),
            "unchanged passes": (transcript({}), "", 0),
            "wrong assertion": (valid.replace("failed history lost", "fixture exit: timeout"), "", 1),
            "panic": (valid.replace("failed history lost", "panic: runtime error"), "", 1),
            "missing test": ("\n".join(line for line in valid.splitlines()
                                      if verifier.JOURNEY not in line), "", 1),
            "skipped test": (valid.replace('"Action": "pass"', '"Action": "skip"', 1), "", 1),
            "unrelated failure": (valid.replace('"Action": "pass"', '"Action": "fail"', 1), "", 1),
            "empty output": ("", "", 1),
            "malformed output": ("build failed", "", 1),
            "duplicate results": (valid + "\n" + valid, "", 1),
        }
        for name, (stdout, stderr, code) in controls.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                verifier.classify(stdout, stderr, code, expected)

    def test_each_mutation_changes_only_one_unique_site(self):
        source = "\n".join(item[0] for item in verifier.MUTATIONS.values())
        for branch, (before, after, _) in verifier.MUTATIONS.items():
            changed = verifier.mutate(source, branch)
            self.assertEqual(changed, source.replace(before, after, 1))
            for invalid in ("", source + source):
                with self.assertRaises(ValueError):
                    verifier.mutate(invalid, branch)


if __name__ == "__main__":
    unittest.main()
