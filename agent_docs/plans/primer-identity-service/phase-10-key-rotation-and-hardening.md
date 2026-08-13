# Phase 10: Key rotation and hardened token path

**File:** `phase-10-key-rotation-and-hardening.md`
**Depends on:** Phases 3, 8–9
**Duration guess:** 3–5 days
**Migration stage:** S1+
**Handoff wave:** W5

## Goal

Operationalize JWKS key overlap rotation, unknown-kid refetch behavior for product verifiers, signer/JWKS consistency health, and remaining token-path hardening (body caps, alg pin regression). Ensure rotation cannot outage validators or leave retired keys trusted forever.

## Scope

### In scope

- Rotation procedure implementation: insert next key → publish both on JWKS → switch signer → wait max_token_ttl+skew → retire
- Admin/ops endpoint or CLI `identity-key-rotate` (break-glass auth)
- Product verifier guidance + shared test proving unknown kid triggers refetch then success when new key published
- ready fails if active signer not in JWKS
- Never delete last verification key while unexpired tokens may exist (guard)
- Body size caps on token/authorize; header limits
- Regression: alg confusion, kid swap

### Out of scope

- HSM vendor integration beyond interface (Phase 14 ops may wire KMS)
- Live multi-replica deploy proof (Phase 14)

## BDD Success Criteria

#### Scenario: P10-S1 — Dual-key JWKS during rotation

- **Given** active kid A and next kid B
- **When** GET JWKS
- **Then** both public keys present
- **And** new tokens may still be A until switch

#### Scenario: P10-S2 — Switch signer to next kid

- **Given** dual publish complete
- **When** ops switches active to B
- **Then** new mints use kid B
- **And** tokens with kid A still verify until retire

#### Scenario: P10-S3 — Unknown kid refetch

- **Given** verifier cache has only A; token signed B after JWKS updated
- **When** verify runs
- **Then** refetch JWKS
- **And** verify succeeds without restart

#### Scenario: P10-S4 — Ready fails if signer missing from JWKS

- **Given** active signer kid removed from publish set (fault inject)
- **When** readyz
- **Then** non-200
- **And** alertable log reason without private key

## Implementation Instructions

1. State machine on signing_keys.status.
2. Clock helper for retire_after.
3. Export `token.Verifier` RefreshOnUnknownKID option used by LMS/Studio.
4. Integration test spins mint A → rotate → mint B → verify both → retire A → A fails after exp simulated.
5. Document runbook snippet in phase completion notes.

## End-to-End Test Plan

| ID | Setup | Action | Assert | Command |
| --- | --- | --- | --- | --- |
| P10-E1 | two keys rotation | mint/verify timeline | overlap works; retire fails old | `go test ./internal/keys -run Rotation -race` |
| P10-E2 | ready inconsistency | remove kid | ready fail | `go test ./internal/api -run ReadySigner` |

## Anti-Cheating Audit

- Rotation not “delete all keys and make one new” without overlap window.
- Verifier refetch must be real HTTP JWKS get in E2E, not only function swap.
- Retired key must not remain in JWKS after window without test justification.

## Completion Gate

- [ ] P10-S*/E* green
- [ ] Runbook steps verified in test comments
- [ ] Anti-cheat clean

## Dependencies

- Upstream: 3, 8–9
- Downstream: 12–14 product verifiers

## Rollback

- Keep previous kid active; postpone retire.
