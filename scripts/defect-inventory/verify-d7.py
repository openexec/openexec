#!/usr/bin/env python3
"""Reproduce D7 before/after without modifying the candidate worktree.

Run: python3 scripts/defect-inventory/verify-d7.py
Requires the inventory baseline commit locally, Git, Go, and writable temp space.
Fails on compilation/setup errors rather than treating them as a red control.
"""
import os
from pathlib import Path
import subprocess
import tempfile

BASELINE = "59f009c690e859e2accd259cdadc5e00c4b34439"
ROOT = Path(__file__).resolve().parents[2]
SOURCE = "internal/router/deterministic.go"
TEST = "internal/router/deterministic_test.go"


def run(args, cwd, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    print(result.stdout, end="", flush=True)
    print(f"exit {result.returncode}: {' '.join(args)}", flush=True)
    return result


with tempfile.TemporaryDirectory(prefix="openexec-d7-") as directory:
    snapshot = Path(directory)
    archive = snapshot / "baseline.tar"
    subprocess.run(["git", "archive", "--format=tar", f"--output={archive}", BASELINE],
                   cwd=ROOT, check=True)
    subprocess.run(["tar", "-xf", str(archive), "-C", directory], check=True)
    archive.unlink()
    # Read-only access to the real Git object database; all builds use the snapshot.
    env = os.environ.copy()
    env["GIT_DIR"] = subprocess.check_output(
        ["git", "rev-parse", "--absolute-git-dir"], cwd=ROOT, text=True).strip()
    env["GIT_WORK_TREE"] = directory

    def reproduce(expected):
        result = run(["bash", "scripts/defect-inventory/reproduce.sh"], snapshot, env)
        rows = [line.split()[:2] for line in result.stdout.splitlines()]
        assert [expected, "D7"] in rows, "D7 reproducer did not give expected result"
        assert result.returncode in (0, 1), "reproducer execution failed"
        if expected == "FIXED?":
            assert result.returncode == 1, "expected documented stale-row exit"

    command = ["go", "test", "-count=1", "-run",
               "^TestDeterministicRouter_ParseIntent_(ListObject|DiffWordBoundary)$",
               "./internal/router/"]
    print(f"BASELINE snapshot: {BASELINE} (reproducer's HEAD labels the source checkout)", flush=True)
    reproduce("REPRODUCED")
    (snapshot / TEST).write_bytes((ROOT / TEST).read_bytes())
    print("NEGATIVE CONTROL: candidate regression tests with baseline router", flush=True)
    red = run(command, snapshot, env)
    assert red.returncode == 1 and 'got "list_directory", want "general_chat"' in red.stdout, (
        "negative control must fail on the routing assertion, not a build/setup error")
    (snapshot / SOURCE).write_bytes((ROOT / SOURCE).read_bytes())
    print("FIXED: same tests with candidate router", flush=True)
    assert run(command, snapshot, env).returncode == 0, "fixed regression failed"
    reproduce("FIXED?")
    print("PASS D7: regression red/green and inventory REPRODUCED/FIXED?", flush=True)
