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
TESTED = 'cc79237b82fcccd4716396984bc2e799d6d35a28'
EVIDENCE_EDITS = {
    'scripts/verification/exhausted_task_delivery.py',
    'scripts/verification/test_exhausted_task_delivery.py',
    'scripts/verification/fixtures/exhausted-task-delivery/refusals.json',
}
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
    require(record['schema'] == 2 and record['task'] == 'T-US-012-001', 'wrong task/schema')
    require(record['branch'] == BRANCH == git(root, 'branch', '--show-current'), 'stale candidate branch')
    require(record['base_revision'] == BASE and record['implementation_base'] == IMPLEMENTATION_BASE,
            'stale base identity')
    for revision in (BASE, IMPLEMENTATION_BASE, TESTED):
        require(subprocess.run(['git', 'merge-base', '--is-ancestor', revision, 'HEAD'], cwd=root).returncode == 0,
                'base not in candidate ancestry')
    rows, digest = content(root)
    require(record['source_files'] == rows and record['source_sha256'] == digest, 'stale source content')
    tested = json.loads(git(root, 'show', TESTED + ':' + RECORD))
    require(record['tested_revision'] == TESTED and record['tested_source_sha256'] == tested['source_sha256'],
            'wrong tested provenance')
    require(record['stage_entry_revision'] == TESTED, 'wrong preparation entry')
    before = {r['path']: r for r in tested['source_files']}
    after = {r['path']: r for r in rows}
    changed = sorted(p for p in before.keys() | after.keys() if before.get(p) != after.get(p))
    require(set(changed) <= EVIDENCE_EDITS and record['evidence_only_changes'] == changed,
            'implementation drift since tested candidate')
    require(record['D1'] == 'verified_native', 'missing D1 result')
    require(set(record['preparation_checks']) == {'lint', 'test'}, 'missing preparation checks')
    for name, receipt in record['preparation_checks'].items():
        path = local(root, receipt['evidence'])
        response = read(path)
        require(receipt['exit_code'] == 0 and receipt['sha256'] == sha(path.read_bytes())
                and not response.get('isError')
                and name + ' exited 0' in path.read_text(), 'failed preparation check')

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
                and check['source_sha256'] == tested['source_sha256'], 'unsuccessful/stale required check: ' + name)
        path = local(root, check['evidence'])
        require(check['evidence'].startswith(RESULTS) and sha(path.read_bytes()) == check['sha256'],
                'stale check evidence: ' + name)
        logs[name] = path.read_text()
        require(logs[name].strip(), 'empty evidence')
    require(record['checks'] == tested['checks'], 'historical checks must not be relabeled')
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
    require(all_result['status'] == 'passed' and all_result['branch'] == BRANCH
            and all_result['revision'] == tested['stage_entry_revision'], 'wrong all-mode provenance')
    require(all_result['required'] == required and all_result['review_cases'] == [c['id'] for c in cases],
            'inconsistent all-mode case scope')
    for name, expected in all_result['logs'].items():
        require(sha(local(root, RESULTS + name).read_bytes()) == expected, 'stale all-mode log')
    matrix = read(local(root, 'scripts/verification/exhausted-task-review-cases.json'))
    require(record['findings'] == matrix['findings'] and len(record['findings']) == 4,
            'missing/inconsistent finding dispositions')
    expected_cases = [dict(id=c['id'], package=c['package'], test=c['test'], outcome='passed',
                           evidence=RESULTS + 'native-events.json') for c in cases]
    require(record['case_outcomes'] == expected_cases, 'missing/inconsistent case outcomes')
    require(all_result['source_sha256'] == {p: before[p]['sha256'] for p in all_checks.source_hashes()},
            'stale all-mode source')
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
    for key, field in (('covered', 'unit_covered'), ('statements', 'unit_statements')):
        total = sum(f[key] for f in measured['functions'])
        require(record[field] == total == all_result[key], 'inconsistent coverage total')
    require('all: PASS' in logs['acceptance'], 'missing acceptance evidence')
    removal = local(root, RESULTS + 'removal.txt').read_text()
    require('removal-sensitive: PASS; native regression exit 1; exact refusal: task repair attempt limit reached' in removal,
            'missing removal-sensitive proof')
    require('Overlay artifacts cleaned; original source SHA256 ' + sha(local(root, 'pkg/manager/task_queue.go').read_bytes()) in removal,
            'stale removal proof')
    from exhausted_task_mutations import CONTROLS
    controls = local(root, RESULTS + 'review-controls.txt').read_text()
    expected_controls = []
    for name, source, *rest in CONTROLS:
        marker = name + ': expected named assertion failure; source restored ' + sha(local(root, source).read_bytes())
        require(marker in controls, 'missing removal control: ' + name)
        expected_controls.append(dict(id=name, outcome='intended assertion failed; source restored',
                                      evidence=RESULTS + 'review-controls.txt', marker=marker))
    extra = {
        RESULTS + 'correction-controls.txt': ['refusal', 'boundary-kind', 'boundary-evidence',
            'task-script-execution', 'task-script-completion', 'original-exhaustion'],
        'docs/verification/task-correct-transcript.txt': ['skip-authorization', 'bypass-operator',
            'bypass-decision', 'change-decision'],
        RESULTS + 'removal.txt': ['removal-sensitive: PASS'],
    }
    for path, names in extra.items():
        log = local(root, path).read_text()
        for name in names:
            require(name in log, 'missing falsifier: ' + name)
            expected_controls.append(dict(id=name, outcome='intended assertion failed; source restored',
                                          evidence=path, marker=name))
    require(record['falsifier_outcomes'] == expected_controls, 'missing/inconsistent falsifier outcomes')
    scratch = local(root, 'docs/verification/task-correct-transcript.txt').read_text()
    for marker in ('PUBLIC_COMMAND', 'RELOADED_CORRECTION', 'exit=1', 'exit=0',
                   'TestTaskCorrectCLI/planned', 'TestTaskCorrectCLI/no-plan', 'exact decision binding mismatch'):
        require(marker in scratch, 'incomplete scratch walkthrough')
    require(record['scratch_evidence'] == 'docs/verification/task-correct-transcript.txt', 'missing scratch reference')
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
    require(len({r['path'] for r in record['references']}) == len(record['references']), 'duplicate references')
    for ref in record['references']:
        require(sha(local(root, ref['path']).read_bytes()) == ref['sha256'], 'stale evidence reference')
    required_refs = {RESULTS + name for name in ('native-events.json', 'unit-events.json', 'unit-coverage.out', 'review-controls.txt', 'removal.txt', 'all-result.json')}
    required_refs.update(RESULTS + name for name in all_result['logs'])
    required_refs.add(record['scratch_evidence'])
    require(required_refs <= {ref['path'] for ref in record['references']}, 'missing evidence references')
    retained = {ref['path']: ref['sha256'] for ref in record['references']}
    require(all(retained.get(ref['path']) == ref['sha256'] for ref in tested['references']),
            'historical receipt provenance changed')
    require(retained[record['scratch_evidence']] == before[record['scratch_evidence']]['sha256'],
            'scratch provenance changed')


def validate_merge(root, record, receipt):
    # Reuse the existing receipt obligations; never synthesize a receipt from notes.
    from delivery import check_merge
    from datetime import datetime, timedelta, timezone
    check_merge(receipt, git(root, 'rev-parse', 'HEAD'), record['source_sha256'])
    require(not git(root, 'status', '--porcelain'), 'uncommitted candidate cannot prove merge')
    require(git(root, 'remote', 'get-url', 'origin') in (
        'https://github.com/openexec/openexec.git', 'https://github.com/openexec/openexec',
        'git@github.com:openexec/openexec.git'), 'wrong Git repository identity')
    require(receipt.get('issuer') == 'agent-console' and receipt.get('feature') == BRANCH.split('/')[1],
            'wrong coordinator merge provenance')
    require(re.fullmatch(r'https://github.com/openexec/openexec/pull/[1-9][0-9]*', receipt['pull_request']),
            'wrong merge pull request')
    observed = datetime.fromisoformat(receipt['observed_at'].replace('Z', '+00:00'))
    require(observed.tzinfo is not None and timedelta(0) <= datetime.now(timezone.utc) - observed <= timedelta(hours=24),
            'stale coordinator observation')
    ref = git(root, 'symbolic-ref', 'refs/remotes/origin/HEAD')
    require(ref == 'refs/remotes/origin/' + receipt['default_branch'], 'wrong default branch')
    tip = git(root, 'rev-parse', '--verify', ref + '^{commit}')
    require(tip == receipt['default_branch_revision'], 'stale default branch')
    merged = receipt['merge_revision']
    require(subprocess.run(['git', 'merge-base', '--is-ancestor', merged, tip], cwd=root).returncode == 0,
            'merge absent from default branch')
    if receipt.get('integration') == 'ancestry':
        require(subprocess.run(['git', 'merge-base', '--is-ancestor', 'HEAD', merged], cwd=root).returncode == 0,
                'candidate absent from merge')
    elif receipt.get('integration') == 'tree':
        require(git(root, 'rev-parse', merged + '^{tree}') == git(root, 'rev-parse', 'HEAD^{tree}'),
                'squash/rebase tree differs')
    else:
        raise ValueError('missing merge integration evidence')


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--phase', required=True, choices=['preparation', 'delivery'])
    parser.add_argument('--record', default=RECORD)
    parser.add_argument('--merge-evidence', help='repository-local trusted coordinator export')
    args = parser.parse_args()
    try:
        record = read(local(ROOT, args.record))
        validate(ROOT, record)
        if args.phase == 'delivery' or args.merge_evidence:
            require(args.merge_evidence, 'actual coordinator merge evidence required')
            validate_merge(ROOT, record, read(local(ROOT, args.merge_evidence)))
    except (ValueError, KeyError, TypeError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, 'delivery evidence refused: ' + str(error) + '\n')
    print('delivery: PASS; coordinator merge verified' if args.phase == 'delivery' else
          'preparation: PASS; candidate/source/checks/reloads/removal proof verified; D2 pending')


if __name__ == '__main__':
    main()
