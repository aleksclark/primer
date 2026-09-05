# Primer Tasks Phase 1 — authoritative final evidence

Branch: `impl/tasks-p1-foundation`
Final reviewed tip: `23b5685` (`review(tasks): approve Phase 1 adaptation`)
Status: **PASS — ready for Phase 2 dispatch**
Review report: [`test-artifacts/phase1-final-review/report.md`](../../test-artifacts/phase1-final-review/report.md)

## Gate decision

Phase 1 is complete at the reviewed tip. The final implementation, browser and
Android public-boundary acceptance, promoted automation, contract generation,
coverage, Stacklane proofs, and anti-cheat review all pass. No Phase 2 source or
workflow was started by this phase review.

## Final acceptance and evidence

- **Android picker acceptance:** Dedicated Terra acceptance commit
  `6e4f2b6` reports PASS in
  [`test-artifacts/android-picker-acceptance/report.md`](../../test-artifacts/android-picker-acceptance/report.md).
  It used a fresh real Parent A UI QR, opened the primary CameraX scanner first,
  then selected the exact rendered QR image through the real Android system
  Photo Picker/SAF. The image reached the bundled decoder,
  `PairingQrParser`, the shared `pair()` path, generated bearer client routes,
  and the real API. Bound name, empty checklist, force-stop/reboot
  persistence, second-emulator replay denial, archive/revocation, replacement
  re-pair, and paired storage/logcat/backup checks all passed.
- **Huma contract:** `b7f83f5` aligned the Android bearer client with the
  server-owned `/device/profile` and `/device/checklist` contract, added the
  bearer checklist route, and added integration coverage separating device
  bearer credentials from browser student cookies. Offline OpenAPI emission and
  Kotlin/TypeScript generation passed with generated outputs untracked.
- **Coverage and builds:** Go tests, race, vet, build, Android JVM tests,
  Android build, web lint/typecheck/build, and the 85% coverage gate passed.
  Measured product coverage was **85.5%**.
- **Browser promotion:** The real exploratory browser PASS preceded promotion.
  The promoted Playwright flow passed (**1 test**), covering real PKCE login,
  HttpOnly/SameSite cookie attributes, student CRUD, two-tenant URL isolation,
  browser pairing/refresh, replay denial, and archive revocation.
- **Android promotion:** The promoted connected picker test passed (**2 tests**)
  on a fresh Android 15 Pixel AVD and verified the primary CameraX action,
  accessible secondary import action, real system Photo Picker foreground
  package/privacy semantics, and return to the app.
- **Stacklane proof:** `make tasks-proof` and the lifecycle checks passed for
  labels, loopback-ephemeral ports, isolated instances, source mounts, named
  state, Go reload without container restart, Vite HMR without navigation, and
  cleanup.
- **Final review:** Fresh anti-cheat review commit `23b5685` explicitly
  approved the adaptation and found no auth, tenancy, pairing, client,
  generated-output, or Compose substitution.

## Transparent Android emulator exception

CameraX remains the primary production pairing path and its CameraX/decoder
integration remains tested. The emulator acceptance used Photo Picker/SAF only
because the recorded Android Emulator 36.4.10 VirtualScene service accepts
poster bytes but does not propagate `Poster.image` into camera-visible scene
geometry. The retained systematic evidence is:

[`test-artifacts/android-foundation-remediation/gateb-systematic-20260819T234500Z/report.md`](../../test-artifacts/android-foundation-remediation/gateb-systematic-20260819T234500Z/report.md)

That evidence records the real 60-pose search, accepted poster metadata, active
CameraX output, and no QR in rendered frames. No further VirtualScene/webcam
pose attempts are authorized. The physical-camera/live-scene limitation remains
visible; the fallback is not a production camera bypass and does not permit
manual code entry, payload recreation, direct API seeding, or internal test
seams.

## Historical evidence

Earlier remediation runs were genuinely blocked and are not erased by this
final PASS. The prior blocked checkpoint remains available in git history at
`67be853` (`primer-tasks/test-artifacts/phase-01-evidence.md`), while the
retained VirtualScene report above preserves the specific upstream blocker and
its evidence path. Those historical blockers must not be read as the status of
the reviewed tip `23b5685`.

## Final cleanliness

The reviewed branch has no pending tracked or untracked changes. Generated
contracts, clients, build products, and test-run outputs remain untracked or
ignored. Phase 2 was not started.
