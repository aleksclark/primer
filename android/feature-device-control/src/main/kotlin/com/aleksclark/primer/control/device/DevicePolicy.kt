package com.aleksclark.primer.control.device

import com.aleksclark.primertasks.client.ApprovedApp
import com.aleksclark.primertasks.client.LockTaskPolicy
import com.aleksclark.primertasks.client.MaintenancePolicy
import com.aleksclark.primertasks.client.Policy
import com.aleksclark.primertasks.client.PolicyReport
import com.aleksclark.primertasks.client.PolicyUpdateInput
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.ReleaseTarget

enum class DeviceSyncStatus { Applied, Pending, Partial, Stale }

object DeviceSync {
    fun status(desiredRevision: Long, appliedRevision: Long, report: PolicyReport?): DeviceSyncStatus {
        if (report?.stale == true) return DeviceSyncStatus.Stale
        val reportStatus = report?.status?.lowercase()
        if (reportStatus == "partial") return DeviceSyncStatus.Partial
        if (appliedRevision < desiredRevision) return DeviceSyncStatus.Pending
        return DeviceSyncStatus.Applied
    }

    fun label(status: DeviceSyncStatus): String = when (status) {
        DeviceSyncStatus.Applied -> "applied"
        DeviceSyncStatus.Pending -> "pending"
        DeviceSyncStatus.Partial -> "partial"
        DeviceSyncStatus.Stale -> "stale"
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

object ReleaseCas {
    fun matching(targets: List<ReleaseTarget>, release: Release): ReleaseTarget? =
        targets.firstOrNull { it.packageName == release.packageName && it.channel == release.channel }

    fun baseTargetVersion(existing: ReleaseTarget?): Long = existing?.targetVersion ?: 0L
}
