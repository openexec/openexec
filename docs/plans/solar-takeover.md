# Solar takeover task split

The [native plan](solar-takeover.json) separates the five supplied conditions
under the existing outcome, “Document and prepare the Solar system takeover”.
It defines tasks; none of their documentation or readiness evidence exists as a
result of this split.

| Task ID | Deliverable in the authorized Solar workspace | Depends on |
| --- | --- | --- |
| system-understanding | `docs/solar/system-understanding.md`: architecture and communication, checked against a real end-to-end flow | — |
| environment-setup | `docs/solar/environment-setup.md`: setup from zero, exercised in an isolated environment including an invalid configuration | — |
| safe-operation | `docs/solar/safe-operation.md`: operations and recovery procedures with isolated recovery evidence | system-understanding, environment-setup |
| development-release | `docs/solar/development-release.md`: development through release and rollback, including a check that refuses a bad change | system-understanding, environment-setup |
| takeover-plan | `docs/solar/takeover-plan.md`: service continuity, handover and aborted-handover walkthroughs | All four preceding tasks |

Each task includes scope, completion evidence and a technical strategy in the
JSON. Its `verification_script` checks only that its deliverable exists and is
nonempty. That check alone cannot prove the documented procedure works; the
task's execution and independent review must inspect the specified evidence.

The request's final condition ends at “Takeover preserves service with”. That
fragment is retained verbatim and flagged in both the story criteria and the
takeover task. Recover the full accepted wording from Solar's durable context
before judging that condition. No missing ending or additional condition is
invented here.

This is a prepared plan in the OpenExec repository, not live Solar backlog
state. Do not place it in this project's `.openexec/stories.json`. Routing,
review and import belong to the existing native planner in the authorized Solar
context; this request grants no access to that workspace. The plan adds no
Goal, changes no accepted Ready condition and authorizes no production effects.

The regression test `TestSolarTakeoverTaskSplit` reads this exact JSON through
the real native planner and review/import path into a disposable SQLite ledger,
then closes and reopens it. Its scripted reviewer is a test control, not an
independent assessment of Solar. A rejected review must leave no imported work.
The test is part of `go test ./...`, already run by the declared test suite.
