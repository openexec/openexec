#!/usr/bin/env python3
"""Prove the run task-selection regression detects execute-all.

Included in make test. Go overlays compile isolated source copies with only a
narrowing to the selected tasks removed, which is the original defect:
runs:execute {"task_ids":["T-001"]} ran every pending task. The batch
scheduler and the task queue (the route `openexec run` takes) each narrow;
each mutation removes one. Neither checkout source nor the ledger fixture
outside the test's temp dir changes.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SCHEDULER = ROOT / "pkg/manager/scheduler.go"
QUEUE = ROOT / "pkg/manager/task_queue.go"
TEST = [
    "go", "test", "-count=1", "-run",
    "^(TestRunsExecuteRunsOnlySelectedTask|TestRunsExecuteRunsSelectedSetInDependencyOrder)$",
    "-v", "./pkg/api",
]
QUEUE_TEST = [
    "go", "test", "-count=1", "-run",
    "^TestRunsExecuteTaskOrientedRunsOnlySelectedInDependencyOrder$",
    "-v", "./pkg/api",
]
CLI_TEST = [
    "go", "test", "-count=1", "-run",
    "^(TestRunTaskIDExecutesOnlyThatTaskThroughRealEngine|TestRunTaskIDRepairsOnlyTheSelectedTask)$",
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


def mutate(source, guard, replacement, directory):
    original = source.read_text()
    # Fail closed if the implementation changes: never accept an unrelated
    # failure (such as a compile error) as evidence that the test detects it.
    require(original.count(guard) == 1, f"FAIL: expected one mutation site: {guard!r}")
    copy = Path(directory) / source.name
    copy.write_text(original.replace(guard, replacement, 1))
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(source): str(copy)}}))
    return original, f"-overlay={overlay}"


with tempfile.TemporaryDirectory(prefix="openexec-run-selection-") as directory:
    original, overlay = mutate(SCHEDULER, "\tif selection != nil {\n\t\tpending = selection\n\t}\n", "\t_ = selection\n", directory)
    negative = run([*TEST[:2], overlay, *TEST[2:]])
    require(
        negative.returncode == 1
        and "--- FAIL: TestRunsExecuteRunsOnlySelectedTask" in negative.stdout
        and "want exactly [T-001] (unrelated pending tasks were executed)" in negative.stdout
        and "recorder ran [T-001 T-002" in negative.stdout,
        "FAIL: negative control did not reproduce the unrelated task T-002 executing",
    )
require(SCHEDULER.read_text() == original, "FAIL: checkout source changed")
print("PASS: without the batch narrowing, task_ids T-001 executes the unrelated pending task", flush=True)

with tempfile.TemporaryDirectory(prefix="openexec-run-selection-queue-") as directory:
    original, overlay = mutate(QUEUE, "\tif taskIDs == nil {\n", "\tif true {\n", directory)
    negative = run([*QUEUE_TEST[:2], overlay, *QUEUE_TEST[2:]])
    require(
        negative.returncode == 1
        and "recorder ran [T-001 T-002" in negative.stdout
        and "want [T-001 T-003] in dependency order" in negative.stdout,
        "FAIL: queue negative control did not reproduce the unrelated task T-002 executing",
    )
    cli = run([*CLI_TEST[:2], overlay, *CLI_TEST[2:]])
    require(
        cli.returncode == 1
        and "recorder ran [T-001 T-UNRELATED]; want exactly [T-001]" in cli.stdout
        and "unselected B was touched" in cli.stdout,
        "FAIL: CLI negative control did not reproduce `openexec run` executing unselected tasks",
    )
require(QUEUE.read_text() == original, "FAIL: checkout source changed")
print("PASS: without the queue narrowing, run T-001 executes the unrelated task and run A runs B (API and CLI)", flush=True)

require(run(TEST).returncode == 0, "FAIL: corrected task-selection regression")
require(run(QUEUE_TEST).returncode == 0, "FAIL: corrected task-queue selection regression")
require(run(CLI_TEST).returncode == 0, "FAIL: corrected CLI task-selection regression")
print("PASS: corrected scheduler and queue run only the selected tasks; source unchanged", flush=True)
