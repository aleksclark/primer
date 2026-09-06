package com.aleksclark.primer.control.device

/**
 * Control-facing self-update plan. This is not A's [com.aleksclark.primer.updates.SelfUpdateSession].
 *
 * Production PackageInstaller wiring stays held: the shared candidate currently trusts
 * caller eligibility and can wedge on pendingConfirmation with a missing session.
 * Do not treat a filename, download path, or caller-supplied eligibility as verification.
 * Archive identity and SHA-256 must come from inspecting the actual APK bytes.
 */
data class ControlSelfUpdatePlan(
    val canAttempt: Boolean,
    val unattendedEligible: Boolean,
    val confirmationRequired: Boolean,
    val settingsRequired: Boolean,
    val notificationFallback: Boolean,
    val verifiedBytes: Boolean,
    val reason: String,
)

enum class ControlSelfUpdatePhase {
    Idle,
    Held,
    EligibleUnattended,
    EligibleConfirm,
    NeedsSettings,
    WaitingConfirmation,
    Failed,
}

data class ControlSelfUpdateUi(
    val phase: ControlSelfUpdatePhase = ControlSelfUpdatePhase.Held,
    val installedVersion: Long = 0,
    val candidateVersion: Long? = null,
    val status: String = "Control self-update is held until the shared adapter validates APK hash, signer, and version.",
    val plan: ControlSelfUpdatePlan? = null,
)

object ControlSelfUpdate {
    const val CONTROL_PACKAGE = "com.aleksclark.primer.control"

    fun plan(
        runningPackage: String,
        installedPackage: String,
        installedVersion: Long,
        installedSigners: Set<String>,
        archivePackage: String,
        archiveVersion: Long,
        archiveSigners: Set<String>,
        expectedSha256: String,
        actualSha256: String,
        sdk: Int,
        canRequestUnattended: Boolean,
        unknownSourcesAllowed: Boolean,
    ): ControlSelfUpdatePlan {
        val verifiedBytes = expectedSha256.matches(SHA) &&
            actualSha256.matches(SHA) &&
            expectedSha256.equals(actualSha256, ignoreCase = true)
        val sameRunning = runningPackage == CONTROL_PACKAGE && archivePackage == runningPackage && installedPackage == runningPackage
        val sameSigner = installedSigners.isNotEmpty() && archiveSigners == installedSigners
        val newer = archiveVersion > installedVersion
        val canAttempt = verifiedBytes && sameRunning && sameSigner && newer
        val unattended = canAttempt && canRequestUnattended && sdk >= 31
        val settings = canAttempt && !unknownSourcesAllowed && !unattended
        val confirm = canAttempt && !unattended
        val reason = when {
            !verifiedBytes -> "APK bytes were not verified against the published SHA-256. Filename is not proof."
            !sameRunning -> "Self-update can only replace the running Control package."
            !sameSigner -> "Self-update signing identity differs."
            !newer -> "Self-update must have a newer version code."
            unattended -> "Android may replace this package without a prompt."
            settings -> "Install permission is off. Open system settings, then confirm the update."
            else -> "Android requires a system install confirmation."
        }
        return ControlSelfUpdatePlan(
            canAttempt = canAttempt,
            unattendedEligible = unattended,
            confirmationRequired = confirm,
            settingsRequired = settings,
            notificationFallback = confirm,
            verifiedBytes = verifiedBytes,
            reason = reason,
        )
    }

    fun phase(plan: ControlSelfUpdatePlan?, productionTrusted: Boolean): ControlSelfUpdatePhase = when {
        !productionTrusted -> ControlSelfUpdatePhase.Held
        plan == null -> ControlSelfUpdatePhase.Idle
        !plan.canAttempt -> ControlSelfUpdatePhase.Failed
        plan.settingsRequired -> ControlSelfUpdatePhase.NeedsSettings
        plan.unattendedEligible -> ControlSelfUpdatePhase.EligibleUnattended
        plan.confirmationRequired -> ControlSelfUpdatePhase.EligibleConfirm
        else -> ControlSelfUpdatePhase.Failed
    }

    /** pendingConfirmation with no live installer session must fail closed, not wedge. */
    fun pendingConfirmationStillLive(pendingConfirmation: Boolean, sessionExists: Boolean): Boolean =
        pendingConfirmation && sessionExists

    private val SHA = Regex("[0-9a-fA-F]{64}")
}
