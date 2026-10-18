#!/usr/bin/env python3
"""Reproduce D8's negative control and artifact-free build via make test.

Uses a temporary Git index and source copy; never changes the real index or
deletes local artifacts. Requires installed UI dependencies and Go toolchain.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
BASELINE = "59f009c690e859e2accd259cdadc5e00c4b34439"
ARTIFACTS = (".gocache", "bin/openexec", "openexec", "data/fwus.jso")


def run(args, cwd=ROOT, env=None, check=True):
    result = subprocess.run(args, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    print(result.stdout, end="", flush=True)
    if check:
        result.check_returncode()
    return result


with tempfile.TemporaryDirectory(prefix="openexec-d8-") as directory:
    temp = Path(directory)
    env = dict(os.environ, GIT_INDEX_FILE=str(temp / "baseline-index"))
    run(["git", "read-tree", BASELINE], env=env)
    test = ["go", "test", "-count=1", "-v", "./internal/repositoryhygiene"]
    before = run(test, env=env, check=False)
    if (before.returncode != 1 or
            "--- FAIL: TestBuildArtifactsUntrackedAndIgnored" not in before.stdout or
            "build artifacts must not be tracked:" not in before.stdout):
        raise SystemExit("FAIL: baseline tracking regression was not demonstrated")
    print("PROVEN: ordinary D8 regression rejects original tracked artifacts", flush=True)
    run(test)
    print("PROVEN: same ordinary D8 regression passes current index", flush=True)

    source = temp / "source"
    source.mkdir()
    # Copy current tracked source, including worktree edits, without any ignored
    # output. No checkout, history mutation, or reliance on removed binaries.
    paths = subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT).decode().split("\0")
    for name in set(paths) - {""}:
        if any(name == a or name.startswith(a + "/") for a in ARTIFACTS):
            raise SystemExit(f"FAIL: artifact still tracked: {name}")
        target = source / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / name, target)
    for name in ARTIFACTS:
        if (source / name).exists():
            raise SystemExit(f"FAIL: artifact exists before build: {name}")
    (source / "ui/node_modules").symlink_to((ROOT / "ui/node_modules").resolve())
    print("PROVEN: all four artifact locations absent before make build", flush=True)
    run(["make", "build", "VERSION=d8-verification", "COMMIT=d8-verification"], cwd=source)
    version = run([str(source / "bin/openexec"), "version"], cwd=source)
    if "d8-verification" not in version.stdout:
        raise SystemExit("FAIL: rebuilt CLI did not report verification identity")
    print("PROVEN: artifact-free make build and rebuilt CLI version pass", flush=True)
