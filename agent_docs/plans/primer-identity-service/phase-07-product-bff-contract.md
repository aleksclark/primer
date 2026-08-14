# Phase 7: Product BFF confidential client contract

**File:** `phase-07-product-bff-contract.md`
**Depends on:** Phases 5–6
**Duration guess:** 5–7 days
**Migration stage:** S1→S2 readiness
**Handoff wave:** W4

## Goal

Specify and implement the **normative BFF OIDC client contract** Identity exposes to Studio/LMS/TV-admin BFFs, plus a reference helper library (`internal/bffref` or `pkg/bffoidc`) so product implementers do not invent APIs. Prove confidential code exchange, host-only product session shape, CSRF on mutations, and open-redirect-safe `return_to` using a real in-repo BFF test double.

## Scope

### In scope

- Document exact endpoints BFFs call (Identity side already Phase 4–6):
  - BFF `GET /auth/login?return_to=` → 302 Identity `/oauth/authorize` with client_id, redirect_uri, state, code_challenge S256, scope, nonce
  - BFF `GET /auth/callback` → verify state binder cookie; POST Identity `/oauth/token`; establish **product** server session; Set-Cookie `__Host-bff_session` host-only
  - BFF attaches `Authorization: Bearer <access JWT>` to product API (mint/refresh)
  - BFF `POST /auth/logout` + CSRF → revoke local + Identity revoke (Phase 9)
- Reference implementation runnable under `primer-identity/internal/bffref` used only in tests **or** thin shared package products may vendor
- return_to allowlist: relative paths only on product origin
- CSRF double-submit or synchronizer for BFF cookie mutations
- SPA never receives access/refresh tokens in JSON for production mode
- Client seed docs for `studio-bff`, `lms-bff`, `tv-admin-bff` audiences
- Negative: token endpoint requires client secret (confidential)

### Out of scope

- Full Studio SPA (platform plan)
- LMS SPA cutover wiring (Phase 13) beyond contract proof
- Refresh rotation details (Phase 9) — stub refresh handle ok if marked

## BDD Success Criteria

#### Scenario: P7-S1 — BFF login redirects to Identity authorize

- **Given** confidential client registered
- **When** user hits BFF `/auth/login?return_to=/app`
- **Then** 302 to Identity authorize with PKCE challenge and state
- **And** BFF sets host-only login binder cookie

#### Scenario: P7-S2 — BFF callback exchanges code server-side

- **Given** Identity issued code to BFF redirect_uri
- **When** browser hits BFF callback with code+state
- **Then** BFF uses client_secret server-side at token endpoint
- **And** browser receives only host-only session cookie (no access token body to JS)

#### Scenario: P7-S3 — BFF session calls product with Bearer JWT

- **Given** BFF session established
- **When** browser XHR same-origin `/api/probe` via BFF
- **Then** upstream product sees valid JWT aud for that product
- **And** JWT obtained without browser-visible token

#### Scenario: P7-S4 — return_to open redirect denied

- **Given** login with `return_to=https://evil.example/`
- **When** flow completes
- **Then** redirect lands on safe default not evil
- **And** relative `return_to=/settings` allowed

#### Scenario: P7-S5 — Access token not in browser storage

- **Given** completed login in browser harness
- **When** read document.cookie and localStorage/sessionStorage
- **Then** no access_token/refresh_token values
- **And** session cookie HttpOnly (not readable from JS)

#### Scenario: P7-S6 — Separate product sessions

- **Given** same Identity account
- **When** login to studio-bff and lms-bff test doubles
- **Then** two distinct product session ids
- **And** no shared parent Domain cookie

#### Scenario: P7-S7 — CSRF required on BFF logout/mutation

- **Given** session cookie without CSRF
- **When** POST `/auth/logout`
- **Then** 403
- **And** with valid CSRF proceeds (revoke integration may be stub until P9)

## Implementation Instructions

1. Write `primer-identity/docs/bff-client-contract.md` (short) linking this phase — or keep contract solely in this plan + godoc on bffref (prefer godoc + plan to avoid extra doc sprawl; plan is enough if bffref is complete).
2. Implement `bffref` HTTP handlers + session store interface (memory ok for unit; Postgres/sqlite for E2E).
3. State binder cookie distinct from Identity correlation cookie.
4. Token response parsing: store refresh handle server-side only when Phase 9 lands; until then access-only short session re-auth acceptable if documented.
5. Provide golden HTTP traces in tests as comments for Studio/LMS implementers.
6. Do not modify LMS web yet (Phase 13).

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P7-E1 | Identity server + bffref + loopback | full login jar | BFF cookie host-only; product probe 200 with JWT aud | `go test ./internal/bffref -run E2ELogin -race` |
| P7-E2 | evil return_to | login | safe redirect | `go test ./internal/bffref -run ReturnTo` |
| P7-E3 | storage inspection harness | login | no tokens in storage | `go test ./internal/bffref -run NoBrowserTokens` |
| P7-E4 | CSRF matrix | logout | 403 then 2xx | `go test ./internal/bffref -run CSRF` |

## Anti-Cheating Audit

- Callback must not return tokens in query fragment to browser.
- client_secret only on server token POST.
- Tests use real HTTP Identity, not stubbed token JSON without OP.
- CSRF not disabled globally in test env for “convenience” without separate unsafe build tag.

## Completion Gate

- [ ] P7-S*/E* green
- [ ] Contract paths stable and named for Studio/LMS
- [ ] Anti-cheat clean
- [ ] S2 unblocked for Studio platform integration

## Dependencies

- Upstream: 5–6
- Downstream: 9, 12–14
- Sibling: platform plan Phase 2/9 consumes this contract

## Rollback

- Revert bffref; Identity OP remains.
