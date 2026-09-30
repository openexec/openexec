#!/usr/bin/env python3
"""Independent native recapture boundary proof, with reopened ledger snapshots."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile

from retention_story import ROOT, check_events

PREFIX = 'TestRecaptureBoundaries'
TERMINALS = ('exhausted', 'restart_remaining', 'restart_spent', 'unresolved',
             'launch_refusal', 'cancellation')
REQUIRED = [PREFIX + 'Terminal', *[PREFIX + 'Terminal/' + s for s in TERMINALS],
            PREFIX + 'Failure', PREFIX + 'SuccessCompletionGuard']
CHECKPOINTS = {
    **{PREFIX + 'Terminal/' + s: {'terminal', 'terminal_restart', 'terminal_stable'}
       for s in TERMINALS},
    PREFIX + 'Failure': {'failed_repair', 'failed_restart'},
    PREFIX + 'SuccessCompletionGuard': {'success_waiting', 'success_completed'},
}
MARKER = 'RECAPTURE_BOUNDARY_STATE '


def check_proof(events):
    scenarios = check_events(events, ['pkg/manager:' + s for s in REQUIRED])
    states = {}
    for line in events.splitlines():
        event = json.loads(line)
        output = event.get('Output', '')
        if MARKER not in output:
            continue
        test = event.get('Test')
        state = json.loads(output.split(MARKER, 1)[1])
        checkpoint = state['checkpoint']
        if test not in CHECKPOINTS or checkpoint not in CHECKPOINTS[test]:
            raise ValueError('unexpected persisted-state evidence')
        records = states.setdefault(test, {})
        if checkpoint in records:
            raise ValueError('duplicate persisted-state evidence')
        if not state.get('task') or not state.get('settings'):
            raise ValueError('missing persisted task snapshots')
        records[checkpoint] = state
    if {test: set(records) for test, records in states.items()} != CHECKPOINTS:
        raise ValueError('missing persisted-state checkpoint')
    return dict(scenarios=scenarios, persisted_states=states)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, help='new evidence directory (must not exist)')
    args = parser.parse_args()
    if args.output:
        output = args.output.resolve()
        output.mkdir(parents=True, exist_ok=False)
    else:
        output = Path(tempfile.mkdtemp(prefix='openexec-recapture-boundaries-'))
    print(f'Evidence: {output}', flush=True)
    command = ['go', 'test', './pkg/manager', '-run', '^TestRecaptureBoundaries',
               '-count=1', '-timeout=60s', '-json']
    log = output / 'tests.jsonl'
    with log.open('w') as stream:
        subprocess.run(command, cwd=ROOT, stdout=stream, check=True)
    proof = check_proof(log.read_text())
    (output / 'result.json').write_text(json.dumps(
        dict(passed=True, command=command, **proof), indent=2) + '\n')
    print(f'recapture-boundaries: PASS; evidence: {output / "result.json"}')


if __name__ == '__main__':
    main()
