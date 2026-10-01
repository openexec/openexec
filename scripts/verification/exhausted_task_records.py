"""Repository-local US-010 checklist validation and post-queue evidence boundary.

Records validate obligations, not their implementation. Preparation additionally
requires native evidence. Merge receipts are trusted coordinator exports supplied
separately; this read-only verifier never grants effects or authenticates owners.
"""
import argparse
from datetime import datetime, timedelta, timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

import exhausted_task_review_contract as review
from exhausted_task_delivery import read, require
from delivery import check_merge

ROOT = Path(__file__).resolve().parents[2]
RECORD = 'scripts/verification/exhausted-task-records.json'
MATRIX = 'scripts/verification/exhausted-task-review-cases.json'
SCOPE = 'scripts/exhausted-task-coverage-scope.json'
EVIDENCE_DIR = 'docs/verification/exhausted-task-records-evidence/'
FEATURE = 'ad5a70cd0924dae18de345dbdf74e623'
PR = 'https://github.com/openexec/openexec/pull/80'
FINDINGS = {'F1', 'F2', 'F3', 'F4'}
REQUIRED_SOURCES = {
    'F1': {('pkg/manager/task_correction.go', 'reconcileTaskCorrection'),
           ('pkg/manager/task_queue.go', 'executeTaskQueue'),
           ('internal/release/task_correction.go', 'AuthorizeTaskCorrection')},
    'F2': {('pkg/manager/task_correction.go', 'AuthorizeTaskCorrection'),
           ('internal/cli/approve_local.go', 'localApprovalManager')},
    'F3': {('pkg/manager/task_correction.go', 'correctionPlan'),
           ('pkg/manager/task_correction.go', 'correctionCheck'),
           ('pkg/manager/task_recapture.go', 'resolveRecaptureCommand')},
    'F4': {('pkg/manager/task_boundary.go', 'retainedTaskBoundary')},
}


def local(root, name, directory=False):
    require(isinstance(name, str) and name and '\\' not in name,
            'invalid repository-local path')
    path = Path(name)
    require(not path.is_absolute() and '..' not in path.parts and path.as_posix() == name,
            'nonlocal or noncanonical path: ' + name)
    resolved = (root / path).resolve()
    require(resolved.is_relative_to(root.resolve()), 'path escapes repository: ' + name)
    require(resolved.is_dir() if directory else resolved.is_file(), 'missing source/evidence: ' + name)
    return resolved


def keys(value, expected, label):
    require(isinstance(value, dict) and set(value) == set(expected.split()), 'incomplete/unknown ' + label)


def texts(value, label):
    require(isinstance(value, list) and value and all(isinstance(s, str) and s.strip() for s in value),
            'incomplete ' + label)


def git(root, *args):
    return subprocess.check_output(['git', *args], cwd=root, text=True, stderr=subprocess.PIPE).strip()


def source_digest(root):
    names = git(root, 'ls-files', '-z', '--cached', '--others', '--exclude-standard').split('\0')
    rows = []
    for name in sorted(set(names) - {''}):
        if name.startswith(EVIDENCE_DIR) or '__pycache__' in Path(name).parts:
            continue
        path = root / name
        if not path.exists() and not path.is_symlink():
            rows.append([name, 'deleted'])
            continue
        local(root, name)
        rows.append([name, oct(path.stat().st_mode & 0o777), hashlib.sha256(path.read_bytes()).hexdigest()])
    require(rows, 'empty candidate')
    return hashlib.sha256(json.dumps(rows, separators=(',', ':')).encode()).hexdigest()


def case_command(case):
    # No shell, no regex metacharacters supplied by records. Each subtest anchored.
    require(re.fullmatch(r'Test[A-Za-z0-9_]+(?:/[A-Za-z0-9_]+)*', case['test']) is not None,
            'invalid exact test selector')
    selector = '/'.join('^' + s + '$' for s in case['test'].split('/'))
    return ['go', 'test', '-json', '-count=1', '-timeout=90s',
            *(['-race'] if case['id'] == 'concurrency' else []), './' + case['package'], '-run', selector]


def validate_records(root, record):
    keys(record, 'schema_version task_id story_id matrix scope dependencies checklists delivery', 'record')
    require(type(record['schema_version']) is int and record['schema_version'] == 1 and record['task_id'] == 'T-US-010-003'
            and record['story_id'] == 'US-010', 'wrong record identity')
    require(record['matrix'] == MATRIX and record['scope'] == SCOPE, 'wrong contract references')
    matrix, scope = read(local(root, record['matrix'])), read(local(root, record['scope']))
    require(matrix['task_id'] == scope['task_id'] == 'T-US-010-002'
            and matrix['review_id'] == 'afa2fdef5783532f6034a12a002ab508'
            and matrix['status'] == 'definition_only', 'wrong upstream contract')
    cases = review.validate(scope, matrix)
    require(scope['production_files'] == ['pkg/manager/task_correction.go', 'internal/release/task_correction.go',
                                         'pkg/manager/task_boundary.go'], 'production denominator narrowed')
    require(scope['inventory'] == 'scripts/verification/exhausted-task-inventory.json', 'inventory replaced')
    require(scope['baseline'] == 'ca2bdf8c' and scope['review_baseline'] == '354f874b56d881f993d4499dc82fc506d66f7046',
            'coverage baseline changed')
    require(scope['packages'] == ['pkg/manager', 'internal/release', 'pkg/db/state', 'internal/cli'],
            'coverage packages narrowed')
    for path in scope['production_files']:
        local(root, path)
    for package in scope['packages']:
        directory = local(root, package, directory=True)
        require(any(directory.glob('*.go')), 'non-runnable Go package')
    inventory = read(local(root, scope['inventory']))
    frozen = json.loads(git(root, 'show', scope['review_baseline'] + ':' + scope['inventory']))
    frozen_ids = {(f['path'], f['symbol']) for f in frozen['functions']}
    current_ids = {(f['path'], f['symbol']) for f in inventory['functions']}
    require(frozen_ids <= current_ids and len(current_ids) == len(inventory['functions']),
            'frozen inventory narrowed or duplicated')
    for function in inventory['functions']:
        local(root, function['path'])
        require(isinstance(function['symbol'], str) and function['symbol'].strip(), 'missing source symbol')
    for field in ('baseline', 'review_baseline'):
        require(re.fullmatch('[0-9a-f]{8,40}', scope[field]) is not None, 'invalid coverage baseline')
        git(root, 'rev-parse', '--verify', scope[field] + '^{commit}')
    assessment, anchor = matrix['assessment'].split('#')
    require(anchor and local(root, assessment).read_text().strip(), 'missing source assessment')
    require(matrix['dependencies'] == [{
        'task': 'T-US-010-001',
        'consumes': ['docs/exhausted-task-discovery.md', 'scripts/verification/exhausted-task-inventory.json'],
        'reason': 'Source diagnoses, native lifecycle boundaries and whole-function inventory are inputs to this contract.'}],
        'upstream dependency changed')
    for dependency in matrix['dependencies']:
        for path in dependency['consumes']:
            local(root, path)
    require(record['dependencies'] == [{'task': 'T-US-010-002', 'consumes': [MATRIX, SCOPE]}],
            'missing, circular or external dependency')
    template = ['go', 'test', '-json', '-count=1', '-timeout=90s', './{package}', '-run', '^{test}$']
    require(matrix['execution']['command_template'] == template, 'non-runnable command template')
    for field in ('rule', 'scratch', 'negative_controls'):
        require(isinstance(matrix['execution'][field], str) and matrix['execution'][field].strip(), 'missing execution obligation')
    require(shutil.which('go') is not None, 'Go executable unavailable')
    for case in cases:
        require(case['finding'] in FINDINGS | {'baseline', 'controls'}, 'unmapped case')
        texts(case['assertions'], 'case assertions')
        require(isinstance(case['setup'], str) and case['setup'].strip()
                and isinstance(case['negative_control'], str) and case['negative_control'].strip(), 'incomplete case/falsifier')
        require(case['implementation'] in ('required_not_implemented', 'existing_requires_strengthening'),
                'definitions cannot claim implemented evidence')
        case_command(case)
    checklists = record['checklists']
    require(isinstance(checklists, dict) and set(checklists) == FINDINGS, 'all four finding checklists required')
    for finding, checklist in checklists.items():
        keys(checklist, 'root_cause fix prove falsify', 'finding checklist ' + finding)
        for section in ('root_cause', 'fix'):
            refs = checklist[section]
            require(isinstance(refs, list) and refs, 'missing source references')
            for ref in refs:
                keys(ref, 'path symbol', 'source reference')
                source = local(root, ref['path']).read_text()
                symbol = ref['symbol']
                require(isinstance(symbol, str) and re.fullmatch('[A-Za-z_][A-Za-z_0-9]*', symbol)
                        and re.search(r'(?m)^func\s+(?:\([^)]*\)\s+)?' + re.escape(symbol) + r'\s*\(', source), 'missing source symbol')
            require({(r['path'], r['symbol']) for r in refs} == REQUIRED_SOURCES[finding]
                    and len(refs) == len(REQUIRED_SOURCES[finding]), 'incomplete source mechanism: ' + finding)
        expected = [c['id'] for c in cases if c['finding'] == finding]
        require(expected and checklist['prove'] == expected and checklist['falsify'] == expected,
                'incomplete/duplicate finding cases or falsifiers: ' + finding)
    keys(record['delivery'], 'native_preparation D1 D2 goal_complete owner', 'delivery boundary')
    require(record['delivery'] == {'native_preparation': 'unverified', 'D1': 'outstanding',
                                   'D2': 'outstanding', 'goal_complete': False, 'owner': 'agent-console'},
            'records alone cannot claim preparation or merge')
    return scope, cases


def denominator(root, scope, temp):
    # Reuse the existing AST union and source instrumentation. Validate all
    # inputs before the helper reads them; profile omissions never shrink it.
    require(root.resolve() == ROOT, 'scope discovery requires this repository')
    names = git(root, 'ls-files', '-z', '--cached', '--others', '--exclude-standard').split('\0')
    for name in names:
        if name.endswith('.go') and name.startswith(('internal/', 'pkg/', 'cmd/')) and (root / name).exists():
            local(root, name)  # Reject escaping Go symlinks before AST discovery reads them.
    helper = temp / 'inventory'
    review.coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
    functions = review.resolve_scope(scope, helper, temp)
    blocks = {}
    for path in {f['path'] for f in functions}:
        local(root, path)
        blocks[path] = review.coverage.expected_blocks(path, temp / 'instrumented.go')
    # An empty profile gives exactly the full source denominator, all uncovered.
    empty, filled = temp / 'empty.out', temp / 'filled.out'
    empty.write_text('mode: count\n')
    review.fill_missing_blocks(blocks, empty, filled)
    result = review.coverage.evaluate(functions, blocks, filled)
    require(result['statements'] > 0 and result['covered'] == 0, 'invalid source denominator')
    return functions, blocks, result['statements']


def artifact(root, ref):
    keys(ref, 'path sha256', 'evidence reference')
    data = local(root, ref['path']).read_bytes()
    require(re.fullmatch('[0-9a-f]{64}', ref['sha256']) is not None
            and hashlib.sha256(data).hexdigest() == ref['sha256'], 'stale evidence hash')
    require(data.strip(), 'empty evidence')
    return data.decode()


def events(text):
    result = [json.loads(line) for line in text.splitlines()]
    require(result and all(isinstance(row, dict) for row in result), 'invalid native events')
    return result


def validate_native(root, report, cases, functions, blocks, temp):
    keys(report, 'schema_version candidate_revision source_sha256 events profile falsifiers checks scratch', 'native evidence')
    require(report['schema_version'] == 1 and report['candidate_revision'] == git(root, 'rev-parse', 'HEAD')
            and report['source_sha256'] == source_digest(root), 'stale native candidate')
    review.check_events(events(artifact(root, report['events'])), cases)
    profile = local(root, report['profile']['path'])
    profile_text = artifact(root, report['profile'])
    for line in profile_text.splitlines()[1:]:
        match = re.fullmatch(r'(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)', line)
        require(match is not None, 'malformed profile')
        local(root, match[1].removeprefix(review.MODULE))
    filled = temp / 'measured.out'
    review.fill_missing_blocks(blocks, profile, filled)
    measured = review.coverage.evaluate(functions, blocks, filled)
    require(measured['passed'], 'whole-function coverage must exceed 90%')
    require(isinstance(report['falsifiers'], dict) and set(report['falsifiers']) == {c['id'] for c in cases},
            'missing case falsifiers')
    for case in cases:
        proof = report['falsifiers'][case['id']]
        keys(proof, 'mutation assertion events source_restored_sha256', 'falsifier')
        require(proof['mutation'] == case['negative_control'] and isinstance(proof['assertion'], str)
                and proof['assertion'] in case['assertions'], 'unmapped negative control')
        require(proof['source_restored_sha256'] == report['source_sha256'], 'mutant source not restored')
        log = events(artifact(root, proof['events']))
        key = (review.MODULE + case['package'], case['test'])
        actions = {e.get('Action') for e in log if (e.get('Package'), e.get('Test')) == key}
        output = ''.join(e.get('Output', '') for e in log if (e.get('Package'), e.get('Test')) == key)
        require({'run', 'fail'} <= actions and not {'pass', 'skip'} & actions and proof['assertion'] in output,
                'negative control lacks intended executed assertion failure')
    require(isinstance(report['checks'], dict) and set(report['checks']) == {'lint', 'test'}, 'required checks missing')
    for name, proof in report['checks'].items():
        keys(proof, 'command exit_code evidence', 'required check')
        require(proof['command'] == ['run_declared_check', name] and type(proof['exit_code']) is int
                and proof['exit_code'] == 0, 'required check failed')
        response = json.loads(artifact(root, proof['evidence']))
        require(not response.get('isError') and name + ' exited 0' in json.dumps(response), 'missing host check success')
    scratch = report['scratch']
    keys(scratch, 'command exit_code evidence', 'scratch journey')
    require(isinstance(scratch['command'], list) and scratch['command'] and type(scratch['exit_code']) is int
            and scratch['exit_code'] == 0, 'missing operator scratch command')
    for arg in scratch['command']:
        require(isinstance(arg, str) and arg.strip(), 'invalid scratch argv')
    result = json.loads(artifact(root, scratch['evidence']))
    keys(result, 'decision_ref persisted_decision_ref candidate_branch candidate_sha256 A Settings original_receipt_sha256 reopened_receipt_sha256', 'scratch observations')
    require(isinstance(result['decision_ref'], str) and result['decision_ref'].strip()
            and result['persisted_decision_ref'] == result['decision_ref'], 'scratch decision mismatch')
    require(result['candidate_branch'] == 'outcome/' + FEATURE
            and re.fullmatch('[0-9a-f]{64}', result['candidate_sha256']), 'scratch candidate missing')
    require(result['A'] == {'status': 'done', 'attempt_count': 3, 'max_attempts': 3}
            and result['Settings'] == {'status': 'done'}
            and re.fullmatch('[0-9a-f]{64}', result['original_receipt_sha256'])
            and result['original_receipt_sha256'] == result['reopened_receipt_sha256'], 'scratch persisted observations incomplete')
    require(source_digest(root) == report['source_sha256'], 'candidate changed during validation')
    return measured


def validate_merge(root, receipt):
    check_merge(receipt, git(root, 'rev-parse', 'HEAD'), source_digest(root))
    require(receipt.get('issuer') == 'agent-console' and receipt.get('feature') == FEATURE
            and receipt['pull_request'] == PR, 'wrong coordinator merge provenance')
    # Local booleans, a Console process revision, or a published PR are not
    # merge proof. Require the coordinator's observed default-branch Git objects.
    require(git(root, 'remote', 'get-url', 'origin') in (
        'https://github.com/openexec/openexec.git', 'https://github.com/openexec/openexec',
        'git@github.com:openexec/openexec.git'), 'wrong Git repository identity')
    observed = datetime.fromisoformat(receipt['observed_at'].replace('Z', '+00:00'))
    require(observed.tzinfo is not None and timedelta(0) <= datetime.now(timezone.utc) - observed <= timedelta(hours=24),
            'stale coordinator observation')
    default_ref = git(root, 'symbolic-ref', 'refs/remotes/origin/HEAD')
    require(default_ref == 'refs/remotes/origin/' + receipt['default_branch'], 'wrong default branch')
    tip = git(root, 'rev-parse', '--verify', default_ref + '^{commit}')
    require(tip == receipt['default_branch_revision'], 'stale default-branch observation')
    merge = receipt['merge_revision']
    require(git(root, 'rev-parse', '--verify', merge + '^{commit}') == merge, 'missing merge object')
    require(subprocess.run(['git', 'merge-base', '--is-ancestor', merge, tip], cwd=root).returncode == 0,
            'merge absent from default branch')
    integration = receipt.get('integration')
    if integration == 'ancestry':
        require(subprocess.run(['git', 'merge-base', '--is-ancestor', receipt['candidate_revision'], merge], cwd=root).returncode == 0,
                'candidate absent from merge')
    elif integration == 'tree':
        require(git(root, 'rev-parse', merge + '^{tree}') == git(root, 'rev-parse', 'HEAD^{tree}'),
                'squash/rebase tree differs from verified candidate')
    else:
        raise ValueError('missing merge integration evidence')


def completion(root, native_passed, receipt=None, demand_merge=False):
    require(native_passed, 'native preparation evidence required')
    result = {'native_preparation': 'passed', 'D1': 'verified_native', 'D2': 'outstanding', 'goal_complete': False}
    if receipt is not None or demand_merge:
        require(receipt is not None, 'actual coordinator merge evidence required')
        validate_merge(root, receipt)
        result.update(D2='verified_external_merge', goal_complete=True)
    return result


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--record', default=RECORD)
    parser.add_argument('--phase', choices=('records', 'preparation', 'delivery'), default='records')
    parser.add_argument('--native-evidence', help='repository-local native report; required after records phase')
    parser.add_argument('--merge-evidence', help='repository-local trusted Agent Console merge export; never self-issued')
    parser.add_argument('--self-test', action='store_true')
    parser.add_argument('--list-commands', action='store_true')
    args = parser.parse_args(argv)
    if args.self_test:
        suite = unittest.defaultTestLoader.discover(str(ROOT / 'scripts/verification'), pattern='test_exhausted_task_records.py')
        if not unittest.TextTestRunner(verbosity=2).run(suite).wasSuccessful():
            return 1
    try:
        record = read(local(ROOT, args.record))
        scope, cases = validate_records(ROOT, record)
        with tempfile.TemporaryDirectory(prefix='exhausted-records-') as directory:
            temp = Path(directory)
            functions, blocks, statements = denominator(ROOT, scope, temp)
            result = dict(records='passed', cases=len(cases), functions=len(functions), statements=statements,
                          native_preparation='unverified', D1='outstanding', D2='outstanding', goal_complete=False)
            if args.phase == 'records':
                require(not args.native_evidence and not args.merge_evidence, 'evidence requires preparation or delivery phase')
            else:
                require(args.native_evidence is not None, 'native preparation evidence required')
                native = read(local(ROOT, args.native_evidence))
                measured = validate_native(ROOT, native, cases, functions, blocks, temp)
                receipt = read(local(ROOT, args.merge_evidence)) if args.merge_evidence else None
                result.update(completion(ROOT, measured['passed'], receipt, args.phase == 'delivery'))
            if args.list_commands:
                result['commands'] = {c['id']: case_command(c) for c in cases}
        print(json.dumps(result, sort_keys=True))
        return 0
    except (ValueError, KeyError, TypeError, AttributeError, OSError, subprocess.SubprocessError) as error:
        print(json.dumps({'records': 'refused', 'error': str(error), 'goal_complete': False}))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
