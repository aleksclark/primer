package com.aleksclark.primer.control.device

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerCheckboxRow
import com.aleksclark.primer.ui.PrimerEmptyState
import com.aleksclark.primer.ui.PrimerRecordRow
import com.aleksclark.primer.ui.PrimerSectionHeader
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerTextField
import com.aleksclark.primertasks.client.ApprovedApp
import com.aleksclark.primertasks.client.DesiredState
import com.aleksclark.primertasks.client.Enrollment
import com.aleksclark.primertasks.client.ManagedDevice
import com.aleksclark.primertasks.client.Release

@Composable
fun DevicesScreen(
    devices: List<ManagedDevice>,
    enrollment: Enrollment?,
    message: String?,
    selfUpdate: ControlSelfUpdateUi = ControlSelfUpdateUi(),
    onIssue: () -> Unit,
    onOpen: (ManagedDevice) -> Unit,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(
            label = "Device management",
            title = "Household devices",
            description = "Control is not a device owner. Enrollment codes are one-use and never long-lived credentials.",
            trailing = { PrimerButton(text = "Issue enrollment", onClick = onIssue) },
        )
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        if (enrollment != null) {
            PrimerRecordRow(label = "Enrollment code", value = enrollment.code)
            androidx.compose.material3.Text(enrollment.qrPayload, style = com.aleksclark.primer.ui.PrimerTheme.typography.mono)
        }
        ControlSelfUpdateScreen(update = selfUpdate)
        if (devices.isEmpty()) PrimerEmptyState(title = "No managed devices", message = "Issue an enrollment QR for Student. Control never becomes device admin.")
        LazyColumn {
            items(devices, key = { it.id }) { device ->
                PrimerRecordRow(label = device.state, value = device.displayName)
                PrimerButton(text = "Open", onClick = { onOpen(device) }, variant = PrimerButtonVariant.Quiet)
            }
        }
    }
}

@Composable
fun DeviceDetailScreen(
    device: ManagedDevice,
    desired: DesiredState?,
    syncStatus: DeviceSyncStatus?,
    releases: List<Release>,
    selectedRelease: String,
    onRelease: (String) -> Unit,
    message: String?,
    recovery: RecoveryBinding?,
    approvedAppDraft: ApprovedAppDraft,
    onApprovedPackage: (String) -> Unit,
    onApprovedSigner: (String) -> Unit,
    onApprovedLabel: (String) -> Unit,
    onApprovedRequired: (Boolean) -> Unit,
    onAddApprovedApp: () -> Unit,
    onRemoveApprovedApp: (String) -> Unit,
    onParentUnlock: (Boolean) -> Unit,
    onAcknowledge: (Boolean) -> Unit,
    onPrepareRecovery: () -> Unit,
    onRotateRecovery: () -> Unit,
    onQuarantine: () -> Unit,
    onRevoke: () -> Unit,
    onTarget: () -> Unit,
    onRequeue: () -> Unit,
    onBack: () -> Unit,
    mutating: Boolean = false,
) {
    val policy = desired?.policyRevision?.policy
    Column(
        Modifier.fillMaxSize().padding(24.dp).verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        PrimerSectionHeader(label = "Device", title = device.displayName, trailing = { PrimerButton(text = "Back", onClick = onBack, variant = PrimerButtonVariant.Quiet) })
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerRecordRow(label = "State", value = device.state)
        PrimerRecordRow(label = "Desired revision", value = device.desiredRevision.toString())
        PrimerRecordRow(label = "Applied revision", value = device.appliedRevision.toString())
        PrimerRecordRow(label = "Sync", value = syncStatus?.let(DeviceSync::label) ?: "unknown")
        PrimerTextField(value = selectedRelease, onValueChange = onRelease, label = "Release id to target")
        PrimerButton(text = "Set release target", onClick = onTarget, enabled = selectedRelease.isNotBlank() && !mutating)
        PrimerButton(text = "Requeue current target", onClick = onRequeue, enabled = selectedRelease.isNotBlank() && !mutating, variant = PrimerButtonVariant.Secondary)
        PrimerButton(text = "Quarantine", onClick = onQuarantine, variant = PrimerButtonVariant.Secondary, enabled = !mutating)
        PrimerButton(text = "Revoke management", onClick = onRevoke, variant = PrimerButtonVariant.Attention, enabled = !mutating)
        androidx.compose.material3.Text("${releases.size} published releases loaded from the server.", style = com.aleksclark.primer.ui.PrimerTheme.typography.small)
        PrimerSectionHeader(label = "Policy", title = "Approved apps")
        if (policy == null) {
            PrimerStatus("No policy revision is available yet.", tone = PrimerStatusTone.Neutral)
        } else {
            policy.approvedApps.forEach { app: ApprovedApp ->
                PrimerRecordRow(label = app.packageName, value = app.signerSha256)
                PrimerButton(text = "Remove ${app.packageName}", onClick = { onRemoveApprovedApp(app.packageName) }, variant = PrimerButtonVariant.Attention, enabled = !mutating)
            }
            PrimerTextField(value = approvedAppDraft.packageName, onValueChange = onApprovedPackage, label = "Package name")
            PrimerTextField(value = approvedAppDraft.signerSha256, onValueChange = onApprovedSigner, label = "Signer SHA-256")
            PrimerTextField(value = approvedAppDraft.label, onValueChange = onApprovedLabel, label = "Label")
            PrimerCheckboxRow(text = "Required", checked = approvedAppDraft.required, onCheckedChange = onApprovedRequired)
            PrimerButton(text = "Add approved app", onClick = onAddApprovedApp, enabled = !mutating)
            PrimerCheckboxRow(
                text = "Allow parent unlock",
                checked = policy.maintenance.allowParentUnlock,
                onCheckedChange = onParentUnlock,
                enabled = !mutating,
            )
        }
        PrimerButton(text = "Prepare recovery codes", onClick = onPrepareRecovery, enabled = !mutating)
        if (recovery != null) {
            PrimerStatus("Store these one-use codes off this phone before rotating.", tone = PrimerStatusTone.Attention)
            PrimerRecordRow(label = "Device", value = recovery.deviceId)
            PrimerRecordRow(label = "Key id", value = recovery.keyId)
            recovery.codes.forEachIndexed { index, code ->
                PrimerRecordRow(label = "Code ${index + 1}", value = code)
            }
            PrimerCheckboxRow(text = "I stored these recovery codes off this device", checked = recovery.acknowledged, onCheckedChange = onAcknowledge)
            PrimerButton(text = "Rotate recovery", onClick = onRotateRecovery, enabled = recovery.acknowledged && !mutating)
        }
    }
}

@Composable
fun ControlSelfUpdateScreen(update: ControlSelfUpdateUi) {
    PrimerSectionHeader(
        label = "Control app",
        title = "Self-update",
        description = "Control is not a device owner. Production install stays held until the shared adapter validates APK hash, signer, and version from bytes — not filename.",
    )
    PrimerStatus(update.status, tone = when (update.phase) {
        ControlSelfUpdatePhase.Held, ControlSelfUpdatePhase.Idle -> PrimerStatusTone.Neutral
        ControlSelfUpdatePhase.Failed -> PrimerStatusTone.Attention
        else -> PrimerStatusTone.Accent
    })
    PrimerRecordRow(label = "Phase", value = update.phase.name.lowercase())
    PrimerRecordRow(label = "Installed version", value = update.installedVersion.toString())
    if (update.candidateVersion != null) PrimerRecordRow(label = "Candidate version", value = update.candidateVersion.toString())
    val plan = update.plan
    if (plan != null) {
        PrimerRecordRow(label = "Verified bytes", value = if (plan.verifiedBytes) "yes" else "no")
        PrimerRecordRow(label = "Unattended eligible", value = if (plan.unattendedEligible) "yes" else "no")
        PrimerRecordRow(label = "Confirmation", value = if (plan.confirmationRequired) "system prompt or notification" else "not required")
        if (plan.settingsRequired) PrimerStatus("Open system install settings, then confirm the update.", tone = PrimerStatusTone.Attention)
        PrimerStatus(plan.reason, tone = PrimerStatusTone.Neutral)
    }
    PrimerButton(text = "Install Control update", onClick = {}, enabled = false)
    PrimerButton(text = "Open install settings", onClick = {}, enabled = false, variant = PrimerButtonVariant.Secondary)
}
