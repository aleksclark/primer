package com.aleksclark.primer.updates

data class SelfUpdateEligibility(
    val unattendedEligible: Boolean,
    val userActionRequired: Boolean,
    val reason: String,
)

object SelfUpdatePolicy {
    const val CONTROL_PACKAGE = "com.aleksclark.primer.control"
    const val TV_PACKAGE = "com.aleksclark.primer.tv"

    fun unattendedFloor(candidateTargetSdk: Int): Int = when {
        candidateTargetSdk >= 36 -> 34
        candidateTargetSdk >= 35 -> 33
        candidateTargetSdk >= 34 -> 31
        candidateTargetSdk >= 33 -> 30
        candidateTargetSdk >= 31 -> 29
        else -> Int.MAX_VALUE
    }

    fun decide(
        runningPackage: String,
        installed: ArchiveIdentity,
        archive: ArchiveIdentity,
        expected: SignedManifest,
        sdk: Int,
        deviceAbis: Set<String>,
        candidateTargetSdk: Int,
        canUpdateWithoutUserAction: Boolean,
        unknownSourcesAllowed: Boolean,
    ): SelfUpdateEligibility {
        ArchiveChecks.validateArchive(
            installed = installed,
            archive = archive,
            sdk = sdk,
            abis = deviceAbis,
            expectedPackage = runningPackage,
            expectedSigners = installed.signers,
            allowFirstInstall = false,
        )
        check(expected.packageName == runningPackage) { "Self-update can only replace the running package" }
        check(archive.packageName == runningPackage) { "Self-update can only replace the running package" }
        check(archive.packageName == expected.packageName) { "APK belongs to another application" }
        check(archive.version == expected.versionCode) { "APK version differs from target" }
        check(archive.signers == setOf(expected.signerSha256)) { "APK signing identity differs" }
        check(archive.minSdk == expected.minSdk) { "APK minSdk differs from target" }
        if (expected.supportedAbis.isNotEmpty()) {
            check(archive.abis.isEmpty() || archive.abis == expected.supportedAbis.toSet() || archive.abis.containsAll(expected.supportedAbis)) {
                "APK native ABI differs from target"
            }
        }
        val floor = unattendedFloor(candidateTargetSdk)
        val unattended = unknownSourcesAllowed && canUpdateWithoutUserAction && sdk >= floor && floor != Int.MAX_VALUE
        val reason = when {
            unattended -> "Android may replace this package without a prompt"
            !unknownSourcesAllowed -> "Unknown-source installs are not permitted; system confirmation is required"
            else -> "Android requires a system install confirmation"
        }
        return SelfUpdateEligibility(
            unattendedEligible = unattended,
            userActionRequired = true,
            reason = reason,
        )
    }
}
