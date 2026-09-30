import json
import unittest

import recapture_variants as verifier


class RecaptureVariantsTest(unittest.TestCase):
    def events(self, names):
        return '\n'.join(json.dumps({'Action': 'pass', 'Test': name}) for name in names)

    def test_complete(self):
        self.assertEqual(verifier.validate(0, self.events(verifier.EXPECTED), '')['journeys'], 48)

    def test_missing_duplicate_skip_failure_and_compiler_error(self):
        complete = self.events(verifier.EXPECTED)
        for code, output, stderr in (
            (0, self.events(verifier.EXPECTED - {verifier.TEST + '/lint/empty/success'}), ''),
            (0, complete + '\n' + self.events([verifier.TEST]), ''),
            (0, complete + '\n' + json.dumps({'Action': 'skip'}), ''),
            (0, complete + '\n' + json.dumps({'Action': 'fail'}), ''),
            (1, complete, ''),
            (0, complete, 'compiler error'),
        ):
            with self.subTest(code=code, stderr=stderr, output=output[-60:]):
                with self.assertRaises(ValueError):
                    verifier.validate(code, output, stderr)


if __name__ == '__main__':
    unittest.main()
