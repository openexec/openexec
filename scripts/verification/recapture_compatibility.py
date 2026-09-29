#!/usr/bin/env python3
"""Fresh protected-format recapture journeys; no unit/boundary report inputs."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PREFIX = 'TestRecaptureCompatibility'
FORMATS = ('openexec', 'uaos', 'tasks_json')
REQUIRED = {PREFIX} | {f'{PREFIX}/{fmt}' for fmt in FORMATS} | {
    f'{PREFIX}/{fmt}/{case}' for fmt in FORMATS
    for case in ('failure', 'success', 'unresolved')}
PACKAGE = 'github.com/openexec/openexec/pkg/manager'


def check_events(events):
    passed = set()
    package_passed = False
    for line in events.splitlines():
        event = json.loads(line)
        if event.get('Package') != PACKAGE:
            raise ValueError('unexpected package')
        action = event.get('Action')
        if action in ('skip', 'fail'):
            raise ValueError(f'failed or skipped scenario: {event}')
        if action == 'pass':
            name = event.get('Test')
            if name is None:
                if package_passed:
                    raise ValueError('duplicate package completion')
                package_passed = True
            else:
                if name in passed:
                    raise ValueError('duplicate scenario completion')
                passed.add(name)
    if passed != REQUIRED or not package_passed:
        raise ValueError(f'incomplete proof: missing={REQUIRED-passed}, unexpected={passed-REQUIRED}')
    return sorted(passed)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, help='new evidence directory, must not exist')
    args = parser.parse_args()
    output = args.output.resolve() if args.output else Path(tempfile.mkdtemp(prefix='openexec-recapture-compatibility-'))
    if args.output:
        output.mkdir(parents=True, exist_ok=False)
    print(f'Evidence: {output}', flush=True)
    env = os.environ.copy()
    env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'openexec-retention-go-cache'))
    temporary = output / 'tmp'
    temporary.mkdir()
    env['TMPDIR'] = str(temporary)
    command = ['go', 'test', './pkg/manager', '-run', '^TestRecaptureCompatibility$',
               '-count=1', '-timeout=60s', '-json']
    result = dict(passed=False, command=command)
    try:
        with (output / 'tests.jsonl').open('w') as stream, (output / 'stderr.log').open('w') as errors:
            run = subprocess.run(command, cwd=ROOT, env=env, stdout=stream, stderr=errors, timeout=110)
        result['exit_code'] = run.returncode
        if run.returncode:
            raise ValueError(f'go test exited {run.returncode}; see tests.jsonl and stderr.log')
        result['scenarios'] = check_events((output / 'tests.jsonl').read_text())
        result['passed'] = True
    except (ValueError, subprocess.TimeoutExpired, OSError) as error:
        result['error'] = str(error)
    (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    if not result['passed']:
        raise SystemExit(result['error'])
    print(f'recapture-compatibility: PASS; evidence: {output / "result.json"}')


if __name__ == '__main__':
    main()
