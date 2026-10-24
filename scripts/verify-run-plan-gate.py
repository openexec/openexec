#!/usr/bin/env python3
"""Prove the run plan-gate regression detects execute-after-failed-plan.

Included in make test. A Go overlay compiles an isolated copy of start.go with
only the runs:plan response check removed (the original behaviour: any reply
that is not a transport error is followed by runs:execute); checkout source is
never changed.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "internal/cli/start.go"
FAILURES = "^(TestRunStopsWhenAutoPlanningFailsOrIsRejected|TestRunRealHandlerRefusesFailedPlanBeforeExecute)$"
CONTROLS = "^(TestRunAcceptedPlanAndNoAutoPlanStillExecute|TestRunNormalInvocationsUnchanged)$"


def test(pattern, *extra):
    return ["go", "test", "-count=1", *extra, "-run", pattern, "-v", "./internal/cli"]


def run(command):
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
    print(f"$ {' '.join(command)}\n{result.stdout}{result.stderr}", flush=True)
    return result


def require(condition, message):
    if not condition:
        raise SystemExit(message)


original = SOURCE.read_text()
# Fail closed if the implementation changes: never accept an unrelated failure
# (such as a compile error) as evidence that this regression detects the defect.
start = "\t\t\t// A failed or rejected plan stops here"
end = '\t\t\t\treturn fmt.Errorf("auto-planning failed: server returned no plan; nothing was executed")\n\t\t\t}\n'
require(original.count(start) == 1 and original.count(end) == 1, "FAIL: expected one plan-gate mutation site")
head, rest = original.split(start, 1)
_, tail = rest.split(end, 1)
mutated = head + "\t\t\tresp.Body.Close()\n\t\t\tvar _ manager.PlanResult\n" + tail

failing = [
    "valid_false_with_issues",
    "valid_false_with_a_rejected_plan_present",
    "400_input_error",
    "500_planner_error",
    "non-JSON_error_body",
    "200_with_a_non-JSON_body",
    "200_with_an_empty_body",
    "valid_true_without_a_plan",
]

with tempfile.TemporaryDirectory(prefix="openexec-run-plan-gate-") as directory:
    copy = Path(directory) / "start.go"
    copy.write_text(mutated)
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(copy)}}))
    negative = run(test(FAILURES, f"-overlay={overlay}"))
    require(negative.returncode == 1, "FAIL: negative control did not fail")
    for case in failing:
        require(
            f"--- FAIL: TestRunStopsWhenAutoPlanningFailsOrIsRejected/{case}" in negative.stdout,
            f"FAIL: negative control did not fail the {case} case",
        )
    require(
        negative.stdout.count("failed plan was followed by 1 execute request(s)") == len(failing),
        "FAIL: fixture cases did not fail on the execute request",
    )
    require(
        negative.stdout.count("real handler path sent 1 execute request(s) after a failed plan") == 2,
        "FAIL: real handler cases did not fail on the execute request",
    )
    # Transport errors were already refused before this fix.
    require(
        "--- PASS: TestRunStopsWhenAutoPlanningFailsOrIsRejected/connection_dropped" in negative.stdout,
        "FAIL: dropped-connection case changed behaviour under the mutation",
    )
    controls = run(test(CONTROLS, f"-overlay={overlay}"))
    require(controls.returncode == 0, "FAIL: controls do not pass on the original behaviour")
require(SOURCE.read_text() == original, "FAIL: checkout source changed")
print("PASS: removing the plan gate sends runs:execute after every failed or rejected plan", flush=True)
require(run(test(FAILURES)).returncode == 0, "FAIL: corrected plan gate regression")
require(run(test(CONTROLS)).returncode == 0, "FAIL: corrected plan gate controls")
print("PASS: corrected run stops before execute; valid plan and --no-auto-plan unchanged", flush=True)
