# Primer Identity Service — Phased Implementation Plan

**Outcome:** A separately deployable **Primer Identity** service with its own PostgreSQL database that authenticates humans (Google OIDC primary; password migration for existing LMS educators), issues short-lived single-audience JWTs + JWKS, runs as OIDC OP for product BFFs (authorization-code + PKCE), issues service client-credentials JWTs, and supports dual-run migration of LMS/TV away from localStorage bearers and static shared secrets — without owning Studio/LMS product authorization or TV/student device tokens.

**Status:** Plan only (no production implementation in this commit).
**Plan directory:** `agent_docs/plans/primer-identity-service/`
**Branch:** `planning/curriculum-studio-plan-identity`
**Base:** `66449725337165c2ef00f7c269633696313e0be2`
**Cite order:** product plan → foundation crosswalk → [identity design](../primer-identity-service-design.md) → LikeC4 auth-trust → Studio contracts/DB → live LMS/TV auth code

---

## Current-state summary

Backed by repository evidence at plan base:

| Area | State | Evidence |
| --- | --- | --- |
| Identity design (D1–D20, S0–S7) | Decision-complete | `agent_docs/plans/primer-identity-service-design.md` |
| Ownership freeze | Locked L3–L4 | `agent_docs/plans/curriculum-studio-foundation-crosswalk.md` |
| LikeC4 auth trust | Decided Identity edges | `architecture/curriculum-studio/views/auth-trust.c4` |
| Studio contracts auth | Bearer JWT end-state; `X-Service-Token` migration-only | `curriculum-studio/contracts/openapi/v1/curriculum-studio.yaml`, contracts README |
| Studio DB subjects | `identity:<uuid>` / `identity:svc:<id>`; no credentials | `curriculum-studio/db/SCHEMA.md`, `00001_identity_and_catalogs.sql` |
| LMS parent login | Email + bcrypt → opaque bearer in `parent_sessions` (12h) | `server/internal/api/auth_parent.go`, `server/internal/repo/parent_auth.go`, migration `00003_learning_activities.sql` |
| LMS SPA | Bearer in `localStorage` (`primer-parent-token`) | `web/src/api/auth.ts` |
| LMS service auth | Static `SERVICE_TOKEN`; **fail-open when empty** | `server/internal/api/secret.go` (`SharedSecretGuard`) |
| TV admin | Static `TV_ADMIN_API_KEY` as `X-Admin-Key`; SPA localStorage | `server/internal/tv/api/auth.go`, `tv-web/src/api/auth.ts` |
| TV/student devices | Product-local opaque tokens | `server/internal/tv/auth`, `server/internal/api/student_api.go` |
| Identity service code | **Missing** | no `primer-identity/` module |
| Stack patterns to mirror | Huma v2 + chi + pgx + goose + testcontainers + Vite SPA | `server/go.mod`, `server/internal/testutil`, `Makefile` |

**Existing:** designs, contracts, LMS/TV legacy auth, Studio subject conventions.
**Partial:** none of the Identity runtime.
**Missing:** entire Identity service + product migration track covered by this plan.

---

## Scope boundaries

### In scope

1. New separately deployable **Primer Identity** Go service + **own completely separate PostgreSQL**
2. Accounts, external identities (`provider`+`provider_subject`), password credentials (migration/break-glass)
3. OAuth clients + redirect URI allowlists; Identity as **OIDC OP** to product BFFs
4. Google OIDC federation (Identity as RP); loopback IdP with **real crypto** in tests
5. Authorization-code + PKCE; browser-bound state/nonce/correlation/replay protection
6. Host-only Identity and product BFF sessions; no `Domain=` parent cookie; no browser localStorage tokens
7. Access JWT (ES256 preferred) short TTL, **single audience**, JWKS + key lifecycle/rotation
8. Service principals + client_credentials; secret hashing/rotation
9. Session/refresh rotation, RFC 7009 revoke, logout fail-closed, jti/sid denylist on security events
10. Audit, privacy minimization, admin, recovery/break-glass
11. Studio integration surfaces (JWKS consumption contract; `subject_ref` mapping) — not Studio schema authorship
12. LMS dual-login + `educators.identity_subject` migration; password lockout prevention
13. Service-secret dual-accept then cutover; production fail-closed empty secret
14. Human TV-admin SSO migration; retire browser-held admin key
15. Migration stages **S0–S7** with rollback; credential-free vs live Google separation

### Out of scope (drop table)

| Dropped / not owned here | Why | Where it lives |
| --- | --- | --- |
| Studio workspace roles / tenants | Product authz | Studio platform plan + `workspace_memberships` |
| LMS educator/student product authz, mastery | Product | `server/` LMS |
| TV device tokens / pairing | Different threat model | TV service |
| Student Identity users (v1) | Explicit deferral | Future COPPA/parental design |
| Studio OpenAPI/proto authorship | Contracts track | `curriculum-studio/contracts/` |
| Studio SQL schema design | DB track | `curriculum-studio/db/` |
| Cross-DB FKs/FDW | Forbidden | — |
| Email auto-link / silent merge | Security decision D8/D9 | Never |
| Broad `Domain=.example.com` session cookie | Security D15 | Never |
| Production use of test/loopback IdP | Fail-closed | Test/dev only |
| Tokens in browser localStorage/sessionStorage | XSS | Never in production SPA |
| Live Google credentials in CI/git | External | Secret manager; Phase 14 BLOCKED |
| Multi-tenant LMS org model, passkeys, DPoP, mTLS, SCIM | Deferred | Identity design §26 |
| Studio→LMS import push | Crosswalk `#uncertainty` | Deferred adapter |

---

## Global constraints (iron rules)

1. **Separate deployable + separate DB.** Module path `github.com/aleksclark/primer/identity` under planned `primer-identity/`. No shared goose table with LMS/TV/Studio. No cross-DB FKs/views/FDW.
2. **Identity authenticates; products authorize.** No Studio/LMS roles in JWT. `sub` = account UUID; products map `identity:<uuid>` / `identity:svc:<id>`.
3. **Stable external key = `(provider, provider_subject)` only.** Never auto-link by email. Same email + different `sub` = two accounts until explicit authenticated link.
4. **Host-only cookies.** `Secure; HttpOnly; SameSite=Lax|Strict`; `__Host-` where path allows. **No** `Domain=` parent-domain session cookie.
5. **Short-lived single-audience JWTs.** Human ≤15m; service ≤10m. Validators require exact `aud`. No multi-aud access tokens in v1. Prefer ES256; reject `alg=none`; no HS256 cross-service access tokens.
6. **BFF pattern.** Product BFFs are confidential OIDC clients of Identity. SPA never holds access/refresh tokens. No production tokens in localStorage.
7. **OAuth 2.1 protocol.** Auth code + PKCE S256 mandatory; state + nonce; one-time codes; exact redirect_uri allowlist; browser-bound correlation secret (login CSRF).
8. **Bounded OIDC HTTP.** Dedicated client Timeout; dial/TLS/header bounds; body caps; **CheckRedirect deny** (301–308 including 307 secret re-POST); Google endpoint pins when `provider=google`.
9. **Fail-closed production.** Reject test/loopback IdP; missing/placeholder secrets; non-HTTPS redirect in prod; empty legacy service secret must not open routes; logout/revoke store errors → 5xx not false success.
10. **Credential-free vs live.** Loopback IdP with real RSA/JWKS/PKCE proves protocol. Live Google remains **BLOCKED** until approved credentials phase. Never mark live complete on loopback evidence.
11. **No token/secret logs.** No code/state/tokens/PII in redirects, OTel attributes, or audit raw fields (hash/jti only).
12. **Students/devices out.** TV device + student device tokens stay product-local; never accepted as human admin credentials.
13. **Migration lockout prevention.** Never disable password for an educator without a second usable factor (Google link or recovery).
14. **Real boundaries in E2E.** Postgres testcontainers, real HTTP, cookie jars, concurrent callback races. No mocked “ValidateIDToken returns ok” as sole proof.
15. **Docs-only discipline in this commit.** Plan files only; no production code.

---

## Decision record

| ID | Decision | Rationale |
| --- | --- | --- |
| D1 | New tree `primer-identity/` Go module `github.com/aleksclark/primer/identity` with `cmd/identity-server`, `cmd/identity-migrate`, `cmd/openapi-gen`, `internal/{config,db,api,repo,domain,oauth,oidc,keys,session,clients,serviceauth,admin,audit,bffref,testutil,oauthtest}` | Parallel to LMS `server/` and planned Studio module; independent deployable |
| D2 | Env prefix `IDENTITY_`; DB name `primer_identity`; goose table `identity_goose_db_version` | Isolation from LMS/TV/Studio goose |
| D3 | Stack: Go (align `server/go.mod` 1.25.x), Huma v2 + chi + pgx/v5 + goose/v3 + envconfig + testcontainers; OAuth libs `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3`, `golang.org/x/crypto` | Repo-proven + mathscan OIDC boundary patterns |
| D4 | Issuer URL config `IDENTITY_ISSUER` (stable, trailing-slash normalized once); audiences enum: `curriculum-studio`, `primer-lms`, `primer-tv-admin` | Design §8; single-aud enforcement |
| D5 | JWT alg ES256 (P-256) default; RS256 optional via config for ops tooling; pin allowlist on validators | Design D4/D18 |
| D6 | Account `sub` = UUID; external unique `(provider, provider_subject)`; email display/recovery only | Design D8 |
| D7 | Explicit link only while authenticated + step-up; no email merge job | Design D9 |
| D8 | Identity host cookies: `__Host-id_session` (+ login correlation cookie Path-scoped to authorize/callback). Product BFF cookies owned by product hosts — this plan specifies **contract** and ships **reference BFF helpers** under `internal/bffref` + LMS/Studio wiring phases | Design §7 |
| D9 | Confidential BFF clients: `studio-bff`, `lms-bff`, `tv-admin-bff` (ids configurable); public clients forbidden for production browser code exchange | Design §6.3 |
| D10 | Service principals: `client_credentials` → JWT; secrets argon2id/bcrypt hashed; shown once; dual-valid rotation grace ≤7d | Design D6/§10 |
| D11 | LMS additive `educators.identity_subject TEXT UNIQUE`; ParentSessionGuard gains JWT path; password path remains until S7 per account | Design §11.3 |
| D12 | `SharedSecretGuard` evolution: production refuse empty secret; dual-accept JWT \| legacy static with metric; remove static at S7 | Closes fail-open in `secret.go` |
| D13 | TV human admin migrates to Identity SSO; device tokens unchanged; distinct guards remain | Design D13/D14 |
| D14 | Students not Identity users in v1 | Design D12 |
| D15 | Loopback IdP package `internal/oauthtest` build-tagged / test-only; production binary must not import it | Packaging isolation |
| D16 | Live Google credentials explicitly **BLOCKED** until Phase 14 gate + human approval; no secrets in repo | Credential-free completion rule |
| D17 | Coverage gate Identity internal packages **80%** initially (raise later); adversarial OAuth suite mandatory regardless of % | Greenfield realism |
| D18 | Root Makefile targets: `identity-build`, `identity-test`, `identity-cover`, `identity-openapi`, `dev-db-identity`, `migrate-identity`, `identity-e2e` | Explicit commands |
| D19 | Implementation waves = small reviewed PR sets (see orchestrator map); one phase coherence per wave preferred | Reviewability |
| D20 | Design D1–D20 remain authoritative; this plan does not reopen them | Crosswalk + design freeze |

---

## Migration stages S0–S7 (owned across phases)

| Stage | Meaning | Primary phases | Rollback |
| --- | --- | --- | --- |
| **S0** | Design freeze (done) + this plan | index | N/A |
| **S1** | Identity deploy dark — discovery/JWKS live; no product traffic | 1–5 | Stop service |
| **S2** | Studio Google login via BFF | 6–7, 11 | Feature flag off Studio auth |
| **S3** | LMS dual login (password + Identity); populate `identity_subject` | 12 | Disable Identity path |
| **S4** | Service JWT dual-accept at LMS/Studio | 13 | Static-only flag |
| **S5** | SPA cookie cutover (remove localStorage bearer/admin key) | 13 | Dev-only token paste |
| **S6** | TV admin human SSO | 14 | Restore `X-Admin-Key` flag |
| **S7** | Disable legacy secrets & optional passwords | 14 | Incident-only re-enable with expiry |

**Lockout rule:** password remains enabled per educator until Google-linked or recovery enrolled.

---

## Phase overview

| Phase | Goal | Depends on |
| --- | --- | --- |
| [Phase 1: Service shell and Identity DB](./phase-01-service-shell-and-db.md) | Runnable identity-server, config, migrate, health/ready, isolated Postgres | None |
| [Phase 2: Accounts and external identities](./phase-02-accounts-and-external-identities.md) | Account model, provider+sub uniqueness, password credential store, no email merge | Phase 1 |
| [Phase 3: Keys, JWKS, and access tokens](./phase-03-keys-jwks-access-tokens.md) | ES256 keys, JWKS publish, mint/verify single-aud JWT claims | Phase 2 |
| [Phase 4: OAuth clients and OP code+PKCE](./phase-04-oauth-clients-and-op.md) | Client registry, discovery, authorize/token auth-code+PKCE (loopback) | Phase 3 |
| [Phase 5: Sessions, cookies, and login CSRF binding](./phase-05-sessions-cookies-login-csrf.md) | Host-only sessions, correlation cookie, atomic consume, replay/concurrency | Phase 4 |
| [Phase 6: Google RP federation (loopback crypto)](./phase-06-google-rp-loopback.md) | Identity as Google RP via loopback IdP; upsert; claim bounds; HTTP deny-redirect | Phase 5 |
| [Phase 7: Product BFF confidential client contract](./phase-07-product-bff-contract.md) | BFF login/callback/token/refresh contract + reference helpers; return_to allowlist | Phases 5–6 |
| [Phase 8: Service principals and client_credentials](./phase-08-service-principals.md) | Service clients, hashed secrets, scoped JWTs, rotation grace | Phase 3, 4 |
| [Phase 9: Refresh, revoke, logout](./phase-09-refresh-revoke-logout.md) | Refresh rotation, RFC7009 revoke, logout fail-closed, denylist, unrelated sessions | Phases 5, 7 |
| [Phase 10: Key rotation and hardened token path](./phase-10-key-rotation-and-hardening.md) | Dual-key JWKS window, unknown-kid refetch contract, signer health | Phases 3, 8–9 |
| [Phase 11: Admin, audit, recovery, privacy](./phase-11-admin-audit-recovery.md) | Admin API, audit_events, recovery codes, break-glass, privacy bounds | Phases 2, 9 |
| [Phase 12: Studio integration](./phase-12-studio-integration.md) | Shared validator notes + Studio JWKS consumer wiring contract; subject_ref | Phases 3, 7, 8 |
| [Phase 13: LMS dual-login and service dual-accept](./phase-13-lms-dual-login-and-service-cutover.md) | identity_subject, dual human login, SPA cookie path, SharedSecretGuard dual-accept (S3–S5) | Phases 7–9, 11 |
| [Phase 14: TV admin SSO, S7, ops, live Google](./phase-14-tv-admin-s7-ops-live.md) | TV human SSO (S6), legacy disable (S7), backup/restore, deploy; **live Google BLOCKED** | Phase 13 |

**Parallelism:** Phase 8 may proceed in parallel with Phases 5–7 after Phase 4. Phase 11 admin surfaces may start after Phase 2 schema exists but complete only after Phase 9 session revoke primitives. Phase 12 Studio wiring tracks Studio platform authz phase.

---

## Testing tiers

| Tier | Command (planned) | Requires |
| --- | --- | --- |
| T0 Unit | `cd primer-identity && go test ./internal/... -short` | none |
| T1 Module integration | `make identity-test` (Postgres testcontainer) | Docker |
| T2 OAuth protocol adversarial | `make identity-test-oauth` (`go test ./internal/api ./internal/oauth ./internal/oidc -race -count=1`) | Docker + loopback IdP |
| T3 Process E2E | `make identity-e2e` real process + DB + cookie jars | Docker |
| T4 Product consumer | `make test` / `make studio-test` slices that validate JWTs | Identity test JWKS or dual harness |
| T5 Browser E2E | Playwright against LMS/Studio/TV admin BFF stacks (phases 13–14) | Node + Docker |
| T6 Architecture | `npx --yes likec4@1.46.0 validate architecture/curriculum-studio` | network first pin |
| T7 Live Google | Explicit suite only Phase 14 | **BLOCKED** credentials |
| T8 Cover gate | `make identity-cover` | same as T1 |

**Permitted fakes:** loopback OIDC IdP with real crypto; in-process confidential BFF test double; testcontainers Postgres; clock skew injectors.
**Not permitted as substitutes:** mock ValidateIDToken-only success; fake Postgres for durability claims; SPA-only authz; hard-coded handler 200 without domain path; production config with `provider=test`.

---

## Contract surfaces (implementer-facing; not Studio schema)

Identity public HTTP (normative outline — detailed fields in phases):

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/.well-known/openid-configuration` | Discovery |
| GET | `/oauth/jwks` | Public JWKS |
| GET | `/oauth/authorize` | Auth code + PKCE; may redirect to Google |
| POST | `/oauth/token` | `authorization_code`, `refresh_token`, `client_credentials` |
| POST | `/oauth/revoke` | RFC 7009 |
| GET | `/v1/userinfo` | Bearer; profile claims; minimize PII |
| POST | `/v1/logout` | RP logout / session end |
| GET/POST | `/v1/account/links` | Explicit external link |
| POST | `/v1/account/recovery/verify` | Recovery codes |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | DB + active signer in JWKS |

Admin (mTLS or break-glass only; not product user JWT):

| Area | Operations |
| --- | --- |
| Clients | register/update redirect URIs, rotate client secret |
| Service principals | mint/rotate/revoke credentials, grants (aud+scopes) |
| Accounts | lock, unlock, revoke-all sessions |
| Keys | initiate rotation, retire kid after window |
| Audit | query auth events (hashed IP/UA) |

Events (optional consumers; never sole authz):
`identity.account.created`, `identity.account.locked`, `identity.session.revoked`, `identity.external_identity.linked`, `identity.client.secret_rotated`.

JWT claim profile: see design Appendix A. Required: `iss,sub,aud,exp,iat,nbf,jti,client_id|azp,scope`; human adds `auth_time,amr,sid`. **Omit** roles, workspace_ids, email from access tokens by default.

---

## Path decisions (new)

| Path | Role |
| --- | --- |
| `primer-identity/go.mod` | Identity Go module |
| `primer-identity/cmd/identity-server` | Main process |
| `primer-identity/cmd/identity-migrate` | Migrate-only entry |
| `primer-identity/cmd/openapi-gen` | Huma → OpenAPI |
| `primer-identity/internal/*` | Packages in D1 |
| `primer-identity/internal/db/migrations/` | Identity goose SQL |
| `primer-identity/internal/oauthtest/` | Loopback IdP (**tests only**) |
| `primer-identity/web/` (optional later) | Minimal Identity login UI if not server-rendered |
| `deploy/primer-identity.nomad.hcl.tmpl` | Nomad job (Phase 14) |
| Root Makefile `identity-*` targets | Build/test entrypoints |
| LMS additive migration under `server/internal/db/migrations/` | `identity_subject` (Phase 13) |
| `server/internal/api/secret.go`, `auth_parent.go` | Dual-accept / JWT guard (Phase 13) |
| `web/src/api/auth.ts`, `tv-web/src/api/auth.ts` | Cookie cutover (Phases 13–14) |

---

## Cross-plan dependencies

| Dependency | Sibling track | This plan needs / provides |
| --- | --- | --- |
| Studio platform authz + BFF | **curriculum-studio-plan-platform** | Consumes Identity JWKS + clients; this plan provides OP + S2 |
| Studio contracts auth metadata | **curriculum-studio-plan-contracts** | bearerAuth/serviceCredential already baseline; keep parity |
| Studio DB subject_ref | **curriculum-studio-plan-db** / frozen SCHEMA | `identity:` prefix already documented |
| Foundation freeze | crosswalk (done) | L3–L4 |
| LMS/TV runtime | this repo `server/`, `web/`, `tv-web/` | Migration phases 13–14 edit these |

Open blockers: production Google client id/secret; production issuer hostnames; KMS/HSM key custody; whether TV admin shares LMS host or separate host (both fit BFF — pick at deploy).

---

## Implementation-orchestrator handoff map

| Wave | Phases | Review focus |
| --- | --- | --- |
| W1 | 1–2 | Process shell + account model |
| W2 | 3–4 | Keys/JWKS + OP code+PKCE |
| W3 | 5–6 | Sessions/CSRF + Google RP loopback adversarial |
| W4 | 7–8 | BFF contract + service principals |
| W5 | 9–10 | Revoke/logout + key rotation hardening |
| W6 | 11–12 | Admin/recovery + Studio integration |
| W7 | 13 | LMS dual-login + service dual-accept + SPA cookies |
| W8 | 14 | TV admin SSO + S7 + ops; live Google **BLOCKED** |

Each wave = one or more reviewed PRs; merge only with that wave's gates green. Prefer `impl/identity-wN-*` worktrees from architecture tip.

---

## Requirement → phase / scenario / E2E traceability matrix

| Req ID | Requirement | Phase | BDD scenarios | E2E tests |
| --- | --- | --- | --- | --- |
| ID-1 | Independently deployable Identity process | 1 | P1-S1, P1-S2 | P1-E1, P1-E2 |
| ID-2 | Separate Postgres + goose isolation | 1 | P1-S3, P1-S4, P1-S5 | P1-E1, P1-E3 |
| ID-3 | Health/ready + fail-fast config | 1 | P1-S2, P1-S6 | P1-E2, P1-E4 |
| ID-4 | Accounts with UUID sub | 2 | P2-S1, P2-S2 | P2-E1 |
| ID-5 | External identity unique (provider, sub) | 2 | P2-S3, P2-S4 | P2-E2 |
| ID-6 | No email auto-link / no silent merge | 2 | P2-S5, P2-S6 | P2-E3 |
| ID-7 | Password credential store (argon2id/bcrypt) | 2 | P2-S7, P2-S8 | P2-E4 |
| ID-8 | Students not Identity users v1 | 2 | P2-S9 | P2-E5 |
| ID-9 | ES256 signing keys + JWKS public only | 3 | P3-S1, P3-S2 | P3-E1 |
| ID-10 | Access JWT mint short TTL single aud | 3 | P3-S3, P3-S4, P3-S5 | P3-E2 |
| ID-11 | iss/aud/sig/exp/nbf/jti claim enforcement | 3 | P3-S6, P3-S7 | P3-E3 |
| ID-12 | No roles/email in default access token | 3 | P3-S8 | P3-E4 |
| ID-13 | OAuth client registry + exact redirect allowlist | 4 | P4-S1, P4-S2 | P4-E1 |
| ID-14 | OIDC discovery document | 4 | P4-S3 | P4-E2 |
| ID-15 | Authorization code + PKCE S256 | 4 | P4-S4, P4-S5 | P4-E3 |
| ID-16 | Auth code single-use short TTL | 4 | P4-S6, P4-S7 | P4-E4 |
| ID-17 | Host-only Identity session cookies | 5 | P5-S1, P5-S2 | P5-E1 |
| ID-18 | Browser correlation binding / login CSRF | 5 | P5-S3, P5-S4 | P5-E2 |
| ID-19 | State/nonce binding + replay reject | 5 | P5-S5, P5-S6 | P5-E3 |
| ID-20 | Concurrent callback exactly one success | 5 | P5-S7 | P5-E4 |
| ID-21 | Google RP via loopback real crypto | 6 | P6-S1, P6-S2 | P6-E1 |
| ID-22 | Upsert stable provider+sub only | 6 | P6-S3, P6-S4 | P6-E2 |
| ID-23 | Claim bounds; verified email required at RP boundary; no sub truncate | 6 | P6-S5, P6-S6 | P6-E3 |
| ID-24 | Bounded HTTP + CheckRedirect deny 301–308 | 6 | P6-S7, P6-S8 | P6-E4 |
| ID-25 | Google endpoint pins when provider=google | 6 | P6-S9 | P6-E5 |
| ID-26 | Prod rejects test IdP | 6 | P6-S10 | P6-E6 |
| ID-27 | Product BFF authorize/callback/token contract | 7 | P7-S1, P7-S2, P7-S3 | P7-E1 |
| ID-28 | return_to allowlist open-redirect deny | 7 | P7-S4 | P7-E2 |
| ID-29 | BFF holds tokens server-side; SPA cookie only | 7 | P7-S5, P7-S6 | P7-E3 |
| ID-30 | CSRF on BFF cookie mutations | 7 | P7-S7 | P7-E4 |
| ID-31 | Service principals client_credentials JWT | 8 | P8-S1, P8-S2 | P8-E1 |
| ID-32 | Service secret hashing + rotation grace | 8 | P8-S3, P8-S4 | P8-E2 |
| ID-33 | Service audience/scope enforcement | 8 | P8-S5 | P8-E3 |
| ID-34 | Refresh rotation + theft detection | 9 | P9-S1, P9-S2 | P9-E1 |
| ID-35 | RFC7009 revoke fail-closed | 9 | P9-S3, P9-S4 | P9-E2 |
| ID-36 | Logout fail-closed; unrelated sessions survive login | 9 | P9-S5, P9-S6, P9-S7 | P9-E3 |
| ID-37 | jti/sid denylist on lock/revoke-all | 9 | P9-S8 | P9-E4 |
| ID-38 | Key overlap rotation + unknown kid | 10 | P10-S1, P10-S2, P10-S3 | P10-E1 |
| ID-39 | Signer missing from JWKS fails ready | 10 | P10-S4 | P10-E2 |
| ID-40 | Admin client/principal/account ops restricted | 11 | P11-S1, P11-S2 | P11-E1 |
| ID-41 | Audit events without raw secrets | 11 | P11-S3, P11-S4 | P11-E2 |
| ID-42 | Recovery codes + break-glass audited | 11 | P11-S5, P11-S6 | P11-E3 |
| ID-43 | Privacy: minimize PII JWT/logs; export/delete hook | 11 | P11-S7 | P11-E4 |
| ID-44 | Studio JWKS validate aud=curriculum-studio | 12 | P12-S1, P12-S2 | P12-E1 |
| ID-45 | subject_ref identity: uuid / svc mapping | 12 | P12-S3 | P12-E2 |
| ID-46 | Cross-tenant authz absence in Identity token | 12 | P12-S4 | P12-E3 |
| ID-47 | LMS dual login password + Identity (S3) | 13 | P13-S1, P13-S2 | P13-E1 |
| ID-48 | identity_subject link; lockout prevention | 13 | P13-S3, P13-S4 | P13-E2 |
| ID-49 | Explicit account link step-up | 13 | P13-S5 | P13-E3 |
| ID-50 | Service dual-accept JWT + legacy (S4) | 13 | P13-S6, P13-S7 | P13-E4 |
| ID-51 | Prod empty service secret fail-closed | 13 | P13-S8 | P13-E5 |
| ID-52 | LMS SPA host-only cookie; no localStorage token (S5) | 13 | P13-S9, P13-S10 | P13-E6 |
| ID-53 | TV admin human SSO (S6) | 14 | P14-S1, P14-S2 | P14-E1 |
| ID-54 | Device tokens still product-local | 14 | P14-S3 | P14-E2 |
| ID-55 | S7 disable legacy secrets/passwords safely | 14 | P14-S4, P14-S5 | P14-E3 |
| ID-56 | Backup/restore accounts+keys; sessions loseable | 14 | P14-S6 | P14-E4 |
| ID-57 | Live Google validation BLOCKED until approved | 14 | P14-S7 | P14-E5 |
| ID-58 | No token/secret in logs across surfaces | 1–14 | P1-S7, P5-S8, P6-S11, P9-S9 | P5-E5, P6-E7 |
| ID-59 | Migration stages S0–S7 documented + gated | index, 13–14 | P13-S11, P14-S8 | P14-E6 |
| ID-60 | Rollout rollback per stage | 13–14 | P13-S12, P14-S9 | P14-E7 |

---

## Completion rule

The plan is finished only when:

1. Every phase Completion Gate is green or explicitly **BLOCKED** with named external dependency (live Google, prod KMS, deploy auth).
2. Credential-free adversarial matrix (login CSRF victim jar, replay/concurrency, PKCE/state/nonce, redirect 301–308 deny, claim bounds, single-aud, logout/revoke fail-closed, dual-accept lockout prevention, separate DB, no secret logs) passes on loopback.
3. S3–S7 product migrations have rollback proofs and lockout-prevention evidence.
4. Traceability matrix rows all resolve; no orphan scenarios/tests.
5. No production claim rests solely on loopback IdP or test auth evidence.
6. Students remain non-users; device tokens remain product-local.

---

## Adversarial proof index (must appear in phase E2E)

| Proof theme | Phase anchors |
| --- | --- |
| Login CSRF / session swap fresh victim jar | P5-S3, P5-E2, P6-E1 |
| Callback replay + concurrency one-success | P5-S6, P5-S7, P5-E3, P5-E4 |
| state/nonce/PKCE | P4-S4–S7, P5-S5 |
| stable provider+sub / no email merge | P2-S5, P6-S3, P6-S4 |
| issuer/audience/signature/expiry/claim bounds | P3-S6, P6-S5, P6-S6 |
| redirect deny discovery/token/JWKS 301–308 incl 307 | P6-S7, P6-S8, P6-E4 |
| bounded HTTP/timeouts/body caps | P6-S7, P3 mint path |
| key overlap/rotation/unknown-kid | P10-S1–S3 |
| single-audience enforcement | P3-S4, P12-S2 |
| logout/revoke fail-closed | P9-S3–S5 |
| unrelated sessions survive login | P9-S6 |
| refresh/session theft + rotation | P9-S1, P9-S2 |
| CSRF/origin on BFF mutations | P7-S7 |
| cross-tenant authorization absence | P12-S4 |
| service client secret hashing/rotation | P8-S3, P8-S4 |
| dual-accept migration lockout prevention | P13-S4, P13-S7 |
| fail-closed production config | P6-S10, P13-S8 |
| Google endpoint pins | P6-S9 |
| loopback IdP real crypto | P6-S1 |
| no token/secret logs | P5-S8, P6-S11, P9-S9 |
| separate DB | P1-S3–S5 |
| backup/restore | P14-S6 |
| rollout rollback | P13-S12, P14-S9 |

---

## Open blockers

| Blocker | Impact | Resolution owner |
| --- | --- | --- |
| Production Google OAuth client + verified redirect URIs | Live Phase 14 | Ops + human approval |
| Production `IDENTITY_ISSUER` hostname / TLS | Deploy | Ops |
| KMS/HSM for signing keys in prod | Key custody | Ops |
| TV admin host choice (shared LMS vs dedicated) | S6 cookie jar | Deploy decision |
| Studio platform BFF ready for S2 | S2 traffic | platform plan track |
| Approval to disable legacy static secrets (S7) | Final cutover | Security + support sign-off |
