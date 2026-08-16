# Read-only JWKS donor evidence

## Provenance and scope

- Donor worktree: `/home/aleks/work/projects/primer/worktrees/impl-stytch-identity-jwks`
- Donor exact clean tip: `53b693cc93c8cccb15109d90bae90811029af893` (`feat(identity): add ES256 key custody`)
- Donor parent/base: `87d5c215134825edb410266a62c15534e1e9ecea`
- IB0 authoritative clean base: `b9fb326b291c45c3e8994c8af98b2ee9f7e714e6`
- Merge base of donor and IB0 base: `87d5c215134825edb410266a62c15534e1e9ecea`

Therefore the donor was built before IA-R changes that lead to the IB0 base. It is **not merged, not rebased onto IA-R, and not independently approved as IB2**. This assessment is inventory evidence only.

The donor commit changes **23 files total: 22 under `primer-identity` plus one plan file**, with 4,327 additions/10 deletions relative to its parent. Its own plan explicitly labels B1 “pending independent specification and quality/security review” and states B2 JWT and B3 JWKS HTTP are deferred: `agent_docs/plans/primer-stytch-jwks.md:1-56`.

## Keep / reshape / drop

| Classification | Exact paths/evidence | IB2 disposition |
|---|---|---|
| **KEEP concept, rebase/review** | `internal/domain/signing_key.go`, `internal/keys/material.go`; P-256/ES256, canonical public JWK, `crypto.Signer`, AES-256-GCM sealed private material, destroy/redaction tests | Strong custody primitives. Reapply/rebase onto post-IB1 IA-R tip and independently review concurrency, cryptography, config and migration. |
| **KEEP concept, rebase/review** | `internal/keys/service.go`, `service*_test.go`; active/next/retired lifecycle, signer fencing, DB revalidation | Candidate key-lifecycle service for IB2/IB7. Preserve tests; reconcile with new issuance transaction/readiness and IB7 rotation policy. |
| **KEEP concept, reshape schema** | `internal/db/migrations/00004_signing_keys.sql`, `signing_keys_migration_test.go`, `internal/repo/signing_keys.go` | Reserve the next migration number on post-IB1, resolve collision, preserve ES256 and unique active/next invariants, add/confirm retention/overlap fields required by IB7. Never copy an already-colliding `00004`. |
| **RESHAPE** | `internal/config/config.go`, `config*_test.go`, `key_config_test.go` | Donor branched before IA-R and touches config/stytch tests. Port only key-custody fields into current namespaced/fail-before-listen policy; do not overwrite IA-R prefix isolation or production Stytch requirements. |
| **RESHAPE** | `internal/db/SCHEMA.md`, accounts/stytch migration tests, `domain/errors.go`, `repo/errors.go` | Integrate narrowly after IB1 migrations; keep current error/index semantics and regenerate exact migration inventory. |
| **DROP from IB2 intake** | donor edits to `stytch_config_test.go`, accounts/stytch-mapping tests not required by key custody | These are branch-base adaptation noise and conflict with IA-R/IB1; use current-tip tests instead. |
| **DROP as completion evidence** | `agent_docs/plans/primer-stytch-jwks.md` PASS claims | Useful historical donor note only. It does not satisfy this IB0 contract or current exact-tip review. |
| **MISSING; implement in IB2** | no token/JWT package or `/oauth/token` access-token issuer in donor search | Donor is key custody only; add strict Primer JWT profile (`sub=accounts.id`, single `aud`, ≤15m, claim allowlist) after IB1 grant/code path. |
| **MISSING; implement in IB2** | no `/.well-known/jwks.json`, authorization-server metadata, signer-aware HTTP readiness, product validator E2E | Add handlers/OpenAPI parity/cache/ETag and product fetch verification. Donor explicitly defers B2/B3 at plan lines 47–56. |
| **MISSING; implement later** | no broker/grant/provider-session binding, BFF, refresh, webhook, MCP | These remain IB1/IB3/IB4/IB6/IB8 and cannot be inferred from custody code. |

## Dependency and intake conflicts

1. **Pre-IA-R ancestry:** wholesale cherry-pick risks restoring outdated config/cache/mapping state. Compare and port path-by-path.
2. **Migration numbering:** IB1 will add OAuth/broker tables. `00004_signing_keys.sql` may collide; assign the next free number only after IB1 lands.
3. **Config overlap:** donor modifies `internal/config/config.go` and tests while IA-R enforces namespaced production policy. Current-tip policy wins.
4. **Scope mismatch:** donor plan used a strict 120-second token as deferred B2; the IB0 candidate specifies a maximum of 15 minutes with actual per-client/provider cap. A shorter default remains allowed, but must be registered and reported accurately.
5. **Subject/profile mismatch:** donor has no JWT claims implementation; it cannot prove `sub=accounts.id`, single audience, no provider roles/org IDs, or `at+jwt`.
6. **No HTTP/product proof:** custody service tests cannot substitute for public token/JWKS and Studio/MCP validator E2Es.
7. **Review status:** donor self-record says pending independent review. Do not claim approved merely because its local test list passed.

## Safe later intake sequence

1. Land IB1 on IA-R and obtain its exact reviewed tip; ensure clean tree and snapshot member-session/grant migrations are final.
2. Create a fresh IB2 branch/worktree from that exact tip. Record donor tip/parent and diff.
3. Reserve migration number after all IB1 migrations. Reimplement or three-way-port the signing-key table/domain/material/repository/service in small TDD slices; never cherry-pick the whole donor commit.
4. Port key config fields manually into current namespaced config and rerun hostile ambient-variable/fail-before-listen tests.
5. Run migration fresh/upgrade/down and concurrency/key corruption/retirement tests on real PostgreSQL.
6. Add the missing strict JWT issuer and JWKS/metadata handlers from this IB0 contract using RED→GREEN public E2Es.
7. Run product validator tests: exact issuer, ES256, single audience, ≤15m, account subject, absent provider/product role claims, wrong-audience/raw-Stytch rejection.
8. Obtain fresh independent specification and quality/security review on the integrated exact tip. Only then may IB2 be called complete.

Recommended verification commands at intake (adapt to repository Make targets without weakening):

```bash
cd primer-identity
GOWORK=off go test -race ./... -count=1
GOWORK=off go build ./...
GOWORK=off go vet ./...
go mod tidy && git diff --exit-code -- go.mod go.sum
git diff --check
```

Also run the disposable real-Postgres migration suite, public HTTP token/JWKS E2Es, config fail-before-listen subprocess probes, secret/log scans, and `govulncheck` if available. No live Stytch credential is needed or allowed for donor intake.
