"""Compiled removal controls; infrastructure errors never count as falsification."""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
# (name, source, original, replacement, package, exact test, assertion)
CONTROLS = [
 ('executor-restrictions', 'pkg/manager/manager.go',
  'StageExecutor:        m.cfg.StageExecutor,', 'StageExecutor:        nil,',
  'pkg/manager', 'TestCorrectionExecutionControls/denied_effects', 'denied executor effect escaped'),
 ('operational-store-error', 'pkg/manager/task_correction.go',
  'err = errors.Join(err, fmt.Errorf("persist correction refusal: %w", saveErr))',
  'err = errRecaptureWaiting', 'pkg/manager',
  'TestCorrectionRefusalStoreFailureIsOperational', 'store failure hidden as boundary'),

 ('conditional-owner', 'internal/release/task_correction.go',
  'if n != 1 {', 'if false && n != 1 {', 'internal/release',
  'TestCorrectionAdmissionRefusalRace', 'double admission'),
 ('stop-control', 'pkg/manager/manager.go',
  'func (m *Manager) Stop(fwuID string) error {',
  'func (m *Manager) Stop(fwuID string) error { return nil;', 'pkg/manager',
  'TestCorrectionExecutionControls/stop_completion', 'control bypassed'),
 ('cancellation-control', 'pkg/manager/task_correction.go',
  'func (m *Manager) reconcileTaskCorrection(ctx context.Context, task *release.Task) (err error) {',
  'func (m *Manager) reconcileTaskCorrection(ctx context.Context, task *release.Task) (err error) { ctx = context.WithoutCancel(ctx);',
  'pkg/manager', 'TestCorrectionExecutionControls/cancel_completion', 'unverified dependency escaped'),

 ('refusal-persistence', 'pkg/manager/task_correction.go',
  'if admitted || err == nil || !invalidCorrectionState(err) {',
  'if admitted || err != nil || !invalidCorrectionState(err) {', 'pkg/manager',
  'TestCorrectionPreAdmissionRefusalPersistence', 'not recorded refusal'),
 ('independent-drain', 'pkg/manager/task_correction.go',
  'if admitted || err == nil || !invalidCorrectionState(err) {',
  'if admitted || err != nil || !invalidCorrectionState(err) {', 'pkg/manager',
  'TestCorrectionIndependentCandidateChange', 'correction candidate mismatch'),
 ('fresh-evidence-guard', 'internal/release/task_correction.go',
  "AND COALESCE(json_extract(metadata,'$.task_correction.fresh_evidence_id'),'')=''",
  "AND 1=1", 'internal/release', 'TestCorrectionRefusalReplacementGuards/fresh', 'fresh evidence refunded'),
 ('snapshot-ownership', 'internal/release/task_correction.go',
  'if !bytes.Equal(observed, retained) || metadata["task_correction"] == nil {',
  'if false && (!bytes.Equal(observed, retained) || metadata["task_correction"] == nil) {',
  'internal/release', 'TestCorrectionRefusalReplacementGuards/stale_snapshot', 'unsafe refusal succeeded'),
 ('spent-authority', 'internal/release/task_correction.go',
  "json_extract(metadata,'$.task_correction.outcome')='pre_admission_refused'",
  "json_extract(metadata,'$.task_correction.outcome')!=''", 'internal/release',
  'TestCorrectionRefusalReopenAndDecisionHistory', 'admitted refusal replaced'),
 ('unsupported-obligation', 'pkg/manager/task_correction.go',
  'if _, _, err := correctionCheck(item); err != nil {',
  'if _, _, err := correctionCheck(item); false && err != nil {', 'pkg/manager',
  'TestCorrectionTaskScriptJourneys/planned/unsupported', 'unsupported required command not refused'),
 ('invented-obligation', 'pkg/manager/task_correction.go',
  'passed := map[string]bool{}',
  'if len(checks) == 0 { return fmt.Errorf("invented verification obligation") }; passed := map[string]bool{}',
  'pkg/manager', 'TestCorrectionWithoutChecksUsesNativeCompletion/no_plan', 'invented verification obligation'),
]


def validate(result, test, marker):
    events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
    relevant = [e for e in events if e.get('Test','') == test or e.get('Test','').startswith(test + '/')]
    output = ''.join(e.get('Output','') for e in relevant)
    if (result.returncode == 0 or not any(e.get('Action')=='run' for e in relevant)
            or not any(e.get('Action')=='fail' for e in relevant)
            or any(e.get('Action')=='skip' for e in events)
            or not re.search(r'\S+\.go:\d+: .*' + re.escape(marker), output)):
        raise ValueError('control did not fail intended named assertion:\n' + result.stdout + result.stderr)


def main():
    for name, relative, before, after, package, test, marker in CONTROLS:
        source = ROOT / relative
        original = source.read_bytes()
        if original.decode().count(before) != 1:
            raise ValueError('ambiguous/missing mutation anchor: ' + name)
        with tempfile.TemporaryDirectory(prefix='exhausted-control-') as d:
            temp = Path(d)
            replacement = temp / source.name
            replacement.write_text(original.decode().replace(before, after))
            overlay = temp / 'overlay.json'
            overlay.write_text(json.dumps({'Replace':{str(source):str(replacement)}}))
            selector = '/'.join('^' + re.escape(part) + '$' for part in test.split('/'))
            command = ['go','test','-overlay',str(overlay),'./'+package,'-json','-count=1','-timeout=90s','-run',selector]
            result = subprocess.run(command,cwd=ROOT,text=True,capture_output=True)
            validate(result,test,marker)
        if source.read_bytes()!=original:
            raise ValueError('source changed: '+relative)
        print(name + ': expected named assertion failure; source restored ' + hashlib.sha256(original).hexdigest(),flush=True)
    # Positive rerun proves the unchanged source remains reachable after overlays.
    for package in sorted({c[4] for c in CONTROLS}):
        tests = sorted({c[5].split('/')[0] for c in CONTROLS if c[4]==package})
        subprocess.run(['go','test','./'+package,'-count=1','-timeout=90s','-run','^('+'|'.join(tests)+')$'],cwd=ROOT,check=True)


if __name__=='__main__':main()
