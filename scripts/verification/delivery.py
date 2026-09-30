#!/usr/bin/env python3
"""Fresh technical delivery proof. Neither mode certifies external D2."""
import argparse
import contextlib
import json
import hashlib
import os
import re
from pathlib import Path
import subprocess
import tempfile

import retention_story as retention
import recapture_story as recapture

ROOT = retention.ROOT
JOURNEYS = [
    'pkg/manager:TestTaskQueueFailedCheckCreatesRepairAndResumesOriginalWork',
    'pkg/manager:TestTaskQueueRestartAfterFailureReceiptBeforeRepair',
    'pkg/manager:TestInjectedExecutorUsesRealQueueAndTrustedRepair',
    *['pkg/manager:TestInjectedExecutorUsesRealQueueAndTrustedRepair/' + mode
      for mode in ('typed_failure', 'untrusted_artifacts', 'legacy_receipt')],
]
REPAIR = ('admitted-all', 'named-recapture', 'recapture-variants',
          'private-storage', 'storage-unit-coverage')
TECHNICAL = (*retention.CASES, *recapture.CASES, *REPAIR,
             'native-journey', 'discovery', 'verifier-controls')
GATES = {
    'test': ['make', 'test'],
    'compat-test': ['make', 'compat-test'],
    'type-check': ['make', 'type-check'],
    'lint': ['make', 'lint'],
    'ui-build': ['make', 'ui-build'],
    'check': ['make', 'check'],
    'pr-gate': ['make', 'pr-gate'],
}


def command_result(command, log):
    with log.open('w') as stream:
        run = subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
    result = dict(command=command, exit_code=run.returncode, passed=run.returncode == 0)
    if run.returncode:
        result['error'] = f'command exited {run.returncode}; see {log}'
    return result


def run_case(case, output, adapter_evidence=None):
    directory = output / case
    directory.mkdir()
    log = directory / 'command.log'
    result = dict(passed=False, log=str(log))
    try:
        if case in retention.CASES:
            manifest = json.loads(retention.MANIFEST.read_text())
            if set(manifest) != set(retention.CASES):
                raise ValueError('missing retention member')
            with log.open('w') as stream, contextlib.redirect_stdout(stream):
                proof = retention.run_case(case, directory, manifest)
            result.update(proof=proof, exit_code=0, passed=True)
        elif case in recapture.CASES:
            manifest = json.loads(recapture.MANIFEST.read_text())
            recapture.check_manifest(manifest)
            result.update(recapture.run_case(case, directory, manifest[case]))
        else:
            if case in REPAIR:
                command = ['bash', 'scripts/verify-verification-repair.sh', '--case', case]
                if case == 'storage-unit-coverage':
                    command = ['python3', 'scripts/verification/storage_unit_coverage.py',
                               '--output', str(directory / 'coverage')]
                if case == 'admitted-all':
                    command = ['python3', 'scripts/verification/admitted_story.py',
                               '--output', str(directory / 'admitted')]
                    if adapter_evidence is not None:
                        command += ['--adapter-evidence', str(adapter_evidence)]
            elif case == 'native-journey':
                pattern = '^(' + '|'.join(s.split(':')[1] for s in JOURNEYS if '/' not in s.split(':')[1]) + ')$'
                command = ['go', 'test', './pkg/manager', '-run', pattern, '-count=1', '-timeout=90s', '-json']
            elif case == 'discovery':
                command = ['python3', 'scripts/verification/discovery.py']
            elif case == 'verifier-controls':
                command = ['python3', '-m', 'unittest', 'discover', '-s', 'scripts/verification', '-p', 'test_*.py', '-v']
            else:
                command = GATES[case]
            result.update(command_result(command, log))
            if case == 'native-journey' and result['passed']:
                result['scenarios'] = retention.check_events(log.read_text(), JOURNEYS)
    except (ValueError, OSError, KeyError, TypeError, subprocess.SubprocessError) as error:
        result.update(passed=False, error=str(error))
        if isinstance(error, subprocess.CalledProcessError):
            result.update(exit_code=error.returncode, command=error.cmd)
    (directory / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    return result


def candidate_files():
    """Bind evidence to bytes, including unstaged and newly added source files."""
    names = subprocess.check_output(
        ['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=ROOT)
    hashes = {}
    for name in sorted(set(names.decode().split('\0')) - {''}):
        path = ROOT / name
        hashes[name] = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else 'missing'
    return hashlib.sha256(json.dumps(hashes, sort_keys=True).encode()).hexdigest()


def check_merge(report, revision, files_hash):
    """Validate a Console receipt, not authority to perform any delivery effect."""
    if not isinstance(report, dict) or report.get('status') != 'merged':
        raise ValueError('D2 incomplete: candidate-matched Console merge evidence required')
    if report.get('candidate_revision') != revision or report.get('candidate_files_sha256') != files_hash:
        raise ValueError('stale Console merge evidence')
    for field in ('console_revision', 'merge_revision', 'default_branch_revision'):
        if not isinstance(report.get(field), str) or not re.fullmatch('[0-9a-f]{40}', report[field]):
            raise ValueError('missing merge provenance: ' + field)
    for field in ('repository', 'default_branch', 'pull_request', 'owner_decision_ref',
                  'independent_review_ref', 'canonical_gate_ref', 'merge_ref'):
        if not isinstance(report.get(field), str) or not report[field].strip():
            raise ValueError('missing merge evidence: ' + field)
    if report['repository'] != 'openexec':
        raise ValueError('wrong merge repository')
    for field in ('candidate_in_default_branch', 'exact_candidate_reviewed',
                  'exact_candidate_approved', 'canonical_gates_passed'):
        if report.get(field) is not True:
            raise ValueError('unproven merge obligation: ' + field)
    if report.get('unresolved_findings') != []:
        raise ValueError('unresolved merge findings')
    return report


def execute(mode, output, adapter_evidence=None, merge_evidence=None):
    if mode not in ('delivery-ready', 'full', 'goal-complete'):
        raise ValueError('unknown delivery mode')
    selected = (*TECHNICAL, *GATES) if mode == 'full' else TECHNICAL
    result = dict(mode=mode, passed=False, d2='externally_pending', cases={})
    result['revision'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    result['worktree_changes'] = subprocess.check_output(['git', 'status', '--short'], cwd=ROOT, text=True)
    result['candidate_files_sha256'] = candidate_files()
    for case in selected:
        result['cases'][case] = (run_case(case, output, adapter_evidence)
                                 if case == 'admitted-all' and adapter_evidence is not None
                                 else run_case(case, output))
        # Persist failed and partial progress, never a premature success.
        (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    result['passed'] = set(result['cases']) == set(selected) and all(
        r.get('passed') is True for r in result['cases'].values())
    if candidate_files() != result['candidate_files_sha256']:
        result.update(passed=False, error='candidate changed during verification')
    result['adapter_adoption'] = 'deferred_after_merge' if adapter_evidence is None else 'supplied_report_checked'
    if mode == 'goal-complete' or merge_evidence is not None:
        try:
            external = json.loads(merge_evidence.read_text()) if merge_evidence else None
            result['merge_evidence'] = check_merge(external, result['revision'], result['candidate_files_sha256'])
            if result['passed']:
                result['d2'] = 'verified_external_merge'
        except (ValueError, OSError, TypeError) as error:
            result.update(passed=False, error=str(error))
    result['canonical_gates'] = 'recorded' if mode == 'full' else ('external_receipt_required' if mode == 'goal-complete' else 'required_in_full')
    (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case', required=True, choices=('full', 'delivery-ready', 'goal-complete'))
    parser.add_argument('--output', type=Path)
    parser.add_argument('--adapter-evidence', type=Path)
    parser.add_argument('--merge-evidence', type=Path)
    args = parser.parse_args()
    os.environ.setdefault('GOCACHE', '/tmp/openexec-retention-go-cache')
    output = args.output or Path(tempfile.mkdtemp(prefix='openexec-delivery-'))
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        parser.error('output must be empty; old evidence cannot be reused')
    print(f'Delivery evidence: {output}', flush=True)
    result = execute(args.case, output, args.adapter_evidence, args.merge_evidence)
    print(f'{args.case}: {"PASS" if result["passed"] else "FAIL"}; evidence: {output / "result.json"}')
    raise SystemExit(0 if result['passed'] else 1)


if __name__ == '__main__':
    main()
