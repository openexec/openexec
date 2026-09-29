#!/usr/bin/env python3
"""Fresh technical delivery proof. Neither mode certifies external D2."""
import argparse
import contextlib
import json
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
TECHNICAL = (*retention.CASES, *recapture.CASES, 'native-journey', 'discovery', 'verifier-controls')
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


def run_case(case, output):
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
            if case == 'native-journey':
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


def execute(mode, output):
    selected = TECHNICAL if mode == 'delivery-ready' else (*TECHNICAL, *GATES)
    result = dict(mode=mode, passed=False, d2='externally_pending', cases={})
    result['revision'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    result['worktree_changes'] = subprocess.check_output(['git', 'status', '--short'], cwd=ROOT, text=True)
    for case in selected:
        result['cases'][case] = run_case(case, output)
        # Persist failed and partial progress, never a premature success.
        (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    result['passed'] = set(result['cases']) == set(selected) and all(
        r.get('passed') is True for r in result['cases'].values())
    result['canonical_gates'] = 'required_in_full' if mode == 'delivery-ready' else 'recorded'
    (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case', required=True, choices=('full', 'delivery-ready'))
    args = parser.parse_args()
    output = Path(tempfile.mkdtemp(prefix='openexec-delivery-'))
    print(f'Delivery evidence: {output}', flush=True)
    result = execute(args.case, output)
    print(f'{args.case}: {"PASS" if result["passed"] else "FAIL"}; evidence: {output / "result.json"}')
    raise SystemExit(0 if result['passed'] else 1)


if __name__ == '__main__':
    main()
