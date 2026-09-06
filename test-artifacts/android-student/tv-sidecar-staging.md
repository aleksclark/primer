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

## Follow-up checkpoint: `faeac75f`

Integrated A's `0ceb1276`. Parent reran the sidecar package with `go test -json
-count=1`: all eight tests passed, including
`TestStageCLIProducesSignedSidecarFromInspectableAPK`; **that test was not
skipped in this local run**. It exercises the production `run` workflow and real
Android inspection tools, rather than spawning the compiled CLI. TV
`TestAppRelease` also passed.

The publisher now inspects/signs its private snapshot, refuses destinations
already present at preflight, keeps universal ABIs empty, rejects unsupported
signer/major-version metadata, and validates the private key's public half.

Operator-safety acceptance still remains open for these precise findings:

- Plain `os.Rename` is not a kernel no-replace operation. Its preflight check
  does not prevent replacement of an empty destination created just before the
  rename syscall.
- Opening a FIFO before checking its file type can block indefinitely.
- The real-APK test still skips when prerequisites are missing. A separately
  required acceptance gate must build/require those prerequisites and fail if
  they are absent.
- The symlink guidance must be implementation-specific or use an explicit
  temporary symlink plus same-directory rename. A temporary-directory `strace`
  probe confirmed that **GNU coreutils 9.11 on this host** implements `ln -sfn`
  using a temporary symlink and `renameat`; that is not a portability guarantee.
  Ancestor symlinks also need resolution when snapshotting the release path.

Follow-up was sent to the implementation lane. No live paths or physical
Android devices were touched by these checks.

## Finalization and executable gate: `3089eb98` plus parent test correction

A's `5adba778` was integrated as `3089eb98`. Linux finalization now uses
`renameat2(RENAME_NOREPLACE)` without an unsafe fallback. APK snapshot opening
uses `O_NOFOLLOW | O_NONBLOCK` before checking the descriptor's file type.
Whole-path symlink resolution and explicit temporary-link/rename publication
instructions address the path issues above.

Parent corrected the required acceptance test to **build and execute** the CLI
inside `t.TempDir()`. Merely building a binary and then calling `run(...)` did
not exercise the executable boundary. The test now passes an ephemeral key via
the subprocess environment, checks real signed APK output, and observes exit
code 2 from a second invocation against the existing destination. No shared
`/tmp` executable is used. The rename regression covers both empty and populated
existing destinations and verifies their inode is preserved.

Passed:

```bash
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  make tv-release-sidecar-acceptance
go test ./server/internal/tv/api -run TestAppRelease -count=1
```

The unit run includes the FIFO deadline and no-replace regressions. The second,
explicit acceptance run invokes the real binary and does not skip. A separate
negative invocation with `TV_RELEASE_SIDECAR_ACCEPTANCE=1` and an explicit
nonexistent `TV_RELEASE_SIDECAR_APK` **failed with exit 1, not a skip**, as required.

These checks close the reported filesystem and executable-test gaps on this
Linux host. They do not establish live operator publication, TV playback,
real APK replacement, production signing custody, or A16 silent updates.
