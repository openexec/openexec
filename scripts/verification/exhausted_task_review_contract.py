#!/usr/bin/env python3
"""Validate US-010 definitions; optionally enforce native test evidence and coverage.

Definition validation is not repair acceptance. --events and --profile consume
fresh native artifacts; missing planned tests or profile blocks cannot pass.
"""
import argparse
import json
from pathlib import Path
import re
import tempfile

import retention_unit_coverage as coverage

ROOT = Path(__file__).resolve().parents[2]
SCOPE = ROOT / 'scripts/exhausted-task-coverage-scope.json'
CASES = ROOT / 'scripts/verification/exhausted-task-review-cases.json'
MODULE = 'github.com/openexec/openexec/'


def validate(scope, contract):
    if scope['missing_coverage'] != 'uncovered' or scope['exclusions']:
        raise ValueError('missing coverage or exclusions weaken denominator')
    if scope['unit'] != 'whole_function_statements' or scope['minimum_percent_exclusive'] != 90:
        raise ValueError('coverage policy changed')
    if scope['changed_function_roots'] != ['internal/', 'pkg/', 'cmd/']:
        raise ValueError('production discovery narrowed')
    if 'internal/cli' not in scope['packages']:
        raise ValueError('CLI omitted')
    findings = contract['findings']
    if {f['id'] for f in findings} != {'F1', 'F2', 'F3', 'F4'} or len(findings) != 4:
        raise ValueError('missing or duplicate finding')
    for finding in findings:
        if finding['disposition'] not in ('accepted', 'rejected') or not finding['evidence'] or not finding['qualification']:
            raise ValueError('unsupported finding disposition')
    cases = contract['cases']
    ids = [c['id'] for c in cases]
    required = set(('candidate_edit candidate_commit candidate_output branch state plan graph binding '
                    'malformed_metadata receipt restart independent_candidate_change concurrency '
                    'reauthorization store_io original_regression public_operator public_legacy '
                    'public_agent_denied public_missing_decision public_decision_binding '
                    'no_plan_success no_plan_failure planned_script_failure planned_success '
                    'unsupported_argv supported_argv empty_obligations boundary_no_authority '
                    'boundary_restart boundary_ordinary_failure boundary_human_reason_privacy '
                    'stop cancellation denied_effects').split())
    if set(ids) != required or len(ids) != len(required):
        raise ValueError('missing or duplicate executable case')
    for case in cases:
        if (case['package'] not in scope['packages'] or not case['test'].startswith('Test')
                or not case['setup'] or not case['assertions'] or not case['negative_control']):
            raise ValueError('case lacks executable mapping or falsifier')
    return cases


def check_events(events, cases):
    expected = {(MODULE + c['package'], c['test']) for c in cases}
    if not expected:
        raise ValueError('empty required native cases')
    runs, passes, packages = set(), set(), set()
    for event in events:
        key = (event.get('Package'), event.get('Test'))
        action = event.get('Action')
        if action in ('fail', 'skip'):
            raise ValueError('failed/skipped native evidence')
        if action == 'run':
            runs.add(key)
        if action == 'pass':
            if event.get('Test'):
                passes.add(key)
            else:
                packages.add(event.get('Package'))
    missing = expected - (runs & passes)
    if missing or not {p for p, _ in expected} <= packages:
        raise ValueError('missing native cases/packages: ' + str(sorted(missing)))


def resolve_scope(scope, helper, temp):
    rows = json.loads((ROOT / scope['inventory']).read_text())['functions']
    identities = {(r['path'], r['symbol']) for r in rows}
    coverage.BASELINE = scope['baseline']
    changed = coverage.scope(helper, temp / 'baseline.go')
    identities.update((f['path'], f['name']) for f in changed)
    for path in scope['production_files']:
        identities.update((path, f['name']) for f in json.loads(coverage.run(str(helper), path)))
    functions = []
    for path in sorted({p for p, _ in identities}):
        declarations = {f['name']: f for f in json.loads(coverage.run(str(helper), path))}
        for _, name in sorted(k for k in identities if k[0] == path):
            if name not in declarations:
                raise ValueError('missing scoped function: ' + path + ':' + name)
            functions.append(dict(declarations[name], path=path, change='review scope union'))
    return functions


def fill_missing_blocks(blocks, profile, output):
    """Keep the source denominator; synthesize zero hits for omitted blocks."""
    lines = profile.read_text().splitlines()
    if not lines or lines[0] not in ('mode: count', 'mode: atomic'):
        raise ValueError('missing count profile')
    present = set()
    for line in lines[1:]:
        match = re.fullmatch(r'(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)', line)
        if not match:
            raise ValueError('malformed profile')
        path, *numbers = match.groups()
        present.add((path.removeprefix(MODULE), *map(int, numbers[:4])))
    for path, body in blocks.items():
        for a, b, c, d, n in body:
            if (path, a, b, c, d) not in present:
                lines.append(f'{MODULE}{path}:{a}.{b},{c}.{d} {n} 0')
    # Statement positions are identical in atomic and count instrumentation.
    # Normalize only for the shared evaluator; retain the original atomic artifact.
    lines[0] = 'mode: count'
    output.write_text('\n'.join(lines) + '\n')


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--events', type=Path, help='native go test -json output; all mapped cases required')
    parser.add_argument('--profile', type=Path, help='native count profile, measured against source denominator')
    args = parser.parse_args()
    scope, contract = json.loads(SCOPE.read_text()), json.loads(CASES.read_text())
    cases = validate(scope, contract)
    with tempfile.TemporaryDirectory(prefix='exhausted-contract-') as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        functions = resolve_scope(scope, helper, temp)
        if args.events:
            check_events([json.loads(line) for line in args.events.read_text().splitlines()], cases)
        if args.profile:
            blocks = {p: coverage.expected_blocks(p, temp / 'instrumented.go') for p in {f['path'] for f in functions}}
            filled = temp / 'complete.out'
            fill_missing_blocks(blocks, args.profile, filled)
            result = coverage.evaluate(functions, blocks, filled)
            print(json.dumps(result))
            if not result['passed']:
                raise ValueError('whole-function coverage must exceed 90%; missing blocks counted uncovered')
    print(f'Definition PASS: {len(cases)} mapped cases; {len(functions)} whole functions. Repair acceptance remains unverified until all journeys and negative controls run.')


if __name__ == '__main__':
    main()
