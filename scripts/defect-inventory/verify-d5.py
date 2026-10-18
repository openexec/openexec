#!/usr/bin/env python3
"""Reproduce D5's regular-test negative control without modifying source files."""
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path(__file__).resolve().parents[2]
ui = root / 'ui'
baseline = 'a6afc8a0b1fc1c807bbe56c6e59f07dfd13e9a23'
command = ['node', 'node_modules/vitest/vitest.mjs', 'run', '--maxWorkers=1', '--minWorkers=1']
with tempfile.TemporaryDirectory(prefix='d5-baseline-', dir=ui / 'src/pages') as folder:
    target = Path(folder)
    source = subprocess.check_output(['git', 'show', f'{baseline}:ui/src/pages/KnowledgeHub.tsx'], cwd=root)
    (target / 'KnowledgeHub.tsx').write_bytes(source)
    shutil.copyfile(ui / 'src/pages/KnowledgeHub.test.tsx', target / 'KnowledgeHub.test.tsx')
    result = subprocess.run(command + [str(target.relative_to(ui) / 'KnowledgeHub.test.tsx')], cwd=ui, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    print(result.stdout)
    if result.returncode != 1 or '4 failed' not in result.stdout or 'Some knowledge endpoints failed to load' not in result.stdout:
        raise SystemExit('D5 baseline did not produce the expected assertion failures')
print('PROVEN: D5 regular regression fails on original page')
subprocess.run(command + ['src/pages/KnowledgeHub.test.tsx'], cwd=ui, check=True)
print('PROVEN: D5 regular regression passes on current page')
subprocess.run(['node', 'node_modules/typescript/bin/tsc', '--noEmit'], cwd=ui, check=True)
subprocess.run(['node', 'node_modules/@playwright/test/cli.js', 'test', '--config=playwright.d5.config.ts'], cwd=ui, check=True)
print('PROVEN: D5 browser navigation, failure, Retry, reopen and reload')
