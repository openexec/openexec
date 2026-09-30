"""Measure the frozen whole-function scope with fresh Go instrumentation."""
import json
from pathlib import Path
import re
import subprocess
import tempfile

import compact_requirement_evidence as evidence
from retention_unit_coverage import expected_blocks, evaluate, run

ROOT = evidence.ROOT
OUTPUT = ROOT / '.openexec/compact-requirement-checks'


def inventory(helper, manifest):
    functions = []
    for entry in manifest['functions']:
        found = json.loads(run(str(helper), entry['path']))
        name = entry['symbol']
        matches = [f for f in found if re.sub(r'\(\*?(\w+)\)\.', r'\1.', f['name']) == name]
        if len(matches) != 1:
            raise ValueError(f'missing/duplicate function: {name}')
        functions.append(dict(matches[0], path=entry['path'], change='frozen scope'))
    return functions


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    result_file = OUTPUT / 'coverage.json'
    result_file.unlink(missing_ok=True)
    manifest = json.loads(evidence.read_local(evidence.MANIFEST))
    evidence.validate(manifest, evidence.read_local(evidence.EVIDENCE))
    with tempfile.TemporaryDirectory(dir=OUTPUT) as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        functions = inventory(helper, manifest)
        blocks = {p: expected_blocks(p, temp / 'instrumented.go') for p in {f['path'] for f in functions}}
        profile = OUTPUT / 'coverage.out'
        command = ['go', 'test', './internal/planner', './pkg/manager', '-count=1', '-timeout=180s', '-run', 'Plan|Requirement|HumanBoundary|Refin|Goal|Contract',
                   '-json', '-covermode=count', '-coverprofile=' + str(profile)]
        result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True)
        (OUTPUT / 'coverage-tests.log').write_text(result.stdout + result.stderr)
        if result.returncode:
            raise ValueError(result.stdout + result.stderr)
        events = [json.loads(line) for line in result.stdout.splitlines()]
        passed = {e.get('Test') for e in events if e.get('Action') == 'pass'}
        required = {f['symbol'] for f in manifest['fixtures'] if f['path'].startswith(('internal/planner/', 'pkg/manager/'))}
        if required - passed:
            raise ValueError(f'missing required test execution: {required - passed}')
        if any(e.get('Action') in ('fail', 'skip') and e.get('Test', '').split('/')[0] in required for e in events):
            raise ValueError('required test skipped or failed')
        measured = evaluate(functions, blocks, profile)
        measured['baseline'] = manifest['baseline']
        measured['source_sha256'] = evidence.source_digest()
        result_file.write_text(json.dumps(measured, indent=2) + '\n')
        print(json.dumps(measured, indent=2))
        if not measured['passed']:
            raise ValueError('scoped statement coverage must be strictly above 0.90')


if __name__ == '__main__':
    main()
