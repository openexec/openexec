#!/usr/bin/env python3
"""Execute named behavioral inventories; package success alone is not evidence."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / 'scripts/autonomy-contract/evidence/exit-1'
PREFIX = 'github.com/openexec/openexec/'
BOUNDARY = {
    'pkg/runtime': ['TestTerminalBoundary', 'TestTerminalMixedErrorsAndNativeBinding', 'TestLocalCommandFailureCompatibility'],
    'internal/execution/gates': ['TestVerificationFailureEvidence'],
    'internal/pipeline': ['TestTerminalVerificationEvidenceRequiresCurrentDeterministicRunner', 'TestAdmittedTerminalBindingAndReceiptReset'],
    'internal/blueprint': ['TestConfiguredVerificationCommandEvidence'],
}
RECOVERY = {
    'pkg/manager': ['TestPersistedExitOneRecovery', 'TestPersistedRecoveryMatrix', 'TestPersistedTerminalRefusalsDoNotRepair',
                    'TestTaskQueueRestartAfterFailureReceiptBeforeRepair',
                    'TestInjectedExecutorUsesRealQueueAndTrustedRepair'],
}
COMPATIBILITY = {
    'internal/validation': ['TestCompatibility_ExistingProjects_StatusCLI',
                            'TestCompatibility_LegacyProjectConfigFallback',
                            'TestCompatibility_LegacyTasksJSONFallback'],
}
LOCAL_CASES = ['exit_' + str(code) for code in (0, 1, 125, 126, 127, 128, 255)] + [
    'forged_text', 'mixed_cancelled', 'mixed_timeout', 'mixed_launch', 'mixed_transport']

REFUSALS = ['tampered', 'missing', 'stale_task', 'stale_stage', 'stale_attempt',
            'stale_stage_attempt', 'worker_source', 'worker_artifacts', 'mixed',
            'cancelled', 'timeout', 'launch', 'transport', 'exit126', 'exit127', 'exit128', 'exit255', 'exit_negative',
            'missing_exit', 'empty_id', 'wrong_id', 'empty_task', 'empty_stage',
            'zero_attempt', 'negative_attempt', 'zero_stage_attempt', 'negative_stage_attempt',
            'nil_loader', 'loader_typed_failure', 'context_cancelled', 'context_timeout',
            'mixed_cancelled', 'mixed_timeout', 'mixed_launch']
BOUNDARY_CASES = ['0', '1', '2', '125', '126', '127', '128', '255', '-1',
                  'task', 'stage', 'task_attempt', 'stage_attempt', 'source', 'id',
                  'missing_exit', 'cancelled', 'timed_out', 'launch_error',
                  'transport_error', 'missing', 'tampered', 'loader_typed_failure',
                  'nil_loader', 'empty_binding', 'context_cancelled', 'empty_id', 'blank_task',
                  'blank_stage', 'zero_task_attempt', 'negative_task_attempt',
                  'zero_stage_attempt', 'negative_stage_attempt', 'context_timeout',
                  'cancel_during_load', 'unknown_outcome']


def validate_events(events, required, expected_failure=None):
    """Require actual execution and terminal verdict for every named test."""
    ran, passed, failed = set(), set(), set()
    for event in events:
        if not isinstance(event, dict):
            raise ValueError("invalid test event")
        name = event.get('Test')
        action = event.get('Action')
        if action == 'skip':
            raise ValueError(f'skipped test/package: {name}')
        if action == 'fail' and not name and expected_failure is None:
            raise ValueError('package failed')
        if not name:
            continue
        if action == 'run': ran.add(name)
        if action == 'pass': passed.add(name)
        if action == 'fail': failed.add(name)
    if not required or not ran:
        raise ValueError('zero-test run')
    if not set(required) <= ran:
        raise ValueError(f'missing tests: {set(required) - ran}')
    if expected_failure:
        if failed != {expected_failure} or not set(required) <= passed | failed:
            raise ValueError('baseline did not fail in the required behavioral test')
    elif failed or not set(required) <= passed:
        raise ValueError(f'failed or incomplete tests: {failed | (set(required) - passed)}')


def run_tests(root, package, names, expected_failure=None):
    env = os.environ.copy()
    env['GOCACHE'] = '/tmp/openexec-exit1-go-cache'
    command = ['go', 'test', '-json', './' + package, '-run', '^(' + '|'.join(names) + ')$', '-count=1', '-timeout=90s']
    run = subprocess.run(command, cwd=root, env=env, text=True, capture_output=True, timeout=120)
    try:
        events = [json.loads(line) for line in run.stdout.splitlines()]
        if any(not isinstance(e, dict) or e.get('Package') != PREFIX + package for e in events):
            raise ValueError('unexpected test package')
        package_verdicts = [e['Action'] for e in events if not e.get('Test') and e.get('Action') in ('pass', 'fail', 'skip')]
        if package_verdicts != ['fail' if expected_failure else 'pass']:
            raise ValueError('missing or contradictory package verdict')
        required = list(names)
        if 'TestLocalCommandFailureCompatibility' in names:
            required += ['TestLocalCommandFailureCompatibility/' + s for s in LOCAL_CASES]
        if 'TestCompatibility_ExistingProjects_StatusCLI' in names:
            required += ['TestCompatibility_ExistingProjects_StatusCLI/' + s
                         for s in ('current_openexec_project', 'legacy_uaos_project')]
        if 'TestCompatibility_LegacyTasksJSONFallback' in names:
            required += ['TestCompatibility_LegacyTasksJSONFallback/' + s for s in ('initial', 'reloaded')]
        if 'TestPersistedRecoveryMatrix' in names:
            required += [f'TestPersistedRecoveryMatrix/exit_{code}/restart_{restart}'
                         for code in (0, 1, 125) for restart in ('false', 'true')]
        if 'TestPersistedExitOneRecovery' in names:
            required += ['TestPersistedExitOneRecovery/restart_false', 'TestPersistedExitOneRecovery/restart_true']
        if 'TestAdmittedTerminalBindingAndReceiptReset' in names:
            required += ['TestAdmittedTerminalBindingAndReceiptReset/' + s for s in ['matched', 'mutated_input', 'agentic', 'cancelled', 'success_clears_receipt', 'exit_zero', 'exit_125', 'mixed_timeout', 'forged_success']]
        if 'TestTerminalMixedErrorsAndNativeBinding' in names:
            required += ['TestTerminalMixedErrorsAndNativeBinding/' + s for s in ['task', 'stage', 'task_attempt', 'stage_attempt', 'source']]
        if 'TestInjectedExecutorUsesRealQueueAndTrustedRepair' in names:
            required += ['TestInjectedExecutorUsesRealQueueAndTrustedRepair/typed_failure', 'TestInjectedExecutorUsesRealQueueAndTrustedRepair/untrusted_artifacts']

        if 'TestPersistedTerminalRefusalsDoNotRepair' in names:
            required += ['TestPersistedTerminalRefusalsDoNotRepair/' + s for s in REFUSALS]
        if 'TestTerminalBoundary' in names:
            required += ['TestTerminalBoundary/' + s for s in BOUNDARY_CASES]
        validate_events(events, required, expected_failure)
        if run.returncode != (1 if expected_failure else 0):
            raise ValueError(f'unexpected test exit {run.returncode}')
    except (ValueError, json.JSONDecodeError):
        print(run.stdout + run.stderr)
        raise
    print(f'{package}: verified {len(required)} required tests (no skips)')
    return run.stdout


def baseline():
    manifest = json.loads((EVIDENCE / 'baseline-manifest.json').read_text())
    for filename, digest in manifest['sha256'].items():
        if hashlib.sha256((EVIDENCE / filename).read_bytes()).hexdigest() != digest:
            raise ValueError('preserved baseline changed: ' + filename)
    preserved = [json.loads(line) for line in (EVIDENCE / 'baseline.jsonl').read_text().splitlines()]
    name = 'TestPersistedExitOneBaseline'
    validate_events(preserved, [name], name)
    revision = (EVIDENCE / 'baseline-revision.txt').read_text().strip()
    with tempfile.TemporaryDirectory(prefix='openexec-baseline-') as tmp:
        archive = subprocess.run(['git', 'archive', revision], cwd=ROOT, check=True, capture_output=True)
        subprocess.run(['tar', '-x', '-C', tmp], input=archive.stdout, check=True)
        target = Path(tmp) / 'pkg/manager/persisted_baseline_test.go'
        target.write_bytes((EVIDENCE / 'baseline_test.go.fixture').read_bytes())
        output = run_tests(tmp, 'pkg/manager', [name], name)
        for marker in ['persisted and reloaded terminal:', 'reopened task A status=failed attempt=1 tasks=2 repaired=false', 'BEHAVIORAL_BASELINE:']:
            if marker not in output:
                raise ValueError('not a behavioral reproduction: ' + marker)
    print('pre-fix behavioral failure reproduced at ' + revision)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--case', required=True, choices=['behavioral-reproduction', 'runtime-boundary', 'native-recovery-tracer', 'exit-1-slice', 'recovery-matrix', 'compatibility-refusal'])
    args = parser.parse_args()
    if args.case in ('behavioral-reproduction', 'exit-1-slice'):
        baseline()
    if args.case in ('runtime-boundary', 'exit-1-slice', 'recovery-matrix', 'compatibility-refusal'):
        for package, names in BOUNDARY.items(): run_tests(ROOT, package, names)
    if args.case in ('native-recovery-tracer', 'exit-1-slice', 'recovery-matrix', 'compatibility-refusal'):
        for package, names in RECOVERY.items(): run_tests(ROOT, package, names)
    if args.case == 'compatibility-refusal':
        for package, names in COMPATIBILITY.items(): run_tests(ROOT, package, names)
    if args.case == 'behavioral-reproduction':
        run_tests(ROOT, 'pkg/manager', ['TestPersistedExitOneRecovery'])
    print(args.case + ': PASS')


if __name__ == '__main__':
    main()
