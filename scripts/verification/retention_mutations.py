#!/usr/bin/env python3
"""Independent discard mutations against the admitted retention journey."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PACKAGE = "github.com/openexec/openexec/pkg/manager"
ENGINE = "TestRetainedResultEngineBranches"
JOURNEY = "TestRetainedResultAdmittedFailureReloadAndRepair"
INTEGRATED = "TestRetentionMutationJourney"
LEAVES = {INTEGRATED + "/ExecuteStage", INTEGRATED + "/Execute", ENGINE + "/ExecuteStage", ENGINE + "/Execute", JOURNEY}
MUTATIONS = {
    "ExecuteStage": (
        "\t\tif result == nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tresult = failedStageResult(result, stage.Name, run.GetRetries(stage.Name)+1, err)",
        "\t\treturn nil, err",
        {ENGINE + "/ExecuteStage": "failed result/error pair lost",
         INTEGRATED + "/ExecuteStage": "persisted stage evidence lost"},
    ),
    "Execute": (
        "\t\t\tresult = failedStageResult(result, stage.Name, attempt, err)",
        "\t\t\tresult = NewStageResult(stage.Name, attempt)\n\t\t\tresult.Fail(err.Error())",
        {ENGINE + "/Execute": "failed history lost",
         JOURNEY: "persisted stage evidence lost",
         INTEGRATED + "/ExecuteStage": "persisted stage evidence lost",
         INTEGRATED + "/Execute": "persisted stage evidence lost"},
    ),
}


def mutate(source, branch):
    before, after, _ = MUTATIONS[branch]
    if source.count(before) != 1:
        raise ValueError(f"{branch}: expected exactly one discard mutation site")
    return source.replace(before, after, 1)


def classify(stdout, stderr, code, expected):
    """Accept only completed named tests with exact assertion diagnostics."""
    if stderr.strip():
        raise ValueError(f"compiler/tool diagnostics: {stderr}")
    events = [json.loads(line) for line in stdout.splitlines()]
    if not events or any(e.get("Package") != PACKAGE for e in events):
        raise ValueError("missing or unexpected package events")
    if any(e.get("Action") == "skip" for e in events):
        raise ValueError("skipped tests")
    endings = {}
    for event in events:
        if event.get("Action") in ("pass", "fail"):
            name = event.get("Test", "")
            if name in endings:
                raise ValueError("duplicate test completion")
            endings[name] = event["Action"]
    required = {name: "pass" for name in LEAVES | {ENGINE, INTEGRATED, ""}}
    if expected:
        required.update({name: "fail" for name in expected})
        required[""] = "fail"
        if any(name.startswith(ENGINE + "/") for name in expected):
            required[ENGINE] = "fail"
        if any(name.startswith(INTEGRATED + "/") for name in expected):
            required[INTEGRATED] = "fail"
    if endings != required or code != (1 if expected else 0):
        raise ValueError(f"unexpected test outcome: exit={code}, tests={endings}")
    for name, message in expected.items():
        lines = [e.get("Output", "").strip() for e in events
                 if e.get("Test") == name and e.get("Action") == "output"]
        if any("panic:" in line or "fatal error:" in line for line in lines):
            raise ValueError(f"{name}: runtime failure")
        diagnostics = [line for line in lines if re.match(r"\w+_test\.go:\d+:", line)]
        if len(diagnostics) != 1 or not re.fullmatch(
                r"retention_(?:journey|mutation)_test.go:\d+: " + re.escape(message), diagnostics[0]):
            raise ValueError(f"{name}: wrong evidence-retention assertion: {diagnostics}")
    return {"status": "passed" if not expected else "rejected_at_expected_assertions",
            "assertions": expected, "tests": endings}


def check(root, expected):
    env = os.environ.copy()
    env.setdefault("GOCACHE", str(Path(tempfile.gettempdir()) / "openexec-retention-go-cache"))
    env["GOWORK"] = "off"
    result = subprocess.run(
        ["go", "test", "./pkg/manager", "-json", "-count=1", "-timeout=60s",
         "-run", "^(TestRetainedResult(EngineBranches|AdmittedFailureReloadAndRepair)|TestRetentionMutationJourney)$"],
        cwd=root, env=env, text=True, capture_output=True, timeout=120)
    try:
        return classify(result.stdout, result.stderr, result.returncode, expected)
    except (ValueError, KeyError) as exc:
        raise ValueError(f"{exc}\n{result.stdout}\n{result.stderr}") from exc


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, help="replace a JSON evidence report")
    args = parser.parse_args()
    if args.output:
        args.output.unlink(missing_ok=True)
    # Snapshot current candidate contents, including unstaged/new source; no git
    # metadata, ignored caches, hard links, or edits to the working candidate.
    files = subprocess.check_output(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=ROOT
    ).decode().split("\0")
    report = {}
    with tempfile.TemporaryDirectory(prefix="retention-mutations-") as temp:
        baseline = Path(temp) / "candidate"
        for name in filter(None, files):
            source = ROOT / name
            if not source.exists():
                continue
            target = baseline / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)
        engine = Path("internal/blueprint/engine.go")
        original = (baseline / engine).read_text()
        report["candidate"] = check(baseline, {})
        for branch, (_, _, expected) in MUTATIONS.items():
            copy = Path(temp) / branch
            shutil.copytree(baseline, copy)
            (copy / engine).write_text(mutate(original, branch))
            report[branch] = check(copy, expected)
        if (ROOT / engine).read_text() != original:
            raise ValueError("candidate engine changed during verification")
    output = json.dumps(report, indent=2) + "\n"
    if args.output:
        args.output.write_text(output)
    print(output, end="")


if __name__ == "__main__":
    main()
