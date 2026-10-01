#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Keep disposable Go output inside the permitted scratch area.
export GOCACHE="${GOCACHE:-/tmp/openexec-correction-gocache}"
go test ./internal/release ./pkg/manager -count=1 -run 'TestCorrection|TestTaskQueueBoundaryKeeps|TestFreshTaskQueueReopensFailed|TestFailedRepairTaskUses'

# Source overlays execute the real native journey with one production guarantee
# removed. Neither the candidate source nor the acceptance assertions are edited.
python3 - <<'PY'
import json
import pathlib
import subprocess
import tempfile

root = pathlib.Path.cwd()
controls = [
    ("refusal", "pkg/manager/task_correction.go",
     "if admitted || err == nil || !invalidCorrectionState(err) {",
     "if admitted || err != nil || !invalidCorrectionState(err) {", "want TaskQueueBoundary"),
    ("boundary-kind", "pkg/manager/task_boundary.go",
     "item.Kind = BoundaryAttemptLimit",
     "item.Kind = BoundaryFailed", "original boundary evidence missing"),
    ("boundary-evidence", "pkg/manager/task_boundary.go",
     'item.EvidenceID, _ = task.Metadata["verification_failure_evidence"].(string)',
     'item.EvidenceID = ""', "original boundary evidence missing"),
]
with tempfile.TemporaryDirectory(prefix="correction-controls-") as scratch:
    tmp = pathlib.Path(scratch)
    controls.extend([
        ("task-script-execution", "pkg/manager/task_correction.go",
         'if strings.TrimSpace(task.VerificationScript) != "" {',
         'if false {', "script journey failed"),
        ("task-script-completion", "internal/release/task_correction.go",
         'if strings.TrimSpace(script) != "" {',
         'if false && strings.TrimSpace(script) != "" {', "missing task script proof accepted"),
        ("original-exhaustion", "pkg/manager/task_queue.go",
         'if task.Metadata["task_correction"] != nil {',
         '''if task.AttemptCount >= task.MaxAttempts {
                 evidenceID, _ := task.Metadata["verification_failure_evidence"].(string)
                 if _, err := rel.CreateFailureRepair(ctx, task.ID, evidenceID, "retained verification failure"); err != nil {
                     return fmt.Errorf("repair task refused: %w", err)
                 }
             }
             if task.Metadata["task_correction"] != nil {''', "wrong terminal cause: repair task refused: task repair attempt limit reached"),
    ])
    for name, relative, before, after, expected in controls:
        source = root / relative
        original = source.read_text()
        if before not in original:
            raise SystemExit(f"{name}: production anchor missing")
        replacement = tmp / (name + ".go")
        replacement.write_text(original.replace(before, after, 1))
        overlay = tmp / (name + ".json")
        overlay.write_text(json.dumps({"Replace": {str(source): str(replacement)}}))
        selector = "^TestCorrectionNativeQueueRefusals$/^candidate$"
        if name == "task-script-execution":
            selector = "^TestCorrectionTaskScriptJourneys$/^(planned|planner_imported|legacy_no_plan)$/^pass$"
        elif name == "task-script-completion":
            selector = "^TestCorrectionNoPlanCompletionRequiresScriptProof$"
        elif name == "original-exhaustion":
            selector = "^TestCorrectionExhaustionControls$"
        command = ["go", "test", "-overlay", str(overlay), "./pkg/manager",
                   "-count=1", "-run", selector]
        run = subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if run.returncode == 0 or expected not in run.stdout:
            raise SystemExit(f"{name}: mutation did not fail the intended assertion\n{run.stdout}")
        print(f"{name}: native journey rejected mutation ({expected})")
PY
go test ./pkg/manager -count=1 -run '^TestCorrectionNativeQueueRefusals$|^TestCorrectionTaskScriptJourneys$|^TestCorrectionNoPlanCompletionRequiresScriptProof$|^TestCorrectionExhaustionControls$'
echo "Exhausted correction scripts, refusal, persistence, races and boundary controls passed."
