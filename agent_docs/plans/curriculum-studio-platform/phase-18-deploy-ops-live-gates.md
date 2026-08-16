# Phase 18: Deploy ops live gates

**File:** `phase-18-deploy-ops-live-gates.md`
**Depends on:** Phases 15–17 (packaging may start earlier but live gates last)
**Duration guess:** 5–8 days + external waits
**Handoff wave:** see index orchestrator map

## Goal

Package Curriculum Studio for deployment (Docker, Nomad template, migrate job, runbooks) and define live proof gates for the approved **Stytch B2B → Identity-hosted broker → Studio BFF** path, real model providers, and production smoke. Credential-free platform remains complete without these; live items stay BLOCKED until the named Identity-owned credentials, configuration, and environments exist.

## Scope

### In scope

- Dockerfile for studio-server (+ web embed)
- `deploy/curriculum-studio.nomad.hcl.tmpl` parallel to primer/tv
- env documentation `STUDIO_*`
- backup/restore runbook for Postgres + object store
- health checks in Nomad
- Live Stytch B2B → Identity-hosted broker → Studio BFF E2E checklist against the Identity deploy
- Live model provider soak test harness
- Production smoke script
- Security: secrets from Nomad vars not images

### Out of scope / YAGNI

- Claiming live complete on loopback evidence
- TV/LMS deploy changes except documented integration client elsewhere

## BDD Success Criteria

#### Scenario: P18-S1 — Container boots

- **Given** image built locally
- **When** run with Postgres+fs store
- **Then** health/ready 200
- **And** migrate applies
- **And** version label present

#### Scenario: P18-S2 — Runbooks exist

- **Given** ops docs committed
- **When** operator follows backup/restore dry-run on throwaway DB
- **Then** restore yields readable workspaces
- **And** artifact objects restored or re-linked procedure documented

#### Scenario: P18-S3 — Approved live Stytch B2B → Identity broker → Studio BFF

- **Given** an approved live Stytch B2B project and its credentials are configured **only in Primer Identity**, with exact Identity-hosted callback redirect URI, Studio BFF origin/redirect URI, and Origin allowlist registered for the target environment
- **When** a human starts login from the Studio BFF and completes the Stytch B2B flow through the Identity-hosted broker
- **Then** the item remains BLOCKED until the Identity-owned live project, credentials, redirects/origins, deploy access, and approval are present
- **And** when unblocked, Studio/browser code has no Stytch credential or raw Stytch token; it receives only Primer BFF/session/JWT material
- **And** Studio accepts only a Primer ES256 JWT for `aud=curriculum-studio`, rejects a raw Stytch bearer/SessionJWT, and independently authorizes the resolved subject through local workspace membership
- **And** signed Stytch webhook/revocation evidence proves cache-and-grant revocation and an access-JWT revocation bound of **≤15 minutes**

#### Scenario: P18-S4 — Live model provider

- **Given** Bedrock/OpenRouter credentials
- **When** materialize with live provider flag
- **Then** BLOCKED until credentials
- **And** when unblocked: run ready with provider provenance not scripted
- **And** no quality claim beyond smoke

#### Scenario: P18-S5 — Production deploy smoke

- **Given** Nomad job registered in target env
- **When** smoke script hits health + authenticated probe
- **Then** BLOCKED without deploy access
- **And** when unblocked: healthy allocation + migrate

## Implementation Instructions

1. Multi-stage Docker build: build web, build go, distroless/alpine runtime.
2. Nomad service + migrate batch pattern from `deploy/deploy.sh` conventions.
3. Separate live test build tag `live` or env `STUDIO_LIVE_E2E=1`; this suite may run only against the approved Identity-hosted broker path, never a direct Studio-to-Stytch or browser-to-Stytch path.
4. Configure the live Stytch B2B project ID, secret, and webhook verification material only in Identity’s secret store. Register the exact Identity callback redirect URI plus the Studio BFF origin/redirect URI and Origin allowlist for the selected environment. Studio deploy variables contain Primer issuer/JWKS/BFF configuration only—never a Stytch client credential, project secret, or raw Stytch token.
5. Completion gate splits: **credential-free packaging complete** versus **approved live Stytch broker proof BLOCKED**; no loopback/fixture proof may be promoted to live.
6. Never store live Stytch or BFF secrets in repo, image layers, browser configuration, or logs.

## End-to-End Test Plan

#### P18-E1 — Docker compose smoke local

- **Setup:** compose Postgres+MinIO+studio
- **Action:** up + health
- **Assert:**
  - ready
- **Command:** `make studio-docker-smoke`

#### P18-E2 — Runbook dry-run

- **Setup:** throwaway
- **Action:** backup/restore script
- **Assert:**
  - data roundtrip
- **Command:** `manual/scripted ops test`

#### P18-E3 — Approved live Stytch broker and Studio BFF

- **Setup:** approved live Stytch B2B project/credentials configured in Identity only; deployed Identity broker; exact registered Identity callback + Studio BFF redirect/origin/Origin allowlist; seeded local Studio workspace membership; signed webhook receiver and audit access
- **Action:** complete a browser login from Studio BFF through the Identity-hosted broker, call `/auth/me` and an authorized Studio route, then revoke/end the upstream session and deliver/replay the signed Stytch webhook
- **Assert:**
  - BLOCKED with the missing named approval/configuration, or pass with an evidence bundle
  - browser/Studio use Primer-only BFF/JWT material; no direct Stytch credential/token is exposed
  - raw Stytch bearer/SessionJWT is rejected; valid Primer JWT is accepted only with local workspace membership
  - forged/replayed/out-of-order webhook is rejected/idempotent as applicable; valid revocation invalidates cache/grant and prevents fresh BFF use within **≤15 minutes**
- **Command:** `make studio-e2e-live-stytch-broker`

#### P18-E4 — Live model

- **Setup:** external
- **Action:** materialize live
- **Assert:**
  - BLOCKED or pass
- **Command:** `make studio-e2e-live-model`

#### P18-E5 — Deploy smoke

- **Setup:** Nomad
- **Action:** deploy.sh SERVICE=studio
- **Assert:**
  - BLOCKED or pass
- **Command:** `deploy smoke`

## Anti-Cheating Audit

- Do not mark S3–S5 complete on test auth/scripted model or a direct Studio/browser Stytch flow
- Verify all Stytch live credentials and webhook secrets are Identity-only; inspect Studio image/env/browser assets/logs for their absence
- Verify accepted Studio requests use Primer JWT/JWKS plus local membership, and raw Stytch tokens fail before authorization
- Verify the live evidence includes signed-webhook signature/timestamp/replay/idempotency results and the ≤15-minute revocation bound
- Image must not embed .env secrets
- Healthcheck must hit ready not only process exists
- Runbooks not empty stubs

## Completion Gate

- [ ] Dockerfile + Nomad template merged
- [ ] Local docker smoke green (credential-free)
- [ ] Live Stytch B2B → Identity broker → Studio BFF/model/deploy items listed BLOCKED with named Identity-owned credentials/configuration/approval or green with evidence bundles
- [ ] P18-E3 proves exact redirects/origins, Identity-only Stytch credentials, Primer-only Studio BFF/JWT, local membership authorization, raw-token rejection, signed revocation evidence, and ≤15-minute JWT revocation bound
- [ ] Platform plan completion rule satisfiable for credential-free scope


## Dependencies

- Upstream: Phases 15–17 (packaging may start earlier but live gates last)
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
