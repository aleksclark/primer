# Identity IB2 resume handoff

**Purpose:** Let a fresh worktree/orchestrator pick up Identity without
re-deriving history. Prefer this file over chat logs.

**As of:** 2026-08-17  
**Authoritative code tip (local, unpushed):** `bee0a5c99b364ad9c2bf3ceddc73abdd28b44960`  
**Dual-reviewed production tip:** `0dd5faac34da7afc19a11c3e463d7d28eef2b538`  
**Branch / worktree:** `impl/IB2-hardening-remed` @  
`/home/aleks/work/projects/primer/worktrees/impl-IB2-hardening-remed`  
**Primary `master`:** `64fdfbfe4e3bd53452def418fbcf7bd026523aa1` (PR #24 already merged
IB2 at `f31559d`; hardening is **not** on master yet)

---

## 1. What is done

### On master (PR #24, tip `f31559d`)

Credential-free IB2/I6: ES256 access tokens (`typ=at+jwt`), JWKS, RFC8414
metadata, `/oauth/token` authorization-code exchange, RFC7009 revoke,
private_key_jwt, sign-before-commit, migrations `00006`–`00007`, E00–E11
evidence, dual review recorded in docs commit `7f3b181`.

### On local hardening branch (dual-reviewed `0dd5faa`, harness `bee0a5c`)

Post-merge hardening that closed dual-review FAILs against `b3ff727` /
`eb28f91` and re-passed at `0dd5faa`:

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

**Parent gates on `0dd5faa`:** `identity-test-oauth`, `identity-e2e`,
`identity-cover` 82.4%≥80, `identity-openapi`, `foundation-check`,
`go build/vet/tidy`, `git diff --check`.

**Dual review on `0dd5faa`:** SPECIFICATION PASS (0C/0I); QUALITY/SECURITY
APPROVED (0C/0I).

**Live harness on `bee0a5c`:** green against
`~/.config/primer/stytch-test.env` (project-test-*); garbage token →
definitive; unknown tuple → unavailable; secrets not logged. Optional
session happy path needs `IDENTITY_LIVE_STYTCH_SESSION_TOKEN`.

---

## 2. What is intentionally not done

| Item | Owner | Notes |
|------|-------|-------|
| Push / PR / merge of hardening branch | Human | Local only until authorized |
| IB3 / I7 product BFF cookie, CSRF, PKCE | Next wave | Start only after hardening on master |
| IB4 signed webhook / two-plane revocation | Later | Hard gate for production BFF/MCP |
| IB5 service principals / client_credentials | Later | Token still rejects CC grant |
| IB6 refresh rotation/reuse/logout lifecycle | Later | Initial refresh family only |
| IB7 full key retire/destroy | Later | At-most-one active/next only |
| IB8 Studio/LMS/TV cutover | Later | — |
| IB8-E10 browser + webhook live proof | Later | Harness is adapter-only |

---

## 3. Resume checklist (new worktree)

1. **Integrate first (preferred):** from primary, open PR from
   `impl/IB2-hardening-remed` (or cherry-pick `1c93be4..bee0a5c` onto fresh
   branch from current master). Do not start IB3 from master `f31559d` alone.
2. **Or continue local:**  
   `git worktree add …/impl-IB2-hardening-remed impl/IB2-hardening-remed`  
   pin `git rev-parse HEAD` = `bee0a5c…` (or later).
3. Re-run parent gates before any mutation of an accepted SHA.
4. Dual-review again only if the tip changes.
5. Next implementation wave: **IB3 / I7** —
   `agent_docs/plans/primer-identity-service/phase-07-product-bff-contract.md`  
   branch name `impl/I7-bff-contract`.
6. TMPDIR: `/var/tmp/primer-ib2-*` (never `/tmp`). No live production Stytch.
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
# Parent gates (hardening tip)
make identity-test-oauth identity-e2e identity-cover identity-openapi foundation-check
(cd primer-identity && go build ./... && go vet ./... && go mod tidy)
git diff --check

# Opt-in Stytch test-project adapter qualification (not IB8-E10)
export IDENTITY_LIVE_STYTCH=1
export IDENTITY_LIVE_STYTCH_ENV_FILE=$HOME/.config/primer/stytch-test.env
export IDENTITY_STYTCH_ENV=test
make identity-live-stytch
```

---

## 6. Lineage (hardening branch after master IB2)

```text
f31559d  fix(identity): harden IB2 replay and clock semantics   # on master
1c93be4  fix(identity): close IB2 review custody leaks
b3ff727  fix(identity): align IB2 issuer metadata and assertion nbf
eb28f91  fix(identity): align IB2 token audiences and fail-closed verify
0dd5faa  fix(identity): close IB2 grant metadata and assertion retention  # dual PASS
bee0a5c  test(identity): add opt-in live Stytch test-project harness
```
