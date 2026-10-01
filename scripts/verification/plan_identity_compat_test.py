"""Fail-fast driver controls; public journey assertions live in the Go tests."""
import contextlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from plan_identity_compat import main
from plan_identity_discovery import ROOT


class CompatibilityControls(unittest.TestCase):
    def test_missing_or_skipped_compatibility_test_refuses(self):
        names = ['TestCompatibility_ExistingProjects_StatusCLI',
                 'TestCompatibility_LegacyProjectConfigFallback',
                 'TestCompatibility_LegacyTasksJSONFallback']
        good = '\n'.join('--- PASS: ' + n + ' (0.1s)' for n in names)
        for output in ['', good.replace(names[0], 'TestWrong'),
                       good + '\n    --- SKIP: TestCompatibility_ExistingProjects_StatusCLI/legacy']:
            with patch('plan_identity_compat.subprocess.run', return_value=
                       subprocess.CompletedProcess([], 0, output, '')):
                with contextlib.redirect_stdout(io.StringIO()), self.assertRaises(ValueError):
                    main('compat-test')
        with patch('plan_identity_compat.subprocess.run', return_value=
                   subprocess.CompletedProcess([], 0, good, '')):
            with contextlib.redirect_stdout(io.StringIO()):
                main('compat-test')

    def test_empty_targeted_execution_refuses(self):
        with patch('plan_identity_compat.subprocess.run', return_value=
                   subprocess.CompletedProcess([], 0, '', '')):
            with contextlib.redirect_stdout(io.StringIO()), self.assertRaises(ValueError):
                main('targeted')

    def test_shell_stops_at_each_failed_command(self):
        # Exercise the actual shell entry point with stubbed expensive commands.
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'scripts').mkdir()
            script = root / 'scripts/verify-plan-identity-compat.sh'
            shutil.copy2(ROOT / 'scripts/verify-plan-identity-compat.sh', script)
            shim = root / 'shim'
            shim.write_text('#!/bin/bash\n'
                            'echo "$*" >> "$DRIVER_LOG"\n'
                            'if [[ -n "$DRIVER_FAIL" && "$*" == "$DRIVER_FAIL" ]]; then exit 42; fi\n')
            shim.chmod(0o755)
            for name in ['python3', 'bash', 'make']:
                (root / name).symlink_to(shim)
            env = dict(os.environ, PATH=str(root) + ':' + os.environ['PATH'],
                       DRIVER_LOG=str(root / 'commands'), DRIVER_FAIL='')
            passed = subprocess.run(['/bin/bash', str(script)], env=env, capture_output=True, text=True)
            self.assertEqual(passed.returncode, 0, passed.stderr)
            commands = (root / 'commands').read_text().splitlines()
            self.assertEqual(len(commands), 9)  # controls, seven phases, restored journey
            self.assertEqual(commands[1], commands[-1])
            self.assertIn('complete plan identity compatibility', passed.stdout)
            for i, command in enumerate(commands[:-1]):
                (root / 'commands').write_text('')
                env['DRIVER_FAIL'] = command
                failed = subprocess.run(['/bin/bash', str(script)], env=env, capture_output=True, text=True)
                self.assertEqual(failed.returncode, 42, command)
                self.assertEqual((root / 'commands').read_text().splitlines(), commands[:i + 1])
                self.assertNotIn('complete plan identity compatibility', failed.stdout)
            invalid = subprocess.run(['/bin/bash', str(script), 'unknown'], env=env, capture_output=True)
            self.assertEqual(invalid.returncode, 2)


if __name__ == '__main__':
    unittest.main()
