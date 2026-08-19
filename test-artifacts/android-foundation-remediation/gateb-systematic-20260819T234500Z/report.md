# Android Gate B systematic virtualscene search — BLOCKED

## Scope and result

A fresh, real Parent A browser session on the clean `primer-tasks-android-gateb-systematic` stack created **Gate B Live Search Student** and visibly issued a one-use QR. The exact rendered QR `<img>` element was written only to `/dev/shm/android-gateb-qr.png`, passed unchanged as the `image` field to **both** virtualscene posters, and was erased after the run. No QR code, payload, token, browser cookie, or raw CameraX image is retained in this directory.

The one wiped Pixel used for the final run was launched with the requested command-line flags and left running in the ordinary scanner at the terminal pose. CameraX has an active client/output streams. No actual analyzer frame across the complete prescribed 60-pose grid decoded with `zbarimg`; the app never paired. Therefore downstream paired-only checks (bound student/empty list, process/reboot persistence, replay denial, archive/revoke re-pair, storage/logcat/backup) are **NOT OBSERVED**, not passed.

**Promotion authorization: NOT GRANTED.** This leaf does not authorize promotion leaves.

## Required gRPC reflection and poster evidence

`grpcurl -plaintext localhost:8554 describe` established:

- `android.emulation.control.incubating.Poster` fields are `name`, `image`, `file_name`, `minWidth`, `maxWidth`, and `scale`.
- `VirtualSceneService.setPoster(Poster) returns Poster`.

Reflection reported valid maximum widths of 2 for `wall` and 1 for `table`. The final exact-image calls set `wall.scale=2` and `table.scale=1`; `final-posters.sanitized.json` reports those values, with image bytes omitted. The live final emulator command line in `emulator-final-commandline.txt` includes `-avd pixel -wipe-data -no-snapshot -no-boot-anim -gpu software -camera-back virtualscene -grpc 8554 -port 5556` (the QEMU child faithfully carries these invocation arguments).

## Deterministic camera search

Known baseline: boot orientation, then a relative `x=-1.0471975512` rotation established the first `pitch=-60,yaw=0` position. `pose-sweep.csv` records every relative command from that known state: pitch bands `-60,-30,0,+30,+60`, yaw `0,30,...,330`. At every pose, the temporary debug-only CameraX hook was armed, an actual `640x480` analyzer PNG was pulled to `/dev/shm`, `zbarimg -q` ran, and the raw PNG plus hook metadata were removed on the immediate no-match path. All 60 rows report `frame=yes,zbar=none`.

A human visual inspection was made of the first real analyzer PNG and the transient 12×5 contact sheet constructed from all 60 actual frame thumbnails. Both were deleted before artifacts were written. The sheet showed floors, furniture, walls, and the renderer's checkerboard/default panel at several poses; it showed no dense QR image. The first-frame visual evidence is summarized safely in `baseline-first-frame.sanitized.txt` (dimensions only; decoder output absent).

## Concrete renderer blocker

Although reflection accepts the QR image and reports both poster names/scales, analyzer frames that reach the table/wall resources render the virtualscene's checkerboard/default panel rather than the submitted rendered QR (notably the `pitch=0` and positive-pitch samples in the transient reviewed sheet). Other poses show floor/furniture/walls only. Thus the current virtualscene renderer does not present the `setPoster.image` content as a camera-visible QR surface for this geometry/orientation grid. This is a renderer/input-propagation blocker, not a CameraX or in-app decoder failure: CameraX emitted all 60 PNGs, `zbarimg` saw no QR in any, and there is no actual frame decode that could justify decoder changes.

## Cleanup and unchanged behavior

- Pre-existing Android Gate-B Compose resources and the stale port-5556 forwarder were scoped-cleaned first (`pre-clean.txt`, `post-clean.txt`).
- The raw capture code was a temporary DEBUG-only observation hook. It never decoded or changed pairing. Source was restored afterward; `source-restoration-and-status.txt` proves no diff for `QrFrameCapture.kt`.
- `raw-cleanup-check.txt` confirms no `gateb-*` raw file in `/dev/shm` and no capture PNG/metadata/arm marker in app-private storage.
- After inspection, the original unmodified source was rebuilt and reinstalled over the temporary DEBUG capture APK. The current emulator/app remains scanning at `pitch=+60,yaw=330`; `final-unchanged-app-scanning.ui.xml` and `final-unchanged-app-camera.relevant.txt` show the normal scanner and live CameraX client.
