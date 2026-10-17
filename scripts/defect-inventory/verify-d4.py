#!/usr/bin/env python3
"""Reproduce D4 and run the regular regression against isolated pre-fix sources."""
import json
import pathlib
import re
import shutil
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parents[2]
result = subprocess.run(['bash', 'scripts/defect-inventory/reproduce.sh'], cwd=root,
                        text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
print(result.stdout, flush=True)
assert result.returncode == 1, f'reproducer exit {result.returncode}, expected 1'
assert re.search(r'^FIXED\?\s+D4\s', result.stdout, re.M), 'D4 still reproduces'

# Copy sources only; dependencies remain read-only. Never mutate the candidate.
with tempfile.TemporaryDirectory(prefix='openexec-d4-') as tmp:
    ui = pathlib.Path(tmp) / 'ui'
    shutil.copytree(root / 'ui', ui, ignore=shutil.ignore_patterns(
        'node_modules', 'dist', 'coverage', 'test-results', 'playwright-report'))
    (ui / 'node_modules').symlink_to((root / 'ui/node_modules').resolve())
    for name in ['src/hooks/useBlueprint.ts', 'src/components/chat/blueprint/ApprovalsPanel.tsx']:
        original = subprocess.check_output([
            'git', 'show', '59f009c690e859e2accd259cdadc5e00c4b34439:ui/' + name], cwd=root)
        (ui / name).write_bytes(original)
    report = pathlib.Path(tmp) / 'vitest.json'
    result = subprocess.run([
        'node', str(ui / 'node_modules/vitest/vitest.mjs'), 'run',
        'src/components/chat/blueprint/ApprovalsPanel.test.tsx',
        '--reporter=json', '--outputFile=' + str(report),
    ], cwd=ui)
    data = json.loads(report.read_text())
    failed = data['numFailedTests']
    assert result.returncode == 1 and failed > 0, 'original sources must fail regression assertions'
    for suite in data['testResults']:
        for case in suite['assertionResults']:
            if case['status'] == 'failed':
                print('Expected regression failure:', case['fullName'], flush=True)
    assert data['numTotalTests'] == 12 and data['numPassedTests'] > 0
    print(f'D4 negative control: {failed} regression tests failed against original sources (expected)', flush=True)
