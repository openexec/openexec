#!/usr/bin/env python3
"""Fail-closed documentation discovery; never claims runtime acceptance."""
from pathlib import Path
import re
import sys

SECTIONS = (
    "Context and scope", "Module map and APIs", "Data flow and observed gaps",
    "Accepted requirement mapping", "Boundaries and conventions",
    "Evidence ownership", "Discovery verification",
)
# Independent tasks reserve distinct test basenames and standalone helpers.
OWNERS = {
    "T-US-008-003": ("retention_unit_test.go", "retention-unit-coverage.sh"),
    "T-US-008-004": ("retention_mutation_test.go", "retention-mutations.sh"),
    "T-US-009-002": ("recapture_boundaries_test.go", "recapture-boundaries.sh"),
    "T-US-009-003": ("recapture_unit_test.go", "recapture-unit-coverage.sh"),
    "T-US-009-004": ("recapture_compatibility_test.go", "recapture-compatibility.sh"),
}
REQUIRED_SOURCES = {
    ("internal/blueprint/engine.go", "ExecuteStage"),
    ("internal/blueprint/engine.go", "Execute"),
    ("internal/pipeline/admitted_executor.go", "Execute"),
    ("pkg/runtime/execution.go", "VerificationCommandFailure"),
    ("internal/execution/gates/failure.go", "CheckFailure"),
    ("pkg/db/state/task_failure.go", "RecordTaskFailureStep"),
    ("pkg/manager/task_failure.go", "persistTaskVerificationFailure"),
    ("pkg/manager/task_failure.go", "repairTaskFromRetainedFailure"),
}


def verify(root):
    doc = root / "docs/ARCHITECTURE.md"
    body = doc.read_text()
    errors = []
    for section in SECTIONS:
        if body.count(f"## {section}\n") != 1:
            errors.append(f"missing or duplicate section: {section}")
    for label, owner in (("REQ-001", "US-012"), ("REQ-002", "US-013"),
                         ("REQ-003", "US-014"), ("REQ-004", "US-015")):
        rows = re.findall(rf"^\| {label} \| (.+)$", body, re.M)
        if len(rows) != 1 or owner not in rows[0].split("|")[-2]:
            errors.append(f"missing/duplicate mapping or wrong owner: {label}")
    sources = re.findall(r"^\| `([^`]+\.go)` \| `([^`]+)` \|", body, re.M)
    if not REQUIRED_SOURCES.issubset(set(sources)):
        errors.append("required source references omitted")
    for path, symbol in sources:
        source = root / path
        if not source.is_file():
            errors.append(f"missing source: {path}")
            continue
        declaration = rf"^(?:type\s+{re.escape(symbol)}\b|func\s+(?:\([^\n]+?\)\s+)?{re.escape(symbol)}\s*\()"
        if not re.search(declaration, source.read_text(), re.M):
            errors.append(f"missing declaration: {path}:{symbol}")
    for task, (test, helper) in OWNERS.items():
        rows = re.findall(rf"^\| {task} \| (.+)$", body, re.M)
        if len(rows) != 1 or test not in rows[0] or f"scripts/verification/{helper}" not in rows[0]:
            errors.append(f"missing/duplicate independent ownership: {task}")
        for other in re.findall(r"^\| (T-US-[^|]+) \| (.+)$", body, re.M):
            if other[0] != task and (test in other[1] or helper in other[1]):
                errors.append(f"shared independent test/helper ownership: {task}, {other[0]}")
    for target in re.findall(r"\]\(([^)]+)\)", body):
        if "://" not in target and not (doc.parent / target.split("#")[0]).is_file():
            errors.append(f"broken documentation link: {target}")
    notes = (root / "NOTES.md").read_text()
    now = notes.split("## Now\n", 1)[-1].split("## Questions\n", 1)[0]
    questions = notes.split("## Questions\n", 1)[-1].split("## For me\n", 1)[0]
    if "T-US-007-002" not in now or "US-007" not in questions:
        errors.append("missing discovery task or unresolved questions in NOTES")
    return errors, len(sources)


def main():
    try:
        errors, count = verify(Path(__file__).resolve().parents[2])
    except (OSError, ValueError) as error:
        print(f"discovery failed: {error}", file=sys.stderr)
        return 1
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"discovery passed: {count} source declarations, requirement mapping, ownership, links and NOTES; runtime acceptance not evaluated")
    return 0


if __name__ == "__main__":
    sys.exit(main())
