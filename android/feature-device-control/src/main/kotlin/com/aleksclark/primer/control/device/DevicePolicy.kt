package com.aleksclark.primer.control.device

import com.aleksclark.primertasks.client.ApprovedApp
import com.aleksclark.primertasks.client.LockTaskPolicy
import com.aleksclark.primertasks.client.MaintenancePolicy
import com.aleksclark.primertasks.client.Policy
import com.aleksclark.primertasks.client.PolicyReport
import com.aleksclark.primertasks.client.PolicyUpdateInput
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.ReleaseTarget

enum class DeviceSyncStatus { NoPolicy, Unknown, Pending, Partial, Stale, Failed, Applied }

object DeviceSync {
    fun status(desiredRevision: Long, appliedRevision: Long, report: PolicyReport?): DeviceSyncStatus {
        if (desiredRevision <= 0L && appliedRevision <= 0L && report == null) return DeviceSyncStatus.NoPolicy
        if (report == null) {
            return if (appliedRevision < desiredRevision) DeviceSyncStatus.Pending else DeviceSyncStatus.Unknown
        }
        if (report.stale) return DeviceSyncStatus.Stale
        val reportStatus = report.status.lowercase()
        if (reportStatus == "partial") return DeviceSyncStatus.Partial
        if (reportStatus == "failed" || reportStatus == "error") return DeviceSyncStatus.Failed
        if (appliedRevision < desiredRevision) return DeviceSyncStatus.Pending
        val appliedReport = reportStatus == "applied" || reportStatus == "ok" || reportStatus == "success"
        val sameRevision = desiredRevision > 0L && appliedRevision == desiredRevision && report.policyRevision == desiredRevision
        return if (appliedReport && sameRevision) DeviceSyncStatus.Applied else DeviceSyncStatus.Unknown
    }

    fun label(status: DeviceSyncStatus): String = when (status) {
        DeviceSyncStatus.NoPolicy -> "no policy"
        DeviceSyncStatus.Unknown -> "unknown"
        DeviceSyncStatus.Pending -> "pending"
        DeviceSyncStatus.Partial -> "partial"
        DeviceSyncStatus.Stale -> "stale"
        DeviceSyncStatus.Failed -> "failed"
        DeviceSyncStatus.Applied -> "applied"
    }
}

data class ApprovedAppDraft(
    val packageName: String = "",
    val signerSha256: String = "",
    val label: String = "",
    val required: Boolean = false,
)

fun ApprovedAppDraft.toApprovedApp(): ApprovedApp {
    val pkg = packageName.trim()
    val signer = signerSha256.trim().lowercase()
    require(pkg.isNotEmpty()) { "Enter an Android package name." }
    require(signer.matches(Regex("^[0-9a-f]{64}$"))) { "Enter the 64-character SHA-256 signer digest." }
    return ApprovedApp(
        packageName = pkg,
        signerSha256 = signer,
        label = label.trim().ifBlank { null },
        required = required,
    )
}

fun Policy.withApprovedApps(apps: List<ApprovedApp>): Policy = copy(approvedApps = apps)

fun Policy.withMaintenance(allowParentUnlock: Boolean): Policy =
    copy(maintenance = MaintenancePolicy(allowParentUnlock = allowParentUnlock))

fun policyUpdate(baseRevision: Long, policy: Policy) =
    PolicyUpdateInput(baseRevision = baseRevision, policy = policy)

object RecoveryHistoryView {
    fun label(intent: com.aleksclark.primertasks.client.RecoveryIntent): String = when (intent.kind) {
        "maintenance_lease" -> "Maintenance lease"
        "rotate_recovery_code" -> "Recovery rotation"
        else -> intent.kind
    }

    fun status(intent: com.aleksclark.primertasks.client.RecoveryIntent): String = intent.status
}

object MaintenanceLease {
    const val DEFAULT_MINUTES = 15L
    const val MIN_MINUTES = 1L
    const val MAX_MINUTES = 30L

    fun boundedMinutes(raw: Long): Long {
        require(raw in MIN_MINUTES..MAX_MINUTES) { "Maintenance lease must be 1–30 minutes." }
        return raw
    }

    fun parseMinutes(text: String): Long {
        val value = text.trim().toLongOrNull() ?: error("Enter lease minutes between 1 and 30.")
        return boundedMinutes(value)
    }
}

object ReleaseCas {
    fun matching(targets: List<ReleaseTarget>, release: Release): ReleaseTarget? =
        targets.firstOrNull { it.packageName == release.packageName && it.channel == release.channel }

    fun baseTargetVersion(existing: ReleaseTarget?): Long = existing?.targetVersion ?: 0L
}
