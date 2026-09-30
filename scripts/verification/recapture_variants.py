#!/usr/bin/env python3
"""Require every named-check legacy/restart journey, with no skips."""
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
TEST = 'TestNamedRecaptureVariants'
MODES = ('success', 'existing_evidence', 'historical_command', 'budget_fresh',
         'budget_remaining', 'budget_spent', 'ambiguous', 'mismatch', 'hitl',
         'review', 'cancellation', 'launch_refusal')
EXPECTED = {'', TEST} | {
    f'{TEST}/{gate}/{phase}/{mode}'
    for gate in ('lint', 'test') for phase in ('empty', 'matching') for mode in MODES
}


def validate(returncode, stdout, stderr):
    if returncode or stderr.strip():
        raise ValueError(f'Go check failed ({returncode}):\n{stdout}\n{stderr}')
    endings = set()
    for event in map(json.loads, stdout.splitlines()):
        if event.get('Action') in ('fail', 'skip'):
            raise ValueError('failed or skipped required journey')
        if event.get('Action') == 'pass':
            name = event.get('Test', '')
            if name in endings:
                raise ValueError('duplicate completion')
            endings.add(name)
    if endings != EXPECTED:
        raise ValueError(f'unexpected completion set: {endings ^ EXPECTED}')
    return {'result': 'passed', 'journeys': len(EXPECTED) - 2,
            'completions': sorted(endings)}


def main():
    env = os.environ.copy()
    env['GOCACHE'] = '/tmp/openexec-retention-go-cache'
    result = subprocess.run(['go', 'test', './pkg/manager', '-json', '-count=1',
                             '-timeout=60s', '-run', '^' + TEST + '$'],
                            cwd=ROOT, env=env, text=True, capture_output=True,
                            timeout=90)
    print(json.dumps(validate(result.returncode, result.stdout, result.stderr), indent=2))


if __name__ == '__main__':
    main()
