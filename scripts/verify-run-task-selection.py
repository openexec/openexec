#!/usr/bin/env python3
"""Prove the run task-selection regression detects execute-all.

Included in make test. A Go overlay compiles an isolated source copy with only
the scheduler's narrowing to the selected tasks removed, which is the original
defect: runs:execute {"task_ids":["T-001"]} ran every pending task. Neither
checkout source nor the ledger fixture outside the test's temp dir changes.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "pkg/manager/scheduler.go"
TEST = [
    "go", "test", "-count=1", "-run",
    "^(TestRunsExecuteRunsOnlySelectedTask|TestRunsExecuteRunsSelectedSetInDependencyOrder)$",
    "-v", "./pkg/api",
]
CLI_TEST = [
    "go", "test", "-count=1", "-run",
    "^TestRunTaskIDExecutesOnlyThatTaskThroughRealEngine$",
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
guard = "\tif selection != nil {\n\t\tpending = selection\n\t}\n"
require(original.count(guard) == 1, f"FAIL: expected one mutation site: {guard!r}")
mutated = original.replace(guard, "\t_ = selection\n", 1)

with tempfile.TemporaryDirectory(prefix="openexec-run-selection-") as directory:
    copy = Path(directory) / "scheduler.go"
    copy.write_text(mutated)
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(copy)}}))
    negative = run([*TEST[:2], f"-overlay={overlay}", *TEST[2:]])
    require(
        negative.returncode == 1
        and "--- FAIL: TestRunsExecuteRunsOnlySelectedTask" in negative.stdout
        and "want exactly [T-001] (unrelated pending tasks were executed)" in negative.stdout
        and "recorder ran [T-001 T-002" in negative.stdout,
        "FAIL: negative control did not reproduce the unrelated task T-002 executing",
    )
    cli = run([*CLI_TEST[:2], f"-overlay={overlay}", *CLI_TEST[2:]])
    require(
        cli.returncode == 1
        and "recorder ran [T-001 T-UNRELATED]; want exactly [T-001]" in cli.stdout,
        "FAIL: CLI negative control did not reproduce `openexec run T-001` executing T-UNRELATED",
    )
require(SOURCE.read_text() == original, "FAIL: checkout source changed")
print("PASS: without the narrowing, run T-001 executes the unrelated pending task (API and CLI)", flush=True)
require(run(TEST).returncode == 0, "FAIL: corrected task-selection regression")
require(run(CLI_TEST).returncode == 0, "FAIL: corrected CLI task-selection regression")
print("PASS: corrected scheduler runs only the selected tasks; source unchanged", flush=True)
