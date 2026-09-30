"""Strict deterministic runtime/manager replay gate for US-008."""
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
MODULE = "github.com/openexec/openexec/"
CASES = {
    "pkg/runtime": {
        "TestPlannerSchemaReplayJourney": [
            "approved", "exhausted", "removed-boundary", "legacy-hitl",
            "missing-approval", "rejected",
        ],
        "TestPlannerSchemaRecoveryPublicRuntime": [],
    },
    "pkg/manager": {
        "TestReviewedPlanSchemaCorrection": [
            "approved", "rejected", "malformed", "interrupted", "boundary-removed",
            "remaining-budget", "missing-approval", "malformed-approval",
            "legacy-hitl", "invalid-boundary",
        ],
        "TestReviewedPlanReplayAtomicImportFailure": [],
        "TestReviewedPlanReplayRefusesConflictAndMissingAdapters": [],
    },
}


def discovery(output, names):
    for name in names:
        if output.splitlines().count(name) != 1:
            raise ValueError(f"test not discovered exactly once: {name}")


def verify_events(output, package, cases):
    required = set(cases)
    required.update(name + "/" + case for name, children in cases.items() for case in children)
    passed, started = set(), set()
    package_passes = 0
    for line in output.splitlines():
        event = json.loads(line)
        if event.get("Package") != MODULE + package:
            raise ValueError("unexpected package in replay evidence")
        action, name = event.get("Action"), event.get("Test")
        if action in ("skip", "fail"):
            raise ValueError(f"replay {action}: {name}")
        if name and action in ("run", "pass"):
            seen = started if action == "run" else passed
            if name in seen or name not in required:
                raise ValueError(f"duplicate or undeclared test: {name}")
            seen.add(name)
        if not name and action == "pass":
            package_passes += 1
    if passed != required or started != required or package_passes != 1:
        raise ValueError(f"incomplete replay evidence: missing {required - passed}")


def main():
    env = dict(os.environ, GOWORK="off", GOENV="off", GOFLAGS="",
               GOCACHE="/tmp/openexec-compact-go-cache")
    for package, cases in CASES.items():
        selection = "^(" + "|".join(re.escape(name) for name in cases) + ")$"
        command = ["go", "test", "./" + package]
        listed = subprocess.check_output(command + ["-list", selection], cwd=ROOT, env=env, text=True)
        discovery(listed, cases)
        result = subprocess.run(command + ["-count=1", "-timeout=60s", "-run", selection, "-json"],
                                cwd=ROOT, env=env, text=True, capture_output=True)
        if result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        verify_events(result.stdout, package, cases)
        print(f"{package}: {sum(1 + len(children) for children in cases.values())} required tests passed; no skips")
    print("Strict schema replay verification passed")


if __name__ == "__main__":
    main()
