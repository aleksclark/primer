# Live Stytch Proof — Residuals

**Tip:** This harness proves the Stytch identity client boundary against the
real Stytch test API (`project-test-*`). It is **not** the IB8-E10 full
browser/webhook end-to-end drill.

## What is covered

| Area | Tests | Credential required |
|------|-------|-------------------|
| Adapter session authenticate (garbage token) | `TestLiveStytchProviderQualification/garbage_session_token_*` | project-id + secret |
| Adapter revalidation (unknown tuple) | `TestLiveStytchProviderQualification/revalidate_*` | project-id + secret |
| Adapter pre-network guards | `empty_token`, `oversized_token` | project-id + secret |
| Adapter secret redaction | `adapter_string_redacts_secret` | project-id + secret |
| **Broker construction** | `TestLiveBrokerConstruction` | project-id + secret + public-token |
| **Broker StartLogin email OTP** | `TestLiveBrokerStartLoginEmailOTP` | project-id + secret (real API call) |
| **Broker StartLogin email magic-link** | `TestLiveBrokerStartLoginEmailMagicLink` | project-id + secret (skipped if product disabled) |
| **Broker SSO URL construction** | `TestLiveBrokerStartLoginSSO*` | public-token only |
| **Broker pre-network guards** | `TestLiveBrokerInvalidEmail*`, `TestLiveBrokerUnsupported*` | project-id + secret |
| **Broker/Config secret redaction** | `TestLiveBrokerSecretRedaction` | project-id + secret |
| **Webhook Svix signature verification (valid)** | `TestLiveWebhookSignatureAcceptsValid` | test webhook secret |
| **Webhook rejects tampered body** | `TestLiveWebhookSignatureRejectsTamperedBody` | test webhook secret |
| **Webhook rejects wrong secret** | `TestLiveWebhookSignatureRejectsWrongSecret` | test webhook secret |
| **Webhook rejects stale timestamp** | `TestLiveWebhookSignatureRejectsStaleTimestamp` | test webhook secret |
| **Webhook rejects missing headers** | `TestLiveWebhookSignatureRejectsMissingHeaders` | test webhook secret |
| **Webhook rejects wrong method/content-type** | `TestLiveWebhookRejects*` | test webhook secret |
| **Webhook project mismatch** | `TestLiveWebhookRejectsProjectMismatch` | test webhook secret |
| **Svix spec algorithm compatibility** | `TestLiveWebhookSignatureMatchesSvixSpec` | test webhook secret |
| Optional: full session happy path | `optional_live_session_authenticate` | session token |

## Exact residuals requiring additional credentials/configuration

| Residual | What's needed | Why blocked |
|----------|--------------|-------------|
| Full session authenticate + revalidate | `IDENTITY_LIVE_STYTCH_SESSION_TOKEN` — a live Stytch member-session opaque token | Requires a real member in the test project with an active session; manual browser login or scripted discovery-exchange produces one |
| Email magic-link start | Enable "Email Magic Links" in the Stytch test Dashboard and register `https://id.primer-test.example/broker/stytch/callback` as a redirect URL | Test project currently doesn't have this product configured |
| Webhook durable receipt (DB path) | A running PostgreSQL with the `webhook_events` table from migration `00010+` | Current tests prove signature verification only; DB commit is tested by the regular integration suite |
| Dashboard-delivered webhook | `IDENTITY_LIVE_STYTCH_WEBHOOK_SECRET` from the Stytch Dashboard → Webhooks panel and an actual configured endpoint | Proves Stytch's production event delivery matches our signature verification |
| Full browser BFF callback | A real browser flow completing magic-link/OTP/SSO and presenting the one-time token to `CompleteTypedCallback` | This is IB8-E10; requires browser automation or manual login |
| Product cookie minting | Out of scope by design | Identity client only, not product session |
| Email account merge | Out of scope by design | `IB0-D03`: email never links identities |

## Environment variables

```bash
# Required
IDENTITY_LIVE_STYTCH=1
IDENTITY_STYTCH_PROJECT_ID=project-test-...
IDENTITY_STYTCH_SECRET=secret-test-...
IDENTITY_STYTCH_ENV=test

# Optional (from IDENTITY_LIVE_STYTCH_ENV_FILE=$HOME/.config/primer/stytch-test.env)
IDENTITY_STYTCH_PUBLIC_TOKEN=public-token-test-...

# Optional extended coverage
IDENTITY_LIVE_STYTCH_SESSION_TOKEN=<opaque session token>
IDENTITY_LIVE_STYTCH_WEBHOOK_SECRET=whsec_<base64-encoded key from Dashboard>
```

## Run

```bash
make identity-live-stytch
```

## File locations

- `primer-identity/internal/testutil/live/stytch_live_test.go` — adapter session proof
- `primer-identity/internal/testutil/live/broker_live_test.go` — broker/BFF proof (NEW)
- `primer-identity/internal/testutil/live/webhook_live_test.go` — signed webhook proof (NEW)
- `primer-identity/internal/testutil/live/doc.go` — package documentation
