# Android additive producer reconciliation — isolated PRE-MERGE checkpoint

## Scope and authority

This worktree starts at corrected CP3 source
`0e8cc6d6d3506dea9d5a19900bed7f5c500eba15`, tree
`d3b63c42a27f3bee5ca3f999ba50c48d2b006089`. That source is independently accepted
for CP3 continuation/pre-merge coordination only, **not full P4, native, CI, or
canonical merge acceptance**. Prior `d84b3ef5` remains blocked/history.

Independent receipt verification checked eight raw artifacts, all 182 source
hashes against Git objects, the exact tree, and reconstructed normalized bytes:
`296b4e17a1024761dcd6af84df5b9c4def70fa941c8500791f2260104545b655`.
Normalization is `json-recursive-object-key-order-v1` over four Go JSON contracts;
arrays and semantic constraints are not altered.

Receipt-byte distinction is preserved:

- Reviewed handoff SHA-256: `1cbe5e24800f05c748ebee7559e5da1985e596bd439296e2397d992280874cbe`.
- Later bookkeeping handoff: `7560dc9bfd8710c5a2d58416a3160cea7341dd888bab4e2144e7b01d30b0497e`;
  **not** described as reviewer-reviewed bytes.
- Accept report: `e34d727d25b55c267247064497e19fc138f7f784ffb7125abdbb1a061f4f5547`.
- Local independent proof: `/tmp/primer-android-p4-reconciliation/input-verification.json`.

No P4 worktree was changed. No canonical merge, publication, deployment, hardware,
or legacy/native dialogue work is authorized by this checkpoint. SAME150's
Kotlin/legacy Android hold remains. Kotlin changes are owned separately in
`integration/android-kotlin-p4`; this producer commit contains none of them.

## Additive port / preservation ledger

Android source: `d9bb765add44c51d527e1143b65a12b5f0a73274`, using only scoped
producer additions relative to `34a4f5c2`. Outstanding management-only changes
from `e7fe88c3` and `678985f5` were included; native Control and TV code were not.

- Preserve canonical `tasksdb.Database`, `agentHub`, `StartAgentWorker`, strict
  parent inspect decoder/lookahead, parent admin/membership checks, agent socket
  authority, and host-cookie-only `/student/ws`.
- Add management enrollment, independent management-device credentials,
  policy/recovery/target/receipt routes, release artifacts, and operator-only CLI.
  Management uses the existing database interface rather than a second DB stack.
- Add optional native authorized parties without removing `PublicOrigin`, issuer,
  audience, local membership/admin, or revocation checks. This is not evidence of
  a real Clerk instance's native `azp`, nor permission to change live policy.
- Keep **one** production `humaAPI()` registry. Add 21 management/release operations
  to the canonical 47 (68 total). APK responses are typed binary transport.
- Port typed action/decision/retry responses and non-null page arrays without
  replacing canonical transaction logic, requirement envelopes or decision
  semantics. Retry retains canonical `requirementId` / `attemptId` query selectors
  and returns `requirementId` / `previousAttemptId`; those are not dropped to fit
  the older Android DTO.
- Preserve all canonical migration bytes `00001`–`00011`. Append Android management
  migrations as `00012`–`00015` in their original order. Existing upgrade assertions
  still count the canonical eleven explicitly; added assertions verify total
  fifteen and unchanged canonical table/ledger contents across the upgrade.
- No generated contract/client output is committed. No root build graph, dependency
  pins, workflow, Docker, Compose, web, Kotlin or Android consumer files changed.

## Contract and observed checks

Initial combined normalized digest:
`535077f155b22786c731c5ee15205b669006e0d0b63fb099c4bb88117902a446`.
Two offline emissions into separate directories were byte-identical, including
manifest files. All three WS/config JSON contracts remain byte-identical to the
accepted CP3 bundle; only REST has additive management/typed-response changes.

Passed on this isolated tree with `GOWORK=off GOFLAGS=-mod=readonly`:

- Go compile, `go vet ./...`, `go build ./...`.
- Typed production-registry/status checks and additive authorized-party tests.
- HTTP-handler-boundary tests (`httptest` requests/recorders) with real PostgreSQL:
  management enrollment, isolation, replay, policy CAS, release publication/receipt/
  artifact and concurrent revoke/retarget. These are not yet networked real-process
  generated-client acceptance.
- Existing canonical Clerk/P3/P4 database upgrade checks.
- New `TestManagementUpgradePreservesCanonicalEleven`: synthetic P3/P4 schema
  preconditions, canonical table/ledger hashes preserved after two migrations,
  exact 11+4=15 ledger entries. This is schema proof, not public dialogue success.
- All standalone management-domain tests passed; that isolated package run reported
  **64.2% coverage**, not an accepted combined 85% gate. API-call coverage is not
  included in that number and must not be inferred.

## Required rebind and remaining gates

A subsequent notice describes uncommitted P4 capability metadata in `phase2.go`
(`OccurrenceVerification`, optional `Occurrence2.verification`, parent detail
`studentName`) with WIP digest `39c2af23…`. **That is not an accepted successor.**
This checkpoint does not adopt it or silently upgrade its baseline. Final
reconciliation acceptance requires the eventual exact reviewed P4 successor and
preservation of those additive fields when replaying these narrow response hunks.

Still owed: source-bound frozen generated-client public tests, separable Kotlin
facade/generator handoff including all `x-maxBytes`, `x-nonBlank`, `x-equalFields`,
and `x-uniqueNormalized` semantics, combined coverage, full race/count10/workspace
and CI gates as applicable, full P4 UI/Chrome acceptance, and actual native/live
identity/update testing. No full-phase acceptance is claimed here.
