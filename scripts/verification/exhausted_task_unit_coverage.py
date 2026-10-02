"""Dedicated isolated/component unit tests, independent of native shell journeys."""
import json
import re
from pathlib import Path
import subprocess
import tempfile

import exhausted_task_review_contract as contract
from exhausted_task_proof import require_coverage
import retention_unit_coverage as coverage

ROOT = contract.ROOT
MANIFEST = ROOT / 'scripts/verification/exhausted-task-unit-tests.json'


def measure(output):
    output.mkdir(parents=True, exist_ok=True)
    for name in ('unit-result.json', 'unit-coverage.out', 'unit-events.jsonl'):
        (output / name).unlink(missing_ok=True)
    scope = json.loads(contract.SCOPE.read_text())
    manifest = json.loads(MANIFEST.read_text())
    required = manifest['tests']
    expected = {p: sorted(set(tests) | set(manifest['subtests'].get(p, []))) for p, tests in required.items()}
    for package in ('pkg/manager', 'internal/cli'):
        path = ROOT / package / ('task_correction_unit_test.go' if package == 'pkg/manager' else 'task_correct_unit_test.go')
        dedicated = set(re.findall(r'^func (Test\w+)\(', path.read_text(), re.M))
        if not dedicated or not dedicated <= set(required.get(package, [])):
            raise ValueError('dedicated unit tests missing from manifest: ' + package)
    with tempfile.TemporaryDirectory(prefix='exhausted-unit-') as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        functions = contract.resolve_scope(scope, helper, temp)
        packages = sorted(set(scope['packages']) | {str(Path(f['path']).parent) for f in functions})
        if any(p not in packages for p in required):
            raise ValueError('unit package not instrumented')
        # Exact names are committed, not inferred from whatever tests remain today.
        pattern = '^(' + '|'.join(sorted({re.escape(t) for tests in required.values() for t in tests})) + ')$'
        command = ['go', 'test', *['./' + p for p in required], '-json', '-count=1', '-timeout=120s',
                   '-run', pattern, '-covermode=atomic', '-coverpkg=' + ','.join('./' + p for p in packages),
                   '-coverprofile=' + str(output / 'unit-coverage.out')]
        with (output / 'unit-events.jsonl').open('w') as log:
            result = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT)
        if result.returncode:
            raise ValueError((output / 'unit-events.jsonl').read_text())
        events = [json.loads(line) for line in (output / 'unit-events.jsonl').read_text().splitlines()]
        contract.check_events(events, [dict(package=p, test=t) for p, tests in expected.items() for t in tests])
        profile = output / 'unit-coverage.out'
        if not profile.read_text().startswith('mode: atomic\n'):
            raise ValueError('dedicated unit run must produce atomic coverage')
        blocks = {p: coverage.expected_blocks(p, temp / 'instrumented.go') for p in {f['path'] for f in functions}}
        filled = temp / 'complete.out'
        contract.fill_missing_blocks(blocks, profile, filled)
        measured = coverage.evaluate(functions, blocks, filled)
        measured.update(command=command, required_tests=expected)
        (output / 'unit-result.json').write_text(json.dumps(measured, indent=2) + '\n')
        print(f"Dedicated unit coverage: {measured['covered']}/{measured['statements']} ({measured['percent']:.4f}%); {len(functions)} complete functions", flush=True)
        require_coverage(measured)
        return measured


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--output', type=Path, default=Path('/tmp/openexec-exhausted-all'))
    measure(parser.parse_args().output.resolve())
