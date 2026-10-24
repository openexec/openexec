#!/usr/bin/env python3
"""Prove the init -> daemon review regression detects a dropped review choice.

Included in make test. A Go overlay compiles an isolated source copy of the
daemon constructor with only the saved review_enabled forwarding removed;
checkout source is never changed. The tests drive real init, server.New, the
HTTP run route and the native blueprint with a labelled deterministic provider
fixture; no model is called.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "internal/server/server.go"
TEST = [
    "go", "test", "-count=1", "-run",
    "^(TestInitReviewChoiceDrivesDaemonReviewStage|TestInitReviewerModelSelectsDaemonReviewRunner|TestDaemonReviewMissingKeySkipsReview)$",
    "-v", "./internal/cli",
]


def run(command):
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
    print(f"$ {' '.join(command)}\n{result.stdout[-4000:]}{result.stderr[-4000:]}", flush=True)
    return result


def require(condition, message):
    if not condition:
        raise SystemExit(message)


original = SOURCE.read_text()


def negative_control(guard, replacement, failing_test, receipt_line):
    # Fail closed if the implementation changes: never accept an unrelated
    # failure (such as a compile error) as evidence of detecting the defect.
    require(original.count(guard) == 1, f"FAIL: expected one mutation site: {guard!r}")
    mutated = original.replace(guard, replacement, 1)
    with tempfile.TemporaryDirectory(prefix="openexec-native-review-") as directory:
        copy = Path(directory) / "server.go"
        copy.write_text(mutated)
        overlay = Path(directory) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(copy)}}))
        negative = run([*TEST[:2], f"-overlay={overlay}", *TEST[2:]])
    require(
        negative.returncode == 1
        and f"--- FAIL: {failing_test}" in negative.stdout
        and receipt_line in negative.stdout
        and "--- PASS: TestDaemonReviewMissingKeySkipsReview" in negative.stdout,
        f"FAIL: removing {guard!r} did not fail {failing_test} on the review stage",
    )


negative_control(
    "return pc.Execution.ReviewEnabled",
    "return false && pc.Execution.ReviewEnabled",
    "TestInitReviewChoiceDrivesDaemonReviewStage",
    'review on: provider receipts = ["claude|implement"], want ["claude|implement" "claude|review"]',
)
print("PASS: dropping the saved review choice makes the daemon skip the review stage", flush=True)
negative_control(
    "return pc.Execution.ReviewerModel",
    'return "" + pc.Execution.ReviewerModel[:0]',
    "TestInitReviewerModelSelectsDaemonReviewRunner",
    'codex reviewer: provider receipts = ["claude|implement" "claude|review"], want ["claude|implement" "codex|"]',
)
print("PASS: dropping the saved reviewer model runs review on the executor runner", flush=True)
require(SOURCE.read_text() == original, "FAIL: checkout source changed")
require(run(TEST).returncode == 0, "FAIL: corrected daemon review regression")
print("PASS: corrected daemon runs the saved review stage; source unchanged", flush=True)
