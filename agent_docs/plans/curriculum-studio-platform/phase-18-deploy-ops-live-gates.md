# Phase 18: Deploy ops live gates

**File:** `phase-18-deploy-ops-live-gates.md`
**Depends on:** Phases 15–17 (packaging may start earlier but live gates last)
**Duration guess:** 5–8 days + external waits
**Handoff wave:** see index orchestrator map

## Goal

Package Curriculum Studio for deployment (Docker, Nomad template, migrate job, runbooks) and define live proof gates for Google OIDC, real model providers, and production smoke. Credential-free platform remains complete without these; live items stay BLOCKED until credentials and environments exist.

## Scope

### In scope

- Dockerfile for studio-server (+ web embed)
- `deploy/curriculum-studio.nomad.hcl.tmpl` parallel to primer/tv
- env documentation `STUDIO_*`
- backup/restore runbook for Postgres + object store
- health checks in Nomad
- Live OIDC E2E checklist against Identity deploy
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

#### Scenario: P18-S3 — Live Google OIDC

- **Given** Identity deployed + Google client + Studio client registration
- **When** human login via real Google
- **Then** BLOCKED until credentials
- **And** when unblocked: host-only cookies, me endpoint real sub

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
3. Separate live test build tag `live` or env `STUDIO_LIVE_E2E=1`.
4. Completion gate splits: **credential-free packaging complete** vs **live BLOCKED**.
5. Never store Google client secret in repo.

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

#### P18-E3 — Live OIDC

- **Setup:** external
- **Action:** browser login Google
- **Assert:**
  - BLOCKED or pass
- **Command:** `make studio-e2e-live-oidc`

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

- Do not mark S3–S5 complete on test auth/scripted model
- Image must not embed .env secrets
- Healthcheck must hit ready not only process exists
- Runbooks not empty stubs

## Completion Gate

- [ ] Dockerfile + Nomad template merged
- [ ] Local docker smoke green (credential-free)
- [ ] Live OIDC/model/deploy items listed BLOCKED with owners or green with evidence bundles
- [ ] Platform plan completion rule satisfiable for credential-free scope


## Dependencies

- Upstream: Phases 15–17 (packaging may start earlier but live gates last)
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
