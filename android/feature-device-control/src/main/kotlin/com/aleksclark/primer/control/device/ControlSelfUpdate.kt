package com.aleksclark.primer.control.device

import com.aleksclark.primer.updates.SelfUpdateEligibility
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.Release

/**
 * Control presentation over [com.aleksclark.primer.updates.SelfUpdateSession].
 * Security (hash, signer, ABI, version) stays in the shared adapter.
 * Missing trust root or unsigned release metadata fails closed. No productionTrusted flag.
 */
enum class ControlSelfUpdatePhase {
    Idle,
    EligibleUnattended,
    EligibleConfirm,
    NeedsSettings,
    WaitingConfirmation,
    Deferred,
    Failed,
}

data class ControlSelfUpdatePlan(
    val unattendedEligible: Boolean,
    val confirmationRequired: Boolean,
    val settingsRequired: Boolean,
    val notificationFallback: Boolean,
    val mustHandlePendingUserAction: Boolean,
    val reason: String,
)

data class ControlSelfUpdateUi(
    val phase: ControlSelfUpdatePhase = ControlSelfUpdatePhase.Idle,
    val installedVersion: Long = 0,
    val candidateVersion: Long? = null,
    val status: String = "No Control self-update attempted.",
    val plan: ControlSelfUpdatePlan? = null,
    val canInstall: Boolean = false,
    val canOpenSettings: Boolean = false,
    val canContinueConfirmation: Boolean = false,
    val presentation: String? = null,
    val discovery: ControlUpdateDiscoverySettings = ControlUpdateDiscoverySettings(),
)

object ControlSelfUpdate {
    const val CONTROL_PACKAGE = "com.aleksclark.primer.control"
    const val CONTROL_CHANNEL = "stable"

    fun present(
        eligibility: SelfUpdateEligibility,
        unknownSourcesAllowed: Boolean,
    ): ControlSelfUpdatePlan {
        val confirm = eligibility.userActionRequired
        return ControlSelfUpdatePlan(
            unattendedEligible = eligibility.unattendedEligible,
            confirmationRequired = confirm,
            settingsRequired = confirm && !unknownSourcesAllowed,
            notificationFallback = eligibility.mustHandlePendingUserAction,
            mustHandlePendingUserAction = eligibility.mustHandlePendingUserAction,
            reason = eligibility.reason,
        )
    }

    fun phase(
        plan: ControlSelfUpdatePlan?,
        pendingConfirmation: Boolean,
        sessionExists: Boolean,
        failed: Boolean,
        deferred: Boolean = false,
    ): ControlSelfUpdatePhase = when {
        failed -> ControlSelfUpdatePhase.Failed
        pendingConfirmation && !sessionExists -> ControlSelfUpdatePhase.Failed
        pendingConfirmation && deferred -> ControlSelfUpdatePhase.Deferred
        pendingConfirmation && sessionExists -> ControlSelfUpdatePhase.WaitingConfirmation
        plan == null -> ControlSelfUpdatePhase.Idle
        plan.settingsRequired -> ControlSelfUpdatePhase.NeedsSettings
        plan.unattendedEligible && !plan.confirmationRequired -> ControlSelfUpdatePhase.EligibleUnattended
        plan.confirmationRequired -> ControlSelfUpdatePhase.EligibleConfirm
        else -> ControlSelfUpdatePhase.Failed
    }

    fun selectCandidate(
        releases: List<Release>,
        installedVersion: Long,
        packageName: String = CONTROL_PACKAGE,
        channel: String = CONTROL_CHANNEL,
    ): Release? = releases
        .filter { it.packageName == packageName && it.channel == channel && it.versionCode > installedVersion }
        .maxByOrNull { it.versionCode }

    fun isNewer(candidateVersion: Long, installedVersion: Long): Boolean = candidateVersion > installedVersion

    fun requireMatchesOuter(decoded: SignedManifest, release: Release) {
        check(decoded.packageName == CONTROL_PACKAGE) { "Self-update can only replace the running Control package." }
        check(decoded.channel == CONTROL_CHANNEL) { "Control self-update requires the stable channel." }
        check(decoded.packageName == release.packageName) { "Signed manifest package does not match the published release." }
        check(decoded.channel == release.channel) { "Signed manifest channel does not match the published release." }
        check(decoded.versionCode == release.versionCode) { "Signed manifest version does not match the published release." }
        check(decoded.signerSha256.equals(release.signerSha256, ignoreCase = true)) {
            "Signed manifest signer does not match the published release."
        }
        check(decoded.byteSize == release.byteSize) { "Signed manifest size does not match the published release." }
        check(decoded.sha256.equals(release.sha256, ignoreCase = true)) {
            "Signed manifest hash does not match the published release."
        }
    }
}
