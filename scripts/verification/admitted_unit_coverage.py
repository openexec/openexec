#!/usr/bin/env python3
"""US-012 admitted-path full-body coverage, independent of Console resources."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile

import retention_unit_coverage as shared

ROOT = shared.ROOT
MANIFEST = ROOT / "docs/verification/admitted-coverage-scope.json"


STUDY = ROOT / "docs/verification/repair-coverage-scopes.json"
BASELINE = "09ac4feb2c7325b42f20a4663b9aa111d43fe386"


def inventory(helper, manifest, temp):
    study = json.loads(STUDY.read_text())
    declared = study["scopes"]["US-012"]
    if study["baseline"] != BASELINE or declared["minimum_statement_percent_exclusive"] != 90:
        raise ValueError("stale baseline or weakened threshold")
    identities = declared["functions"] + declared["planned_functions"]
    if not identities or len(identities) != len(set(identities)):
        raise ValueError("empty/duplicate declared scope")
    if sorted(identities) != sorted(manifest["functions"]):
        raise ValueError("study scope omitted or drifted from admitted inventory")
    sources = {identity.split(":", 1)[0] for identity in identities}
    # Include every added/modified body in a declared source, including new
    # helpers. Never use the old inventory as an exemption for new branches.
    previous_baseline = shared.BASELINE
    try:
        shared.BASELINE = BASELINE
        changed = shared.scope(helper, temp)
    finally:
        shared.BASELINE = previous_baseline
    owners = {identity.split(":", 1)[0] for entry in study["scopes"].values()
              for identity in entry["functions"] + entry["planned_functions"]}
    # Shared public fixture is test support, not deployed runtime behavior.
    unowned = {f["path"] for f in changed} - owners - {"internal/testutil/admittedevidence/executor.go"}
    if unowned:
        raise ValueError("unowned changed production source: " + str(sorted(unowned)))
    identities = set(identities) | {f["path"] + ":" + f["name"] for f in changed if f["path"] in sources}
    functions = []
    for path in sorted(sources):
        for fn in json.loads(shared.run(str(helper), path)):
            if path + ":" + fn["name"] in identities:
                fn.pop("source")
                fn.update(path=path, change="declared")
                functions.append(fn)
    actual = {f["path"] + ":" + f["name"] for f in functions}
    if actual != identities:
        raise ValueError("missing scoped implementation: " + str(sorted(identities - actual)))
    return functions


def check_sources(hashes):
    if any(hashlib.sha256((ROOT / p).read_bytes()).hexdigest() != digest for p, digest in hashes.items()):
        raise ValueError("source changed during coverage measurement")


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument("--output", type=Path, default=Path("/tmp/openexec-admitted-coverage"))
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    for name in ("result.json", "scope.json", "coverage.out", "tests.jsonl"):
        (output / name).unlink(missing_ok=True)
    manifest = json.loads(MANIFEST.read_text())
    subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", "scripts/verification",
                    "-p", "test_admitted_unit_coverage.py"], cwd=ROOT, check=True)
    required = manifest["tests"]
    if not required or len(required) != len(set(required)):
        raise ValueError("empty or duplicate tests")
    packages = sorted({t.split(":", 1)[0] for t in required})
    with tempfile.TemporaryDirectory(prefix="admitted-coverage-") as tmp:
        tmp = Path(tmp)
        helper = tmp / "inventory"
        shared.run("go", "build", "-o", str(helper), "./scripts/verification/retentioncoverage")
        functions = inventory(helper, manifest, tmp / "baseline.go")
        sources = {f["path"] for f in functions}
        hashes = {p: hashlib.sha256((ROOT / p).read_bytes()).hexdigest() for p in sources}
        packages = sorted(set(packages) | {str(Path(p).parent) for p in sources})
        for path in manifest["dedicated_tests"]:
            found = json.loads(shared.run(str(helper), path))
            identities = {str(Path(path).parent) + ":" + f["name"] for f in found if f["name"].startswith("Test")}
            if not identities or not identities.issubset(required):
                raise ValueError("omitted dedicated admitted tests")
        blocks = {path: shared.expected_blocks(path, tmp / "instrumented.go") for path in sources}
        (output / "scope.json").write_text(json.dumps(functions, indent=2) + "\n")
        pattern = "^(" + "|".join(sorted({re.escape(t.split(":", 1)[1]) for t in required})) + ")$"
        command = ["go", "test", *["./" + p for p in packages], "-run", pattern, "-count=1", "-timeout=60s", "-json",
                   "-covermode=count", "-coverpkg=" + ",".join("./" + p for p in packages),
                   "-coverprofile=" + str(output / "coverage.out")]
        with (output / "tests.jsonl").open("w") as log:
            subprocess.run(command, cwd=ROOT, stdout=log, check=True)
        previous_packages = shared.PACKAGES
        try:
            shared.PACKAGES = packages
            shared.check_tests((output / "tests.jsonl").read_text(), required)
        finally:
            shared.PACKAGES = previous_packages
        result = shared.evaluate(functions, blocks, output / "coverage.out")
        check_sources(hashes)
        result.update(baseline=BASELINE, required_tests=required, command=command, revision=shared.run("git", "rev-parse", "HEAD").strip(),
                      source_sha256=hashes, external_scope_measured=False)
        (output / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps(result, indent=2))
        if not result["passed"]:
            raise ValueError("admitted-path statement coverage must be strictly greater than 90%")


if __name__ == "__main__":
    main()
