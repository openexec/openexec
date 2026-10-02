#!/usr/bin/env python3
"""Run fresh native journeys; a missing, skipped or zero-test run fails closed."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
MODES = {
    "admission-completion": {
        "internal/release": ["TestCorrectionAdmissionPersistence"],
        "pkg/manager": ["TestCorrectionCompletionObligations", "TestCorrectionNativeQueueSuccess", "TestCorrectionDiagnosticQueueSuccess"],
    },
    "native-queue": {
        "pkg/manager": ["TestCorrectionNativeQueueSuccess", "TestCorrectionNativeQueueRefusals",
                        "TestCorrectionNativeStopAndFailure"],
    },
}

MODES["exhaustion-controls"] = {
    "internal/release": ["TestCorrectionAdmissionPersistence"],
    "pkg/manager": [
        "TestCorrectionExhaustionControls",
        *["TestCorrectionExhaustionControls/" + case for case in (
            "continuing_failure", "unchanged_failure", "no_authority", "blocked_dependents", "independent_drain")],
        "TestCorrectionExecutionControls",
        *["TestCorrectionExecutionControls/" + case for case in (
            "denied_effects", "agent_claim", "cancel_after_admission", "cancel_completion", "stop_completion", "unmet_validation")],
        "TestCorrectionNativeStopAndFailure/stop",
        "TestCorrectionCompletionObligations",
    ],
}
# Preserve required subcases, not just parent tests that pass with an empty table.
MODES["native-queue"]["pkg/manager"] += [
    "TestCorrectionNativeQueueRefusals/" + case for case in (
        "absent", "candidate", "task_binding", "review", "dependency", "failed_check",
        "cancel", "admitted_restart", "admitted_failure_restart", "stale_plan")]
MODES["native-queue"]["pkg/manager"] += [
    "TestCorrectionNativeStopAndFailure/" + case for case in ("stop", "check_failure", "drift")]
MODES["acceptance"] = {}
for mode in list(MODES.values()):
    for package, tests in mode.items():
        MODES["acceptance"].setdefault(package, [])
        MODES["acceptance"][package] = sorted(set(MODES["acceptance"][package]) | set(tests))


# The final acceptance gate also pins the refusal/coverage journeys added here.
MODES["acceptance"]["internal/release"] += [
    "TestCorrectionEligibilityRefusesLostStory",
    "TestCorrectionInvalidAuthorityAndClosedStore", "TestCorrectionTaskMoveRoundTrip",
    "TestCorrectionRepairRollbackAndHistory", "TestCorrectionCreationMetadataAndBulkRollback",
]
MODES["acceptance"]["pkg/manager"] += [
    "TestCorrectionOptionalChecksAndTimeout",
    "TestCorrectionRefusesLostStoreAndWriter", "TestCorrectionLateEventStatus",
    "TestCorrectionCandidateInputs", "TestCorrectionPlanRefusalCoverage",
    "TestCorrectionQueueAdmissionErrors", "TestCorrectionLegacyHumanBoundary",
    "TestCorrectionEvidenceWriteFailures",
]
for test, cases in {
    "TestCorrectionCandidateInputs": "missing not_git subdirectory detached unborn deleted internal_link external_link broken_link directory_link mode bytes",
    "TestCorrectionPlanRefusalCoverage": "missing_plan wrong_hash unsupported empty not_accepted named optional",
    "TestCorrectionQueueAdmissionErrors": "parallel active_queue active_pipeline missing_task_wait missing_task_retry invalid_authority consumed missing_receipt",
    "TestCorrectionEvidenceWriteFailures": "step link disposition",
}.items():
    MODES["acceptance"]["pkg/manager"] += [test + "/" + case for case in cases.split()]
MODES["acceptance"]["internal/release"] += [
    "TestCorrectionRepairRollbackAndHistory/" + case for case in
    "missing metadata dependencies story_tasks completed_story inherited_branch insert_failure update_failure closed".split()
]


def validate(events, required):
    runs, passes, packages = set(), set(), set()
    reloaded = []
    output_by_test = {}
    for event in events:
        action, package, test = event.get("Action"), event.get("Package"), event.get("Test")
        if action in ("skip", "fail"):
            raise ValueError(f"verification {action}: {package}:{test}")
        key = (package, test)
        if action == "run":
            if key in runs:
                raise ValueError(f"duplicate test: {key}")
            runs.add(key)
        if action == "pass":
            if test:
                passes.add(key)
            else:
                packages.add(package)
        output_by_test[key] = output_by_test.get(key, "") + event.get("Output", "")
    # test2json may split one long persisted snapshot over several Output
    # events. Reassemble per test before inspecting or publishing the evidence.
    for output in output_by_test.values():
        for line in output.splitlines():
            if "RELOADED_EXHAUSTION " in line:
                task = json.loads(line.split("RELOADED_EXHAUSTION ", 1)[1])
                if task["attempt_count"] != task["max_attempts"] or task["metadata"]["verification_failure_evidence"] != "legacy":
                    raise ValueError("exhaustion lost original history/receipt")
                c = task["metadata"].get("task_correction")
                if c is not None and (not c["consumed"] or c["outcome"] != "continuing_failure" or not c["fresh_evidence_id"] or not c["reason"]):
                    raise ValueError("missing continuing-failure disposition")
                if c is None and task["metadata"]["exhaustion"]["outcome"] != "attempt_limit":
                    raise ValueError("missing exhaustion disposition")
                reloaded.append(line.strip())
            if "RELOADED_FAILURE " in line:
                receipt = json.loads(line.split("RELOADED_FAILURE ", 1)[1])
                if receipt["Status"] != "failed":
                    raise ValueError("missing persisted fresh failure")
                reloaded.append(line.strip())
            if "RELOADED_CORRECTION " in line:
                payload = line.split("RELOADED_CORRECTION ", 1)[1]
                persisted = json.loads(payload)
                task = persisted["task"]
                correction = task["metadata"]["task_correction"]
                if (task["attempt_count"], task["max_attempts"], correction["consumed"]) != (3, 3, True):
                    raise ValueError("persisted correction history/consumption mismatch")
                reloaded.append(line.strip())
    expected = {(f"github.com/openexec/openexec/{pkg}", test)
                for pkg, tests in required.items() for test in tests}
    if not expected or not expected <= runs or not expected <= passes:
        raise ValueError(f"named tests did not execute and pass: {expected - (runs & passes)}")
    if not {pkg for pkg, _ in expected} <= packages:
        raise ValueError("missing package pass")
    if not reloaded:
        raise ValueError("missing reloaded persistence evidence")
    return reloaded


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument("mode", nargs="?", default="acceptance", choices=[*MODES, "all", "coverage", "removal-sensitive"])
    args = parser.parse_args()
    if args.mode == "all":
        from exhausted_task_all import main as verify_all
        verify_all()
        return
    required = MODES.get(args.mode, MODES["acceptance"])
    controls = subprocess.run(["python3", "-m", "unittest", "discover", "-s",
                               "scripts/verification", "-p", "test_exhausted_task_reconciliation.py"], cwd=ROOT)
    if controls.returncode:
        raise SystemExit(controls.returncode)
    if args.mode in ("acceptance", "coverage", "removal-sensitive"):
        from exhausted_task_proof import measure, removal_sensitive
        if args.mode != "removal-sensitive":
            measure(required, validate)
        if args.mode != "coverage":
            removal_sensitive()
        print(f"{args.mode}: PASS")
        return
    events = []
    env = os.environ.copy()
    env.setdefault("GOCACHE", str(Path(tempfile.gettempdir()) / "openexec-correction-go-cache"))
    for package, tests in required.items():
        command = ["go", "test", "-json", "-count=1", "-timeout=90s", "./" + package,
                   "-run", "^(" + "|".join(sorted({test.split("/")[0] for test in tests})) + ")$"]
        result = subprocess.run(command, cwd=ROOT, env=env, text=True, capture_output=True)
        if result.returncode:
            raise SystemExit(result.stdout + result.stderr)
        events.extend(json.loads(line) for line in result.stdout.splitlines() if line.strip())
    reloaded = validate(events, required)
    for evidence in reloaded:
        print(evidence)
    print(f"{args.mode}: PASS; {sum(map(len, required.values()))} named tests executed; no skips; persisted state reloaded")


if __name__ == "__main__":
    main()
