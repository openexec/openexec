"""Complete correction contract: units, every named native case and falsifiers."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

import exhausted_task_review_contract as review
from exhausted_task_reconciliation import MODES, validate
from exhausted_task_unit_coverage import measure

ROOT = review.ROOT
OUTPUT = Path(os.environ.get('OPENEXEC_EXHAUSTED_OUTPUT', '/tmp/openexec-exhausted-all'))


def required_cases():
    scope, matrix = json.loads(review.SCOPE.read_text()), json.loads(review.CASES.read_text())
    cases = review.validate(scope, matrix)
    required = {p: set(tests) for p, tests in MODES['acceptance'].items()}
    for c in cases:
        required.setdefault(c['package'], set()).add(c['test'])
    extra = {
        'pkg/manager': {
            'TestCorrectionPreAdmissionRefusalPersistence': 'candidate_edit candidate_commit candidate_output actual_branch receipt missing_receipt branch state_hash current_plan newer_plan graph metadata_array metadata_scalar metadata_fields metadata_types plan_metadata',
            'TestTaskCorrectCLI': 'planned planned/denials no-plan no-plan/denials',
            'TestCorrectionWithoutChecksUsesNativeCompletion': 'no_plan optional_plan',
            'TestCorrectionIndependentCandidateChange': '',
            'TestCorrectionUnitConcurrentQueues': '',
            'TestCorrectionSupportedArgv': 'sh absolute_sh lint test',
            'TestTaskQueueBoundaryKeepsLegacyHumanFailureAndLimitsDistinct': '',
        },
        'internal/release': {
            'TestCorrectionRefusalReopenAndDecisionHistory': '',
            'TestCorrectionAdmissionRefusalRace': '',
            'TestCorrectionRefusalReplacementGuards': 'fresh stale_snapshot write_failure closed concurrent_authorization',
        },
        'internal/cli': {'TestTaskCorrectAgentCannotSelfPromote': ''},
    }
    for package, tests in extra.items():
        for test, subcases in tests.items():
            required.setdefault(package, set()).add(test)
            required[package].update(test + '/' + c for c in subcases.split())
    for origin in ('planned', 'planner_imported', 'legacy_no_plan'):
        modes = 'pass missing_file exit_3 quoted effect_denial no_authority'.split()
        if origin == 'planned':
            modes += ['plan_failure', 'unsupported']
        required['pkg/manager'].update('TestCorrectionTaskScriptJourneys/' + origin + '/' + mode for mode in modes)
    return {p: sorted(t) for p, t in required.items()}, cases


def run(name, argv):
    path = OUTPUT / (name + '.txt')
    with path.open('w') as log:
        log.write('$ ' + ' '.join(argv) + '\n')
        log.flush()
        result = subprocess.run(argv, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT)
        log.write('\nexit=' + str(result.returncode) + '\n')
    print(name + ': exit ' + str(result.returncode), flush=True)
    if result.returncode:
        raise ValueError(path.read_text())


def source_hashes():
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=ROOT, text=True).split('\0')
    return {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest()
            for name in sorted(set(names)) if name and name.startswith(('internal/', 'pkg/', 'cmd/', 'scripts/'))
            and (ROOT / name).is_file() and Path(name).suffix in ('.go', '.py', '.sh', '.json')}


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    (OUTPUT / 'all-result.json').unlink(missing_ok=True)
    sources = source_hashes()
    for name in ('all', 'reconciliation', 'review_contract'):
        run('self-tests-' + name, ['python3', '-m', 'unittest', 'discover', '-s', 'scripts/verification', '-p', 'test_exhausted_task_' + name + '.py'])
    unit = measure(OUTPUT)
    required, cases = required_cases()
    events = []
    for package, tests in required.items():
        pattern = '^(' + '|'.join(sorted({re.escape(t.split('/')[0]) for t in tests})) + ')$'
        command = ['go', 'test', './' + package, '-json', '-count=1', '-timeout=120s', '-run', pattern]
        result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True)
        log = OUTPUT / (package.replace('/', '-') + '-native.jsonl')
        log.write_text(result.stdout)
        if result.returncode:
            raise ValueError(result.stdout + result.stderr)
        events.extend(json.loads(line) for line in result.stdout.splitlines())
    persisted = validate(events, required)
    review.check_events(events, cases)
    (OUTPUT / 'reloads.txt').write_text('\n'.join(persisted) + '\n')
    print(f'Native cases PASS: {sum(map(len, required.values()))} required names; all {len(cases)} review cases; no skips', flush=True)
    from concurrent.futures import ThreadPoolExecutor
    checks = [
        ('race', ['go', 'test', './internal/release', './pkg/manager', '-race', '-count=1', '-timeout=120s', '-run', '^TestCorrectionUnitConcurrentQueues$|^TestCorrectionAdmissionRefusalRace$|^TestCorrectionRefusalReplacementGuards$|^TestCorrectionQueueAdmissionErrors$']),
        ('discovery', ['bash', 'scripts/verify-exhausted-task-discovery.sh']),
        ('correction-controls', ['bash', 'scripts/verify-exhausted-task-correction.sh']),
        ('public-command', ['python3', 'scripts/verify-task-correct.py']),
        ('removal', ['bash', 'scripts/verify-exhausted-task-reconciliation.sh', 'removal-sensitive']),
        ('review-controls', ['python3', 'scripts/verification/exhausted_task_mutations.py']),
    ]
    with ThreadPoolExecutor(max_workers=3) as pool:
        futures = [pool.submit(run, name, command) for name, command in checks]
        for future in futures:
            future.result()
    if source_hashes() != sources:
        raise ValueError('candidate source changed during verification')
    result = dict(status='passed', source_sha256=sources, required=required, review_cases=[c['id'] for c in cases],
                  covered=unit['covered'], statements=unit['statements'],
                  revision=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),
                  branch=subprocess.check_output(['git','branch','--show-current'],cwd=ROOT,text=True).strip(),
                  logs={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in OUTPUT.iterdir() if p.is_file() and p.name!='all-result.json'})
    (OUTPUT / 'all-result.json').write_text(json.dumps(result, indent=2) + '\n')
    if json.loads((OUTPUT / 'all-result.json').read_text()) != result:
        raise ValueError('persisted all-result changed')
    print('all: PASS; result reread at ' + str(OUTPUT / 'all-result.json'))


if __name__ == '__main__':
    main()
