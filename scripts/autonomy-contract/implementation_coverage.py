#!/usr/bin/env python3
"""Unit-only statement coverage of complete added/modified implementation bodies."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

from runtime_evidence import validate_events

ROOT = Path(__file__).resolve().parents[2]
BASELINE = '245746baaa66372c4641d137053e330fbc75f679'  # resolved and recorded in the report
PREFIX = 'github.com/openexec/openexec/'
UNITS = {
    'internal/blueprint': ['TestEngine_Execute_SimpleFlow', 'TestEngine_Execute_WithRetry',
        'TestEngine_Execute_MaxRetriesExceeded', 'TestEngine_Execute_ContextCancellation',
        'TestEngineUnitFailureAndSingleStage', 'TestEngine_Callbacks', 'TestEngine_Execute_StageError'],
    'internal/execution/gates': ['TestReceiptUnitMalformedChecks'],
    'internal/pipeline': ['TestAdmittedTerminalBindingAndReceiptReset', 'TestBlueprintUnitInputsAndOutcomes', 'TestBlueprintUnitNativeContext'],
    'pkg/manager': ['TestStartUnitConfigurationAndRefusals'],
    'pkg/runtime': ['TestTerminalBoundary', 'TestTerminalMixedErrorsAndNativeBinding'],
}


def run(command, **kwargs):
    result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True, **kwargs)
    if result.returncode:
        raise RuntimeError(f'{command!r} exited {result.returncode}\n{result.stdout}\n{result.stderr}')
    return result.stdout


def inventory(sources, env):
    return json.loads(run(['go', 'run', './scripts/autonomy-contract/go-function-scope'],
                          input=json.dumps(sources), env=env))


def changed_functions(env):
    baseline = run(['git', 'rev-parse', BASELINE]).strip()
    paths = run(['git', 'diff', '--name-only', baseline, '--', '*.go']).splitlines()
    paths += run(['git', 'ls-files', '--others', '--exclude-standard', '--', '*.go']).splitlines()
    paths = sorted(set(paths))
    paths = [p for p in paths if not p.endswith('_test.go') and p.startswith(('internal/', 'pkg/'))]
    before = {}
    for path in paths:
        old = subprocess.run(['git', 'show', baseline + ':' + path], cwd=ROOT, text=True, capture_output=True)
        if old.returncode == 0:
            before[path] = old.stdout
    old = {(f['file'], f['name']): f['body'] for f in inventory(before, env)}
    current = inventory({p: (ROOT / p).read_text() for p in paths if (ROOT / p).is_file()}, env)
    changed = [f for f in current if old.get((f['file'], f['name'])) != f['body']]
    if not changed:
        raise ValueError('empty implementation scope')
    return baseline, sorted(changed, key=lambda f: (f['file'], f['start']))


def measure(functions, profiles):
    rows = []
    for f in functions:
        blocks = []
        for line in profiles:
            match = re.fullmatch(r'(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)', line)
            if not match:
                raise ValueError('malformed coverage block: ' + line)
            path, start, _, end, _, statements, count = match.groups()
            if path == PREFIX + f['file'] and f['start'] <= int(start) and int(end) <= f['end']:
                blocks.append((int(statements), int(count), int(start), int(end)))
        if not blocks:
            raise ValueError('missing coverage for ' + f['name'])
        total = sum(b[0] for b in blocks)
        covered = sum(b[0] for b in blocks if b[1] > 0)
        rows.append({k: f[k] for k in ('file', 'name', 'start', 'end')} | {
            'covered': covered, 'total': total,
            'uncovered_lines': [[b[2], b[3]] for b in blocks if b[1] == 0 and b[0]]})
    return rows


def enforce(rows):
    covered, total = sum(r['covered'] for r in rows), sum(r['total'] for r in rows)
    if total <= 0 or covered * 100 <= total * 90:
        raise ValueError(f'changed implementation unit coverage must exceed 90%: {covered}/{total}')
    for row in rows:
        if row['covered'] * 100 <= row['total'] * 90:
            raise ValueError(f"function must exceed 90%: {row['name']} {row['covered']}/{row['total']}")


def validate_unit_run(events, package, names, baseline_units=False):
    if any(not isinstance(e, dict) or e.get('Package') != PREFIX + package for e in events):
        raise ValueError('unexpected test package')
    verdicts = [e['Action'] for e in events if not e.get('Test') and e.get('Action') in ('pass', 'fail', 'skip')]
    if verdicts != ['pass']:
        raise ValueError('missing or contradictory package verdict')
    if names or not baseline_units:
        validate_events(events, names)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline-units', action='store_true', help='measure the pre-task unit inventory; still enforce the threshold')
    args = parser.parse_args()
    env = os.environ.copy()
    env['GOCACHE'] = '/tmp/openexec-exit1-go-cache'
    baseline, functions = changed_functions(env)
    profiles, commands = [], []
    with tempfile.TemporaryDirectory(prefix='openexec-unit-coverage-') as temporary:
        for package, names in UNITS.items():
            if args.baseline_units:
                names = [n for n in names if 'Unit' not in n]
            profile = str(Path(temporary) / (package.replace('/', '-') + '.out'))
            command = ['go', 'test', './' + package, '-json', '-count=1', '-timeout=60s',
                       '-run', '^(' + '|'.join(names) + ')$' if names else '^$',
                       '-covermode=count', '-coverpkg=' + ','.join('./' + p for p in UNITS), '-coverprofile=' + profile]
            commands.append(command[:-1] + ['-coverprofile=<temporary>'])
            output = run(command, env=env, timeout=90)
            events = [json.loads(line) for line in output.splitlines()]
            validate_unit_run(events, package, names, args.baseline_units)
            lines = Path(profile).read_text().splitlines()
            if lines[0] != 'mode: count':
                raise ValueError('unexpected coverage mode')
            profiles.extend(lines[1:])
    merged = {}
    for line in profiles:
        block, count = line.rsplit(' ', 1)
        merged[block] = merged.get(block, 0) + int(count)
    rows = measure(functions, [f'{block} {count}' for block, count in merged.items()])
    report = {'baseline': baseline, 'unit_inventory': 'pre-task' if args.baseline_units else 'final', 'commands': commands, 'functions': rows,
              'covered': sum(r['covered'] for r in rows), 'total': sum(r['total'] for r in rows)}
    print(json.dumps(report, indent=2), flush=True)
    enforce(rows)


if __name__ == '__main__':
    main()
