"""Offline control of the supervisor-supplied, pinned Console source excerpt.

Full api-replay.py identity (not this excerpt's hash):
544bad733f239a063d7f1871e2175fd495ead1d194d3e19d141fdc6dda5b9195
Supervisor source revision: 002e792e92bcc334fccd7dec530fb4cbf66dca6f.
The excerpt ends immediately after the real comparison closure. Appending a
return exposes that closure to tests, without implementing replay or HTTP.
"""
import pathlib
import socket
import unittest
from unittest.mock import patch


source = pathlib.Path(__file__).with_name('console_signature_excerpt.py')
namespace = {}
exec(compile(source.read_text() + '\n    return same\n', str(source), 'exec'), namespace)
signature = namespace['signature']
replay = namespace['replay']

# Invented public error text, not a private capture or reconstructed wire request.
NATIVE = ('server_error: HTTP error: Post "https://example.invalid/v1/chat/completions": '
          'context deadline exceeded (Client.Timeout exceeded while awaiting headers) '
          '(waited 2m0.123s of 2m0s request timeout; request body 12345 bytes)')
PYTHON = 'HTTP error: request timed out after 120.2s'
TARGET_MESSAGE = ('server_error: http error: post "<url> context deadline exceeded '
                  '(client.timeout exceeded while awaiting headers) (waited')


def comparator(error, outcome='failed'):
    # This is replay's real capture validation/target selection/comparison path.
    return replay({'request': {'messages': []}, 'error': error, 'outcome': outcome}, None)


class SignatureControl(unittest.TestCase):
    def test_native_and_python_timeout_are_not_exact_reproduction(self):
        self.assertEqual(signature('failed', NATIVE),
                         {'cls': 'timeout', 'message': TARGET_MESSAGE})
        self.assertEqual(signature('failed', PYTHON),
                         {'cls': 'timeout', 'message': 'http error: request timed out after #.#s'})
        self.assertFalse(comparator(NATIVE)(PYTHON))
        self.assertFalse(comparator(PYTHON)(NATIVE))

    def test_identical_and_existing_normalization_controls(self):
        for error in (NATIVE, PYTHON):
            with self.subTest(error=error):
                self.assertTrue(comparator(error)(error))
        self.assertTrue(comparator(PYTHON)('HTTP error: request timed out after 999.9s'))
        self.assertTrue(comparator(NATIVE)(NATIVE.replace('example.invalid', 'other.invalid')))

    def test_near_misses_need_both_class_and_message(self):
        cases = [
            ('HTTP error: socket timeout', 'timeout'),
            ('HTTP 401: authentication refused', 'http-401'),
            ('HTTP 503: timeout at gateway', 'http-503'),
            ('context canceled', 'error'),
            ('inconclusive:budget_exhausted', 'inconclusive:budget_exhausted'),
        ]
        for error, cls in cases:
            with self.subTest(error=error):
                self.assertEqual(signature('failed', error)['cls'], cls)
                self.assertFalse(comparator(NATIVE)(error))
        # Equal messages still cannot cross the target outcome/class boundary.
        self.assertFalse(comparator(NATIVE, 'inconclusive')(NATIVE))

    def test_real_capture_validation(self):
        for request in (None, {}, {'messages': 'not a list'}):
            with self.subTest(request=request):
                with self.assertRaisesRegex(ValueError, 'no request with messages'):
                    replay({'request': request, 'error': NATIVE}, None)


if __name__ == '__main__':
    # Any accidental network use fails locally, before an endpoint is contacted.
    with patch.object(socket, 'socket', side_effect=AssertionError('network forbidden')), \
         patch.object(socket, 'create_connection', side_effect=AssertionError('network forbidden')):
        unittest.main(verbosity=2)
