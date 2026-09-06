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
make tasks-clients
cd android && ./gradlew :tasks-client:test --no-daemon --max-workers=1
```
