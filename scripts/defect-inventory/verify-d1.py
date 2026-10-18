#!/usr/bin/env python3
"""Prove the ordinary D1 regression fails on the original code and passes now.

Run from any directory with python3 scripts/defect-inventory/verify-d1.py.
Also runs first in make test (the host's declared test check).
Uses a temporary Go overlay; never modifies repository sources. Requires the
inventory baseline in local git history and writable Go build/cache directories.
"""
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
BASELINE = "59f009c690e859e2accd259cdadc5e00c4b34439"
SOURCE = "internal/loop/api_runner.go"


def run_test(extra):
    result = subprocess.run(
        ["go", "test", *extra, "-count=1", "-run",
         "^TestAPIFileToolsWorkspaceBoundary$", "-v", "./internal/loop/"],
        cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
    )
    print(result.stdout, end="", flush=True)
    return result


with tempfile.TemporaryDirectory(prefix="openexec-d1-") as directory:
    temp = Path(directory)
    original = subprocess.check_output(
        ["git", "show", f"{BASELINE}:{SOURCE}"], cwd=ROOT
    )
    replacement = temp / "api_runner.go"
    replacement.write_bytes(original)
    overlay = temp / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(ROOT / SOURCE): str(replacement)}}))
    print(f"Original resolver from {BASELINE}", flush=True)
    before = run_test(["-overlay", str(overlay)])
    # A build/tool failure is not regression evidence. Require the actual test
    # to fail and demonstrate successful unauthorized reads AND writes.
    required = [
        "--- FAIL: TestAPIFileToolsWorkspaceBoundary",
        'escape must be refused: output="secret" err=<nil>',
        'escape must be refused: output="Wrote 9 bytes to ../secret.txt" err=<nil>',
    ]
    if before.returncode != 1 or not all(s in before.stdout for s in required):
        raise SystemExit("FAIL: original-code regression failure was not demonstrated")
    print("PROVEN: original code permits outside reads and writes", flush=True)
    print("Current resolver", flush=True)
    after = run_test([])
    if after.returncode != 0 or "--- PASS: TestAPIFileToolsWorkspaceBoundary" not in after.stdout:
        raise SystemExit("FAIL: current-code regression did not pass")
    print("PROVEN: current code passes the same ordinary regression", flush=True)
