# Conversation adoption: caller-category contradiction

This is a bounded diagnosis and engineering reconciliation proposal, **not a
native repair or Console recovery claim**. The original eleven-expectation
replay remains blocked. No corpus, expiry, category, extraction deadline,
output bound, installed binary, Stop or accounting state changes here.

## Source and minimal counterexample

The incident source is Console commit
`bbbda1c6e7d746c4bb605f3ddb736283828811fb`, not a mutable working checkout.

| Immutable Console source | Relevant relation |
| --- | --- |
| `ui/src/app/App.tsx:132` | App calls `useConversations`, destructuring sessions and mutation callbacks. |
| `ui/src/app/App.tsx:269–284` | Sessions are filtered into owner sessions and earlier conversations. |
| `ui/src/app/App.tsx:687–695` | Nested `adoptConversation(chosen)` calls `adoptProjectChat(chosen)`, then updates and archives sessions and opens the adopted conversation. |
| `ui/src/app/App.tsx:816–837` | Close/delete routing uses hook callbacks and owner sessions; Conversation receives earlier conversations and the adoption callback. |
| `ui/src/features/conversations/Conversation.tsx:628–634` | A list selection forwards the selected item to the adoption callback. |
| `ui/src/features/conversations/ConversationList.tsx:33–40` | A row's click passes its own session to `onSelect`. |
| `ui/src/features/conversations/adoptProjectChat.ts:11–14` | The helper uses the selected session's project ID in the URL and ID in the JSON body. It does not reference the hook or App. |

There is real value flow:

```text
hook.sessions → ownerSessions → earlierConversations → list row
  → selected item → adoptConversation(chosen) → adoptProjectChat(chosen)
```

That is not the direction of the structural caller graph:

```text
Boot --references--> App --calls--> useConversations
                     |
                     +----calls--> adoptProjectChat
                     |
                     +----calls--> unrelatedSibling
```

`internal/knowledge/testdata/conversation-adoption/` is a public synthetic
counterexample. It preserves the returned hook state and callbacks, deletion
and archive routing, list selection, adoption callback, and selected-session
URL/body payload. It omits polling, concurrency guards, display formatting,
activity ordering and unrelated application UI: none adds a helper-to-hook
reference. React-shaped declarations supply compiler context only. No fake
extractor or fabricated graph is used. Boot is an added genuine depth-2 caller;
`unrelatedSibling` is an added unrelated callee of App.

The first fixture run used unchanged native code at OpenExec
`0b4aebd1f8ecfa2450762b5f872e235dc7e5bccf`. Before adding Boot and the stronger
controls it yielded App as a direct caller and `affected_callers: []`, while
extracting both App calls as `compiler_exact`. The final executable
characterization is `TestConversationAdoptionDependencyEvidence`:

- Runs the real TypeScript compiler extractor with its existing 45-second
  deadline; refuses diagnostics and asserts compiler-resolved file targets and
  source ranges for all three App calls.
- Scans through `Store.ScanRepository`, then reads the persisted outgoing
  `calls` edges with `FindSymbolRelationships`, including their exact source
  ranges and compiler resolution.
- Runs `ChangedImpactAnalysis` at depth 2 and unchanged default output bounds.
  App is direct; Boot is affected through the persisted compiler-exact
  `references → calls` path. Adoption and the unrelated sibling are absent.
  Querying impact of the adoption helper also reaches App, without reaching
  the hook or unrelated sibling.
- Closes/reopens the store with refresh disabled and compares the entire
  response, provenance and outgoing edges. Invalid paths, excessive depth,
  a tighter caller depth budget, a missing anchor, a node-budget truncation
  and stale source all remain explicit refusals/unavailability/truncation.
- As a control only, inserts an actual helper-to-hook call in a disposable
  fixture. Stale reads refuse; an explicit scan reaches that real direct
  caller; restoring the original source and rescanning removes the reach.
  Calling a hook from this helper is not a proposed application change.

This distinguishes three causes: the two App calls are **not lost during
extraction or persistence**; a real incoming depth-2 path is **not lost during
propagation**; the expected adoption helper is **not an incoming caller of the
hook**. Source value flow and structural incoming reach are different
relations. More traversal depth cannot reverse an outgoing arm of the fork.
The fixture's JSON also discloses the generic dynamic-registration limitation
and unavailable Go type checking (this is a TypeScript-only fixture). Its
TypeScript call edges and the asserted paths are compiler-exact; these warnings
are not presented as zero global unresolved output or as external replay proof.

## Existing primitives and scope

`ts_compiler_extractor.mjs` extracts top-level declarations and class methods,
resolving uses with the TypeScript checker. It does not emit nested callback
symbols or argument/return-value provenance edges. `graph_scan.go` attributes
uses to the narrowest extracted containing symbol; these calls belong to App.
`graph_impact.go:impactAnalysisForGeneration` traverses incoming call/reference
edges for caller categories. Neither `ImpactPath` nor an existing edge type
turns argument flow into an incoming helper-to-hook reference.

Sibling paths were checked: file-anchored `ChangedImpactAnalysis` and
symbol-anchored `ImpactAnalysis` share that traversal. CLI changed-file impact,
symbol impact and validation proposals in `internal/cli/knowledge.go`, plus HTTP query and
validation in `internal/server/repository_graph_{query,validation}.go`, reuse
these store methods. Module dependants use incoming import edges; effects and
registered tests use their separate typed outgoing edges. None provides
call-site value provenance. Go/Python/Svelte extraction is outside this
TypeScript counterexample; no common traversal is changed.

The user outcome belongs to native verification at the existing deterministic
delivery boundary. Existing compiler references, persisted edges and
`ImpactPath` suffice to prove the contradiction. Broad value-flow analysis
would introduce a new analysis concept; fabricating a helper-to-hook edge or
walking every App callee would falsely include the unrelated sibling. Neither
is justified by this diagnosis.

Complexity delta: concepts added/removed 0; new persistent state 0; new
transitions 0; new owner decisions 0; new runtime failure modes 0; existing
machinery replaced 0. Only test fixtures, characterization coverage and this
proposal are added. Compatibility evaluation: production code, graph schema,
project loading, legacy `.uaos` and `.openexec` fallbacks are unchanged.

## Smallest same-task reconciliation proposal

Have Console engineering reconcile this one expectation on the same task,
with this counterexample and the immutable source as review input. Preserve
the incident corpus and original eleven-expectation failure as evidence.

The concrete proposal is to keep the hook-anchored App and registered browser
expectations, and separately anchor `adoptProjectChat` to assert its actual
incoming caller App. The selected-session forwarding, replacement archiving,
deletion routing and URL/body payload belong in the existing Console browser
journey/regression tests, where those behaviors can actually be exercised.
This is an explicit proposed corpus reconciliation, **not** a retagging of the
missing node, and is not applied or approved by this native change. Any
replacement corpus needs engineering review explaining the semantic change
and its behavioral coverage. A requirement for typed value provenance instead
needs a separately bounded analysis proposal, not a false caller edge.

Until engineering reconciliation is resolved through the normal routes, the
original eleven-expectation proof cannot be declared green. Preserve PR #1583,
its head `20a4221948f0f187c1171aa204c3ab705111c63c`, the original Stop, both
original admissions and all original proof. Do not infer recovery from green
native tests. Do not retry or duplicate compiler-capture `e763` or
request-wire `09678` work.

## Authorized external verification handoff

The supervisor runs this only from its own disposable Console checkout at
`bbbda1c6e7d746c4bb605f3ddb736283828811fb` or a documented newer containing
revision, using `REVIEWED_OPENEXEC` for a reviewed candidate binary. These are
handoff commands, not evidence that external replay passed here. Native access
refusal must be handed to the supervisor; never retry forbidden owner/agent
trees. Record both exact repository revisions and binary identity with reports.

Confirm the containing revision, then mechanically prepare and validate the
diagnostic corpus copy. This removes only the named expired `gap` member:

```bash
git rev-parse HEAD
git merge-base --is-ancestor bbbda1c6e7d746c4bb605f3ddb736283828811fb HEAD
python3 - <<'PY'
import copy
import json
from pathlib import Path

path = Path('docs/impact-replay-corpus.json')
original = json.loads(path.read_text())
assert len(original['cases']) == 4
assert sum(len(case['expected']) for case in original['cases']) == 11
assert all(case['maxDepth'] == 2 for case in original['cases'])
prepared = copy.deepcopy(original)
case = next(case for case in prepared['cases']
            if case['id'] == 'conversation-delete-adopts-existing-route')
expectation = next(item for item in case['expected']
                   if item['category'] == 'affected_callers'
                   and item['value'] == 'ui/src/features/conversations/adoptProjectChat.adoptProjectChat')
gap = expectation.pop('gap')
assert gap['expiresOn'] == '2026-10-28'
output = Path('docs/impact-replay-corpus.no-gap.json')
output.write_text(json.dumps(prepared, indent=2) + '\n')
reopened = json.loads(output.read_text())
assert reopened == prepared
reopened_case = next(case for case in reopened['cases']
                     if case['id'] == 'conversation-delete-adopts-existing-route')
reopened_expectation = next(item for item in reopened_case['expected']
                            if item['category'] == 'affected_callers'
                            and item['value'] == 'ui/src/features/conversations/adoptProjectChat.adoptProjectChat')
reopened_expectation['gap'] = gap
assert reopened == original, 'changed something other than the one gap member'
print('verified: four cases, eleven expectations; only the expired gap removed')
PY
go run ./cmd/impact-replay --openexec "$REVIEWED_OPENEXEC" --repository . --corpus docs/impact-replay-corpus.no-gap.json --scan --report no-gap-first.json
go run ./cmd/impact-replay --openexec "$REVIEWED_OPENEXEC" --repository . --corpus docs/impact-replay-corpus.no-gap.json --report no-gap-reopened.json
```

The second process reopens the persisted graph. Run it even if the first replay
reports a missing expectation. Recovery requires both reports to show **11
matched, zero missing, zero bound-unavailable and zero unresolved anchors**, with
actual typed evidence for each expectation. This diagnosis predicts the
unsupported caller expectation remains missing; it supplies no bypass.

Linux/macOS hosted checks and independent exact-head review are delivery gates,
not claimed results of this document. Supervisor measurements after deploy
remain owed: install only reviewed green native by the existing release route,
replay on the fresh real candidate, normal train/checked activation for the
same #1583, then the same cart's exact-entry split/readback and original edited
artifact/back/refusal/reload journey. The owner settled these as after-deploy
measurements, not a premerge requirement. This diagnosis leaves their impact
gate blocked.
