#!/usr/bin/env python3
"""US-014 complete storage and registered-reference compatibility coverage."""
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
MANIFEST = ROOT / "docs/verification/storage-coverage-scope.json"


def inventory(helper, manifest):
    functions = []
    for path, names in manifest["sources"].items():
        found = json.loads(shared.run(str(helper), path))
        if names == "*":
            selected = found
        else:
            selected = [f for f in found if f["name"] in names]
            if sorted(f["name"] for f in selected) != sorted(names):
                raise ValueError("missing or duplicate scope functions")
        for fn in selected:
            fn.pop("source")
            fn.update(path=path, change="declared")
        functions.extend(selected)
    if not functions:
        raise ValueError("empty storage scope")
    return functions


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument("--output", type=Path, default=Path("/tmp/openexec-storage-coverage"))
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    for name in ("result.json", "scope.json", "coverage.out", "tests.jsonl"):
        (output / name).unlink(missing_ok=True)
    manifest = json.loads(MANIFEST.read_text())
    subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", "scripts/verification",
                    "-p", "test_storage_unit_coverage.py"], cwd=ROOT, check=True)
    required = manifest["tests"]
    if not required or len(required) != len(set(required)):
        raise ValueError("empty or duplicate tests")
    packages = sorted({t.split(":", 1)[0] for t in required})
    with tempfile.TemporaryDirectory(prefix="storage-coverage-") as tmp:
        tmp = Path(tmp)
        helper = tmp / "inventory"
        shared.run("go", "build", "-o", str(helper), "./scripts/verification/retentioncoverage")
        functions = inventory(helper, manifest)
        for path in manifest["dedicated_tests"]:
            found = json.loads(shared.run(str(helper), path))
            identities = {str(Path(path).parent) + ":" + f["name"] for f in found if f["name"].startswith("Test")}
            if not identities or not identities.issubset(required):
                raise ValueError("omitted dedicated storage tests")
        blocks = {path: shared.expected_blocks(path, tmp / "instrumented.go") for path in manifest["sources"]}
        (output / "scope.json").write_text(json.dumps(functions, indent=2) + "\n")
        pattern = "^(" + "|".join(sorted({re.escape(t.split(":", 1)[1]) for t in required})) + ")$"
        command = ["go", "test", *["./" + p for p in packages], "-run", pattern, "-count=1", "-timeout=60s", "-json",
                   "-covermode=count", "-coverpkg=" + ",".join("./" + p for p in packages),
                   "-coverprofile=" + str(output / "coverage.out")]
        with (output / "tests.jsonl").open("w") as log:
            subprocess.run(command, cwd=ROOT, stdout=log, check=True)
        shared.PACKAGES = packages
        shared.check_tests((output / "tests.jsonl").read_text(), required)
        result = shared.evaluate(functions, blocks, output / "coverage.out")
        result.pop("baseline")
        result.update(required_tests=required, command=command, revision=shared.run("git", "rev-parse", "HEAD").strip(),
                      source_sha256={path: hashlib.sha256((ROOT / path).read_bytes()).hexdigest() for path in blocks})
        (output / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps(result, indent=2))
        if not result["passed"]:
            raise ValueError("storage and compatibility statement coverage must be strictly greater than 90%")


if __name__ == "__main__":
    main()
