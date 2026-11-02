#!/usr/bin/env python3
"""Mutation check for context discipline (pkg/execution/context_*.go, handover.go).

Each mutation removes one guard; the discipline tests must then fail. A
mutation the tests survive means a guard nothing proves. Run from the repo root:
    python3 scripts/context-discipline-mutations.py
"""
import subprocess, sys, os

TESTS = r'TestLongRun|TestHuge|TestHandover|TestRequestThat|TestDispatched|TestCheckpointPasses|TestFinalSynthesisStill|TestThresholds|TestSizeFailure|TestNativeRoute'
EXEC = 'pkg/execution/'
MUTATIONS = [
    ('hard threshold no longer restarts the chain', 'api_provider.go',
     'if verdict == admitHard {', 'if false && verdict == admitHard {'),
    ('soft threshold no longer asks for a handover update', 'api_provider.go',
     'if verdict == admitSoft && !askedCheckpoint {', 'if false && verdict == admitSoft && !askedCheckpoint {'),
    ('large tool results enter requests whole', 'handover.go',
     'if len(output) <= s.policy.MaxInlineToolBytes ||', 'if true || len(output) <= s.policy.MaxInlineToolBytes ||'),
    ('no time bound on prompt processing', 'context_policy.go',
     'if c.PromptTokensPerSecond <= 0 || c.RequestTimeout <= 0 {', 'if true || c.PromptTokensPerSecond <= 0 || c.RequestTimeout <= 0 {'),
    ('no byte bound', 'context_policy.go',
     'if bytes > c.hardBytes() {', 'if false && bytes > c.hardBytes() {'),
    ('an endpoint size refusal is a failed pass', 'api_provider.go',
     'if state != nil && ctx.Err() == nil && sizeFailure(err, tokens, state.policy) {', 'if false {'),
    ('a checkpoint ends the pass as a failure', 'api_provider.go',
     'result.Outcome, result.Reason = OutcomeInconclusive, ReasonContextCheckpoint', 'result.Outcome, result.Reason = OutcomeFailed, ""'),
    ('handover updates use the tool-round allowance', 'api_provider.go',
     'if external {', 'if true {'),
    ('final synthesis keeps the full history and tools', 'api_provider.go',
     'if verdict, _ := state.policy.admit(tokens, size); verdict != admitOK {', 'if false {'),
    ('handover readable by others', 'request_capture.go',
     'tmp.Chmod(0o600)', 'tmp.Chmod(0o644)'),
    ('files touched are not read from git', 'handover.go',
     'b.WriteString("Files touched (git):\\n" + gitSummary(s.data.WorkingDir) + "\\n")', 'b.WriteString("Files touched (git):\\n")'),
    ('command exit codes are not recorded', 'handover.go',
     'record.Exit = m[1]', 'record.Exit = "?"'),
    ('an empty Done is not flagged or filled', 'handover.go',
     'b.WriteString("FLAG: the model left this empty; the harness filled it from the log.\\n")', ''),
    ('an empty Next is not flagged or filled', 'handover.go',
     'b.WriteString("FLAG: the model left this empty; do these in order:', 'b.WriteString("(x) do these in order:'),
    ('a later pass does not resume the handover', 'handover.go',
     'saved.Status != "checkpointed"', 'true'),
    ('saved outputs are not indexed', 'handover.go',
     's.data.Outputs = append(s.data.Outputs, savedOutput{ID: id, Tool: name, Summary: summary, Bytes: len(output), Lines: lines})', '_ = summary'),
]

def run():
    r = subprocess.run(['go', 'test', '-buildvcs=false', './pkg/execution/', '-run', TESTS, '-count=1'], capture_output=True, text=True)
    return r.returncode == 0

survived = []
if not run():
    sys.exit('baseline fails; fix the tests first')
for name, file, old, new in MUTATIONS:
    path = EXEC + file
    source = open(path).read()
    if source.count(old) < 1:
        sys.exit('mutation target not found: %s (%s)' % (name, old))
    open(path, 'w').write(source.replace(old, new, 1))
    try:
        killed = not run()
    finally:
        open(path, 'w').write(source)
    print('%-8s %s' % ('killed' if killed else 'SURVIVED', name))
    if not killed:
        survived.append(name)
sys.exit(1 if survived else 0)
