#!/usr/bin/env python3
"""Exercise the empty-phase incident and independently restore each resolver defect."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path('pkg/manager/task_recapture.go')
TEST = 'TestNamedRecaptureIncident'
MUTATIONS = {
    'no_phase_inference': ('\tif phase == "" {\n\t\tphase = checks[0].Gate\n\t}', ''),
    'no_named_fallback': ('\tif command == "" && (phase == "lint" || phase == "test") {\n\t\treturn "", nil\n\t}', ''),
}


def check(root, mutation=False):
    env = os.environ.copy()
    env['GOCACHE'] = '/tmp/openexec-retention-go-cache'
    env['GOWORK'] = 'off'
    selection = '^' + TEST + '$'
    if mutation:
        selection += '/^lint$/^empty$'
    result = subprocess.run(['go', 'test', './pkg/manager', '-json', '-count=1',
                             '-timeout=60s', '-run', selection], cwd=root,
                            env=env, text=True, capture_output=True, timeout=90)
    if result.stderr.strip():
        raise ValueError('unexpected compiler/tool diagnostics: ' + result.stderr)
    events = [json.loads(line) for line in result.stdout.splitlines()]
    expected = {'', TEST, TEST + '/lint/empty'} if mutation else {
        '', TEST,
        *(TEST + '/' + gate + '/' + phase for gate in ('lint', 'test') for phase in ('empty', 'matching'))}
    endings = {}
    for event in events:
        if event.get('Action') == 'skip':
            raise ValueError('required test skipped')
        if event.get('Action') in ('pass', 'fail'):
            name = event.get('Test', '')
            if name in endings:
                raise ValueError('duplicate completion')
            endings[name] = event['Action']
    if endings != dict.fromkeys(expected, 'fail' if mutation else 'pass') or result.returncode != int(mutation):
        raise ValueError('unexpected test outcomes: ' + result.stdout)
    if mutation:
        output = ''.join(e.get('Output', '') for e in events if e.get('Test') == TEST + '/lint/empty')
        if 'named recapture missing: calls=0 status=needs_review recapture_outcome=unresolved' not in output or 'panic:' in result.stdout:
            raise ValueError('mutation failed for wrong reason: ' + output)
    return {'result': 'rejected at unresolved/needs_review with zero executions' if mutation else 'passed',
            'completions': sorted(endings)}


def main():
    files = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    with tempfile.TemporaryDirectory(prefix='named-recapture-') as tmp:
        candidate = Path(tmp) / 'candidate'
        for name in filter(None, files):
            source = ROOT / name
            if source.is_file():
                target = candidate / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
        report = {'baseline': check(candidate)}
        source = candidate / SOURCE
        original = source.read_text()
        for name, (before, after) in MUTATIONS.items():
            if original.count(before) != 1:
                raise ValueError('mutation anchor absent or ambiguous: ' + name)
            source.write_text(original.replace(before, after))
            report[name] = check(candidate, True)
            source.write_text(original)
        print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
