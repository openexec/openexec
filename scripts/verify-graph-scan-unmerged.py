#!/usr/bin/env python3
"""Prove the real conflicted-index regression detects duplicate scan inputs.

Included in make test. A Go overlay compiles an isolated source copy with only
path deduplication removed; neither checkout source nor any real index changes.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "internal/knowledge/graph_scan.go"
TEST = [
    "go", "test", "-count=1", "-run", "^TestScanRepositoryResolvedUnmergedIndex$",
    "-v", "./internal/knowledge",
]


def run(command):
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
    print(f"$ {' '.join(command)}\n{result.stdout}{result.stderr}", flush=True)
    return result


def require(condition, message):
    if not condition:
        raise SystemExit(message)


original = SOURCE.read_text()
start = original.index("func gitListedFiles(")
end = original.index("\n}\n", start) + 3
mutated = original[start:end]
# Fail closed if the implementation changes: never accept an unrelated failure
# (such as a compile error) as evidence that this regression detects the defect.
for before, after in [
    ("\tseen := make(map[string]bool)\n", ""),
    ('\t\tif rel != "" && !seen[rel] {\n', '\t\tif rel != "" {\n'),
    ("\t\t\tseen[rel] = true\n", ""),
]:
    require(mutated.count(before) == 1, f"FAIL: expected one mutation site: {before!r}")
    mutated = mutated.replace(before, after, 1)

with tempfile.TemporaryDirectory(prefix="openexec-unmerged-index-") as directory:
    copy = Path(directory) / "graph_scan.go"
    copy.write_text(original[:start] + mutated + original[end:])
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(copy)}}))
    negative = run([*TEST[:2], f"-overlay={overlay}", *TEST[2:]])
    require(
        negative.returncode == 1
        and "--- FAIL: TestScanRepositoryResolvedUnmergedIndex" in negative.stdout
        and "insert scan input sample.go:" in negative.stdout
        and "UNIQUE constraint failed: graph_scan_inputs.generation_id, graph_scan_inputs.file_path" in negative.stdout,
        "FAIL: negative control did not reproduce the duplicate-input uniqueness failure",
    )
require(SOURCE.read_text() == original, "FAIL: checkout source changed")
print("PASS: removing deduplication reproduces duplicate-input uniqueness failure", flush=True)
require(run(TEST).returncode == 0, "FAIL: corrected scanner regression")
print("PASS: corrected scanner persists resolved unmerged input; source unchanged", flush=True)
