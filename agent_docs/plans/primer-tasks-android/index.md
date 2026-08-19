# Primer Tasks Android continuation plan

## Outcome

Complete the native student client independently from the standalone Tasks
web/server delivery sequence. The existing paired app and checklist foundation
remain usable while the server and SPA proceed through dialogue, media, external
verification, and release phases. Native work consumes stable public contracts
after each corresponding web/server phase and never blocks those phases.

The completed native loop is:

```text
existing paired student → checklist → dialogue/media/external verification
  → durable server decision → restart/reconnect/revocation-safe native UX
```

## Current-state summary

| Area | State | Evidence |
|---|---|---|
| Pairing/auth | Complete baseline: one-student binding, Keystore-encrypted bearer, QR camera primary plus documented system-picker fallback | Primer Tasks Phase 1 reviewed evidence |
| Checklist/manual approval | Complete baseline: Today/Upcoming/detail/start and parent-approval state refresh | Primer Tasks Phase 2 reviewed evidence |
| Dialogue native client | Partial implementation exists in the Phase 4 worktree, but native exploratory/promotion was intentionally halted before acceptance | `primer-tasks/android/`, `clients/kotlin/` on the active Phase 4 lineage |
| Media capture/upload | Missing | Extracted from former main Phase 5 |
| External verifier progress | Missing | Extracted from former main Phase 6 |
| Release/device replacement matrix | Missing | Extracted from former main Phase 7 |

The main [Primer Tasks plan](../primer-tasks/index.md) now owns only web/server
work for Phases 4–7. This plan owns every remaining native-client requirement
from those phases.

## Scope boundaries

### In scope

- Native dialogue verification, media capture/file submission, asynchronous
  rubric progress, external-verifier progress, and release hardening.
- Existing per-device bearer authentication and single-student binding.
- Generated Kotlin clients and one owned WebSocket/binary transport façade.
- CameraX, video capture, audio recording, system file/photo picker, WorkManager
  or equivalent bounded background upload, cache cleanup, and lifecycle recovery.
- System C Compose UI, accessibility, process/reboot/reconnect/revocation tests.
- Dedicated emulator exploratory acceptance before connected/emulator promotion.

### Out of scope

- Redesigning server verification policy, completion authority, task schemas, or
  parent web workflows owned by the main plan.
- Raw database access, shared secrets, manually copied DTOs, ad-hoc endpoint URLs,
  client-set completion, hidden reasoning, or offline authoritative completion.
- Blocking main web/server phases while native adapters lag.

## Global constraints

1. **API dependency, not delivery dependency:** Each native phase begins only
   after the corresponding Tasks server contract is reviewed. Main Tasks phases
   may complete without native acceptance.
2. **Server authority:** Native code submits messages/artifacts and displays
   decisions; it never creates evaluation/decision/completion facts directly.
3. **Generated clients:** Huma/Go boundaries emit ignored Kotlin outputs; the app
   imports committed façades only. Raw OkHttp/WebSocket/binary helpers stay in
   those façades.
4. **Credential custody:** Bearers remain Android-Keystore encrypted, omitted from
   backups/logs/URIs/media metadata, and cleared on revocation.
5. **Camera boundary:** CameraX is primary. The recorded Emulator 36.4.10
   VirtualScene poster-propagation bug permits selecting the exact web-rendered
   QR through the real system Photo Picker for emulator pairing only; no manual
   codes, payload recreation, direct pair API, or internal injection.
6. **Safe progress:** Display generic thinking/evaluating/tool progress, never raw
   reasoning/tool input/provider secrets.
7. **Durability:** Backgrounding/disconnect never cancels a server-owned run or
   upload. Reopen fetches/replays durable state with idempotency keys.
8. **Media safety:** Bound size/count/duration/pixels, sniff content rather than
   trust extension, use server-generated object keys, strip derivative metadata,
   and clean temporary media.
9. **System C:** Consume generated native tokens; dark primary/light parity,
   square/ruled hierarchy, visible focus/accessibility semantics, no bubbles.
10. **Testing order:** Dedicated emulator exploration on real Stacklane/API first;
    fix and rerun to PASS; only then write/run connected or black-box automation.

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Android Phase 1: Dialogue verification](./phase-01-dialogue-verification.md) | Complete and prove the native three-question dialogue, reconnect, concurrency, and revocation flow. | Reviewed main Phase 4 API |
| [Android Phase 2: Media evidence and rubric](./phase-02-media-evidence.md) | Capture/select image, video, and audio; upload safely; display durable no-chat rubric results. | Android Phase 1; reviewed main Phase 5 API |
| [Android Phase 3: External verifier progress](./phase-03-external-verifier.md) | Submit to external verification and survive wait/retry/reconnect/restart. | Android Phase 2; reviewed main Phase 6 API |
| [Android Phase 4: Release hardening](./phase-04-release-hardening.md) | Prove replacement/revocation, full native regression, privacy, operations, and release packaging. | Android Phase 3; reviewed main Phase 7 API |

## Requirement traceability

| Extracted requirement | Android phase |
|---|---|
| Student dialogue stream, background/kill/reopen, concurrent web/native conflict, revoke mid-attempt | 1 |
| Camera photo/video, audio recording, system files, permission/error/retry/cache cleanup, image rubric | 2 |
| External verifier waiting/progress/reconnect/terminal state | 3 |
| Device replacement, network/server restart, full native matrix, storage/logcat/backup/privacy, app diagnostics | 4 |

## Completion rule

This Android plan is complete only when every phase has independent emulator
exploratory PASS followed by promoted automation; all native consumers use the
generated façades; public API behavior is exercised against real Tasks/PostgreSQL/
object/external services; process/reboot/network/revocation/replay cases pass;
privacy scans find no credentials or retained sensitive media; and a fresh
anti-cheat review confirms that no native path bypasses server authority.
