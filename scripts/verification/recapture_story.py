#!/usr/bin/env python3
"""Compose fresh recapture proofs and preserve every helper failure."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

import legacy_recapture as legacy
import recapture_boundaries as boundaries
import recapture_compatibility as compatibility
import recapture_unit_coverage as coverage
from retention_story import ROOT, check_events

CASES = ('legacy-recapture', 'recapture-boundaries', 'recapture-unit-coverage', 'recapture-compatibility')
MANIFEST = ROOT / 'docs/verification/recapture-scenarios.json'


def check_manifest(manifest):
    expected = {
        CASES[0]: ['pkg/manager:' + s for s in legacy.REQUIRED],
        CASES[1]: ['pkg/manager:' + s for s in boundaries.REQUIRED],
        CASES[2]: json.loads(coverage.MANIFEST.read_text())['tests'],
        CASES[3]: ['pkg/manager:' + s for s in compatibility.REQUIRED],
    }
    if set(manifest) != set(CASES):
        raise ValueError('missing or unexpected aggregate member')
    for case, required in expected.items():
        actual = manifest[case]
        if not actual or len(actual) != len(set(actual)) or not set(required).issubset(actual):
            raise ValueError(f'incomplete or duplicate scenario manifest: {case}')
        if case != CASES[2] and set(actual) != set(required):
            raise ValueError(f'unexpected scenarios: {case}')


def check_report(case, directory, required):
    report = json.loads((directory / 'result.json').read_text())
    if report.get('passed') is not True:
        raise ValueError('helper did not pass')
    events = (directory / 'tests.jsonl').read_text()
    scenarios = check_events(events, required)
    if case == 'recapture-boundaries':
        proof = boundaries.check_proof(events)
        if report.get('persisted_states') != proof['persisted_states']:
            raise ValueError('missing or inconsistent persisted-state assertions')
    elif case == 'recapture-compatibility':
        compatibility.check_events(events)
    elif case == 'recapture-unit-coverage':
        functions = json.loads((directory / 'scope.json').read_text())
        coverage.check_scope(functions, json.loads(coverage.MANIFEST.read_text()))
        with tempfile.TemporaryDirectory(prefix='recapture-proof-') as tmp:
            blocks = {p: coverage.expected_blocks(p, Path(tmp) / 'instrumented.go')
                      for p in {f['path'] for f in functions}}
            measured = coverage.evaluate(functions, blocks, directory / 'coverage.out')
        if not measured['passed'] or any(report.get(k) != v for k, v in measured.items()):
            raise ValueError('incomplete or insufficient full-body coverage')
        hashes = {p: hashlib.sha256((ROOT / p).read_bytes()).hexdigest() for p in blocks}
        if report.get('source_sha256') != hashes:
            raise ValueError('stale production coverage')
    return dict(scenarios=scenarios, report=report)


def run_case(case, output, required):
    directory = output / case
    command = ['python3', str(ROOT / 'scripts/verification' / (case.replace('-', '_') + '.py')),
               '--output', str(directory)]
    result = dict(passed=False, command=command)
    try:
        with (output / (case + '.log')).open('w') as stream:
            run = subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
        result['exit_code'] = run.returncode
        if run.returncode:
            raise ValueError(f'helper exited {run.returncode}; see {case}.log')
        result.update(check_report(case, directory, required), passed=True)
    except (ValueError, OSError, KeyError, TypeError, AttributeError, subprocess.SubprocessError) as error:
        result['error'] = str(error)
    return result


def execute(selected, output, manifest=None):
    result = dict(passed=False, cases={})
    try:
        if manifest is None:
            manifest = json.loads(MANIFEST.read_text())
        check_manifest(manifest)
        for case in selected:
            result['cases'][case] = run_case(case, output, manifest[case])
        result['passed'] = bool(selected) and all(r['passed'] for r in result['cases'].values())
    except (ValueError, OSError, KeyError, TypeError, AttributeError, subprocess.SubprocessError) as error:
        result['error'] = str(error)
    (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case', required=True, choices=(*CASES, 'recapture-story'))
    args = parser.parse_args()
    output = Path(tempfile.mkdtemp(prefix='openexec-recapture-story-'))
    print(f'Recapture evidence: {output}', flush=True)
    selected = CASES if args.case == 'recapture-story' else (args.case,)
    result = execute(selected, output)
    print(f'{args.case}: {"PASS" if result["passed"] else "FAIL"}; evidence: {output / "result.json"}')
    raise SystemExit(0 if result['passed'] else 1)


if __name__ == '__main__':
    main()
