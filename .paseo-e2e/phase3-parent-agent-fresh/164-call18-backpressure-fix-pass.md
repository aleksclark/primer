# Call #18 — 8182524 public slow-subscriber re-drive PASS

## Fresh stack

- Preserved evidence, reset to `8182524b528afed8a5dd4a6f9b7db663314523a8`, then destroyed and recreated the real Stacklane-aware Compose/Postgres/scripted-Fantasy stack.
- `primer-tasks/scripts/dev check` passed. Stacklane FQDN was unresolved; used documented fresh loopback `http://127.0.0.1:38177`.

## Actual close and cursor recovery

1. Fresh Parent B BFF sign-in ran a real public `List my tasks.` command to cursor 11 (`157`–`160`).
2. A temporary, deleted public WebSocket probe used the browser's authenticated BFF cookie and CSRF subprotocol. It subscribed at cursor 11 with a 1 KiB receive buffer and deliberately did not read; an independent public authenticated WebSocket submitted 20 read-only List commands. No private mutation or fake state was used.
3. When the delayed slow reader resumed, it observed an actual closed network connection after the command burst (`161-call18-public-slow-subscriber-probe.json`). This is the required socket-level close after subscriber eviction, rather than the prior open-but-silent behavior.
4. A fresh public authenticated reconnect subscribing from cursor 220 replayed ordered durable progress through terminal cursor 231 (`162-call18-public-reconnect-cursor-replay.json`). Authenticated public reads still showed zero Parent B tasks and schedules (`163-call18-post-backpressure-public-effects.json`).

## Full exploratory disposition

The corrected restricted-active-tool, durable remount/restart, cancellation/failure/disabled, confirmation/stale, tenant/isolation, Origin/CSRF, reasoning/storage/log sanitation, responsive theme/mobile, and axe evidence are recorded across this suite. Axe treatment is explicit: mobile dark/light have zero violations/no incomplete checks; desktop dark/light have zero violations with axe `color-contrast` incomplete, recorded transparently alongside 100 Lighthouse accessibility evidence rather than mislabeled as an axe full pass.

Evidence sanitization scan found no cookie, CSRF secret, authorization, or bearer material. Temporary public WS helper and axe asset are deleted. No Playwright was written.

Exploratory acceptance is PASS; set the suite to `codify` for the next call.
