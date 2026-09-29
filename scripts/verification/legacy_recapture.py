#!/usr/bin/env python3
"""Run the implementation-owned legacy recapture journeys, rejecting skips."""
import json
from pathlib import Path
import subprocess
import tempfile

from retention_story import ROOT, check_events

REQUIRED = [
    'TestLegacyRecaptureFailureReloadRepair',
    'TestLegacyRecaptureSuccessReload',
    'TestLegacyRecaptureTerminalReload',
    'TestLegacyRecaptureTerminalReload/exhausted',
    'TestLegacyRecaptureTerminalReload/unresolved',
    'TestLegacyRecaptureTerminalReload/refusal',
    'TestLegacyRecaptureTerminalReload/cancel',
    'TestLegacyRecaptureSuccessQueueConverges',
    'TestLegacyRecaptureInterruptedBudgetReload',
    'TestLegacyRecaptureAuthoritativeResolution',
]


def main():
    output = Path(tempfile.mkdtemp(prefix='openexec-legacy-recapture-'))
    command = ['go', 'test', './pkg/manager', '-run', '^TestLegacyRecapture',
               '-count=1', '-timeout=60s', '-json']
    log = output / 'tests.jsonl'
    print(f'Evidence: {output}', flush=True)
    with log.open('w') as stream:
        subprocess.run(command, cwd=ROOT, stdout=stream, check=True)
    scenarios = check_events(log.read_text(), ['pkg/manager:' + name for name in REQUIRED])
    (output / 'result.json').write_text(json.dumps(
        dict(passed=True, command=command, scenarios=scenarios), indent=2) + '\n')
    print('legacy-recapture: passed')


if __name__ == '__main__':
    main()
