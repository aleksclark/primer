package com.aleksclark.primer.control.device

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerEmptyState
import com.aleksclark.primer.ui.PrimerRecordRow
import com.aleksclark.primer.ui.PrimerSectionHeader
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerCheckboxRow
import com.aleksclark.primer.ui.PrimerTextField
import com.aleksclark.primertasks.client.Enrollment
import com.aleksclark.primertasks.client.ManagedDevice
import com.aleksclark.primertasks.client.Release

@Composable
fun DevicesScreen(
    devices: List<ManagedDevice>,
    enrollment: Enrollment?,
    message: String?,
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
    releases: List<Release>,
    selectedRelease: String,
    onRelease: (String) -> Unit,
    message: String?,
    recoveryCodes: List<String>,
    acknowledged: Boolean,
    onAcknowledge: (Boolean) -> Unit,
    onPrepareRecovery: () -> Unit,
    onRotateRecovery: () -> Unit,
    onQuarantine: () -> Unit,
    onRevoke: () -> Unit,
    onTarget: () -> Unit,
    onBack: () -> Unit,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        PrimerSectionHeader(label = "Device", title = device.displayName, trailing = { PrimerButton(text = "Back", onClick = onBack, variant = PrimerButtonVariant.Quiet) })
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerRecordRow(label = "State", value = device.state)
        PrimerRecordRow(label = "Desired revision", value = device.desiredRevision.toString())
        PrimerRecordRow(label = "Applied revision", value = device.appliedRevision.toString())
        PrimerTextField(value = selectedRelease, onValueChange = onRelease, label = "Release id to target")
        PrimerButton(text = "Set release target", onClick = onTarget, enabled = selectedRelease.isNotBlank())
        PrimerButton(text = "Quarantine", onClick = onQuarantine, variant = PrimerButtonVariant.Secondary)
        PrimerButton(text = "Revoke management", onClick = onRevoke, variant = PrimerButtonVariant.Attention)
        androidx.compose.material3.Text("${releases.size} published releases loaded from the server.", style = com.aleksclark.primer.ui.PrimerTheme.typography.small)
        PrimerButton(text = "Prepare recovery codes", onClick = onPrepareRecovery)
        if (recoveryCodes.isNotEmpty()) {
            PrimerStatus("Store these one-use codes off this phone before rotating.", tone = PrimerStatusTone.Attention)
            recoveryCodes.forEachIndexed { index, code ->
                PrimerRecordRow(label = "Code ${index + 1}", value = code)
            }
            PrimerCheckboxRow(text = "I stored these recovery codes off this device", checked = acknowledged, onCheckedChange = onAcknowledge)
            PrimerButton(text = "Rotate recovery", onClick = onRotateRecovery, enabled = acknowledged)
        }
    }
}
