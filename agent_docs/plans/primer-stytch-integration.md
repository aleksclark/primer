# Primer Stytch Integration - Wave A (Foundation)

**Status:** Wave A implemented; quality/security approved. Specification re-review was requested after the documentation corrections below. No JWT, JWKS, webhooks, BFF exchange, or live-provider proof is claimed.

**Base commit:** ae5e4c6890c29a0a97095816e8d217d00471d72b (worktree impl/stytch-identity-core)
**Date:** 2026-08-15
**Scope (Wave A only):** primer-identity sole Stytch client. Fail-closed Stytch config. Vendor-neutral B2B client iface + v18.1.0 production adapter returning bounded internal session snapshot (project/org/member IDs, active/eligible state, expiry, coarse roles). Concurrency-safe bounded validation cache (HMAC key, 15s positive TTL capped, 5s negative TTL, bounded deduplicating load registry, bounded eviction, no raw token log, no negative caching on transient errors). Additive normalized mapping for (project_id, organization_id, member_id) -> account UUID (transaction/concurrency safe first-login; same email across orgs = separate accounts). TDD vertical slices with RED/GREEN evidence. No email as identity key. Products never see Stytch tokens.

**Explicitly deferred (Wave B+ after review):** ES256/JWKS, Primer access JWT minting, BFF exchange endpoints, Stytch webhook handling/invalidation, full OAuth facade, persistent key material, live credential tests.

## Official Stytch Go SDK Contract (v18.1.0, authoritative)
- Module: `github.com/stytchauth/stytch-go/v18`
- B2B client: `github.com/stytchauth/stytch-go/v18/stytch/b2b/b2bstytchapi`
  - `b2bstytchapi.NewClient(projectID, secret, opts...)` → `*b2bstytchapi.API`
  - Supports `WithBaseURI`, `WithHTTPClient` (for fake boundaries in tests), `WithClient`.
- Sessions: `api.Sessions.Authenticate(ctx, &sessions.AuthenticateParams{SessionToken: tok, SessionDurationMinutes: 0})`
  - Returns `*sessions.AuthenticateResponse` containing:
    - `MemberSession`: `OrganizationID`, `MemberID`, `ExpiresAt`, `Roles []string`, `MemberSessionID`, ...
    - `Member`, `Organization`
    - `SessionToken`, `SessionJWT` (we use opaque token only; never rely on local JWT for authority)
  - Errors: `stytcherror.Error` with `ErrorType` for definitive cases (e.g. session revoked/invalid/not_found).
- Project ID is configuration; not returned in every session response — captured from client config for snapshot.
- No raw tokens or PII in our logs/metrics. RequestID used for correlation only.
- Evidence from: SDK source (types.go, b2bstytchapi.go, stytcherror.go), Stytch B2B docs (Sessions.Authenticate, data model, session object).

See architecture review for full contract (cache keys, mapping tuple, fail-closed, boundaries).

## Decisions (Wave A)
- **Fail-closed config:** `IDENTITY_STYTCH_PROJECT_ID`, `IDENTITY_STYTCH_SECRET` (required when Stytch enabled). `IDENTITY_STYTCH_ENV=test|live` (default test), optional development/test-only `IDENTITY_STYTCH_BASE_URI`. Production requires the live environment and rejects every endpoint override. Cache TTLs/capacities are hard-capped at 15s/5s and 10,000/2,000. Health/OpenAPI tests use explicit disabled/fake config.
- **Vendor-neutral iface:** `type StytchClient interface { AuthenticateSession(ctx context.Context, sessionToken string) (StytchSessionSnapshot, error); InvalidateSession(ctx context.Context, token string) error }`. Prod adapter wraps official v18 B2B client. Fake for tests.
- **Snapshot (bounded, no email):**
  ```go
  type StytchSessionSnapshot struct {
      ProjectID      string
      OrganizationID string
      MemberID       string
      Active         bool
      Eligible       bool
      ExpiresAt      time.Time
      Roles          []string // coarse from MemberSession only
  }
  ```
- **Cache:** Key = HMAC-SHA256(per-process-secret, projectID + "\x00" + token). Positive TTL min(15s, provider expiry). Negative TTL 5s for definitive invalid/revoked provider errors and for successful snapshots that fail project, active, eligible, identity, or expiry checks. Never negative-cache 5xx/429/timeout/transport failures. Separate bounded LRUs use O(1) requested-key expiry checks and a 16 MiB aggregate positive payload budget. A generation-fenced, deduplicating registry admits at most 128 unique provider loads by default before spawning work; each uses an independent 3s context. Opaque tokens are capped at 4096 bytes. Invalidation barriers prevent stale insert/return. Bounded counters only (no high-card labels). Never log token/hash/ID/email.
- **Mapping:** New additive migration for normalized table (preferred over ambiguous concat in provider_subject):
  - stytch_mappings (project_id, organization_id, member_id) UNIQUE → account_id.
  - Repo: `FindAccountIDByStytchTuple` performs exact lookup; `CreateStytchMapping` is a strict insert; `ResolveOrCreateStytchMapping` owns serializable creation, bounded retry, and conflict re-read.
  - Same email across orgs → distinct accounts (existing ListByEmail behavior + no auto-link).
  - The generic `external_identities` provider model remains unchanged; Stytch uses only the normalized mapping table.
- **TDD:** RED tests first (config fail, adapter fake HTTP, cache behaviors, definitive vs transient, non-leak, same-email mapping, concurrent first-login, migration constraints). Then GREEN. Use testcontainers + real patterns.
- **Gates:** `cd primer-identity && go test ./... -race -count=1`, `go build ./...`, focused cover, `git diff --check`, and credential-shaped scan (no values printed). Commit/push remain separately authorized delivery actions.

## Wave A implementation
- Added the direct current `stytch-go/v18 v18.1.0` dependency; the base had no Stytch SDK dependency.
- Added fail-closed optional Stytch configuration and the bounded v18 session adapter under `internal/stytch`.
- Added the HMAC-keyed, bounded positive/negative validation cache and deduplicating load registry under `internal/stytchcache`.
- Added normalized domain/repository mapping and migration `00003_stytch_mappings.sql`; the existing generic external-identity provider model was not overloaded.
- Added config, adapter, cache, mapping, concurrency, and migration behavior tests.
- Updated migration rollback coverage so one down removes only the additive Stytch table and a second down reaches the account-free foundation.
- Remediated review findings with live/test project-environment binding, production/live endpoint pinning, a 1 MiB provider-response limit, bounded/validated snapshot strings and roles, bounded unique-load admission, O(1) cache lookup maintenance, generation barriers and cancellation isolation, and serializable mapping transactions with bounded SQLSTATE retry.
- No JWT, exchange, webhook, app/API wiring, browser BFF, or live Stytch call was added.

## Remaining after Wave A review
- Wave B: JWT mint (ES256), JWKS, Primer token profile per contract.
- Wave C: BFF-facing exchange, webhooks + invalidation, full flows.
- Live Stytch integration tests (credential-free only here).
- OpenAPI surface, metrics, etc.

## Wave A verification

- `GOWORK=off go test -race ./... -count=1` — PASS
- `GOWORK=off go build ./...` — PASS
- Changed-package aggregate coverage — 81.6%; config 98.6%, adapter 81.7%, cache 90.2%, domain 89.1%, repository 69.4%, database 78.9%
- `git diff --check` — PASS

The live Stytch project remains a later explicit gate. Credential-free tests exercise the official SDK through a bounded fake HTTP boundary.
