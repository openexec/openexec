#!/usr/bin/env python3
"""Compose local US-012 proofs without treating a fixture as Console adoption."""
import argparse
import json
import os
import re
from pathlib import Path
import subprocess
import sys
import tempfile

from retention_story import check_events

ROOT = Path(__file__).resolve().parents[2]
CASES = ('admitted-tracer', 'admitted-adapter', 'admitted-unit-coverage')
ADAPTER_CASES = {
    f'{gate}/{kind}/nil={nil}'
    for gate in ('lint', 'test') for kind in ('silent', 'diagnostic')
    for nil in ('false', 'true')
}
ADOPTION_ERROR = ('Actual Console adapter adoption is unverified: supply a candidate-bound '
                  'external report with --adapter-evidence. Local WithOutput compatibility '
                  'does not prove exact argv/cwd/private-reference retention.')

DEFERRED_ADOPTION = ('Console adopts RetainCommandEvidence and VerificationCommandFailureWithEvidence '
                     'in executeOpenExecCheck after this OpenExec change merges; the API does not '
                     'exist on OpenExec main before then.')


def check_adoption(report, revision):
    """Check an externally supplied report, never synthesize external success."""
    if not isinstance(report, dict) or not report:
        raise ValueError(ADOPTION_ERROR)
    if report.get('candidate_revision') != revision or report.get('status') != 'passed':
        raise ValueError('stale or unsuccessful Console adapter evidence')
    for field in ('console_revision', 'openexec_dependency', 'source_sha256', 'command'):
        if not report.get(field):
            raise ValueError('missing adapter provenance: ' + field)
    if not re.fullmatch(r'[0-9a-f]{40}', report['console_revision']):
        raise ValueError('invalid Console revision')
    hashes = report['source_sha256']
    if not isinstance(hashes, dict) or not hashes or any(
            not isinstance(v, str) or not re.fullmatch(r'[0-9a-f]{64}', v) for v in hashes.values()):
        raise ValueError('invalid Console source hashes')
    if not isinstance(report['command'], list) or not all(isinstance(v, str) and v for v in report['command']):
        raise ValueError('invalid actual adapter command')
    if report.get('exit_code') != 0 or report.get('unresolved_work') != []:
        raise ValueError('unresolved or failed actual adapter work')
    scenarios = report.get('scenarios', {})
    if not isinstance(scenarios, dict) or set(scenarios) != ADAPTER_CASES:
        raise ValueError('missing or unexpected actual adapter scenarios')
    obligations = {'exact_argv', 'workdir', 'exit_2', 'private_reference',
                   'database_reopen', 'repair_reload', 'redacted_public_output', 'diagnostic_tail'}
    for scenario in scenarios.values():
        if not isinstance(scenario, dict) or set(scenario) != obligations or any(v is not True for v in scenario.values()):
            raise ValueError('missing actual adapter reload or evidence assertion')
    coverage = report.get('coverage', {})
    if not isinstance(coverage, dict):
        raise ValueError('invalid adapter coverage')
    covered, total = coverage.get('covered', 0), coverage.get('statements', 0)
    if not (type(covered) is int and type(total) is int and 0 < total >= covered and 10 * covered > 9 * total):
        raise ValueError('actual adapter scope coverage must exceed 90%')
    return report


def adapter_journey(output):
    package = 'pkg/manager'
    name = 'TestAdmittedAdapterFailureReloadAndRepair'
    required = [package + ':' + name] + [
        f'{package}:{name}/{gate}/noisy={noisy}/nil={nil}'
        for gate in ('lint', 'test') for noisy in ('false', 'true') for nil in ('false', 'true')]
    command = ['go', 'test', './' + package, '-run', '^' + name + '$', '-count=1', '-timeout=60s', '-json']
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
    (output / 'adapter.jsonl').write_text(result.stdout)
    if result.returncode or result.stderr.strip():
        raise ValueError(result.stdout + result.stderr)
    return check_events(result.stdout, required)


def execute(output, adapter_evidence=None):
    output.mkdir(parents=True, exist_ok=True)
    report_path = output / 'result.json'
    report_path.unlink(missing_ok=True)
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    report = dict(status='failed', candidate_revision=revision, local_checks={}, adapter_adoption={})
    for case in CASES:
        command = ['bash', 'scripts/verify-verification-repair.sh', '--case', case]
        if case == 'admitted-unit-coverage':
            command = [sys.executable, 'scripts/verification/admitted_unit_coverage.py', '--output', str(output / 'coverage')]
        if case == 'admitted-adapter':
            try:
                report['adapter_compatibility_scenarios'] = adapter_journey(output)
                report['local_checks'][case] = dict(exit_code=0, log=str(output / 'adapter.jsonl'))
            except ValueError as error:
                report['local_checks'][case] = dict(exit_code=1, error=str(error))
            report_path.write_text(json.dumps(report, indent=2) + '\n')
            continue
        log = output / (case + '.log')
        with log.open('w') as stream:
            result = subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
        report['local_checks'][case] = dict(command=command, exit_code=result.returncode, log=str(log))
        report_path.write_text(json.dumps(report, indent=2) + '\n')
    local_passed = all(r['exit_code'] == 0 for r in report['local_checks'].values())
    if adapter_evidence is None:
        # The public evidence API exists only in this candidate until it merges,
        # so Console cannot adopt it first. Adoption is follow-up Console work
        # after the merge; it is recorded as deferred, never as passed.
        report['adapter_adoption'] = dict(status='deferred', follow_up=DEFERRED_ADOPTION)
        if local_passed:
            report['status'] = 'passed'
    else:
        try:
            external = json.loads(adapter_evidence.read_text())
            report['adapter_adoption'] = check_adoption(external, revision)
            if local_passed:
                report['status'] = 'passed'
        except (ValueError, OSError, TypeError) as error:
            report['adapter_adoption'] = dict(status='incomplete', error=str(error))
    report_path.write_text(json.dumps(report, indent=2) + '\n')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--adapter-evidence', type=Path,
                        help='Externally verified Console adapter report; schema in check_adoption')
    args = parser.parse_args()
    os.environ.setdefault('GOCACHE', '/tmp/openexec-retention-go-cache')
    output = args.output or Path(tempfile.mkdtemp(prefix='openexec-admitted-story-'))
    print('Admitted story evidence: ' + str(output), flush=True)
    report = execute(output.resolve(), args.adapter_evidence)
    print(json.dumps(report, indent=2))
    raise SystemExit(0 if report['status'] == 'passed' else 1)


if __name__ == '__main__':
    main()
