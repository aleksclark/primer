# Phase 4 student dialogue evidence (sanitized)

## Prior Terra findings before current remediation

- Dedicated Chrome-only contexts successfully created/published/scheduled a fresh dialogue, paired a fresh student, and started a real student WebSocket.
- Earlier runs stopped fail-closed on: evaluator delivery timeout; duplicate parent-inspect rows/React keys; and an append-only override that temporarily replaced the inspect projection with empty state until reload.
- No pairing codes, cookies, QR material, raw browser snapshots, raw inspect payloads, or bulky logs are retained in this workspace.
- Playwright was not promoted. Android connected acceptance was not previously observed because the prerequisite emulator flow lacked required arguments.

## Current gate

The implementation has since received durable worker, inspect reconciliation, policy-boundary, and coverage-test remediation. A fresh configured Terra matrix must be run from a new parent context and new student context on the committed tip before this file is marked PASS. The matrix remains fail-closed until create/publish/schedule/pair, two accepted answers, reconnect, rejection/injection, isolation, concurrent recovery, provider failure, append-only override, responsive/a11y, and no-reasoning wire/DB/log checks all pass.
