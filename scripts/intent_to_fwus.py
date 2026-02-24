#!/usr/bin/env python3
"""
Convert an INTENT.md file into a minimal Tract FWU JSON spec.

This is a pragmatic bootstrap: it maps each numbered goal in the intent
to one FWU with a generated ID and a basic "tests" verification gate.

Usage:
  python3 uaos/scripts/intent_to_fwus.py uaos/INTENT.md uaos/data/fwus.json

You can then load it with:
  docker compose -f uaos/code/docker-compose.dev.yml run --rm tract \
    load fwus /workspace/uaos/data/fwus.json --store ${TRACT_STORE:-uaos}
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any, List, Dict


def extract_goals(markdown: str) -> List[str]:
    # Try to scope to a Goals section if present
    goals_section = markdown
    m = re.search(r"^##\s+Goals\s*$([\s\S]*?)^(##\s+|\Z)", markdown, re.MULTILINE)
    if m:
        goals_section = m.group(1)

    goals: List[str] = []
    for line in goals_section.splitlines():
        m2 = re.match(r"^\s*\d+\.\s+(.+?)\s*$", line)
        if m2:
            goals.append(m2.group(1).strip())
    return goals


def make_fwu(idx: int, goal_text: str) -> Dict[str, Any]:
    # Generate a stable-ish ID block under 01.1 series
    seq = f"{idx:02d}"
    fwu_id = f"FWU-01.1.{seq}"
    feature_id = f"F-01.{idx}"
    name = goal_text[:60]
    return {
        "id": fwu_id,
        "feature_id": feature_id,
        "name": name if name else f"Goal {idx}",
        "status": "pending",
        "intent": goal_text,
        "addresses_acs": [],
        "planning_version": 1,
        "boundaries": [],
        "dependencies": [],
        "design_decisions": [],
        "interface_contracts": [],
        "verification_gates": [
            {
                "id": f"{fwu_id}:VG:tests",
                "gate": "tests",
                "expectation": "Baseline tests passing",
            }
        ],
    }


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: intent_to_fwus.py <INTENT.md> <output fwus.json>")
        return 2

    intent_path = Path(sys.argv[1])
    out_path = Path(sys.argv[2])

    if not intent_path.exists():
        print(f"Error: {intent_path} not found", file=sys.stderr)
        return 1

    md = intent_path.read_text(encoding="utf-8")
    goals = extract_goals(md)
    if not goals:
        print("Warning: no numbered goals found; creating a single FWU from title", file=sys.stderr)
        title = next((l.strip("# ") for l in md.splitlines() if l.startswith("#")), "Untitled")
        goals = [title]

    fwus = [make_fwu(i + 1, g) for i, g in enumerate(goals)]
    data = {"version": "1", "fwus": fwus}

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(data, indent=2), encoding="utf-8")
    print(f"Wrote {out_path} with {len(fwus)} FWUs")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

