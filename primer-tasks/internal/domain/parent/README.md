# Parent tools, authority and confirmation

`Tools` defines domain-service seams. Model inputs cannot select the tenant,
actor, run identity, idempotency key or active tool set. Production constructs
these from authenticated, durable server records. Empty/unknown allowlists
fail closed; unavailable tools are omitted from Fantasy's schema. The model
has no tool that can acknowledge its own preview.

All model-requested mutations prepare a preview. The scripted create-and-schedule
request prepares one composite action; confirmation atomically creates a draft,
publishes it, creates its schedule and materializes occurrences through the
ordinary P2 handlers. Other task/schedule effects use those same handlers,
including validation, timezone/DST rules, publication rules and tenant predicates.
`db.Database` permits binding them to a PostgreSQL transaction/savepoint; it is
not a fake persistence implementation.

`SQLConfirmationStore` owns opaque, single-use handles. The production adapter
additionally binds the preview to a run and effect step, verifies current parent
membership, actor/conversation ownership, cancellation, expiry, current tool
allowlist and server-read CAS versions. Consuming the preview, applying domain
effects, recording the effect results and terminal event are one transaction.
Failure rolls everything back; duplicate acknowledgement observes the committed
result without a second mutation. Versionless bulk edits fail closed.
The memory confirmation double is compiled only in tests.

Provider configuration remains explicit:

- `disabled`: no model is invoked; a durable unavailable outcome directs the
  parent to the ordinary task/schedule pages.
- `scripted`: credential-free development/test qualification, rejected in
  production. It uses Fantasy's public tool loop, not fake HTTP or persistence.
- Bedrock/OpenRouter: server-side factories only. Credentials never enter
  prompts, tool context, DB events or UI. No live-provider qualification is
  claimed for this integration.

Model output is untrusted. Stored action payloads are canonical domain values,
not raw model envelopes; progress uses server-owned summaries and labels.
Reasoning deltas and provider error bodies are not persisted or streamed.
