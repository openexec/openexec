"""Validate discovery evidence; this does not claim runtime repair coverage."""
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = 'scripts/verification/plan-identity-discovery.json'
DOCUMENT = 'docs/verification/plan-identity-discovery.md'


def run(*args, cwd=ROOT):
    return subprocess.check_output(args, cwd=cwd, text=True)


def validate(root, manifest, helper):
    def local(path):
        target = (root / path).resolve()
        if not target.is_relative_to(root.resolve()) or not target.is_file():
            raise ValueError('missing or nonlocal reference: ' + path)
        return target

    if manifest['task'] != 'T-US-010-002' or manifest['story'] != 'US-010':
        raise ValueError('wrong discovery identity')
    if manifest['coverage'] != {'metric': 'statements', 'operator': '>', 'percent': 90,
                                'scope': 'whole functions including closures'}:
        raise ValueError('coverage must exceed 90% for whole functions')
    if manifest['identities'] != ['US-001', 'US-005', 'T-US-001-001']:
        raise ValueError('literal fixture identities changed')
    if len(manifest['fixtures']) != 2:
        raise ValueError('two local fixtures required')
    retained, subsequent = [json.loads(local(p).read_text()) for p in manifest['fixtures']]
    first = {s['id']: s for s in retained['stories']}
    second = {s['id']: s for s in subsequent['stories']}
    for identity in ('US-001', 'US-005'):
        a, b = first[identity], second[identity]
        if a['title'] != b['title'] or a['description'] == b['description'] or a['goal_id'] == b['goal_id'] or a['tasks'] == b['tasks']:
            raise ValueError('changed-content fixture semantics lost')
        if a.get('depends_on') or a['tasks'][0].get('depends_on'):
            raise ValueError('native empty dependency fixture lost')
    if first['US-001']['tasks'][0]['id'] != 'T-US-001-001':
        raise ValueError('literal task fixture lost')
    if second['US-005']['depends_on'] != ['US-001'] or second['US-005']['tasks'][0]['depends_on'] != ['T-US-001-002']:
        raise ValueError('reference rewrite fixture lost')

    base = json.loads(local('scripts/verification/reviewed-plan-identity-scope.json').read_text())
    mandatory = set(base['functions']) | {
        'pkg/manager/planner.go:(*Manager).importBoundPlan',
        'internal/release/sqlite_store.go:(*SQLiteStore).getStoryInternal',
        'internal/release/sqlite_store.go:(*SQLiteStore).getTaskInternal'}
    if not mandatory.issubset(manifest['functions']) or len(set(manifest['functions'])) != len(manifest['functions']):
        raise ValueError('missing or duplicate identity/normalization inventory')
    if not set(base['tests']).issubset(manifest['existing_tests']):
        raise ValueError('retained regression inventory lost')
    inventories = {}
    sources = {}
    for entry in manifest['functions'] + manifest['helpers'] + manifest['existing_tests'] + manifest['entry_points']:
        path, name = entry.split(':')
        if not path.endswith('.go'):  # package:test from the retained execution manifest
            matches = []
            for testfile in (root / path).glob('*_test.go'):
                if testfile not in inventories:
                    inventories[testfile] = json.loads(run(str(helper), str(testfile)))
                matches += [f for f in inventories[testfile] if f['name'] == name]
        else:
            testfile = local(path)
            if testfile not in inventories:
                inventories[testfile] = json.loads(run(str(helper), str(testfile)))
            matches = [f for f in inventories[testfile] if f['name'] == name]
        if len(matches) != 1:
            raise ValueError('missing or ambiguous source declaration: ' + entry)
        sources[entry] = matches[0]['source']
    required_anchors = {'st := &release.Story{', 'task := &release.Task{',
                        'insert := func(', 'if jsonFields[column] {', 'array := func('}
    anchors = set()
    for block in manifest['blocks']:
        if block['function'] not in manifest['functions'] or not block['purpose']:
            raise ValueError('unscoped source block')
        for anchor in block['anchors']:
            if anchor not in sources[block['function']]:
                raise ValueError('missing source block: ' + anchor)
            anchors.add(anchor)
    if not required_anchors.issubset(anchors):
        raise ValueError('native storage or SQL predicate block omitted')
    document = local(DOCUMENT).read_text()
    required_sections = {'Fixtures and public boundaries', 'Native identical replay',
                         'Legacy JSON null', 'Schema-supported SQL NULL',
                         'Changed-content allocation', 'Concurrent conflict refusal',
                         'Persisted snapshots and receipts', 'Mutation expectations',
                         'Coverage inventory', 'Console follow-up and delivery boundary', 'Verification'}
    if set(manifest['required_sections']) != required_sections:
        raise ValueError('required case inventory changed')
    for heading in required_sections:
        if document.count('## ' + heading + '\n') != 1:
            raise ValueError('missing or duplicate case evidence: ' + heading)
    for literal in manifest['identities'] + manifest['fixtures'] + ['strictly greater than 90%', 'Agent Console owns', 'close/reopen', 'rollback-only']:
        if literal not in document:
            raise ValueError('missing discovery evidence: ' + literal)
    return sources


def changed_functions(manifest, helper):
    """Do not let new production identity changes escape the discovery inventory."""
    paths = set(run('git', 'diff', '--name-only', manifest['baseline'], '--', '*.go').splitlines())
    paths.update(run('git', 'ls-files', '--others', '--exclude-standard', '--', '*.go').splitlines())
    with tempfile.TemporaryDirectory(prefix='identity-baseline-') as directory:
        oldpath = Path(directory) / 'old.go'
        for path in sorted(paths):
            if path.endswith('_test.go') or not path.startswith(('internal/', 'pkg/')):
                continue
            old = subprocess.run(['git', 'show', manifest['baseline'] + ':' + path], cwd=ROOT, text=True, capture_output=True)
            previous = {}
            if old.returncode == 0:
                oldpath.write_text(old.stdout)
                previous = {f['name']: f['source'] for f in json.loads(run(str(helper), str(oldpath)))}
            for fn in json.loads(run(str(helper), path)):
                if previous.get(fn['name']) != fn['source'] and path + ':' + fn['name'] not in manifest['functions']:
                    raise ValueError('changed function omitted: ' + path + ':' + fn['name'])


def main():
    manifest = json.loads((ROOT / MANIFEST).read_text())
    with tempfile.TemporaryDirectory(prefix='plan-identity-discovery-') as directory:
        helper = Path(directory) / 'inventory'
        run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        validate(ROOT, manifest, helper)
        changed_functions(manifest, helper)
    print(f"PASS T-US-010-002: 2 local fixtures, {len(manifest['functions'])} functions, native/SQL source blocks and 11 evidence sections; required statement coverage >90% (not measured by discovery)")


if __name__ == '__main__':
    main()
