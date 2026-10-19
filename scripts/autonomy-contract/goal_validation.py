"""Candidate-bound preparation checks. No owner decision or delivery effects.

Use --state-dir outside the checkout under the repository runner for one
resumable check per invocation (exit 3 continues, 0 complete, 1 failure).
Without it, execute all checks synchronously; long runs still need the runner.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = 'scripts/autonomy-contract/verify-runtime-evidence.sh'
CHECKS = (
    ('architecture', ['bash', SCRIPT, '--case', 'architecture']),
    ('engine', ['bash', SCRIPT, '--case', 'story-evidence']),
    ('external-consumer', ['python3', '-B', 'scripts/autonomy-contract/external_consumer.py']),
    ('make-test', ['make', 'test']),
    ('make-compat-test', ['make', 'compat-test']),
    ('make-type-check', ['make', 'type-check']),
)


def identity():
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=ROOT):
        raise ValueError('goal-validation requires a clean committed candidate')
    return revision


def handoff():
    text = (ROOT / 'docs/runtime-verification-handoff.md').read_text()
    for term in ('T-US-010-001', 'T-US-010-002', 'launch-and-policy', 'native-stage', 'real-HTTP',
                 'Pending', 'default-branch', 'owner-acceptance', 'goal-validation'):
        if term not in text:
            raise ValueError('handoff missing ' + term)


def step(directory):
    revision = identity()
    handoff()
    directory = directory.resolve()
    if directory == ROOT or ROOT in directory.parents:
        raise ValueError('runner evidence must live outside the candidate')
    directory.mkdir(parents=True, exist_ok=True)
    path = directory / 'results.json'
    report = json.loads(path.read_text()) if path.exists() else {'candidate': revision, 'results': []}
    if report.get('candidate') != revision:
        raise ValueError('runner evidence belongs to another candidate')
    results = report['results']
    for index, result in enumerate(results):
        if index >= len(CHECKS) or result != {'check': CHECKS[index][0], 'command': CHECKS[index][1], 'exit': 0}:
            raise ValueError('failed or malformed prior check; collect failure before retrying')
    if len(results) < len(CHECKS):
        name, command = CHECKS[len(results)]
        env = os.environ.copy()
        env['GOCACHE'] = '/tmp/openexec-exit1-go-cache'
        print(f'candidate {revision}: {name}: {command}', flush=True)
        with (directory / (name + '.log')).open('w') as log:
            result = subprocess.run(command, cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT)
        if identity() != revision:
            raise ValueError('candidate changed during verification')
        results.append({'check': name, 'command': command, 'exit': result.returncode})
        path.write_text(json.dumps(report, indent=2) + '\n')
        if json.loads(path.read_text()) != report:
            raise ValueError('runner report did not round trip')
        print(f'{name}: exit {result.returncode}; log {directory / (name + ".log")}', flush=True)
        if result.returncode:
            return 1
    if len(results) == len(CHECKS):
        print(f'goal-validation: PASS candidate {revision}; preparation only; downstream D1/D2 and owner decision pending')
        return 0
    return 3


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--state-dir', type=Path)
    args = parser.parse_args()
    if args.state_dir:
        return step(args.state_dir)
    directory = Path(tempfile.mkdtemp(prefix='openexec-goal-validation-'))
    print(f'goal-validation evidence: {directory}', flush=True)
    while True:
        code = step(directory)
        if code != 3:
            return code


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        print('goal-validation: FAIL: ' + str(error), file=sys.stderr)
        sys.exit(1)
