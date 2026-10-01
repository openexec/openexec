#!/usr/bin/env python3
"""Build the shipped CLI and falsify its authorization and decision binding."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
source = root / 'internal/cli/task_correct.go'
record = root / 'docs/verification/task-correct-transcript.txt'
env = dict(os.environ)
env.setdefault('GOCACHE', '/tmp/openexec-correction-go-cache')
(root / '.openexec').mkdir(exist_ok=True)
lines = []

def run(argv, expected=0, marker=None):
    p = subprocess.run(argv, cwd=root, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    lines.append('$ ' + ' '.join(argv) + '\nexit=' + str(p.returncode))
    # Retain the public command transcript and explicit persistence/falsifier evidence.
    lines.extend(line for line in p.stdout.splitlines() if any(key in line for key in
        ('PUBLIC_COMMAND', 'Authorized task', 'RELOADED_CORRECTION', 'PASS', 'FAIL',
         'binding mismatch', 'TaskQueueBoundary', 'Error:', 'ok\t')))
    if (p.returncode == 0) != (expected == 0) or (marker and marker not in p.stdout):
        raise RuntimeError(p.stdout)

with tempfile.TemporaryDirectory(prefix='task-correct-', dir=root / '.openexec') as tmp:
    tmp = Path(tmp)
    env['TMPDIR'] = str(tmp)
    binary = tmp / 'openexec'
    env['OPENEXEC_CORRECTION_BINARY'] = str(binary)
    run(['go', 'build', '-o', str(binary), './cmd/openexec'])
    run(['go', 'test', './pkg/manager', '-run', '^TestTaskCorrectCLI$', '-count=1', '-v'])
    original = source.read_text()
    for name, old, new, marker in [
        ('skip-authorization', 'mgr.AuthorizeTaskCorrection(cmd.Context(), c)',
         'func() error { return nil }()', 'A failed 3/3 with TaskQueueBoundary'),
        ('change-decision', 'DecisionRef: decision,', 'DecisionRef: decision + "-mutated",',
         'exact decision binding mismatch'),
    ]:
        assert original.count(old) == 1
        mutated = tmp / (name + '.go')
        mutated.write_text(original.replace(old, new))
        overlay = tmp / (name + '.json')
        overlay.write_text(json.dumps({'Replace': {str(source): str(mutated)}}))
        run(['go', 'build', '-overlay', str(overlay), '-o', str(binary), './cmd/openexec'])
        run(['go', 'test', './pkg/manager', '-run', '^TestTaskCorrectCLI$/(planned|no-plan)$/^$', '-count=1', '-v'], 1, marker)
    run(['go', 'build', '-o', str(binary), './cmd/openexec'])
    run(['go', 'test', './internal/cli', './pkg/manager', '-run', '^TestTaskCorrect', '-count=1', '-v'])
record.write_text('\n'.join(lines) + '\n')
print('PASS: public CLI scratch journeys, SQLite reopen, two compiled mutations, restored-source rerun')
