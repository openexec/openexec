"""Fresh public-path journeys, with mandatory execution rather than exit-only proof."""
import json
import re
import subprocess
import sys

from plan_identity_discovery import ROOT, MANIFEST
from reviewed_plan_identity import check_execution


def main(phase):
    if phase == 'targeted':
        required = json.loads((ROOT / MANIFEST).read_text())['existing_tests']
        names = sorted({test.split(':', 1)[1] for test in required})
        command = ['go', 'test', './pkg/manager', './internal/release', './internal/planner',
                   '-count=1', '-timeout=120s', '-json', '-run', '^(' + '|'.join(names) + ')$']
    elif phase == 'compat-test':
        required = ['internal/validation:' + test for test in (
            'TestCompatibility_ExistingProjects_StatusCLI',
            'TestCompatibility_LegacyProjectConfigFallback',
            'TestCompatibility_LegacyTasksJSONFallback')]
        command = ['make', 'compat-test']
    else:
        raise ValueError('unknown phase: ' + phase)
    print('+ ' + ' '.join(command), flush=True)
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise ValueError(result.stdout + result.stderr)
    if phase == 'targeted':
        events = [json.loads(line) for line in result.stdout.splitlines()]
    else:
        print(result.stdout, end='')
        events = [dict(Action={'PASS': 'pass', 'SKIP': 'skip', 'FAIL': 'fail'}[action],
                       Package='internal/validation', Test=test)
                  for action, test in re.findall(r'^\s*--- (PASS|SKIP|FAIL): (\S+)', result.stdout, re.M)]
    check_execution(events, required)
    for test in required:
        print('PASS ' + test)


if __name__ == '__main__':
    main(sys.argv[1])
