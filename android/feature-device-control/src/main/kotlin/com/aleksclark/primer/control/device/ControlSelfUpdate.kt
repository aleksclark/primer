package com.aleksclark.primer.control.device

import com.aleksclark.primer.updates.SelfUpdateEligibility

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
)

object ControlSelfUpdate {
    const val CONTROL_PACKAGE = "com.aleksclark.primer.control"

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
    ): ControlSelfUpdatePhase = when {
        failed -> ControlSelfUpdatePhase.Failed
        pendingConfirmation && sessionExists -> ControlSelfUpdatePhase.WaitingConfirmation
        pendingConfirmation && !sessionExists -> ControlSelfUpdatePhase.Failed
        plan == null -> ControlSelfUpdatePhase.Idle
        plan.settingsRequired -> ControlSelfUpdatePhase.NeedsSettings
        plan.unattendedEligible && !plan.confirmationRequired -> ControlSelfUpdatePhase.EligibleUnattended
        plan.confirmationRequired -> ControlSelfUpdatePhase.EligibleConfirm
        else -> ControlSelfUpdatePhase.Failed
    }
}
