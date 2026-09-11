# Native empty-response evidence

## Observed failure and remaining uncertainty

Console 6914830 / OpenExec a90166c ordinary Fresh run
534ae0cb0085a97c4e769c8a3959d774 received the #167 task packet but performed
only index/list discovery. Its second inference returned no visible outcome;
empty-response recovery then correctly refused another inference: 26,588
tokens remained in a 65,536 reservation, below the unchanged 34,816 admission.
Automatic replacement 3151b1c0a5773be4dc717cbe242d0607 returned no tools or text
after one bounded recovery attempt. Neither changed the original task.
Receipts: /mnt/data1/work-owned-166/task-start-167/.

These facts falsify packet-only sufficiency, not packet identity or budget
enforcement. Native termination reason and per-call usage were not retained;
the executor also checked reasoning_content while the native adapter supplied
thinking. Therefore the existing error cannot distinguish output exhaustion,
reasoning-only output or another empty protocol response. Do not infer which
one occurred, or increase limits, from that error alone.

## Smallest correction

Preserve typed, sanitized native completion observations: whitelisted reason,
native input/output counts, their existing caps, and presence of visible text,
reasoning and tool calls. Attach observations to the existing empty-response
failure, including when the recovery request is refused before dispatch.
Recognize both reasoning metadata keys. Never log private reasoning, response
text, tool arguments, model identifiers, credentials or request bodies.

This changes diagnosis only: one bounded retry, admission, accounting,
truncate:false, task completion and authority all remain unchanged. It does not
establish useful task advancement or fix a still-unknown generation cause.
After deployment, the next ordinary Fresh must provide discriminating evidence
or useful persisted work; another opaque unchanged retry is not justified.

## Candidate completeness and validation

Affected path: native HTTP response -> typed adapter metadata -> bounded
accounting -> empty-response recovery -> existing failure event/receipt.
Adjacent cases checked: invalid/unexpected provider reason, private thinking,
native versus compatible reasoning key, recovery admission failure and the
same hard resource limits. Existing project/legacy loading formats and CLI
worker paths are untouched; native protocol integration tests cover this change.

Focused tests pass. Integrated HTTP fixture reproduces native empty output then
insufficient recovery capacity, proving exactly one HTTP request and unchanged
native accounting, while retaining numeric evidence and excluding private text.
The native thinking-only fixture verifies truthful classification and exactly
one existing recovery attempt. Full Go gate and independent review are required
before publication. Live evidence remains pending; no V3 Ready claim.

Negative controls: omitting the first native observation fails the
recovery-refusal fixture; ignoring the native thinking key fails the
classification fixture. Both are restored. Focused race tests and vet pass;
independent source review found no blocking findings.
