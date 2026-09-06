# TV sidecar publisher — isolated staging check

## Scope and result

At integrated `e3ad2e3d` (A's `335514e9`), the new Go command successfully
processed an actual TV debug APK in a fresh private temporary directory. This
was a filesystem/signature check, **not live publication, installation, playback,
or operator-safety acceptance**.

- Input: a copy of `android/app/build/outputs/apk/debug/app-debug.apk`.
- Package: `com.aleksclark.primer.tv`; versionCode: `1`.
- APK byte size: `71588984`.
- APK SHA-256: `c6fd932e1a07f51773f69b5e407246d36873838892f26442e7b762bf4c7e827c`.
- Tools: Android build-tools 35.0.0 `aapt2` and `apksigner`, Java 17.
- Signing input: an ephemeral Ed25519 key generated for this check, passed only
  to the subprocess environment. No production signing custody was used and the
  ephemeral key was not retained.
- Output: a fresh sibling `staging` directory containing the APK, `version`, and
  `release-manifest.json`. No running server's release directory was touched.

## Checked

The command was built with:

```bash
go build -o /tmp/primer-tv-release-sidecar-qa ./server/cmd/tv-release-sidecar
```

A Python driver copied the real APK to a fresh private directory, generated the
key, invoked the command with `-apk <copy> -out <new-directory>`, and then:

1. Verified the Go-produced Ed25519 signature over the exact decoded payload
   using Python `cryptography`, not the command's own signing helper.
2. Checked the expected TV package and stable channel.
3. Compared the signed SHA-256 and byte size with the actual staged APK bytes.
4. Confirmed the input copy's SHA-256 remained unchanged on this positive path.
5. Checked that the `version` file equals the signed versionCode, and that
   duplicated sidecar fields match the signed payload.

The following also passed:

```bash
go test ./server/internal/tv/api -run TestAppRelease -count=1
go test ./server/cmd/tv-release-sidecar -count=1
```

## Review findings still open at this checkpoint

The positive path does not establish safe behavior for mutable inputs or
existing destinations. Follow-up was requested for:

- Existing output files can be truncated; source equal to destination can destroy
  the input. Require no-clobber staging destinations.
- Inspection occurs before a second read/copy of the source. Use a bounded,
  owned snapshot for inspection, signing, and staged output.
- The documented two-`mv` sequence is not atomic and can cross filesystems. Use
  immutable release directories and an atomic same-filesystem pointer switch;
  resolve the pointer once for a coherent metadata read.
- Accurate universal-APK ABI handling, signer/long-version parsing, and private
  key consistency validation need additional negative cases.

These findings are not waived by successful helper tests or this real-APK
positive-path check. No TV update, Play Protect bypass, or physical A16 change
was performed.
