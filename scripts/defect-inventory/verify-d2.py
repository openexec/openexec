#!/usr/bin/env python3
"""Verify D2 and its negative control without modifying checkout sources.

Run from any directory with Python 3 and Go installed. If the sandbox cannot
build Go, run this same script through the console's run_command on the host.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[2]


def run(*args):
    result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True)
    print(f"$ {' '.join(args)}\n{result.stdout}{result.stderr}", flush=True)
    return result


def require(condition, message):
    if not condition:
        raise SystemExit(message)


test = ["go", "test", "-count=1", "-run", "^TestExecuteOutput$", "-v", "./internal/cli"]
require(run(*test).returncode == 0, "FAIL: fixed Execute regression test")

# Go's overlay removes only the fix for compilation; the source file on disk
# stays untouched even if this verification is interrupted.
source = ROOT / "internal/cli/root.go"
wiring = "\trootCmd.SetOut(os.Stdout)\n"
original = source.read_text()
require(original.count(wiring) == 1, "FAIL: expected exactly one stdout wiring line")
with tempfile.TemporaryDirectory(prefix="openexec-d2-") as directory:
    mutated = Path(directory) / "root.go"
    mutated.write_text(original.replace(wiring, "", 1))
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(source): str(mutated)}}))
    negative = run("go", "test", f"-overlay={overlay}", *test[2:])
    require(
        negative.returncode != 0
        and "--- FAIL: TestExecuteOutput/version" in negative.stdout
        and 'got stdout="" stderr="OpenExec CLI' in negative.stdout,
        "FAIL: negative control did not fail for the missing stdout wiring",
    )
require(source.read_text() == original, "FAIL: source changed during verification")
print("PASS: negative control detects missing SetOut; source unchanged", flush=True)

# Check execution success before the legacy reproducer, which suppresses Go
# errors and could otherwise mistake a build failure for empty CLI output.
version = run("go", "run", "./cmd/openexec", "version")
require(version.returncode == 0, "FAIL: CLI did not run")
require(version.stdout.startswith("OpenExec CLI v") and not version.stderr,
        "FAIL: CLI version output is not exclusively on stdout")
print(f"PASS: CLI version produced {len(version.stdout.encode())} stdout bytes", flush=True)
inventory = run("bash", "scripts/defect-inventory/reproduce.sh")
require(inventory.returncode == 1, "FAIL: expected inventory exit 1 for fixed D2")
require(any(line.split()[:2] == ["FIXED?", "D2"]
            for line in inventory.stdout.splitlines()), "FAIL: D2 did not flip to FIXED?")
print("PASS: D2 reproducer reports FIXED?", flush=True)
