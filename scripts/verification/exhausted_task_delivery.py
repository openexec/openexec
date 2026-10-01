"""Read-only, candidate-bound preparation evidence; no delivery authority."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
RECORD = 'docs/exhausted-task-delivery.json'
RESULTS = 'docs/verification/exhausted-task-delivery-results/'
BRANCH = 'outcome/ad5a70cd0924dae18de345dbdf74e623'
BASE = 'dbfec11b6934dec8b1fa72c50cc6f871f44043a6'
IMPLEMENTATION_BASE = 'ca2bdf8c254cfa95d7140064b177f54e7529a763'
CHECKS = {
    'lint': 'run_declared_check lint (make lint)',
    'test': 'run_declared_check test (make test)',
    'acceptance': 'scripts/verify-exhausted-task-reconciliation.sh all',
    'reload': 'scripts/verify-exhausted-task-reconciliation.sh exhaustion-controls',
    'compatibility': 'make compat-test',
    'types': 'make type-check',
}


def require(ok, message):
    if not ok:
        raise ValueError(message)


def git(root, *args):
    return subprocess.check_output(['git', *args], cwd=root).decode().strip()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def local(root, name):
    require(isinstance(name, str) and name and not Path(name).is_absolute(), 'nonlocal reference')
    path = root / name
    require('..' not in Path(name).parts and path.resolve().is_relative_to(root.resolve())
            and path.is_file(), 'missing/nonlocal evidence: ' + name)
    return path


def content(root):
    # Union includes new, unstaged and staged files; deleted paths are absent.
    names = git(root, 'ls-files', '-z', '--cached', '--others', '--exclude-standard').split('\0')
    rows = []
    for name in sorted(set(names)):
        if (not name or name in (RECORD, 'docs/exhausted-task-delivery.md', 'NOTES.md')
                or name.startswith((RESULTS, 'bin/', 'ui/dist/', 'ui/coverage/', '.gocache/'))
                or '__pycache__' in Path(name).parts):
            continue
        path = root / name
        if not path.exists() and not path.is_symlink():
            continue
        if path.is_symlink():
            data, mode = os.readlink(path).encode(), '120000'
        else:
            require(path.is_file(), 'unsupported source entry: ' + name)
            data, mode = path.read_bytes(), '100755' if path.stat().st_mode & 0o111 else '100644'
        rows.append({'path': name, 'mode': mode, 'sha256': sha(data)})
    require(rows, 'empty source manifest')
    digest = sha(json.dumps(rows, sort_keys=True, separators=(',', ':')).encode())
    return rows, digest


def read(path):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, 'duplicate JSON key')
            result[key] = value
        return result
    return json.loads(path.read_text(), object_pairs_hook=unique)


def validate(root, record):
    require(record['schema'] == 1 and record['task'] == 'T-US-011-004', 'wrong task/schema')
    require(record['branch'] == BRANCH == git(root, 'branch', '--show-current'), 'stale candidate branch')
    require(record['base_revision'] == BASE and record['implementation_base'] == IMPLEMENTATION_BASE,
            'stale base identity')
    for revision in (BASE, IMPLEMENTATION_BASE):
        require(subprocess.run(['git', 'merge-base', '--is-ancestor', revision, 'HEAD'], cwd=root).returncode == 0,
                'base not in candidate ancestry')
    rows, digest = content(root)
    require(record['source_files'] == rows and record['source_sha256'] == digest, 'stale source content')
    require(record['preparation'] == 'completed' and record['D2'] == 'pending'
            and record['delivery'] == 'pending' and record['goal_complete'] is False,
            'unsupported completion claim')
    for section in ('changed_behavior', 'refusal_cases', 'persisted_reload_assertions',
                    'limitations', 'historical_console_evidence', 'console_followups'):
        require(isinstance(record[section], list) and record[section]
                and all(isinstance(s, str) and s.strip() for s in record[section]), 'missing ' + section)
    require(set(record['checks']) == set(CHECKS), 'missing required check')
    logs = {}
    for name, command in CHECKS.items():
        check = record['checks'][name]
        require(check['command'] == command and type(check['exit_code']) is int and check['exit_code'] == 0
                and check['source_sha256'] == digest, 'unsuccessful/stale required check: ' + name)
        path = local(root, check['evidence'])
        require(check['evidence'].startswith(RESULTS) and sha(path.read_bytes()) == check['sha256'],
                'stale check evidence: ' + name)
        logs[name] = path.read_text()
        require(logs[name].strip(), 'empty evidence')
    for name in ('lint', 'test'):
        response = read(local(root, record['checks'][name]['evidence']))
        require(not response.get('isError'), 'host check error')
        require(name + ' exited 0' in logs[name], 'missing successful host exit')
    require('PASS' in logs['compatibility'] and 'tsc --noEmit' in logs['types'], 'missing check output')
    # Unit coverage has its own events/profile; native integration events cannot
    # satisfy the threshold. Recompute the complete source-derived denominator.
    import tempfile
    import exhausted_task_review_contract as contract
    import exhausted_task_all as all_checks
    import exhausted_task_unit_coverage as unit_checks
    from exhausted_task_proof import require_coverage
    required, cases = all_checks.required_cases()
    all_result = read(local(root, RESULTS + 'all-result.json'))
    require(all_result['source_sha256'] == all_checks.source_hashes(), 'stale all-mode source')
    events = read(local(root, RESULTS + 'native-events.json'))
    from exhausted_task_reconciliation import validate as validate_native
    validate_native(events, required)
    contract.check_events(events, cases)
    unit_manifest = read(unit_checks.MANIFEST)
    unit_required = {p: sorted(set(tests) | set(unit_manifest['subtests'].get(p, []))) for p, tests in unit_manifest['tests'].items()}
    unit_events = read(local(root, RESULTS + 'unit-events.json'))
    contract.check_events(unit_events, [dict(package=p, test=t) for p, tests in unit_required.items() for t in tests])
    profile = local(root, RESULTS + 'unit-coverage.out')
    require(profile.read_text().startswith('mode: atomic\n'), 'missing dedicated atomic unit profile')
    with tempfile.TemporaryDirectory(prefix='exhausted-delivery-') as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        contract.coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        functions = contract.resolve_scope(read(contract.SCOPE), helper, temp)
        blocks = {p: contract.coverage.expected_blocks(p, temp / 'instrumented.go') for p in {f['path'] for f in functions}}
        contract.fill_missing_blocks(blocks, profile, temp / 'complete.out')
        measured = contract.coverage.evaluate(functions, blocks, temp / 'complete.out')
    require(record['affected_function_coverage'] == measured['functions'], 'coverage differs from evidence')
    require_coverage(measured)
    require('all: PASS' in logs['acceptance'], 'missing acceptance evidence')
    removal = local(root, RESULTS + 'removal.txt').read_text()
    require('removal-sensitive: PASS; native regression exit 1; exact refusal: task repair attempt limit reached' in removal,
            'missing removal-sensitive proof')
    require('Overlay artifacts cleaned; original source SHA256 ' + sha(local(root, 'pkg/manager/task_queue.go').read_bytes()) in removal,
            'stale removal proof')
    from exhausted_task_mutations import CONTROLS
    controls = local(root, RESULTS + 'review-controls.txt').read_text()
    for name, *_ in CONTROLS:
        require(name + ': expected named assertion failure; source restored' in controls, 'missing removal control: ' + name)
    # Inspect actual persisted JSON payloads, not prose claims.
    output = logs['reload']
    require('exhaustion-controls: PASS' in output, 'missing reload execution')
    for marker in ('RELOADED_EXHAUSTION ', 'RELOADED_FAILURE '):
        require(marker in output, 'missing persisted reload assertion')
    for line in output.splitlines():
        if 'RELOADED_EXHAUSTION ' in line:
            task = json.loads(line.split('RELOADED_EXHAUSTION ', 1)[1])
            require(task['attempt_count'] == task['max_attempts'] and
                    task['metadata']['verification_failure_evidence'] == 'legacy', 'lost persisted history')
            correction = task['metadata'].get('task_correction')
            if correction is not None:
                require(correction['consumed'] and correction['outcome'] == 'continuing_failure'
                        and correction['fresh_evidence_id'] and correction['reason'], 'lost failure disposition')
            else:
                require(task['metadata']['exhaustion']['outcome'] == 'attempt_limit', 'lost exhaustion')
        if 'RELOADED_FAILURE ' in line:
            receipt = json.loads(line.split('RELOADED_FAILURE ', 1)[1])
            require(receipt['Status'] == 'failed', 'lost persisted fresh failure')
    for ref in record['references']:
        require(sha(local(root, ref['path']).read_bytes()) == ref['sha256'], 'stale evidence reference')
    required_refs = {RESULTS + name for name in ('native-events.json', 'unit-events.json', 'unit-coverage.out', 'review-controls.txt', 'removal.txt', 'all-result.json')}
    require(required_refs <= {ref['path'] for ref in record['references']}, 'missing evidence references')


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--phase', required=True, choices=['preparation'])
    parser.add_argument('--record', default=RECORD)
    args = parser.parse_args()
    try:
        validate(ROOT, read(local(ROOT, args.record)))
    except (ValueError, KeyError, TypeError, OSError) as error:
        parser.exit(1, 'delivery evidence refused: ' + str(error) + '\n')
    print('preparation: PASS; candidate/source/checks/reloads/removal proof verified; D2 pending')


if __name__ == '__main__':
    main()
