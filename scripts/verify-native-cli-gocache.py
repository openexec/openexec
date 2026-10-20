#!/usr/bin/env python3
"""Prove the native CLI GOCACHE regression detects a dropped Go build cache.

Included in make test. A Go overlay compiles an isolated source copy with only
GOCACHE preservation removed; neither checkout source nor any real cache changes.
"""

import json
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "pkg/execution/agent_cli_provider.go"
TEST = [
    "go", "test", "-count=1", "-run",
    "^(TestAgentCLIProviderCarriesConfiguredGoCache|TestAgentCLIProviderKeepsGoDefaultWithoutConfiguredCache)$",
    "-v", "./pkg/execution",
]


def run(command):
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
    print(f"$ {' '.join(command)}\n{result.stdout}{result.stderr}", flush=True)
    return result


def require(condition, message):
    if not condition:
        raise SystemExit(message)


original = SOURCE.read_text()
# Fail closed if the implementation changes: never accept an unrelated failure
# (such as a compile error) as evidence that this regression detects the defect.
guard = '|| name == "GOCACHE" && isConfiguredGoCache(value)'
require(original.count(guard) == 1, f"FAIL: expected one mutation site: {guard!r}")
mutated = original.replace(guard, '|| name == "GOCACHE_GUARD_REMOVED" && isConfiguredGoCache(value)', 1)

with tempfile.TemporaryDirectory(prefix="openexec-native-gocache-") as directory:
    copy = Path(directory) / "agent_cli_provider.go"
    copy.write_text(mutated)
    overlay = Path(directory) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(copy)}}))
    negative = run([*TEST[:2], f"-overlay={overlay}", *TEST[2:]])
    for kind in ("claude", "codex"):
        require(
            f"--- FAIL: TestAgentCLIProviderCarriesConfiguredGoCache/{kind}" in negative.stdout,
            f"FAIL: negative control did not fail the {kind} configured-cache path",
        )
    require(
        negative.returncode == 1
        and negative.stdout.count("want configured") == 2
        and "--- PASS: TestAgentCLIProviderKeepsGoDefaultWithoutConfiguredCache" in negative.stdout,
        "FAIL: negative control did not reproduce the dropped-GOCACHE fallback to Go's default",
    )
require(SOURCE.read_text() == original, "FAIL: checkout source changed")
print("PASS: removing GOCACHE preservation sends both CLI agents to Go's default cache", flush=True)
require(run(TEST).returncode == 0, "FAIL: corrected provider regression")
print("PASS: corrected provider carries the configured Go cache; source unchanged", flush=True)
