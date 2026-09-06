package com.aleksclark.primer.tv.app.ui.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.tv.app.ui.components.SettingsInfoRow
import com.aleksclark.primer.tv.app.ui.components.SettingsSection
import com.aleksclark.primer.tv.app.ui.components.UpdateCard
import com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerRecordRow
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.tv.app.update.UpdateState
import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.domain.FormFactor
import com.aleksclark.primer.tv.core.presentation.SettingsPresenter
import com.aleksclark.primer.tv.core.presentation.SettingsSectionId
import com.aleksclark.primer.tv.core.presentation.SettingsUiModel
import com.aleksclark.primer.tv.core.presentation.UpdateStatus

/** User intents from Settings. */
sealed interface SettingsEvent {
    data object CheckForUpdate : SettingsEvent
    data object InstallUpdate : SettingsEvent
    data class SetDarkTheme(val darkTheme: Boolean) : SettingsEvent
    data object Unpair : SettingsEvent
}

/**
 * Grouped settings: Device, Server, Updates, Danger Zone. Navigation chrome
 * lives on the scaffold — no redundant Back header here.
 */
@Composable
fun SettingsScreen(
    settings: DeviceSettings,
    installedVersion: Int,
    update: UpdateState,
    onEvent: (SettingsEvent) -> Unit,
    modifier: Modifier = Modifier,
) {
    val model = remember(settings, installedVersion, update) {
        SettingsPresenter.present(
            deviceName = settings.deviceName,
            deviceKind = settings.deviceKind,
            deviceId = settings.deviceId,
            serverUrl = settings.baseUrl,
            installedVersionCode = installedVersion,
            updateStatus = update.toStatus(),
            availableVersionCode = (update as? UpdateState.Available)?.release?.versionCode ?: 0,
            failureMessage = (update as? UpdateState.Failed)?.message,
        )
    }
    SettingsScreenContent(
        model = model,
        darkTheme = settings.darkTheme,
        onEvent = onEvent,
        modifier = modifier,
    )
}

@Composable
fun SettingsScreenContent(
    model: SettingsUiModel,
    darkTheme: Boolean,
    onEvent: (SettingsEvent) -> Unit,
    modifier: Modifier = Modifier,
) {
    val spacing = PrimerTheme.spacing
    val colors = PrimerTheme.colors
    val typography = PrimerTheme.typography
    val scroll = rememberScrollState()
    val minHeight = if (PrimerTheme.formFactor == FormFactor.TELEVISION) 56.dp else 48.dp

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(scroll)
            .padding(horizontal = spacing.screenHorizontal)
            .padding(top = spacing.md, bottom = spacing.xl),
        verticalArrangement = Arrangement.spacedBy(spacing.md),
    ) {
        // Title is present for screen readers / context but kept quiet —
        // scaffold already identifies the destination.
        Text(
            text = "Settings",
            style = typography.screenTitle,
            color = colors.onSurface,
            modifier = Modifier.semantics { contentDescription = "Settings" },
        )

        SettingsSection(SettingsSectionId.DEVICE) {
            SettingsInfoRow(label = "Name", value = model.deviceName)
            SettingsInfoRow(label = "Kind", value = model.deviceKind)
            model.deviceId?.let { SettingsInfoRow(label = "Device ID", value = it) }
            PrimerRecordRow(
                label = "Appearance",
                value = if (darkTheme) "Dark (System C default)" else "Light (full parity)",
                status = if (darkTheme) "DARK" else "LIGHT",
                statusTone = PrimerStatusTone.Accent,
            )
            PrimerButton(
                text = if (darkTheme) "Use light theme" else "Use dark theme",
                onClick = { onEvent(SettingsEvent.SetDarkTheme(!darkTheme)) },
                modifier = Modifier.fillMaxWidth(),
                variant = PrimerButtonVariant.Secondary,
            )
        }

        SettingsSection(SettingsSectionId.SERVER) {
            SettingsInfoRow(label = "Address", value = model.serverUrl)
        }

        SettingsSection(SettingsSectionId.UPDATES) {
            UpdateCard(
                installedVersionLabel = model.installedVersionLabel,
                model = model.update,
                onCheckForUpdate = { onEvent(SettingsEvent.CheckForUpdate) },
                onInstallUpdate = { onEvent(SettingsEvent.InstallUpdate) },
            )
        }

        SettingsSection(SettingsSectionId.DANGER_ZONE) {
            Text(
                text = "Unpairing forgets this device’s token. The server address is kept so you can pair again quickly.",
                style = typography.body,
                color = colors.onSurfaceMuted,
            )
            PrimerButton(
                text = "Unpair",
                onClick = { onEvent(SettingsEvent.Unpair) },
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = minHeight)
                    .semantics { contentDescription = "Unpair this device" },
                variant = PrimerButtonVariant.Attention,
            )
        }
    }
}

private fun UpdateState.toStatus(): UpdateStatus = when (this) {
    UpdateState.UpToDate -> UpdateStatus.UP_TO_DATE
    is UpdateState.Available -> UpdateStatus.AVAILABLE
    UpdateState.Downloading -> UpdateStatus.DOWNLOADING
    UpdateState.Installing -> UpdateStatus.INSTALLING
    is UpdateState.Failed -> UpdateStatus.FAILED
}
