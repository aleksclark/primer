package com.aleksclark.primer.updates

data class SelfUpdateEligibility(
    val samePackage: Boolean,
    val sameSigner: Boolean,
    val newerVersion: Boolean,
    val unattendedEligible: Boolean,
    val userActionRequired: Boolean,
    val reason: String,
) {
    val canAttempt: Boolean get() = samePackage && sameSigner && newerVersion
}

object SelfUpdatePolicy {
    const val CONTROL_PACKAGE = "com.aleksclark.primer.control"
    const val TV_PACKAGE = "com.aleksclark.primer.tv"
    const val STUDENT_PACKAGE = "com.aleksclark.primer.student"

    fun decide(
        installed: ArchiveIdentity,
        archive: ArchiveIdentity,
        sdk: Int,
        canRequestUnattended: Boolean,
        unknownSourcesAllowed: Boolean,
        deviceOwner: Boolean,
    ): SelfUpdateEligibility {
        val samePackage = archive.packageName == installed.packageName
        val sameSigner = installed.signers.isNotEmpty() && archive.signers == installed.signers
        val newer = archive.version > installed.version
        val unattended = deviceOwner || (canRequestUnattended && sdk >= 31 && samePackage && sameSigner && newer)
        val reason = when {
            !samePackage -> "Self-update can only replace the running package"
            !sameSigner -> "Self-update signing identity differs"
            !newer -> "Self-update must have a newer version code"
            unattended -> "Android may replace this package without a prompt"
            !unknownSourcesAllowed && !deviceOwner -> "Unknown-source installs are not permitted; system confirmation is required"
            else -> "Android requires a system install confirmation"
        }
        return SelfUpdateEligibility(
            samePackage = samePackage,
            sameSigner = sameSigner,
            newerVersion = newer,
            unattendedEligible = unattended,
            userActionRequired = !unattended,
            reason = reason,
        )
    }
}
