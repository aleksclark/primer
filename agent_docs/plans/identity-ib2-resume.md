# Identity IB2 resume handoff

**Purpose:** Let a fresh worktree/orchestrator pick up Identity without
re-deriving history. Prefer this file over chat logs.

**As of:** 2026-08-17  
**Dual-reviewed production tip:** `f5d5b5b372dc6988a65d082f948b96e7de9f432b`  
**Harness tip (same branch):** `e8ecd183b9354bcf9b200f2c1ace81ee1bdb47b5`  
**Branch that carried integration:** `impl/IB2-hardening-remed`  
**Prior master IB2 tip (superseded):** `f31559db92445826aabd98bbc0903a22d80d6e80` (PR #24)

---

## 1. What is done

### Master baseline (PR #24, tip `f31559d`)

Credential-free IB2/I6: ES256 access tokens (`typ=at+jwt`), JWKS, RFC8414
metadata, `/oauth/token` authorization-code exchange, RFC7009 revoke,
private_key_jwt, sign-before-commit, migrations `00006`–`00007`, E00–E11
evidence, dual review recorded in docs commit `7f3b181`.

### Hardening tip (dual-reviewed `f5d5b5b`, harness `e8ecd18`)

Post-merge hardening that closed dual-review FAILs and re-passed:

| Area | Change |
|------|--------|
| Audiences | Host-rooted `/oauth/token` and `/oauth/revoke` match routes for pathful issuers |
| Assertions | `AssertionClockSkew=60s`; opaque bounded control-free `jti` |
| Verify | Human Verify requires `ClientLookup`; E10 allowlists `studio-bff` |
| Wire | Exact `{"error":"invalid_client"}`; exact auth-code field sets; Basic must not send form `client_id` |
| OpenAPI | Regenerated `openapi.yaml` + `client.gen.go` |
| Signer | `TransactionSigner` value-receiver Format/JSON refusal |
| DB `00008` | Active-family live-token cardinality; `token_issuance_audit.signing_key_id` FK RESTRICT |
| DB `00009` | Assertion replay retention = JWT `exp+60s`; purge after retention; lifetime-edge → `invalid_client` not 503 |
| Metadata | RFC8414 `grant_types_supported` exact triple; token still rejects refresh/client_credentials |
| Live harness | Opt-in `make identity-live-stytch` (`-tags=live_stytch`); test-project only; **not IB8-E10** |

**Parent gates on dual-reviewed tip:** `identity-test-oauth`, `identity-e2e`,
`identity-cover` 82.4%≥80, `identity-openapi`, `foundation-check`,
`go build/vet/tidy`, `git diff --check`.

**Dual review:** SPECIFICATION PASS (0C/0I); QUALITY/SECURITY APPROVED (0C/0I).

**Live harness:** green against `~/.config/primer/stytch-test.env` (project-test-*).
Optional session happy path needs `IDENTITY_LIVE_STYTCH_SESSION_TOKEN`.

---

## 2. What is intentionally not done

| Item | Owner | Notes |
|------|-------|-------|
| IB3 / I7 product BFF cookie, CSRF, PKCE | **Next wave** | Branch `impl/I7-bff-contract` |
| IB4 signed webhook / two-plane revocation | Later | Hard gate for production BFF/MCP |
| IB5 service principals / client_credentials | Later | Token still rejects CC grant |
| IB6 refresh rotation/reuse/logout lifecycle | Later | Initial refresh family only |
| IB7 full key retire/destroy | Later | At-most-one active/next only |
| IB8 Studio/LMS/TV cutover | Later | — |
| IB8-E10 browser + webhook live proof | Later | Harness is adapter-only |

---

## 3. Resume checklist (new worktree)

1. Branch from current `master` after IB2 hardening is merged.
2. Next implementation wave: **IB3 / I7** —
   `agent_docs/plans/primer-identity-service/phase-07-product-bff-contract.md`  
   branch name `impl/I7-bff-contract`.
3. Re-run parent identity gates before claiming green on a new tip.
4. TMPDIR: `/var/tmp/primer-ib2-*` (never `/tmp`). No live production Stytch.
   Test credentials: `IDENTITY_LIVE_STYTCH=1` +
   `IDENTITY_LIVE_STYTCH_ENV_FILE=$HOME/.config/primer/stytch-test.env`.

---

## 4. Key paths

```text
primer-identity/internal/token/     # mint, verify, assertion
primer-identity/internal/oauth/     # Exchange, Revoke
primer-identity/internal/api/       # token, revoke, metadata, JWKS
primer-identity/internal/keys/      # signing custody, TransactionSigner
primer-identity/internal/db/migrations/00006..00009
primer-identity/internal/testutil/e2e/
primer-identity/internal/testutil/live/   # live_stytch tag
agent_docs/plans/primer-identity-service/
agent_docs/plans/stytch-identity-ib0/     # contract authority
agent_docs/plans/curriculum-studio-delivery/execution-index.md  # wave cursor
```

---

## 5. Commands

```bash
make identity-test-oauth identity-e2e identity-cover identity-openapi foundation-check
(cd primer-identity && go build ./... && go vet ./... && go mod tidy)
git diff --check

export IDENTITY_LIVE_STYTCH=1
export IDENTITY_LIVE_STYTCH_ENV_FILE=$HOME/.config/primer/stytch-test.env
export IDENTITY_STYTCH_ENV=test
make identity-live-stytch
```

---

## 6. Lineage (hardening after master IB2)

```text
f31559d  fix(identity): harden IB2 replay and clock semantics   # PR #24 on master
90af37d  fix(identity): close IB2 review custody leaks
e64455a  fix(identity): align IB2 issuer metadata and assertion nbf
b86b6eb  fix(identity): align IB2 token audiences and fail-closed verify
f5d5b5b  fix(identity): close IB2 grant metadata and assertion retention  # dual PASS
e8ecd18  test(identity): add opt-in live Stytch test-project harness
```
