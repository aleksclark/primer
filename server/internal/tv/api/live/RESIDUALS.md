# IB8 TV Admin Live Identity Proof — Residuals

## What is covered (opt-in `live_stytch` build tag)

| Area | Test | Evidence |
|------|------|----------|
| JWT accepted via live JWKS endpoint | `jwt_accepted_via_live_jwks` | Real HTTP round trip: JWKS fetch → ES256 verify → admin route authenticated |
| Stytch/Identity outage (JWKS unreachable) | `stytch_outage_jwks_unreachable` | JWT rejected; service key provides degraded access |
| Delayed webhook / key rotation | `delayed_webhook_key_rotation` | Token with unknown kid rejected → JWKS propagates new key → token accepted |
| JWT expiry | `jwt_expiry_rejected` | Expired token rejected; near-expiry token accepted |
| Raw Stytch session JWT rejected | `raw_stytch_session_jwt_rejected` | typ=JWT token always rejected regardless of signature validity |
| Device token cannot auth admin | `device_token_cannot_auth_admin` | Opaque token on admin route → 401 |
| No Stytch SDK structural | `no_stytch_sdk_structural` | Compilation proves no Stytch import |

## Evidence location

```
/var/tmp/primer-ib8-tv-admin-<unix-timestamp>.log
```

Each test run writes a timestamped log file with status codes and timing.

## Run commands

```bash
# Full live identity proof (writes evidence to /var/tmp):
TV_LIVE_IDENTITY_PROOF=1 go test -tags live_stytch \
  -run TestLiveTVIdentityProof -v \
  ./server/internal/tv/api/live/ -count=1

# Quick verification (no build tag, runs without evidence):
go test -run "TestTVAdmin" ./server/internal/tv/api/ -v -count=1
```

## Exact residuals requiring additional infrastructure

| Residual | What's needed | Why unavailable in this environment |
|----------|--------------|-------------------------------------|
| Real Identity service JWKS endpoint | A running `primer-identity` binary with `/.well-known/jwks.json` | Requires PostgreSQL + key material + the identity-server process; the live test uses an httptest JWKS server that exercises the same code path |
| Live Stytch test-project session → Identity → TV JWT flow | Active Stytch test session → Identity mints at+jwt → TV verifies | Requires browser login to Stytch test project and a running Identity service; the live test proves the TV verification boundary using identical cryptographic operations |
| Webhook delivery from Stytch → Identity → key rotation | Real Stytch dashboard webhook push | The `delayed_webhook_key_rotation` test simulates the exact JWKS propagation delay; the webhook delivery mechanism itself is proven by `primer-identity/internal/testutil/live/webhook_live_test.go` |
| Production-like network latency | Real network between TV ↔ Identity | httptest exercises the full HTTP stack; only TCP latency differs |
| Certificate validation (TLS) | HTTPS JWKS endpoint | Not testable with httptest; production uses TLS by default |

## Boundary guarantees proven

1. **No Stytch SDK in TV** — The TV server only imports `identityauth` (pure ES256 + JWKS fetch). No `stytch-go` or provider client exists in the dependency tree.

2. **No raw Stytch token acceptance** — The `typ=at+jwt` header check rejects Stytch session JWTs (`typ=JWT`) before signature verification even begins.

3. **No provider role authorization** — The TV admin guard accepts any valid Primer JWT with the correct audience (`primer-tv`). It does not inspect claims for Stytch org/member roles.

4. **Device tokens remain TV-local** — Device token auth and admin auth are completely separated at the middleware level. A device token cannot access admin routes and vice versa.

5. **Fail-closed on partial config** — When the identity verifier IS configured, anonymous access and invalid tokens are rejected. Service key path provides separate degraded access during outage.

6. **JWKS outage resilience** — If the Identity service is down, JWT auth fails but the service key path allows continued operation for automated callers (content-ingest, LMS).

## Sanitization

- No production credentials used or referenced
- No secrets logged in evidence files
- Test keys are ephemeral (generated per test run)
- Evidence files contain only status codes and timing
