# Student Tasks — managed-emulator exploratory evidence

## Scope and status

**Provisional selected-image/manual Tasks loop: PASS. Not Phase 2/3 acceptance.**

Tested on dedicated `primer-student-full-qa` emulator, API 35, Emulator 36.6.11,
using the integrated Student debug APK built at `b362025d`. Physical A16 was not
changed and remains on the earlier qualification build. Backend is this Android
integration branch's isolated host Make stack, **not the forthcoming canonical
P3/P4-reconciled producer**. This flow must be replayed after that reconciliation.

Real Tasks/PostgreSQL/Vite were used. The repository's test issuer supplied parent
identity through its public browser flow; this is **not live Clerk acceptance**.
No first-party backend mock, direct database seeding, pasted app bearer, decoder
result injection, or preference pretending device ownership was used.

## Observed public workflow

1. Chrome DevTools, isolated Parent-A browser context: sign in through test issuer,
   add `Android Student QA`, create/publish `Android checklist acceptance`, assign
   a one-off task for the current day through the real schedule form.
2. Student debug APK installed on dedicated emulator, real device-owner enrollment,
   parent setup/recovery-code custody and Clock approval through app UI. Android
   reported managed `LOCK_TASK_MODE_LOCKED`.
3. Original normal-mode camera permission action was blocked by lock-task's
   PermissionController boundary. The native fix added an explicit parent-maintenance
   DPM camera grant. That path worked without any ADB permission grant; CameraX
   opened and produced redacted frame metadata. **Camera QR decoding remains unproven.**
4. Original normal-mode media import was also blocked. The tested import used
   authenticated parent maintenance with the temporary trusted picker allowlist.
   It did not add a permanent Settings/SystemUI exception.
5. Parent browser issued a fresh one-use QR. Captured the actual `.qr-frame`,
   including its rendered white border, at browser DPR 3; selected that exact PNG
   through Android's real Photo Picker. No QR payload reconstruction.
6. Native importer decoded the image itself, paired through the generated Tasks
   façade, and displayed the correct student and assigned task.
7. Closed parent maintenance, reopened Tasks, started the task and submitted for
   parent approval. Native status became `awaiting_verification` while lock task
   remained active.
8. Parent-A web Assigned Work showed the same record as Waiting for Parent.
   Clicking Approve persisted completion; native Refresh from Server showed
   `Status: completed`.
9. Parent-B isolated browser context independently created a different student,
   task and schedule through the UI. Sending that genuine foreign occurrence URI
   to the already-running native app showed generic unavailable, with neither the
   foreign title nor ID exposed. A subsequent warm own URI displayed completed.
10. Rebooted the emulator and used ordinary wake/swipe-to-open. Pairing and the
    completed task reloaded from the real server; managed lock task remained.
11. Parent-A archived the test student through the UI and confirmation dialog.
    Native refresh returned to pairing and removed the previous student/task data.
    Device ownership/lock task remained; another unused local recovery code still
    opened and closed maintenance.

## Independent records

These IDs were observed in the public browser UI, not invented or used to seed data:

- Parent-A student: `ec5aea57-a91f-4610-bda9-d999067929a8`
- Own occurrence: `22ddba58-f015-482c-ab3c-1ea89317ef31`
- Parent-B student: `580717ef-10ae-4dca-a3cc-453cc6a2fd9e`
- Foreign occurrence: `56763a14-4bbe-473d-8b34-7bf71873ca71`

Chrome network evidence included pairing POST 200, occurrence decision POST 200,
subsequent occurrence list GET 200, student archive DELETE 204 and roster GET 200.
No browser console errors/warnings were present in the retained final navigation
window. This is not a claim covering console logs lost across browser restarts.

## QR diagnostic boundary

The first SVG-only screenshot lacked its quiet border and was not an acceptable
capture of the whole rendered frame. Corrected 1x frame captures still failed
ZXing. A genuinely re-rendered high-DPI frame decoded with ZXing's documented
PURE_BARCODE mode in a host diagnostic. That diagnostic emitted only success and
length, never fed decoded text into Student.

The product fix preserves ZXing hints and adds a selected-image-only pure-barcode
fallback after normal detection. The real native importer then decoded the
browser-rendered high-DPI image. This does **not** establish normal-DPI screenshot
robustness, arbitrary photographed QR acceptance, or CameraX decoding success.

Raw screenshots, emulator recovery values and detailed exploration state remain
under ignored `.paseo-e2e/android-student-tasks/`, not in repository evidence.

## Still unproven

- Live Clerk and native Control-to-Student end-to-end loop.
- Canonical P3/P4 producer replay, current migration numbering and combined gates.
- Physical A16 camera/Tasks flow and physical TV D-pad/playback parity.
- Remote management/recovery/release transport, silent updates and full-day soak.
- Full native regression/connected automation promotion. This was exploration,
  not a Playwright or instrumented test suite falsely marked green.
