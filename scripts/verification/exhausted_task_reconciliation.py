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
        "pkg/manager": ["TestCorrectionCompletionObligations", "TestCorrectionNativeQueueSuccess"],
    },
    "native-queue": {
        "pkg/manager": ["TestCorrectionNativeQueueSuccess", "TestCorrectionNativeQueueRefusals",
                        "TestCorrectionNativeStopAndFailure"],
    },
}


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
            if "RELOADED_CORRECTION " in line:
                payload = line.split("RELOADED_CORRECTION ", 1)[1]
                if payload.startswith("{"):
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
    parser.add_argument("mode", choices=MODES)
    args = parser.parse_args()
    required = MODES[args.mode]
    events = []
    env = os.environ.copy()
    env.setdefault("GOCACHE", str(Path(tempfile.gettempdir()) / "openexec-correction-go-cache"))
    for package, tests in required.items():
        command = ["go", "test", "-json", "-count=1", "-timeout=90s", "./" + package,
                   "-run", "^(" + "|".join(tests) + ")$"]
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
