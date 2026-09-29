#!/usr/bin/env python3
"""Compose fresh retention proofs; absent or skipped scenarios fail closed."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

import retention_mutations as mutations

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / 'docs/verification/retention-scenarios.json'
CASES = ('retained-result', 'evidence-boundaries', 'retention-unit-coverage', 'retention-mutations')
MODULE = 'github.com/openexec/openexec/'
os.environ.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'openexec-retention-go-cache'))


def check_events(events, required):
    if not required or len(required) != len(set(required)):
        raise ValueError('empty or duplicate scenario manifest')
    passed = set()
    packages = set()
    for line in events.splitlines():
        event = json.loads(line)
        action = event.get('Action')
        if action in ('fail', 'skip'):
            raise ValueError(f'failed/skipped scenario: {event}')
        if action == 'pass':
            package = event['Package'].removeprefix(MODULE)
            if 'Test' not in event:
                packages.add(package)
                continue
            identity = package + ':' + event['Test']
            if identity in passed:
                raise ValueError(f'duplicate completion: {identity}')
            passed.add(identity)
    if passed != set(required) or packages != {t.split(':')[0] for t in required}:
        raise ValueError(f'scenario mismatch: missing={set(required)-passed}, unexpected={passed-set(required)}')
    return sorted(passed)


def check_mutations(report):
    if set(report) != {'candidate', *mutations.MUTATIONS}:
        raise ValueError('missing or unexpected mutation proof')
    for branch, result in report.items():
        assertions = {} if branch == 'candidate' else mutations.MUTATIONS[branch][2]
        expected = {name: 'pass' for name in mutations.LEAVES | {mutations.ENGINE, mutations.INTEGRATED, ''}}
        if assertions:
            expected.update({name: 'fail' for name in assertions})
            expected[''] = 'fail'
            for parent in (mutations.ENGINE, mutations.INTEGRATED):
                if any(name.startswith(parent + '/') for name in assertions):
                    expected[parent] = 'fail'
        status = 'rejected_at_expected_assertions' if assertions else 'passed'
        if result != dict(status=status, assertions=assertions, tests=expected):
            raise ValueError(f'invalid mutation proof: {branch}')


def run_case(case, output, manifest):
    command = None
    if case in CASES[:2]:
        required = manifest[case]
        packages = sorted({t.split(':')[0] for t in required})
        pattern = '^TestRetainedResult' if case == 'retained-result' else '^TestRetentionBoundaries'
        command = ['go', 'test', *['./' + p for p in packages], '-run', pattern, '-count=1', '-timeout=60s', '-json']
        log = output / (case + '.jsonl')
        with log.open('w') as stream:
            subprocess.run(command, cwd=ROOT, stdout=stream, check=True)
        return dict(command=command, scenarios=check_events(log.read_text(), required))
    if case == 'retention-unit-coverage':
        directory = output / 'coverage'
        command = ['bash', 'scripts/verification/retention-unit-coverage.sh', '--output', str(directory)]
        subprocess.run(command, cwd=ROOT, check=True)
        report = json.loads((directory / 'result.json').read_text())
        if report.get('passed') is not True or not (report['statements'] > 0 and 10 * report['covered'] > 9 * report['statements']):
            raise ValueError('absent or insufficient coverage')
        check_events((directory / 'tests.jsonl').read_text(), manifest[case])
        return dict(command=command, report=report)
    report_path = output / 'mutations.json'
    command = ['bash', 'scripts/verification/retention-mutations.sh', '--output', str(report_path)]
    subprocess.run(command, cwd=ROOT, check=True)
    report = json.loads(report_path.read_text())
    check_mutations(report)
    if sorted(report['candidate']['tests']) != manifest[case]:
        raise ValueError('mutation scenario manifest mismatch')
    return dict(command=command, report=report)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case', required=True, choices=(*CASES, 'retention-story'))
    args = parser.parse_args()
    # A unique directory cannot inherit any previous successful proof.
    output = Path(tempfile.mkdtemp(prefix='openexec-retention-story-'))
    print(f'Retention evidence: {output}', flush=True)
    manifest = json.loads(MANIFEST.read_text())
    if set(manifest) != set(CASES) or any(not value or len(value) != len(set(value)) for value in manifest.values()):
        raise ValueError('missing/duplicate required scenarios or aggregate member')
    selected = CASES if args.case == 'retention-story' else (args.case,)
    results = {}
    for case in selected:
        results[case] = run_case(case, output, manifest)
    (output / 'result.json').write_text(json.dumps(dict(status='passed', cases=results), indent=2) + '\n')
    print(f'{args.case}: PASS; evidence: {output / "result.json"}')


if __name__ == '__main__':
    main()
