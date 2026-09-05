# Phase 1 final anti-cheat review

## Verdict

**PASS — approve Phase 1 adaptation.** The reviewed tip is `81de71c` on
`impl/tasks-p1-foundation`. Phase 1 is genuinely ready for integration. Phase
2 is not started, not reviewed, and is not authorized by this review.

The Android exception is accepted only as the documented emulator-test
adaptation. CameraX remains the production primary. The upstream
`VirtualScene` limitation remains visible and blocked at
`test-artifacts/android-foundation-remediation/gateb-systematic-20260819T234500Z/report.md`.
No further VirtualScene poses, webcam injection, or renderer experiments are
authorized.

## Reviewed scope and evidence

Read `AGENTS.md`, the amended `agent_docs/plans/primer-tasks/index.md` and
`phase-01-foundation-pairing.md`, commits `a3c98e3`, `5904d66`, `06544ab`,
`b7f83f5`, `6e4f2b6`, `8f47bf8`, `fdfd484`, `e90cd0d`, `81de71c`, and
`ded1d878`, the prior browser exploration under
`.paseo-e2e/tasks-foundation-remediation`,
`test-artifacts/android-picker-acceptance/report.md`, and the retained Gate B
blocker report. The older tracked Phase 1 remediation report is retained as
historical blocked evidence; it predates the documented fallback and promotion
commits and is not treated as the final result.

### Anti-cheat audit

- **Camera boundary:** The final tree shows no CameraX route change after the
  fallback addition. `CameraSelector.DEFAULT_BACK_CAMERA`, CameraX Preview and
  `ImageAnalysis`, RGBA frame conversion, bundled ZXing decoding, and the
  `PairingQrParser`/`pair()` path remain the primary route. The filled
  **Scan pairing QR** action precedes the outlined accessible **Import pairing QR
  image** action. The promoted accessibility test and the acceptance
  `primary-camerax-*` UI/logcat/dumpsys evidence establish the visible ordering
  and a genuinely active CameraX client.
- **Fallback boundary:** Gate B is a real, fresh, 60-pose VirtualScene search.
  It records accepted poster bytes, active CameraX output, `frame=yes`, and
  `zbar=none` at every pose, with the renderer showing default geometry rather
  than the supplied poster. It explicitly says paired-only checks were not
  observed and grants no promotion. The acceptance run therefore used the
  visible secondary Android Photo Picker only after that blocker.
- **Exact image and real path:** The Android PASS report records the exact QR
  image rendered by the real parent UI, unchanged into shared Pictures, and
  selected through the real system Photo Picker/SAF UI. The UI XML shows the
  system picker package and privacy text. The image importer applies bounded
  bitmap decoding, calls the same bundled decoder, and its decoded text is sent
  to the same `PairingQrParser` and `pair()` function as CameraX. The generated
  device client uses `/api/device/pair`, `/api/device/profile`, and
  `/api/device/checklist`; the live acceptance reached the real API and rendered
  the bound student and empty checklist. No manual code, payload recreation,
  direct pair/API seeding, private repository, test seam, or fake success path
  was used. The committed debug frame hook is redacted metadata-only and is not
  an injection or pairing path; the Gate B report records source restoration.
- **Network/build assertion:** `AndroidManifest.xml` declares both CAMERA and
  INTERNET, and `assertRequiredManifestPermissions` is a pre-build assertion.
  The acceptance APK inventory confirms both permissions.
- **Retention and bounds:** `QrImageImporter` closes source streams, bounds
  source and decoded dimensions/pixels, clears the temporary pixel array, and
  recycles the bitmap. No URI, raw image, decoded payload, pairing code, or
  token is put in retained UI state or evidence. The acceptance cleanup and
  image-retention scans show no retained decodable QR image. Private storage,
  logcat, package/dumpsys, and backup checks contain no plaintext secret
  markers. `allowBackup=false` and `fullBackupContent=false` are present; the
  backup drill returned **Backup is not allowed**.
- **Credential custody and lifecycle:** The source uses an Android Keystore
  AES-GCM key for the bearer and DataStore only for encrypted bearer plus
  non-secret profile metadata. The real acceptance proved force-stop/relaunch
  and emulator-reboot persistence, same-image replay denial on a second fresh
  Pixel, parent archive/revoke rejection and clearing on the first device, and
  successful fresh replacement QR re-pair. It did not infer pairing from an
  unpaired screen or screenshots alone.
- **Browser boundary:** The independent real Chrome PASS exploration observed
  visible authorization-code PKCE with `code_challenge_method=S256`, fresh
  Parent A and Parent B contexts, and callback headers with
  `HttpOnly; SameSite=Lax`. It also observed two-tenant UI URL, request-body,
  and list-filter IDOR denials, real student CRUD/QR issuance, browser pairing
  and refresh persistence, replay denial, archive revocation, and empty
  JavaScript cookie/storage surfaces. No browser bearer token was exposed.
- **Contracts and clients:** Offline OpenAPI emission is deterministic; the
  generated TypeScript/Kotlin outputs are ignored build products, while the
  committed Kotlin file is the documented client façade. The web boundary scan
  reports no ad-hoc transport or browser storage use. The promoted Android and
  Playwright tests were added only after the corresponding exploratory PASS
  evidence: browser PASS was recorded before the Playwright promotion commits,
  and the Android picker PASS report predates `ded1d878`.
- **Stack and scope:** Stacklane labels, loopback-ephemeral publishing,
  Compose DNS, named caches/state, source mounts, hot reload, and two-instance
  isolation passed the product proof. No Phase 2 source, workflow, or claim was
  included.

## Exact checks run or inspected

| Check | Result |
|---|---|
| `git diff --check` | PASS |
| tracked generated scan under `primer-tasks` | PASS; only ignored generated roots and the committed façade remain |
| `cd primer-tasks && go test ./... -count=1` | PASS |
| `cd primer-tasks && go test -race ./... -count=1` | PASS |
| `cd primer-tasks && go vet ./... && go build ./cmd/...` | PASS |
| `make tasks-cover` | PASS — 85.5% >= 85% |
| `npm --prefix web run lint` | PASS |
| `npm --prefix web run typecheck` and `npm --prefix web run build` | PASS |
| `cd primer-tasks/android && ./gradlew --no-daemon testDebugUnitTest assembleDebug` | PASS |
| `cd primer-tasks/android && ./gradlew --no-daemon connectedDebugAndroidTest` with no device | expected environment failure: `No connected devices`; rerun on fresh regular `pixel` AVD, Android 15, software GPU, passed both tests |
| `go run ./cmd/openapi-gen` twice plus `cmp` | PASS; deterministic SHA-256 `5b08551dead66d209206adcebd37b29575574f147d3ab6e50c8df8ad3aaa2e89` |
| TypeScript and Kotlin client generation | PASS; outputs remained ignored/untracked |
| `STACKLANE_INSTANCE=... ./primer-tasks/scripts/dev check` | PASS |
| `make tasks-proof` | PASS — labels/isolation, Go reload, Vite HMR without navigation, source restore, cleanup |
| promoted Playwright, direct loopback fallback configuration, 1 worker | PASS — `1 passed` |
| documented Android picker acceptance | PASS — real system UI and real API flow, report and artifacts retained |
| retained Gate B VirtualScene evidence | BLOCKED as required; upstream limitation remains visible |

The ordinary default Stacklane-FQDN Playwright attempt was also exercised and
failed at `chrome-error://chromewebdata/` because that FQDN was unreachable
from the browser. This was not used as acceptance evidence. The documented
same-origin `/issuer` direct-loopback configuration used by the exploratory
run was then applied without changing product source, and the promoted test
passed. The promotion gate remained enabled (`PRIMER_TASKS_EXPLORATORY_BROWSER_PASS=1`)
and was not bypassed.

## Final working-tree decision

Before adding this report there were no intended tracked product changes
pending; build/codegen artifacts and existing retained evidence are ignored or
untracked. No product source was edited during this review. This report is the
only intended new tracked review artifact. Existing evidence was not deleted.
