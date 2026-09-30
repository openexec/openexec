"""Discover recovery tests and enforce complete changed-function statement coverage."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

import retention_unit_coverage as coverage

ROOT = coverage.ROOT
BASELINE = 'c13c3ec4c035a0febd724a28825dfe7dfe2c74f7'
PACKAGES = ['internal/planner', 'pkg/runtime', 'pkg/manager']
MANIFEST = ROOT / 'docs/verification/planner-schema-coverage-scope.json'
SELECTION = 'Schema|ReviewedPlan|Planner|PlanReplay|Refin|Compact|Requirement|HumanBoundary|Goal|Contract'


def inventory(helper, temp, manifest):
    changed = set(coverage.run('git', 'diff', '--name-only', BASELINE, '--', '*.go').splitlines())
    changed.update(coverage.run('git', 'ls-files', '--others', '--exclude-standard', '--', '*.go').splitlines())
    functions, removed = [], []
    for path in sorted(changed):
        if path.endswith('_test.go'):
            continue
        old = subprocess.run(['git', 'show', f'{BASELINE}:{path}'], cwd=ROOT, text=True, capture_output=True)
        previous = {}
        if old.returncode == 0:
            temp.write_text(old.stdout)
            previous = {f['name']: f for f in json.loads(coverage.run(str(helper), str(temp)))}
        current = json.loads(coverage.run(str(helper), path)) if (ROOT / path).exists() else []
        for fn in current:
            before = previous.pop(fn['name'], None)
            if before is None or before['source'] != fn['source']:
                functions.append(dict(fn, path=path, change='added' if before is None else 'modified'))
        removed.extend(path + ':' + name for name in previous)
    identities = [f['path'] + ':' + f['name'] for f in functions]
    if sorted(identities) != sorted(manifest['functions']) or sorted(removed) != sorted(manifest['removed']):
        raise ValueError(f'changed function scope mismatch: {identities}; removed={removed}')
    return functions


def main():
    os.environ.update(GOWORK='off', GOENV='off', GOFLAGS='', GOCACHE='/tmp/openexec-compact-go-cache')
    output = ROOT / '.openexec/planner-schema-checks'
    output.mkdir(parents=True, exist_ok=True)
    result_path = output / 'coverage.json'
    result_path.unlink(missing_ok=True)
    subprocess.run(['python3', '-m', 'unittest', 'discover', '-s', 'scripts/verification', '-p', 'planner_schema_recovery_test.py'], cwd=ROOT, check=True)
    manifest = json.loads(MANIFEST.read_text())
    with tempfile.TemporaryDirectory() as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        functions = inventory(helper, temp / 'baseline.go', manifest)
        required = set(manifest['tests'])
        if not required or len(required) != len(manifest['tests']):
            raise ValueError('empty or duplicate test manifest')
        for package in PACKAGES:
            dedicated = json.loads(coverage.run(str(helper), package + '/schema_recovery_test.go' if package == 'internal/planner' else package + '/planner_schema_recovery_test.go'))
            names = {package + ':' + f['name'] for f in dedicated if f['name'].startswith('Test')}
            if not names or not names <= required:
                raise ValueError(f'undocumented dedicated tests: {names - required}')
            discovered = coverage.run('go', 'test', './' + package, '-list', SELECTION)
            for test in required:
                pkg, name = test.split(':', 1)
                if pkg == package and discovered.splitlines().count(name.split('/')[0]) != 1:
                    raise ValueError(f'test not discovered exactly once: {test}')
        blocks = {p: coverage.expected_blocks(p, temp / 'instrumented.go') for p in {f['path'] for f in functions}}
        profile = output / 'coverage.out'
        command = ['go', 'test', *['./' + p for p in PACKAGES], '-count=1', '-timeout=180s', '-run', SELECTION, '-json', '-covermode=count', '-coverprofile=' + str(profile)]
        with (output / 'tests.jsonl').open('w') as log:
            subprocess.run(command, cwd=ROOT, stdout=log, check=True)
        coverage.PACKAGES = PACKAGES
        coverage.check_tests((output / 'tests.jsonl').read_text(), required)
        coverage.BASELINE = BASELINE
        result = coverage.evaluate(functions, blocks, profile)
        result_path.write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps(result, indent=2))
        if not result['passed']:
            raise ValueError('complete scoped statement coverage must exceed 90%')


if __name__ == '__main__':
    main()
