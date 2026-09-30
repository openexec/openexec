#!/usr/bin/env python3
"""Offline discovery, not proof that runtime scalar enforcement/recovery is fixed."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
MODULE = "github.com/openexec/openexec"
PACKAGE = MODULE + "/pkg/runtime"
TESTS = ("TestPlannerSchemaDiscoverySchema", "TestPlannerSchemaDiscoveryRuntime")
FIXTURES = ("incident-array", "empty-array", "multi-array", "single-array", "scalar")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def command(args, env):
    result = subprocess.run(args, cwd=ROOT, env=env, text=True,
                            capture_output=True, timeout=120)
    if result.returncode:
        raise ValueError(f"{args!r} exited {result.returncode}\n{result.stdout}{result.stderr}")
    return result.stdout


def check_package(package, root):
    module = package.get("Module", {})
    require(package.get("ImportPath") == PACKAGE and
            Path(package.get("Dir", "")).resolve() == root / "pkg/runtime" and
            module.get("Path") == MODULE and module.get("Main") is True and
            Path(module.get("Dir", "")).resolve() == root and
            "Replace" not in module,
            "runtime import does not resolve to this candidate module")


def check_discovery(output):
    found = [line for line in output.splitlines() if line.startswith("Test")]
    require(sorted(found) == sorted(TESTS), f"required tests not discovered exactly once: {found}")


def expected_tests():
    expected = set(TESTS)
    for name in FIXTURES:
        expected.add(TESTS[0] + "/" + name)
        prefix = TESTS[1] + "/" + name
        for shape in ("array", "object"):
            expected.add(prefix + "/" + shape)
            for route in ("decode", "generate", "compact", "refine"):
                expected.add(prefix + "/" + shape + "/" + route)
    return expected


def check_execution(output):
    ran, passed, diagnostics = set(), set(), []
    package_passes = 0
    for line in output.splitlines():
        event = json.loads(line)
        require(event.get("Package") == PACKAGE, "test executed outside candidate runtime package")
        action, name = event.get("Action"), event.get("Test")
        require(action not in ("fail", "skip"), f"test failed or skipped: {name}")
        if name and action in ("run", "pass"):
            target = ran if action == "run" else passed
            require(name not in target, f"duplicate test event: {action} {name}")
            target.add(name)
        if action == "pass" and not name:
            package_passes += 1
        message = event.get("Output", "").strip()
        if "declared scalar schema diagnostic:" in message or "public runtime " in message:
            diagnostics.append({"test": name, "message": message})
    require(ran == expected_tests() and passed == ran and package_passes == 1,
            f"incomplete test execution: missing={sorted(expected_tests() - passed)} extra={sorted(passed - expected_tests())}")
    require(sum("declared scalar schema diagnostic:" in d["message"] for d in diagnostics) == 4,
            "underlying Go field/type diagnostics missing")
    return diagnostics


def check_fixtures():
    folder = ROOT / "pkg/runtime/testdata/planner-schema"
    manifest = json.loads((folder / "provenance.json").read_text())
    require(manifest["source_evidence"] == "docs/verification/planner-schema-inspection.md" and
            manifest["source_sha256"] == "c02f1f38f893717faa92d34bdacc136c3c1a820ff8791c93e0c3dd2a79e1df0c" and
            manifest["source_sha256"] in (ROOT / manifest["source_evidence"]).read_text(),
            "fixture source does not match inspected incident provenance")
    require(set(manifest["fixtures"]) == {name + ".json" for name in FIXTURES}, "fixture manifest mismatch")
    bodies = {}
    for name, digest in manifest["fixtures"].items():
        data = (folder / name).read_bytes()
        require(hashlib.sha256(data).hexdigest() == digest, f"fixture digest mismatch: {name}")
        bodies[name] = json.loads(data)
    incident = bodies["incident-array.json"]
    require([(s["id"], s["requirement_id"]) for s in incident] == [
        ("US-001", []), ("US-002", ["REQ-001", "REQ-002"]),
        ("US-004", ["REQ-003"]), ("US-005", [])], "incident mappings drifted")
    for name, index in (("empty-array", 0), ("multi-array", 1), ("single-array", 2)):
        require(bodies[name + ".json"] == [incident[index]], f"isolated fixture drifted: {name}")
    scalar = bodies["scalar.json"]
    require(len(scalar) == len(incident), "scalar control lost stories")
    for invalid, valid in zip(incident, scalar):
        require(isinstance(valid["requirement_id"], str), "control is not scalar")
        require({k: v for k, v in invalid.items() if k != "requirement_id"} ==
                {k: v for k, v in valid.items() if k != "requirement_id"},
                "scalar control changed more than requirement_id")
    return manifest


def fingerprint(env):
    # Include untracked additions as well as tracked candidate inputs. Hash content,
    # not merely HEAD: discovery is useful both before and after the task commit.
    names = command(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], env).split("\0")
    paths = sorted({name for name in names if name and (
        name in ("go.mod", "go.sum", "scripts/verify-planner-schema-recovery.sh") or
        name.startswith(("internal/planner/", "pkg/runtime/", "scripts/verification/planner_schema")))})
    return {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in paths}


def main():
    require(Path.cwd().resolve() == ROOT, "invoke discovery from the candidate repository root")
    env = os.environ.copy()
    # Neither an ambient workspace, persisted Go settings, nor GOFLAGS may silently
    # select cached tests, overlays or a different module for this evidence.
    env.update(GOWORK="off", GOENV="off", GOFLAGS="", GOCACHE="/tmp/openexec-planner-schema-go-cache")
    require(Path(command(["git", "rev-parse", "--show-toplevel"], env).strip()).resolve() == ROOT,
            "script is outside the candidate Git root")
    revision = command(["git", "rev-parse", "HEAD"], env).strip()
    require(Path(command(["go", "env", "GOMOD"], env).strip()).resolve() == ROOT / "go.mod",
            "Go resolved another candidate go.mod")
    check_package(json.loads(command(["go", "list", "-json", PACKAGE], env)), ROOT)
    manifest = check_fixtures()
    before = fingerprint(env)
    selection = "^(" + "|".join(TESTS) + ")$"
    check_discovery(command(["go", "test", "./pkg/runtime", "-list", selection], env))
    diagnostics = check_execution(command(["go", "test", "./pkg/runtime", "-json", "-count=1",
                                           "-timeout=60s", "-run", selection], env))
    require(before == fingerprint(env) and revision == command(["git", "rev-parse", "HEAD"], env).strip(),
            "candidate changed during discovery")
    print(json.dumps({"case": "discovery", "result": "passed", "candidate": str(ROOT),
                      "revision": revision, "input_sha256": before, "fixtures": manifest,
                      "tests_passed": len(expected_tests()), "diagnostics": diagnostics,
                      "scope": "Scalar schema rejection and scalar runtime success verified; array runtime behavior observed, recovery not certified."}, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
        print(f"planner schema discovery failed: {error}", file=sys.stderr)
        sys.exit(1)
