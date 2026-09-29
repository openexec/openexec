#!/usr/bin/env python3
"""Full-body, fail-closed US-008 statement coverage. No provider or network tests."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

os.environ.setdefault("GOCACHE", str(Path(tempfile.gettempdir()) / "openexec-retention-go-cache"))

ROOT = Path(__file__).resolve().parents[2]
BASELINE = "1eb4cb69"
MODULE = "github.com/openexec/openexec/"
MANIFEST = ROOT / "docs/verification/retention-coverage-scope.json"
PACKAGES = ["internal/blueprint", "internal/execution/evidence", "internal/execution/gates",
            "internal/pipeline", "pkg/manager", "pkg/runtime"]


def run(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs)


def scope(helper, temp):
    # Include tracked edits, deletions and newly created production sources.
    changed = set(run("git", "diff", "--name-only", BASELINE, "--", "*.go").splitlines())
    changed.update(run("git", "ls-files", "--others", "--exclude-standard", "--", "*.go").splitlines())
    result = []
    for path in sorted(changed):
        if path.endswith("_test.go") or not path.startswith(("internal/", "pkg/", "cmd/")):
            continue
        old = subprocess.run(["git", "show", f"{BASELINE}:{path}"], cwd=ROOT, text=True, capture_output=True)
        previous = {}
        if old.returncode == 0:
            temp.write_text(old.stdout)
            previous = {f["name"]: f for f in json.loads(run(str(helper), str(temp)))}
        current = json.loads(run(str(helper), path)) if (ROOT / path).exists() else []
        for fn in current:
            before = previous.pop(fn["name"], None)
            if before is None or before["source"] != fn["source"]:
                fn.update(path=path, change="added" if before is None else "modified")
                fn.pop("source")
                result.append(fn)
        if previous:
            raise ValueError(f"removed production functions require scope review: {path}: {list(previous)}")
    if not result:
        raise ValueError("empty production scope")
    return result


def expected_blocks(path, instrumented):
    run("go", "tool", "cover", "-mode=count", "-var=RetentionCover", "-o", str(instrumented), path)
    source = instrumented.read_text()
    positions = re.search(r"Pos:.*?\{(.*?)\n\s*\},", source, re.S)
    statements = re.search(r"NumStmt:.*?\{(.*?)\n\s*\},", source, re.S)
    if not positions or not statements:
        raise ValueError(f"missing expected instrumentation: {path}")
    positions = re.findall(r"(\d+), (\d+), (0x[0-9a-f]+),", positions[1])
    statements = re.findall(r"(\d+),", statements[1])
    if len(positions) != len(statements) or not positions:
        raise ValueError(f"invalid expected instrumentation: {path}")
    return [(int(a), int(c, 16) & 65535, int(b), int(c, 16) >> 16, int(n))
            for (a, b, c), n in zip(positions, statements)]


def evaluate(functions, blocks, profile):
    measured = {}
    lines = profile.read_text().splitlines()
    if not lines or lines[0] != "mode: count":
        raise ValueError("missing count instrumentation")
    for line in lines[1:]:
        match = re.fullmatch(r"(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)", line)
        if not match:
            raise ValueError("malformed coverage block")
        path, *nums = match.groups()
        key = (path.removeprefix(MODULE), *map(int, nums[:4]))
        n, count = map(int, nums[4:])
        if key in measured:
            if measured[key][0] != n:
                raise ValueError(f"inconsistent repeated instrumentation: {key}")
            count += measured[key][1]
        measured[key] = (n, count)
    results = []
    for fn in functions:
        start = (fn["start"]["Line"], fn["start"]["Column"])
        end = (fn["end"]["Line"], fn["end"]["Column"])
        body = [b for b in blocks[fn["path"]] if start <= b[:2] and b[2:4] <= end]
        if not body or not sum(b[4] for b in body):
            raise ValueError(f"empty instrumentation: {fn['name']}")
        covered = total = 0
        for a, b, c, d, n in body:
            key = (fn["path"], a, b, c, d)
            if key not in measured or measured[key][0] != n:
                raise ValueError(f"missing/mismatched instrumentation: {key}")
            total += n
            covered += n if measured[key][1] > 0 else 0
        results.append(dict(path=fn["path"], name=fn["name"], change=fn["change"],
                            covered=covered, statements=total))
    total = sum(f["statements"] for f in results)
    covered = sum(f["covered"] for f in results)
    if not total:
        raise ValueError("empty statement scope")
    return dict(baseline=run("git", "rev-parse", BASELINE).strip(), functions=results,
                covered=covered, statements=total, percent=100 * covered / total,
                passed=10 * covered > 9 * total)


def check_tests(events, required):
    passed = set()
    package_passed = set()
    for line in events.splitlines():
        event = json.loads(line)
        action = event.get("Action")
        if action in ("skip", "fail"):
            raise ValueError(f"required test execution {action}: {event}")
        if action == "pass":
            if "Test" in event:
                passed.add(event["Package"].removeprefix(MODULE) + ":" + event["Test"])
            else:
                package_passed.add(event["Package"].removeprefix(MODULE))
    missing = set(required) - passed
    if missing or set(PACKAGES) - package_passed:
        raise ValueError(f"missing required tests/packages: {sorted(missing)}, {set(PACKAGES)-package_passed}")


def check_scope(functions, manifest):
    identities = sorted(f["path"] + ":" + f["name"] for f in functions)
    if (not identities or manifest.get("baseline") != BASELINE
            or identities != sorted(manifest["functions"])
            or len(identities) != len(set(identities))):
        raise ValueError(f"omitted or stale changed-function scope: {identities}")



def slice_scope(functions, manifest, companion):
    """Account for every changed function while measuring this slice separately."""
    identities = {f['path'] + ':' + f['name'] for f in functions}
    owned = set(manifest['functions'])
    other = set(companion['functions'])
    if not owned or not other or identities != owned | other:
        raise ValueError('omitted or unexpected consolidated production function')
    selected = [f for f in functions if f['path'] + ':' + f['name'] in owned]
    check_scope(selected, manifest)
    return selected


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument("--output", type=Path, default=Path("/tmp/openexec-retention-coverage"))
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    # Remove stale success evidence before any operation that may fail.
    for name in ("result.json", "scope.json", "coverage.out", "tests.jsonl"):
        (output / name).unlink(missing_ok=True)
    manifest = json.loads(MANIFEST.read_text())
    with tempfile.TemporaryDirectory(prefix="retention-coverage-") as tmp:
        tmp = Path(tmp)
        helper = tmp / "inventory"
        run("go", "build", "-o", str(helper), "./scripts/verification/retentioncoverage")
        functions = scope(helper, tmp / "baseline.go")
        companion = json.loads((ROOT / "docs/verification/recapture-coverage-scope.json").read_text())
        functions = slice_scope(functions, manifest, companion)
        (output / "scope.json").write_text(json.dumps(functions, indent=2) + "\n")
        blocks = {path: expected_blocks(path, tmp / "instrumented.go") for path in {f["path"] for f in functions}}
        # Pin selected test identities; discover every dedicated unit test as well.
        required = manifest["tests"]
        if not required or len(required) != len(set(required)):
            raise ValueError("empty/duplicate required tests")
        for package in PACKAGES:
            path = ROOT / package / "retention_unit_test.go"
            if not path.is_file():
                raise ValueError(f"missing dedicated tests: {package}")
            found = [package + ":" + f["name"] for f in json.loads(run(str(helper), str(path))) if f["name"].startswith("Test")]
            if not found or not set(found).issubset(required):
                raise ValueError(f"omitted dedicated tests: {package}")
        pattern = "^(" + "|".join(sorted({re.escape(t.split(":", 1)[1]) for t in required})) + ")$"
        command = ["go", "test", *["./" + p for p in PACKAGES], "-run", pattern, "-count=1", "-timeout=60s", "-json",
                   "-covermode=count", "-coverpkg=" + ",".join("./" + p for p in PACKAGES),
                   "-coverprofile=" + str(output / "coverage.out")]
        with (output / "tests.jsonl").open("w") as log:
            subprocess.run(command, cwd=ROOT, stdout=log, check=True)
        check_tests((output / "tests.jsonl").read_text(), required)
        result = evaluate(functions, blocks, output / "coverage.out")
        result.update(required_tests=required, command=command, revision=run("git", "rev-parse", "HEAD").strip(),
                      source_sha256={path: hashlib.sha256((ROOT / path).read_bytes()).hexdigest()
                                     for path in sorted(blocks)})
        (output / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps(result, indent=2))
        if not result["passed"]:
            raise ValueError("aggregate full-body statement coverage must be strictly greater than 90%")

if __name__ == "__main__":
    main()
