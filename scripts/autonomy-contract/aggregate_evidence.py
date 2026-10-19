#!/usr/bin/env python3
"""Compose existing verifiers; a failed child always blocks aggregate success."""
import argparse
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/autonomy-contract/verify-runtime-evidence.sh'
AGGREGATES = {
    'native-recovery': ('native-recovery-tracer', 'recovery-matrix', 'compatibility-refusal'),
    'engine-recovery': ('behavioral-reproduction', 'runtime-boundary', 'exit-1-slice',
                        'native-recovery', 'implementation-unit-coverage'),
    'story-evidence': ('engine-recovery', 'harness-tests'),
}


def leaves(case):
    result = []
    for child in AGGREGATES[case]:
        for leaf in leaves(child) if child in AGGREGATES else (child,):
            if leaf not in result:
                result.append(leaf)
    return result


def verify(case):
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    # Bind the run to source bytes as well as HEAD, including untracked files.
    before = subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=ROOT)
    if before:
        raise ValueError('aggregate verification requires a clean committed candidate')
    print(f'{case}: candidate {revision}', flush=True)
    for leaf in leaves(case):
        command = (['python3', '-B', '-m', 'unittest', 'discover', '-s',
                    'scripts/autonomy-contract', '-p', 'test_*.py'] if leaf == 'harness-tests'
                   else ['bash', '-euo', 'pipefail', str(SCRIPT), '--case', leaf])
        print('COMMAND: ' + ' '.join(command), flush=True)
        subprocess.run(command, cwd=ROOT, check=True)
        print(f'RESULT: {leaf}: PASS', flush=True)
    after = subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=ROOT)
    final_revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    if after or final_revision != revision:
        raise ValueError('candidate changed during aggregate verification')
    print(f'{case}: PASS candidate {revision}', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case', required=True, choices=AGGREGATES)
    args = parser.parse_args()
    verify(args.case)


if __name__ == '__main__':
    main()
