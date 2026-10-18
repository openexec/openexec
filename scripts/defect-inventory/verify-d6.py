#!/usr/bin/env python3
"""Run the ordinary D6 regression against the original list and current list.

Included in make test so independent host verification exercises both controls.
A temporary Go test overlay redirects only the input file read; repository
sources and the regression assertions remain untouched.
"""
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
BASELINE = "59f009c690e859e2accd259cdadc5e00c4b34439"
SOURCE = "ui/src/types/blueprint.ts"
TEST = "internal/blueprint/ui_stages_test.go"
TEST_NAME = "TestDefaultBlueprintStagesAppearInUI"


def run_test(extra):
    result = subprocess.run(
        ["go", "test", *extra, "-count=1", "-run", f"^{TEST_NAME}$",
         "-v", "./internal/blueprint/"],
        cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
    )
    print(result.stdout, end="", flush=True)
    return result


with tempfile.TemporaryDirectory(prefix="openexec-d6-") as directory:
    temp = Path(directory)
    original = temp / "blueprint.ts"
    original.write_bytes(subprocess.check_output(
        ["git", "show", f"{BASELINE}:{SOURCE}"], cwd=ROOT,
    ))
    test_source = (ROOT / TEST).read_text()
    input_literal = '"../../ui/src/types/blueprint.ts"'
    if test_source.count(input_literal) != 1:
        raise SystemExit("FAIL: expected exactly one regression input path")
    replacement = temp / "ui_stages_test.go"
    replacement.write_text(test_source.replace(input_literal, json.dumps(str(original))))
    overlay = temp / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(ROOT / TEST): str(replacement)}}))
    print(f"Original stage list from {BASELINE}", flush=True)
    before = run_test(["-overlay", str(overlay)])
    required = [f"--- FAIL: {TEST_NAME}"] + [
        f'default blueprint stage "{stage}" is missing from BLUEPRINT_STAGES'
        for stage in ("generate_contract_tests", "fix_lint", "fix_tests",
                      "production_readiness_check")
    ]
    # Infrastructure or compilation failure cannot count as a negative control.
    if before.returncode != 1 or not all(s in before.stdout for s in required):
        raise SystemExit("FAIL: original-list regression failure was not demonstrated")
    print("PROVEN D6: original list fails the ordinary regression with all four omissions", flush=True)
    after = run_test([])
    if after.returncode != 0 or f"--- PASS: {TEST_NAME}" not in after.stdout:
        raise SystemExit("FAIL: current-list regression did not pass")
    print("PROVEN D6: repaired list passes the same ordinary regression", flush=True)
