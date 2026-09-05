# Phase 08: Integrate external verifiers

## Goal

Land donor Phase 6 final snapshot (`d6e89d4a`) so allowlisted external tools can
receive signed, versioned, idempotent verification jobs and return durable
progress/results through the same authoritative verification engine. Proof uses
a separate verifier process and real persistence, with SSRF/egress, callback,
secret-rotation, lease, retry, and race defenses.

## BDD Success Criteria

### Scenario: Separate verifier completes a signed job once

- **Given** an enabled allowlisted verifier, bound protocol version/secret
  version, and eligible occurrence
- **When** the Tasks worker dispatches to the separate fixture process and the
  fixture sends signed progress and a final callback
- **Then** authenticated progress is visible through the public UI/API
- **And** one durable verification decision and occurrence completion result.

### Scenario: Replay, race, and restart remain idempotent

- **Given** duplicate/out-of-order callbacks, concurrent workers, lease handoff,
  process restart, timeout, and retry/supersession
- **When** delivery/callbacks race
- **Then** fencing and idempotency accept only the current job/attempt/secret
  binding
- **And** terminal state cannot be overwritten or duplicated.

### Scenario: Untrusted endpoints and callbacks fail closed

- **Given** loopback/link-local/private/metadata/DNS-rebinding targets, redirects,
  wrong protocol/secret version, forged/unsigned/oversized/stale callbacks, or a
  disabled catalog entry
- **When** selection, dispatch, redirect, or callback occurs
- **Then** the request is rejected before unsafe egress or state mutation
- **And** diagnostics expose bounded reason codes without secrets or payloads.

### Scenario: Catalog selection is tenant/admin bounded

- **Given** parents, product admins, two tenants, and multiple verifier catalog
  entries
- **When** a task selects or invokes a verifier
- **Then** only authorized safe catalog choices are accepted
- **And** another tenant cannot observe configuration, progress, callback facts,
  or decisions.

## Implementation Instructions

- Apply the non-plan donor delta `e196dd2c..d6e89d4a` after P5. Preserve the
  final migrations 00015–00020, protocol envelopes, verifier catalog/admin
  boundary, worker/lease/fence logic, callback canonical storage, secret-version
  binding, retry supersession, and separate fixture service.
- Reconcile all four P6 leaf lines against final donor. Specifically compare
  `7b0231c3`'s callback/egress changes with final files and retain any safety
  invariant not semantically present; record proof before classifying the leaf
  superseded.
- Keep endpoint resolution and redirect validation at every network hop. Use
  restrictive allowlists and production configuration; a test-only bypass must
  never activate in production.
- Keep signatures canonical/versioned, compare securely, bind job/attempt/body,
  and support explicit rotation without accepting an unbound old secret.
- The fixture must run as a separate process/container and keep its own replay
  ledger. Do not link it in-process for E2E.
- Open one P6 PR. Do not add Phase 7 release/invitation/production claims.

## End-to-End Test Plan

- Run `make tasks-test tasks-cover tasks-clients tasks-web tasks-e2e` and `make
  tasks-external-e2e`.
- Start real PostgreSQL plus Tasks and the independent verifier fixture as
  separate processes. Dispatch through the public API/UI, observe progress in a
  managed headless browser, restart both sides, replay callbacks, and assert one
  persisted terminal decision/completion.
- Run signature/idempotency/version/rotation/lease/concurrency/retry and
  supersession tests, including real DB contention.
- Run planted-red SSRF/redirect/DNS/metadata/private-network tests and malformed,
  oversized, stale, wrong-job, wrong-attempt, forged, and unsigned callbacks.
- Run two-tenant/product-admin catalog authorization and public progress IDOR
  negatives.
- Compare final integrated non-plan tree with `d6e89d4a`; separately audit the
  protocol leaf `7b0231c3` invariant-by-invariant.

## Anti-Cheating Audit

- Verify E2E process identities/ports prove a separate verifier and that no
  in-process fake handles the claimed protocol.
- Inspect durable job/callback/decision rows and uniqueness/fencing constraints;
  HTTP 200 or fixture ledger output alone is insufficient.
- Inspect signature canonicalization, constant-time verification, body limits,
  protocol and secret-version binding.
- Inspect URL parse/resolve/dial/redirect paths for alternate-IP, rebinding,
  private-network, credential, and scheme bypasses.
- Check retries are bounded and do not swallow terminal failures or mutate a
  superseded attempt.
- Check catalog/tenant/admin authorization server-side and secret redaction in
  logs/browser/evidence.

## Completion Gate

- [ ] All P6 BDD scenarios pass across separate real processes and PostgreSQL.
- [ ] Managed-headless exploration, promoted external-progress Playwright, and
      `tasks-external-e2e` pass.
- [ ] Signature, replay, restart, lease, race, secret rotation, retry,
      supersession, SSRF, redirect, and tenancy planted-red gates pass.
- [ ] Exactly one durable terminal decision/completion is proven.
- [ ] Tasks 85% coverage and Go/client/web/build/diff gates pass.
- [ ] Final donor and protocol-leaf semantic audits have no unexplained gap.
- [ ] Phase 7/native continuation remain unclaimed.
- [ ] Green independently reviewed P6 PR merges.
