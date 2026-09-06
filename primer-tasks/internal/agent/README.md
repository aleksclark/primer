# Phase 3 runtime contract

Fantasy is pinned to `charm.land/fantasy v0.41.1` (requires Go 1.26.5).
`Runtime` bounds steps, tokens, retries and time. It projects only final text,
usage and allowlisted progress labels. Reasoning callbacks discard content;
provider metadata and raw tool results never enter the event projection.
The pin ignores some `OnToolResult` callback errors, so the adapter also
cancels and records callback failure, and rejects invalid/provider-executed
tool calls before dispatch. A confirmation result stops the model turn.

Production wiring lives in `internal/api/agent_*.go`. Admission commits the
user message, run, job and message acknowledgement together. PostgreSQL holds
all conversations, messages, jobs, run events, previews and effects. Socket
subscribers read cursor-ordered database pages (32 events), with a 64-event
acknowledgement window and bounded writes. A hub notification is only a wakeup;
it is neither history nor a tenant-wide broadcast. Unsubscribed peers receive
no durable events. Slow subscribers close with code 1013; healthy sockets and
workers do not wait for them. Both reconnect and a different server process
use the same database query.

`internal/jobs.Worker` leases real PostgreSQL jobs using conditional claims and
renewal. Mutations validate the active lease at their transaction boundary.
Cancellation locks the same run row as an effect. A committed pending preview
survives worker replacement without rerunning the model; a terminal run is not
rerun. A recorded mutation boundary is not replayed from prompt zero. Run state,
final assistant text and terminal event commit together; exhausted/expired
work is reconciled to a safe terminal outcome. All post-admission work is
owned by the server/worker context, not the websocket request.

The incoming production migrations are `internal/db/migrations/00006*` through
`00009_parent_agent_authority.sql`, renumbered after the already-applied Clerk
migration `00005_clerk_parents.sql`, whose bytes remain unchanged.
`schema.sql` is historical contract documentation, not an alternate persistence
backend; integration tests now execute the real migration chain.

Confirmed mutations publish a system-authored receipt (`source=domain`), not
fabricated model output. Semantic text clauses and an authoritative text-end
replacement are committed with the actual canonical effects and final message;
there is no additional model/tool turn or artificial typing delay. A version-stale
confirmation is durably retired with an actionable failed terminal result, while
credential-refreshable pending previews retain their normal authority fences.

Tasks directly pins the maintained `github.com/coder/websocket v1.8.14` transport.
The retired nhooyr pin echoed a second Close frame after a peer replied to a
server-initiated Close; first-CloseError-only Go/Node tests missed that browser
protocol violation. Qualification now asserts exactly one Close frame followed
by EOF after the peer reply. The existing root workspace graph selects v1.8.15;
both the standalone direct pin and workspace graph are tested without changing
another module's pins. Handler cleanup uses CloseNow rather than a competing
normal-close handshake. The 64-event window and write deadlines are unchanged.

Qualification uses a credential-free scripted Fantasy model and real PostgreSQL.
It establishes runtime/API behavior, not live-model quality or readiness.
