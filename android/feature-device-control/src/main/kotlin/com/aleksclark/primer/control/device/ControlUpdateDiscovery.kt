package com.aleksclark.primer.control.device

/**
 * Parent-owned Control catalog discovery. Never a silent-install guarantee and
 * never a verifier bypass. WorkManager's 15-minute floor is the minimum period.
 */
data class ControlUpdateDiscoverySettings(
    val checkOnResume: Boolean = true,
    val periodicEnabled: Boolean = false,
    val unattendedCatchUp: Boolean = false,
    val lastCatalogCheckAtMs: Long = 0,
)

enum class ControlDiscoveryAction {
    None,
    RefreshCatalog,
    RequestUnattendedInstall,
}

object ControlUpdateDiscovery {
    const val MIN_PERIOD_MS = 15L * 60L * 1000L

    fun shouldRefreshCatalog(settings: ControlUpdateDiscoverySettings, nowMs: Long): Boolean {
        if (nowMs < 0) return false
        val elapsed = nowMs - settings.lastCatalogCheckAtMs
        if (settings.lastCatalogCheckAtMs <= 0) {
            return settings.checkOnResume || settings.periodicEnabled
        }
        if (elapsed < MIN_PERIOD_MS) return false
        return settings.checkOnResume || settings.periodicEnabled
    }

    fun shouldRequestUnattendedInstall(
        settings: ControlUpdateDiscoverySettings,
        phase: ControlSelfUpdatePhase,
        canInstall: Boolean,
        pendingConfirmation: Boolean,
    ): Boolean {
        if (!settings.unattendedCatchUp) return false
        if (pendingConfirmation) return false
        if (!canInstall) return false
        return phase == ControlSelfUpdatePhase.EligibleUnattended
    }

    fun action(
        settings: ControlUpdateDiscoverySettings,
        nowMs: Long,
        phase: ControlSelfUpdatePhase,
        canInstall: Boolean,
        pendingConfirmation: Boolean,
    ): ControlDiscoveryAction = when {
        shouldRequestUnattendedInstall(settings, phase, canInstall, pendingConfirmation) ->
            ControlDiscoveryAction.RequestUnattendedInstall
        shouldRefreshCatalog(settings, nowMs) -> ControlDiscoveryAction.RefreshCatalog
        else -> ControlDiscoveryAction.None
    }
}
