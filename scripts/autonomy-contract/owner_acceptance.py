"""Consume an authenticated Console decision; never create or persist decisions.

Console must supply the trusted loader argv and expected PR/owner out of band.
The loader must authenticate the actor and immutable decision provenance before
returning JSON. Repository files and worker-supplied `authorized` flags are not
an authentication source. No loader is configured by this repository.
"""
import json
import os
import re
import subprocess
import sys


def verify(candidate, pr, owner, load):
    if not re.fullmatch(r'[0-9a-f]{40}', candidate) or not pr or not owner:
        raise ValueError('missing exact candidate, PR or authorized owner')
    if load is None:
        raise ValueError('authenticated Console decision loader absent')
    record = load()
    if not isinstance(record, dict):
        raise ValueError('owner decision absent or malformed')
    expected = {'candidate': candidate, 'pull_request': pr, 'actor': owner,
                'goal': 'G-006', 'task': 'T-US-010-002', 'decision': 'accept-merge'}
    for key, value in expected.items():
        if record.get(key) != value:
            raise ValueError('owner decision mismatch: ' + key)
    if not isinstance(record.get('decision_ref'), str) or not record['decision_ref'].strip():
        raise ValueError('authentic decision reference absent')
    return record['decision_ref']


def main():
    # These values are trusted caller configuration, never inferred from a PR,
    # worker output, task completion or the candidate's own assertions.
    argv = json.loads(os.environ.get('OPENEXEC_OWNER_DECISION_LOADER', 'null'))
    if not isinstance(argv, list) or not argv or not all(isinstance(x, str) and x for x in argv):
        raise ValueError('authenticated Console decision loader absent')
    candidate = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    def load():
        return json.loads(subprocess.check_output(argv, text=True, timeout=30))
    verify(candidate, os.environ.get('OPENEXEC_OWNER_PR', ''),
           os.environ.get('OPENEXEC_OWNER_ID', ''), load)
    print('owner-acceptance: PASS (authenticated decision matched; not merge or downstream evidence)')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print('owner-acceptance: FAIL: ' + str(error), file=sys.stderr)
        sys.exit(1)
