# Primer Tasks Kotlin client

Committed façade: `com.aleksclark.primertasks.client`.
Generated internals: `com.aleksclark.primertasks.generated` (gitignored).

```text
Huma handlers → `go run ./cmd/openapi-gen` → `generate-client.mjs` → :tasks-client
```

Apps import this package only. Parent, student-device, and management-device
credentials are separate `CredentialProvider`s. Pass the exact mounted origin
(`/tasks/api` stays `/tasks/api`).

Go-owned extensions `x-maxBytes`, `x-nonBlank`, `x-equalFields`, and
`x-uniqueNormalized` are interpreted by `ContractConstraints`. Unknown `x-*`
keys fail closed in the generator and matcher. This package does not implement
device websocket dialogue or bearer fallback.

```bash
node generate-client.test.mjs
# Generate internals from a producer OpenAPI into the gitignored generated/ tree:
node generate-client.mjs
```

**Provisional producer binding (not final canonical acceptance):**
producer `7a83b07454e0be9a99f1634cad07a871f3775fd0` tree
`3c3b7c0332395e42d881d6873af66af300629385`; combined digest
`535077f155b22786c731c5ee15205b669006e0d0b63fb099c4bb88117902a446`
from `/tmp/primer-android-p4-reconciliation/bundle-a`. Input baseline remains
accepted `0e8cc6d6` / `296b4e17…`. CP4 WIP notice `39c…` is not adopted.
Final verification rebinds to a reviewed P4 successor bundle that preserves
optional verification/studentName fields.

The canonical Android graph on this branch is TV-only (`:core`/`:app`) and
cannot compile `:tasks-client` without native `settings.gradle.kts` edits,
which this lane does not make. Qualify the generator and constraint matcher
here; façade HTTP tests require the Android include graph owned elsewhere.
