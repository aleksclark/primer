# 07: IB3 / product BFF cookie, CSRF, and PKCE contract

**Status: NEXT — dispatch only after dual-reviewed IB2 hardening tip `0dd5faa` (branch `impl/IB2-hardening-remed`) is on `master`. Do not start from master `f31559d` alone. See [`../identity-ib2-resume.md`](../identity-ib2-resume.md).**

## Goal

Implement each product’s server-side OAuth client boundary against Primer Identity only after the candidate routes/cookie/redirect/resource/PKCE rules in [`../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md`](../stytch-identity-ib0/02-http-oauth-bff-mcp-contract.md) pass the IB0 zero-finding exact-tip review.

## BDD Success Criteria

#### Scenario: IB3-S1 — Product-owned browser session
- **Given** a product `/auth/login`
- **When** the broker flow completes
- **Then** the BFF validates state/`iss`, exchanges code+S256 verifier server-to-server and sets only `__Host-<product>-session`
- **And** JS/URL/storage contain no access/refresh/Stytch material.

#### Scenario: IB3-S2 — No implicit cross-product SSO
- **Given** successful Studio login
- **When** LMS authorization starts
- **Then** it creates a new broker transaction and separate host-only LMS cookie.

#### Scenario: IB3-S3 — CSRF/redirect fail closed
- **Given** missing/bad Origin/CSRF/state/verifier or unregistered redirect/resource/audience
- **When** a mutation/callback executes
- **Then** it is rejected without token issuance or open redirect.

## Implementation Instructions

Product BFF owns `/auth/login`, exact `/auth/callback`, `/auth/logout`, `/auth/me`, server-side pre-auth/session/refresh custody, and CSRF. Identity owns no product cookie. Token/revoke routes have no browser CORS. Keep registration static and exact; no DCR.

## End-to-End Test Plan

Run IB3-E01..E06 with real browser+BFF+Identity processes: exact state/iss/PKCE, separate products/cookies, browser storage/network scan, CSRF/Origin, open redirect/Host poisoning, restart durability and CORS denial.

## Anti-Cheating Audit

No localStorage, browser-readable token cookie, parent Domain cookie, frontend token exchange, wildcard redirect, implicit mock user, or inherited Identity SSO.

## Completion Gate

- [ ] IB3-S* and IB3-E01..E06 green.
- [ ] Production product/MCP remains blocked until IB4.
- [ ] Browser artifact scan and independent security review pass.
