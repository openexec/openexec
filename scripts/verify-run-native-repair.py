#!/usr/bin/env python3
"""Prove the run repair journey detects the non-repairing route.

Included in make test. A Go overlay compiles an isolated copy of start.go with
only the request for the task queue removed, which is the original defect:
`openexec run` reached the batch scheduler, which marks a task whose check
failed, and everything after it, "error" and creates no fix. Checkout source
never changes.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "internal/cli/start.go"
TEST = [
    "go", "test", "-count=1", "-run",
    "^(TestRunRepairsFailedCheckAndContinuesRemainingWork|TestRunTaskIDRepairsOnlyTheSelectedTask)$",
    "-v", "./internal/cli",
]


def run(command):
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
    lines = [l for l in result.stdout.splitlines() if not l[:4].isdigit()]
    print(f"$ {' '.join(command)}\n" + "\n".join(lines) + f"\n{result.stderr[-2000:]}", flush=True)
    return result


def require(condition, message):
    if not condition:
        raise SystemExit(message)


original = SOURCE.read_text()
# Fail closed if the implementation changes: never accept an unrelated failure
# (such as a compile error) as evidence that this regression detects the defect.
guard = '\t\t\t"task_oriented": true,\n'
require(original.count(guard) == 1, f"FAIL: expected one mutation site: {guard!r}")

with tempfile.TemporaryDirectory(prefix="openexec-run-native-repair-") as directory:
    copy = Path(directory) / "start.go"
    copy.write_text(original.replace(guard, '\t\t\t"task_oriented": false,\n', 1))
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(copy)}}))
    negative = run([*TEST[:2], f"-overlay={overlay}", *TEST[2:]])
    require(
        negative.returncode == 1
        and "--- FAIL: TestRunRepairsFailedCheckAndContinuesRemainingWork" in negative.stdout
        and "--- FAIL: TestRunTaskIDRepairsOnlyTheSelectedTask" in negative.stdout
        and negative.stdout.count("A was not closed with its fix after one attempt") == 2
        and "A status=error attempts=0" in negative.stdout,
        "FAIL: negative control did not reproduce A ending \"error\" without a fix",
    )
require(SOURCE.read_text() == original, "FAIL: checkout source changed")
print("PASS: on the batch route, run leaves the failed task \"error\" with no fix", flush=True)
require(run(TEST).returncode == 0, "FAIL: corrected run repair journey")
print("PASS: corrected run repairs the failed task and continues; source unchanged", flush=True)
