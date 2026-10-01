"""Whole-function coverage and source-overlay removal proof for US-008."""
import hashlib
import json
import re
from pathlib import Path
import subprocess
import tempfile

import retention_unit_coverage as coverage

ROOT = Path(__file__).resolve().parents[2]
BASELINE = 'ca2bdf8c'
INVENTORY = ROOT / 'scripts/verification/exhausted-task-inventory.json'
REFUSAL = 'task repair attempt limit reached'
REGRESSION = 'TestCorrectionDiagnosticQueueSuccess'
PACKAGES = ['pkg/manager', 'internal/release', 'pkg/db/state']


def inventory(helper, temp):
    from verify_exhausted_task_discovery import validate_discovery, DOCUMENT
    discovered = json.loads(INVENTORY.read_text())
    validate_discovery(discovered, DOCUMENT.read_text())
    rows = discovered['functions']
    keys = [r['path'] + ':' + r['symbol'] for r in rows]
    if not keys or len(keys) != len(set(keys)):
        raise ValueError('empty or duplicate function inventory')
    # Discovery keeps its own fixed initial denominator and verifier. This gate
    # additionally reconciles every production body changed since implementation.
    coverage.BASELINE = BASELINE
    changed = coverage.scope(helper, temp / 'baseline.go')
    if any(f['path'] + ':' + f['name'] not in keys for f in changed):
        raise ValueError('changed function missing from inventory')
    functions = []
    for row in rows:
        candidates = json.loads(coverage.run(str(helper), row['path']))
        matches = [f for f in candidates if f['name'] == row['symbol']]
        if len(matches) != 1:
            raise ValueError('missing or ambiguous function: ' + str(row))
        functions.append(dict(matches[0], path=row['path'], change='reconciled inventory'))
    return functions


def require_coverage(result):
    if not result['functions'] or result['statements'] <= 0:
        raise ValueError('empty coverage denominator')
    if 10 * result['covered'] <= 9 * result['statements']:
        raise ValueError('inventoried whole-function statement coverage must be strictly greater than 90%')


def measure(required, validate):
    with tempfile.TemporaryDirectory(prefix='.exhausted-coverage-', dir=ROOT) as directory:
        temp = Path(directory)
        helper = temp / 'inventory'
        coverage.run('go', 'build', '-o', str(helper), './scripts/verification/retentioncoverage')
        functions = inventory(helper, temp)
        blocks = {f['path']: coverage.expected_blocks(f['path'], temp / 'instrumented.go') for f in functions}
        profile = temp / 'coverage.out'
        command = ['go', 'test', *['./' + p for p in PACKAGES], '-json', '-count=1', '-timeout=120s',
                   '-run', ('TestCorrection|TestRecapture|TestRetention|TestTask|TestWait|TestSQLite|'
                            'TestFailure|TestRunnable|TestRecordTask|TestCanComplete|TestAuditIncludes|'
                            'TestUpdateInfo|TestConsumeEvents|TestManager_|TestStartAndStatus$|'
                            'TestStartAfterComplete$|TestFreshTask|TestLiveWorkspace|TestFailedRepair|'
                            'TestPaused|TestCancelledQueue|TestStartRefuses|TestAnAttempt|'
                            'TestScheduler_(EmptyTaskList|SingleTask|DiamondDependency|'
                            'FailedTaskBlocksDependents|StoryDependencyRespected|'
                            'SkipsNonPendingTasks|DefaultWorkerCount)$'),
                   '-coverpkg=' + ','.join('./' + p for p in PACKAGES),
                   '-covermode=count', '-coverprofile=' + str(profile)]
        result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True, timeout=180)
        if result.returncode:
            raise ValueError(result.stdout + result.stderr)
        events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
        validate(events, required)
        measured = coverage.evaluate(functions, blocks, profile)
        for f in measured['functions']:
            print(f"{f['path']}:{f['name']}: {f['covered']}/{f['statements']}")
        print(f"Coverage: {measured['covered']}/{measured['statements']} ({measured['percent']:.4f}%)")
        require_coverage(measured)
        print(f"Coverage PASS; {len(functions)} whole functions; {sum(map(len, required.values()))} required tests/subcases; no skips; reopened evidence parsed")
        return measured


def replacement(source):
    start = '\t\t\tif task.Metadata["task_correction"] != nil {'
    end = '\t\t\tid, _ := task.Metadata["verification_failure_evidence"].(string)'
    if source.count(start) != 1 or source.count(end) != 1:
        raise ValueError('reconciliation integration moved or ambiguous')
    a, b = source.index(start), source.index(end)
    removed = source[a:b]
    if 'm.reconcileTaskCorrection(ctx, task)' not in removed or 'm.persistTaskExhaustion(ctx, task)' not in removed:
        raise ValueError('actual reconciliation/disposition integration absent')
    return source[:a] + source[b:]


def validate_mutation(result):
    events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
    relevant = [e for e in events if e.get('Test') == REGRESSION]
    output = ''.join(e.get('Output', '') for e in relevant)
    if (result.returncode == 0 or not any(e.get('Action') == 'run' for e in relevant)
            or not any(e.get('Action') == 'fail' for e in relevant)
            or any(e.get('Action') in ('pass', 'skip') for e in relevant)
            or not re.search(r'^\s+\S+\.go:\d+: ' + re.escape(REFUSAL) + r'\s*$', output, re.M)):
        raise ValueError('mutation must fail native regression with exact original refusal:\n' + result.stdout + result.stderr)
    return output


def removal_sensitive():
    path = ROOT / 'pkg/manager/task_queue.go'
    original = path.read_bytes()
    try:
        with tempfile.TemporaryDirectory(prefix='.exhausted-mutation-', dir=ROOT) as directory:
            temp = Path(directory)
            substitute = temp / 'task_queue.go'
            substitute.write_text(replacement(original.decode()))
            overlay = temp / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(path): str(substitute)}}))
            command = ['go', 'test', '-overlay=' + str(overlay), './pkg/manager', '-json',
                       '-run', '^' + REGRESSION + '$', '-count=1', '-timeout=45s']
            result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True, timeout=120)
            output = validate_mutation(result)
            print(output, end='')
            print('removal-sensitive: PASS; native regression exit ' + str(result.returncode) + '; exact refusal: ' + REFUSAL)
    finally:
        if path.read_bytes() != original:
            raise ValueError('candidate source changed during overlay verification')
    print('Overlay artifacts cleaned; original source SHA256 ' + hashlib.sha256(original).hexdigest())
