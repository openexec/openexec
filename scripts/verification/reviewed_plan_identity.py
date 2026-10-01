"""Reviewed identity whole-function coverage and mandatory lifecycle execution."""
import json
from pathlib import Path
import subprocess
import tempfile

from retention_unit_coverage import expected_blocks, evaluate, run

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / 'scripts/verification/reviewed-plan-identity-scope.json'


def check_execution(events, required):
    if not required or len(required) != len(set(required)):
        raise ValueError('empty or duplicate mandatory tests')
    passed = {e.get('Package', '').removeprefix('github.com/openexec/openexec/') + ':' + e['Test']
              for e in events if e.get('Action') == 'pass' and 'Test' in e}
    if not passed or set(required) - passed:
        raise ValueError('missing lifecycle execution: ' + str(set(required) - passed))
    if any(e.get('Action') in ('skip', 'fail') for e in events):
        raise ValueError('skipped/failed execution')


def check_threshold(functions):
    if not functions or any(f['statements'] <= 0 or 10 * f['covered'] <= 9 * f['statements'] for f in functions):
        raise ValueError('every scoped function must exceed 90% statement coverage')


def main(manifest=None, validate_inventory=None):
    if manifest is None:
        manifest = json.loads(MANIFEST.read_text())
    entries = manifest['functions']
    if not entries or len(entries) != len(set(entries)) or not manifest['tests']:
        raise ValueError('empty or duplicate coverage/lifecycle manifest')
    with tempfile.TemporaryDirectory(prefix='reviewed-identity-') as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        if validate_inventory:
            validate_inventory(helper)
        functions = []
        inventories = {}
        for entry in entries:
            path, name = entry.split(':')
            if path not in inventories:
                inventories[path] = json.loads(run(str(helper), path))
            matches = [f for f in inventories[path] if f['name'] == name]
            if len(matches) != 1:
                raise ValueError('missing or ambiguous function: ' + entry)
            functions.append(dict(matches[0], path=path, change='reviewed identity scope'))
        # Diff the fixed pre-repair baseline to detect newly added/changed functions
        # omitted from the manifest, including untracked production files.
        paths = set(run('git', 'diff', '--name-only', manifest['baseline'], '--', '*.go').splitlines())
        paths.update(run('git', 'ls-files', '--others', '--exclude-standard', '--', '*.go').splitlines())
        for path in sorted(paths):
            if path.endswith('_test.go') or not path.startswith(('internal/', 'pkg/')):
                continue
            old = subprocess.run(['git', 'show', manifest['baseline'] + ':' + path], cwd=ROOT, text=True, capture_output=True)
            previous = {}
            if old.returncode == 0:
                (temp / 'old.go').write_text(old.stdout)
                previous = {f['name']: f['source'] for f in json.loads(run(str(helper), str(temp / 'old.go')))}
            for fn in json.loads(run(str(helper), path)):
                if previous.get(fn['name']) != fn['source'] and path + ':' + fn['name'] not in entries:
                    raise ValueError('changed function missing from manifest: ' + path + ':' + fn['name'])
        blocks = {p: expected_blocks(p, temp / 'instrumented.go') for p in inventories}
        profile = temp / 'coverage.out'
        command = ['go', 'test', './internal/planner', './internal/release', './pkg/manager',
                   '-count=1', '-timeout=120s', '-run', 'TestRemap|TestRewrite|TestNextFree|TestReviewed|TestIdentity|TestPlanReview|TestImportPlan|TestPlanner|TestNativeIdenticalReimport|TestSQLiteStore|TestWait',
                   '-json', '-covermode=count', '-coverpkg=./internal/planner,./internal/release,./pkg/manager', '-coverprofile=' + str(profile)]
        result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True)
        if result.returncode:
            raise ValueError(result.stdout + result.stderr)
        events = [json.loads(line) for line in result.stdout.splitlines()]
        check_execution(events, manifest['tests'])
        measured = evaluate(functions, blocks, profile)
        for fn in measured['functions']:
            percent = 100 * fn['covered'] / fn['statements']
            print(f"{fn['path']}:{fn['name']}: {fn['covered']}/{fn['statements']} ({percent:.2f}%)")
        for name in manifest['tests']:
            print('PASS ' + name)
        check_threshold(measured['functions'])
        print('Reviewed identity verification PASS')


if __name__ == '__main__':
    main()
