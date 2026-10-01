"""Reviewed identity whole-function coverage and mandatory lifecycle execution."""
import json
from pathlib import Path
import subprocess
import tempfile

from retention_unit_coverage import expected_blocks, evaluate, run

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / 'scripts/verification/reviewed-plan-identity-scope.json'


def main():
    manifest = json.loads(MANIFEST.read_text())
    entries = manifest['functions']
    if not entries or len(entries) != len(set(entries)) or not manifest['tests']:
        raise ValueError('empty or duplicate coverage/lifecycle manifest')
    with tempfile.TemporaryDirectory(prefix='reviewed-identity-') as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
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
                   '-count=1', '-timeout=120s', '-run', 'TestRemap|TestRewrite|TestNextFree|TestReviewed|TestIdentity|TestPlanReview|TestImportPlan|TestPlanner',
                   '-json', '-covermode=count', '-coverpkg=./internal/planner,./internal/release,./pkg/manager', '-coverprofile=' + str(profile)]
        result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True)
        if result.returncode:
            raise ValueError(result.stdout + result.stderr)
        events = [json.loads(line) for line in result.stdout.splitlines()]
        passed = {e.get('Package', '').removeprefix('github.com/openexec/openexec/') + ':' + e.get('Test', '') for e in events if e.get('Action') == 'pass'}
        if set(manifest['tests']) - passed:
            raise ValueError('missing lifecycle execution: ' + str(set(manifest['tests']) - passed))
        for e in events:
            if e.get('Action') in ('skip', 'fail'):
                raise ValueError('skipped/failed execution: ' + str(e))
        measured = evaluate(functions, blocks, profile)
        for fn in measured['functions']:
            percent = 100 * fn['covered'] / fn['statements']
            print(f"{fn['path']}:{fn['name']}: {fn['covered']}/{fn['statements']} ({percent:.2f}%)")
        for name in manifest['tests']:
            print('PASS ' + name)
        if any(10 * f['covered'] <= 9 * f['statements'] for f in measured['functions']):
            raise ValueError('every scoped function must exceed 90% statement coverage')
        print('Reviewed identity verification PASS')


if __name__ == '__main__':
    main()
