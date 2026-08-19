# 13: IB8 / LMS transitional cutover

## Goal

Migrate LMS without weakening local educator roles. This plan is Stytch-backed: Stytch is upstream human-session authority; Primer Identity is the only Stytch client and mints only downstream Primer material where this phase authorizes it.

## BDD Success Criteria

### Scenario: IB8 / LMS transitional cutover

- **Given** the preceding phase gates and a bounded, sanitized test environment
- **When** lMS dual-runs explicit legacy path and Primer JWT path, accepts no raw Stytch token, and maintains local educator-role enforcement.
- **Then** the behavior is observable through the named public boundary and durable local evidence
- **And** no raw Stytch session, SessionJWT, provider payload, or provider-derived product role crosses the Identity boundary

## Implementation Instructions

- Preserve exact tuple mapping and no-email-merge semantics; never use Stytch organization/member roles as product authorization.
- Keep product authorization and host-only cookie/CSRF ownership in the product BFF; Identity is neither a product membership store nor an independent human session authority.
- Record the durable source of truth, failure semantics, migration/rollout constraints, audit fields, and focused test command before implementation.
- **Dependency gate:** IB3–IB6.

## End-to-End Test Plan

Migration E2E proves class separation and legacy sunset gates. Use public endpoints/processes and a real local durable store where applicable; permitted provider fakes prove only the bounded Identity adapter boundary and cannot substitute for the explicit live-provider gate.

## Anti-Cheating Audit

No email attach, provider role sync, or Stytch call from LMS. Review handlers, caches, persistence and audit logs for hard-coded success, test-only bypasses, raw-token persistence, swallowed provider errors, role/tenant derivation, or direct product-to-Stytch paths.

## Completion Gate

- [x] The scenario and its negative/cross-boundary cases pass at a public boundary.
- [x] Durable state, replay/retry behavior, and sanitized audit evidence are verified where applicable.
- [x] No Stytch material or provider authorization leaks to products.
- [x] `git diff --check`, documentation links/headings, and applicable build/test gates pass.
- [ ] The next dependency is not unblocked merely by a library-only or mocked proof.

## Implementation Notes (I13)

Implemented 2026-08-19.

### Architecture

```
Browser/SPA
  │
  ├─ Bearer <opaque token>    → legacy path  → parent_sessions DB lookup
  ├─ Bearer <Primer JWT>      → JWT path     → ES256 verify → identity_subject lookup
  │
  └─ local educator role check (parent | admin) ── source of truth
```

### Files Changed

| File | Purpose |
|------|--------|
| `server/internal/identityauth/verifier.go` | Standalone ES256 JWT verifier with JWKS fetch |
| `server/internal/identityauth/verifier_test.go` | 20+ unit tests: valid, expired, wrong-aud, Stytch, JWKS |
| `server/internal/db/migrations/00009_identity_subject.sql` | `identity_subject` column on educators |
| `server/internal/domain/domain.go` | `IdentitySubject` field on Educator struct |
| `server/internal/config/config.go` | `IDENTITY_ISSUER`, `IDENTITY_AUDIENCE`, `IDENTITY_JWKS_URL` |
| `server/internal/config/config_test.go` | Config tests for identity fields |
| `server/internal/repo/parent_auth.go` | `EducatorByIdentitySubject`, `LinkEducatorIdentity` |
| `server/internal/api/auth_parent.go` | Dual-path `ParentSessionGuard` (variadic verifier) |
| `server/internal/api/parent_learning.go` | `parentOpWith` helper, verifier threading |
| `server/internal/api/parent_course.go` | opts param added |
| `server/internal/api/agent_runtime.go` | `parentOpWith` usage |
| `server/internal/api/artifacts_import.go` | `parentOpWith` usage |
| `server/internal/api/api.go` | `IdentityVerifier` on Options |
| `server/internal/api/dual_auth_test.go` | Integration tests for dual-path guard |
| `server/internal/testutil/api.go` | `IdentityVerifier` option in test helper |
| `server/internal/testutil/factory/factory.go` | `ParentSession`, `EducatorOpts` helpers |
| `server/cmd/primer-server/main.go` | Wire identity verifier at startup |

### Test Command

```bash
go test ./internal/identityauth/ ./internal/api/ ./internal/config/ -count=1 -run "TestDualAuth|TestIdentity|TestVerify"
make test
IDENTITY_LIVE_STYTCH=1 IDENTITY_LIVE_STYTCH_ENV_FILE=$HOME/.config/primer/stytch-test.env make identity-live-stytch
```

### Key Decisions

- **No Stytch SDK in LMS**: the `identityauth` package verifies ES256 with stdlib crypto only.
- **Fail-closed**: nil verifier (empty config) rejects all JWTs; legacy path unaffected. The LMS instruction-log service boundary also rejects every request when `SERVICE_TOKEN` is empty; generic local/spec-only guards retain their existing inert behavior.
- **Local roles remain authoritative**: JWT subject maps to local educator; role check is always against `educators.role`.
- **No email merge**: identity subjects map 1:1; no automatic account linking by email.
