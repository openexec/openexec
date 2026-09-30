"""Bounded offline checks of the native planner correction and durable import path."""
import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[2]
env = dict(os.environ, GOWORK="off", GOENV="off", GOFLAGS="",
           GOCACHE="/tmp/openexec-compact-go-cache")
selection = "^(TestSchema.*|TestReviewedPlan.*|TestPlannerSchemaRecoveryPublicRuntime)$"
result = subprocess.run(
    ["go", "test", "./internal/planner", "./pkg/runtime", "./pkg/manager",
     "-count=1", "-timeout=60s", "-run", selection], cwd=root, env=env, timeout=90)
sys.exit(result.returncode)
