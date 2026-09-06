# feature-tasks-student

Native Tasks pairing/checklist/start/submit for Primer Student.

## Entry point

- Compose: `com.aleksclark.primer.student.tasks.StudentTasksRoute(deepLink, onLeave)`
- Session: `TasksSession` uses the existing `com.aleksclark.primertasks.client.TasksClient` façade.
- Student shell: `MainActivity` → `Open Tasks` / `primerstudent://occurrences/{id}` (legacy `primertasks://occurrences/{id}` still parsed, no credential forwarding).

## Lifecycle / session contracts

- Tasks bearer lives in Keystore+DataStore (`student_tasks_metadata`, alias `primer_student_tasks_device`).
- 401/403 pairing/session failures call `TasksSession.clearPairing()` only. They never touch `StudentRuntime`, `RecoveryStore`, or device-owner policy.
- Old `com.aleksclark.primertasks` tokens cannot be migrated. Parents issue a new Student QR and revoke the prototype pairing.
- Prototype `make tasks-android` / `primer-tasks/android` remains until this flow is proven.

Gradle depends on the real `:tasks-client` module at `primer-tasks/clients/kotlin`; no client source-set is copied into the feature. Run `make tasks-clients` before a fresh build. Generated internals stay ignored.
