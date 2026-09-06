package com.aleksclark.primer.control.device

/**
 * Control presentation over the shared updater adapter.
 *
 * Does not import :core-updates (minSdk 28 vs Control 26). Eligibility comes from
 * the upcoming SelfUpdateSession/SelfUpdatePolicy API as [AdapterEligibility].
 * Production PackageInstaller wiring stays held until A's session compares actual
 * APK SHA-256 and device ABIs. Filename is not proof. No duplicate validators.
 */
data class AdapterEligibility(
    val unattendedEligible: Boolean,
    val userActionRequired: Boolean,
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

data class ControlSelfUpdatePlan(
    val unattendedEligible: Boolean,
    val confirmationRequired: Boolean,
    val settingsRequired: Boolean,
    val notificationFallback: Boolean,
    val reason: String,
)

data class ControlSelfUpdateUi(
    val phase: ControlSelfUpdatePhase = ControlSelfUpdatePhase.Held,
    val installedVersion: Long = 0,
    val candidateVersion: Long? = null,
    val status: String = "Control self-update is held until the shared adapter validates APK hash, signer, ABI, and version.",
    val plan: ControlSelfUpdatePlan? = null,
)

object ControlSelfUpdate {
    const val CONTROL_PACKAGE = "com.aleksclark.primer.control"

    fun present(
        eligibility: AdapterEligibility,
        unknownSourcesAllowed: Boolean,
        productionTrusted: Boolean = false,
    ): ControlSelfUpdatePlan {
        val confirm = eligibility.userActionRequired
        val settings = confirm && !unknownSourcesAllowed
        return ControlSelfUpdatePlan(
            unattendedEligible = eligibility.unattendedEligible,
            confirmationRequired = confirm,
            settingsRequired = settings,
            notificationFallback = confirm,
            reason = eligibility.reason,
        )
    }

    fun phase(plan: ControlSelfUpdatePlan?, productionTrusted: Boolean): ControlSelfUpdatePhase = when {
        !productionTrusted -> ControlSelfUpdatePhase.Held
        plan == null -> ControlSelfUpdatePhase.Idle
        plan.settingsRequired -> ControlSelfUpdatePhase.NeedsSettings
        plan.unattendedEligible -> ControlSelfUpdatePhase.EligibleUnattended
        plan.confirmationRequired -> ControlSelfUpdatePhase.EligibleConfirm
        else -> ControlSelfUpdatePhase.Failed
    }

    fun pendingConfirmationStillLive(pendingConfirmation: Boolean, sessionExists: Boolean): Boolean =
        pendingConfirmation && sessionExists
}
