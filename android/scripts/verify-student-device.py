#!/usr/bin/env python3
"""Read-only A16 qualification probe. NOT a substitute for the manual recovery/update tests.

Run only after public enrollment/setup and with no maintenance session open.
No credentials, application-state injection, policy changes or APK installs.
"""
import argparse
from datetime import datetime, timezone
import json
import re
import subprocess
import sys

PACKAGE = "com.aleksclark.primer.student"
ADMIN = PACKAGE + "/.admin.PrimerDeviceAdminReceiver"


def check(condition, message):
    if not condition:
        raise RuntimeError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--adb", default="adb")
    parser.add_argument("--serial", required=True, help="Explicit authorized USB device; never auto-select")
    parser.add_argument("--version", required=True, type=int)
    parser.add_argument("--approved-package", action="append", default=[])
    args = parser.parse_args()

    def adb(*command):
        return subprocess.check_output(
            [args.adb, "-s", args.serial, *command], text=True, stderr=subprocess.STDOUT, timeout=25,
        )

    owners = adb("shell", "dpm", "list-owners")
    check(any(ADMIN in line and "DeviceOwner" in line for line in owners.splitlines()),
          "Student is not the device owner")
    package = adb("shell", "dumpsys", "package", PACKAGE)
    check(re.search(r"\bversionCode=" + str(args.version) + r"\s", package), "Installed version differs")
    # A debug/testOnly APK cannot satisfy signed-release qualification.
    flag_lines = "\n".join(line for line in package.splitlines() if "flags=[" in line or "pkgFlags=[" in line)
    check("DEBUGGABLE" not in flag_lines and "TEST_ONLY" not in flag_lines, "Debug/testOnly APK is not release evidence")

    home = adb("shell", "cmd", "package", "resolve-activity", "--brief", "-a",
               "android.intent.action.MAIN", "-c", "android.intent.category.HOME")
    check(PACKAGE + "/.StudentHome" in home or PACKAGE + "/" + PACKAGE + ".StudentHome" in home,
          "Student is not the resolved home activity")
    activities = adb("shell", "dumpsys", "activity", "activities")
    check("mLockTaskModeState=LOCKED" in activities, "Device is not in managed lock task")
    marker = "mLockTaskPackages (userId:packages)="
    check(marker in activities, "Cannot read OS lock-task allowlist")
    match = re.search(r"u0:\[([^\]]*)\]", activities.split(marker, 1)[1].split("\n\n", 1)[0])
    check(match is not None, "Cannot parse user-0 lock-task allowlist")
    actual = {x.strip() for x in match.group(1).split(",") if x.strip()}
    expected = {PACKAGE, *args.approved_package}
    check(actual == expected, "Allowlist differs (maintenance may still be open): " + ", ".join(sorted(actual)))

    # Retain only relevant metadata, never the serial or unrelated device/app inventory.
    print(json.dumps({
        "result": "PASS",
        "observedAt": datetime.now(timezone.utc).isoformat(),
        "scope": "owner, installed version/release flags, persistent home, actual managed lock-task state and exact allowlist",
        "model": adb("shell", "getprop", "ro.product.model").strip(),
        "android": adb("shell", "getprop", "ro.build.version.release").strip(),
        "build": adb("shell", "getprop", "ro.build.display.id").strip(),
        "package": PACKAGE,
        "versionCode": args.version,
        "approvedPackages": sorted(actual),
        "notAsserted": ["silent update", "recovery", "all escape paths", "setup-time QR provisioning"],
    }, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, subprocess.SubprocessError) as error:
        print("FAIL: " + str(error), file=sys.stderr)
        sys.exit(1)
