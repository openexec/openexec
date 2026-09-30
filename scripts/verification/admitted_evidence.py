#!/usr/bin/env python3
"""Verify public silent evidence and independently falsify all three loss paths."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

import retention_mutations as retained

ROOT = Path(__file__).resolve().parents[2]
RUNTIME = Path('pkg/runtime/execution.go')
ATTACH = 'return gates.CommandFailureWithEvidence(ctx, name, err, hash, path)'
PIPELINE = 'github.com/openexec/openexec/internal/pipeline'
MANAGER = 'github.com/openexec/openexec/pkg/manager'
EVENT = 'TestPublicSilentFailureEvent'
REPAIR = 'TestPublicSilentFailureReloadAndRepair'
CLASSIFY = 'TestPublicEvidenceFailureClassification'
EXPECTED = {
    (PIPELINE, EVENT + '/' + gate): 'terminal event lost wrapper evidence attachment'
    for gate in ('lint', 'test')
} | {
    (MANAGER, REPAIR + '/' + gate): 'silent receipt lost attached evidence or became diagnostic-free'
    for gate in ('lint', 'test')
} | {(PIPELINE, CLASSIFY + '/failure'): 'wrapper lost reference or changed classification digest'}


def check(root, mutated=False):
    env = os.environ.copy()
    env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'openexec-retention-go-cache'))
    env['GOWORK'] = 'off'
    result = subprocess.run([
        'go', 'test', './internal/pipeline', './pkg/manager', '-json', '-count=1',
        '-timeout=60s', '-run', '^TestPublic(SilentFailure(Event|ReloadAndRepair)|EvidenceFailureClassification)$'
    ], cwd=root, env=env, capture_output=True, text=True, timeout=120)
    if result.stderr.strip():
        raise ValueError('compiler/tool diagnostics: ' + result.stderr)
    events = [json.loads(line) for line in result.stdout.splitlines()]
    expected = {(p, name): 'pass' for p, name in [
        (PIPELINE, ''), (MANAGER, ''), (PIPELINE, EVENT), (MANAGER, REPAIR),
        (PIPELINE, CLASSIFY),
        *((PIPELINE, EVENT + '/' + gate) for gate in ('lint', 'test')),
        *((MANAGER, REPAIR + '/' + gate) for gate in ('lint', 'test')),
        *((PIPELINE, CLASSIFY + '/' + case) for case in
          ('failure', 'success', 'cancelled', 'transport', 'launch', 'reserved-exit', 'mixed')),
    ]}
    if mutated:
        for key in EXPECTED:
            expected[key] = 'fail'
            expected[key[0], key[1].split('/')[0]] = 'fail'
            expected[key[0], ''] = 'fail'
    endings = {}
    for event in events:
        key = event.get('Package'), event.get('Test', '')
        if event.get('Action') == 'skip':
            raise ValueError('skipped required test')
        if event.get('Action') in ('pass', 'fail'):
            if key in endings:
                raise ValueError('duplicate test completion')
            endings[key] = event['Action']
    if endings != expected or result.returncode != int(mutated):
        raise ValueError('unexpected outcomes: ' + result.stdout)
    if mutated:
        for key, message in EXPECTED.items():
            output = ''.join(e.get('Output', '') for e in events
                             if (e.get('Package'), e.get('Test', '')) == key)
            if message not in output or 'panic:' in output or (key[0] == MANAGER and 'diagnosticFree=true' not in output):
                raise ValueError('wrong assertion: ' + output)
    return {'status': 'rejected_at_expected_assertions' if mutated else 'passed',
            'completed': len(endings),
            'assertions': list(EXPECTED.values()) if mutated else []}



def boundaries(root):
    env = os.environ.copy()
    env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'openexec-retention-go-cache'))
    env['GOWORK'] = 'off'
    names = {
        'github.com/openexec/openexec/internal/execution/evidence': ['TestDiagnosticTailCapture'],
        PIPELINE: ['TestPublicDiagnosticFailureEvent', 'TestPublicDiagnosticFailureEvent/nil=false',
                   'TestPublicDiagnosticFailureEvent/nil=true', 'TestPublicNilResultRefusals',
                   *('TestPublicNilResultRefusals/' + c for c in ('launch', 'cancelled', 'transport'))],
        MANAGER: ['TestPublicDiagnosticFailureReloadAndRepair',
                  *('TestPublicDiagnosticFailureReloadAndRepair/' + g + '/nil=' + n
                    for g in ('lint', 'test') for n in ('false', 'true'))],
    }
    result = subprocess.run(['go', 'test', './internal/execution/evidence', './internal/pipeline',
        './pkg/manager', '-json', '-count=1', '-timeout=60s', '-run',
        '^Test(DiagnosticTailCapture|PublicDiagnosticFailure(Event|ReloadAndRepair)|PublicNilResultRefusals)$'],
        cwd=root, env=env, capture_output=True, text=True, timeout=120)
    expected = {(p, n) for p, tests in names.items() for n in ['', *tests]}
    passed = set()
    for line in result.stdout.splitlines():
        event = json.loads(line)
        if event.get('Action') in ('fail', 'skip'):
            raise ValueError(result.stdout)
        if event.get('Action') == 'pass':
            key = event.get('Package'), event.get('Test', '')
            if key in passed:
                raise ValueError('duplicate boundary completion')
            passed.add(key)
    if result.returncode or result.stderr.strip() or passed != expected:
        raise ValueError('boundary verification failed: ' + result.stdout + result.stderr)
    return {'status': 'passed', 'completed': len(passed)}


def tail_check(root, assertion=None):
    env = os.environ.copy()
    env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'openexec-retention-go-cache'))
    env['GOWORK'] = 'off'
    result = subprocess.run(['go', 'test', './internal/execution/evidence', '-json', '-count=1',
        '-timeout=60s', '-run', '^TestDiagnosticTailCapture$'], cwd=root, env=env,
        capture_output=True, text=True, timeout=120)
    events = [json.loads(line) for line in result.stdout.splitlines()]
    status = 'fail' if assertion else 'pass'
    endings = [(e.get('Test', ''), e['Action']) for e in events if e.get('Action') in ('pass', 'fail', 'skip')]
    if result.stderr.strip() or result.returncode != int(bool(assertion)) or endings != [('TestDiagnosticTailCapture', status), ('', status)]:
        raise ValueError('unexpected tail-control outcome: ' + result.stdout + result.stderr)
    output = ''.join(e.get('Output', '') for e in events if e.get('Test') == 'TestDiagnosticTailCapture')
    if assertion and (assertion not in output or 'panic:' in output):
        raise ValueError('wrong tail assertion: ' + output)
    return dict(status='rejected_at_expected_assertions' if assertion else 'passed', assertion=assertion)


def tail_mutations(baseline, temp):
    path = Path('internal/execution/evidence/capture.go')
    source = (baseline / path).read_text()
    start = source.index('func (b *Buffer) Write(p []byte) (int, error) {')
    end = source.index('\n// Command is private:', start)
    prefix_only = """func (b *Buffer) Write(p []byte) (int, error) {
        n := len(p)
        remaining := StreamLimit - len(b.data)
        if n > remaining { b.Truncated = true; p = p[:remaining] }
        b.data = append(b.data, p...)
        return n, nil
    }
"""
    public = 'return Public(head+"[truncated]"+tail, secrets)'
    if source.count(public) != 1:
        raise ValueError('expected exactly one public tail site')
    variants = {
        'prefix_capture': (source[:start] + prefix_only + source[end:], 'lost prefix/tail'),
        'prefix_public': (source.replace(public, 'return Public(head+"[truncated]", secrets)', 1), 'unsafe or missing public tail'),
    }
    report = {'baseline': tail_check(baseline)}
    for name, (mutant, assertion) in variants.items():
        clone = temp / name
        shutil.copytree(baseline, clone)
        (clone / path).write_text(mutant)
        report[name] = tail_check(clone, assertion)
    if (ROOT / path).read_text() != source:
        raise ValueError('candidate capture changed during verification')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if args.output:
        args.output.unlink(missing_ok=True)
    files = subprocess.check_output(
        ['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=ROOT
    ).decode().split('\0')
    with tempfile.TemporaryDirectory(prefix='admitted-evidence-') as temp:
        baseline = Path(temp) / 'candidate'
        for name in filter(None, files):
            source = ROOT / name
            if source.is_file():
                target = baseline / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
        report = {'tail_controls': tail_mutations(baseline, Path(temp)), 'diagnostic_boundaries': boundaries(baseline), 'public_baseline': check(baseline), 'engine_baseline': retained.check(baseline, {})}
        for branch, (_, _, expected) in retained.MUTATIONS.items():
            clone = Path(temp) / branch
            shutil.copytree(baseline, clone)
            engine = clone / 'internal/blueprint/engine.go'
            engine.write_text(retained.mutate(engine.read_text(), branch))
            report[branch] = retained.check(clone, expected)
        clone = Path(temp) / 'wrapper'
        shutil.copytree(baseline, clone)
        source = (clone / RUNTIME).read_text()
        if source.count(ATTACH) != 1:
            raise ValueError('expected exactly one wrapper attachment site')
        (clone / RUNTIME).write_text(source.replace(ATTACH, 'return gates.NewCommandFailure(ctx, name, err)', 1))
        report['wrapper'] = check(clone, mutated=True)
        if (ROOT / RUNTIME).read_text() != source:
            raise ValueError('candidate wrapper changed during verification')
    output = json.dumps(report, indent=2) + '\n'
    if args.output:
        args.output.write_text(output)
    print(output, end='')


if __name__ == '__main__':
    main()
