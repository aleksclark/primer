# External verifier fixture

This is a standalone Go process. It intentionally has its own module, JSON
ledger, Docker image, and volume; it does not import `primer-tasks/internal`,
connect to PostgreSQL, or call a Tasks function. It is suitable for process
boundary and restart tests, not as a production verifier.

## Public contract

The fixture implements the Phase 6 `external_callback.v1` contract:

- `POST /v1/verify` receives a signed `RequestEnvelope` (the `/verify` alias is
  retained for local probes).
- The request uses the production camel-case fields: `version`, `requestId`,
  `attemptRef`, `requirementRef`, `schemaVersion`, `expiresAt`,
  `idempotencyKey`, `callbackPath`, `payloadDigest`, `payload`, and optional
  `allowedHandles`. `callbackPath` is a path only; a caller cannot supply a
  callback URL or headers.
- `POST <callbackPath>` is sent to the operator-configured
  `FIXTURE_CALLBACK_BASE_URL` and contains the production `CallbackEnvelope`
  (with `verifierId` set from the operator-owned `FIXTURE_VERIFIER_ID`),
  fields (`callbackId`, `requestId`, `attemptRef`, `verifierId`, `sequence`,
  `requestDigest`, `type`, and exactly one typed result).
- Both directions use the same public signature contract. The canonical input
  is `METHOD + "\\n" + PATH + "\\n" + RFC3339Nano timestamp + "\\n" +
  SHA-256(body) + "\\n" + request ID`; the HMAC-SHA256 header is
  `X-External-Signature: sha256=<hex>`, with `X-External-Timestamp` and
  `X-External-Request-ID` alongside it. Callback requests also include
  `Idempotency-Key: <callbackId>`.
- `POST /v1/reconcile` retries durable, unfinished records after a fixture
  restart. `GET /v1/ledger` exposes only redacted receipt/callback metadata.
  `POST /v1/control` accepts `{"action":"release","requestId":"..."}` or
  `{"action":"faults","faults":[...]}` and is protected by
  `FIXTURE_ADMIN_TOKEN` when set.

The ledger is written atomically with mode 0600. A repeated request with the
same idempotency key and body returns the original request ID and does not
re-run callbacks. A conflicting body/request ID returns 409. Callback IDs are
stable by request and sequence, so duplicates exercise the receiver's durable
idempotency rather than creating a new result.

## Faults

Set `FIXTURE_FAULT_MODE` to a comma-separated list, or use the control endpoint:

- `barrier`: persist receipt, then wait for the release control call;
- `lost_ack`: close the request socket after processing;
- `duplicate_callbacks`: send each callback twice;
- `out_of_order`: send accepted/rejected before progress;
- `corrupt_callback` / `stale_callback`: break callback body/signature or time;
- `http_429`, `http_500`, `timeout`, `retryable_error`, `terminal_error`.

`FIXTURE_OUTCOME=rejected` selects a typed rejected terminal result. All
progress and terminal results are safe, bounded fixture text; the submitted
payload is never logged or returned by the ledger endpoint.

## Egress modes

`FIXTURE_CALLBACK_BASE_URL` and `FIXTURE_CALLBACK_ALLOWLIST` are deployment
configuration, never request data. Redirects are rejected and no credential is forwarded to callbacks. The
Compose default is explicitly `FIXTURE_EGRESS_MODE=test` for local HTTP
service-DNS operation. Non-test mode requires an HTTPS callback base. The
Tasks production catalog owns the stronger HTTPS/allowlist/resolution/private
address/DNS-rebinding policy in `internal/securityreview`; its policy tests are
independent of this fixture process.
