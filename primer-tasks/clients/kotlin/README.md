# Primer Tasks Kotlin client

Committed façade: `com.aleksclark.primertasks.client`.
Generated internals: `com.aleksclark.primertasks.generated` (gitignored).

```text
Huma handlers → `go run ./cmd/openapi-gen` → `generate-client.mjs` → :tasks-client
```

Apps import this package only. Parent, student-device, and management-device
credentials are separate `CredentialProvider`s. Pass the exact mounted origin
(`/tasks/api` stays `/tasks/api`).

```bash
make tasks-clients
cd android && ./gradlew :tasks-client:test --no-daemon --max-workers=1
```
