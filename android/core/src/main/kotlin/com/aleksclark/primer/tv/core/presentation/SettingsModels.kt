package com.aleksclark.primer.tv.core.presentation

/**
 * Grouped settings surface for Device, Server, Updates, and Danger Zone.
 * Pure presentation — no Android types so JVM tests can cover copy and state.
 */
data class SettingsUiModel(
    val deviceName: String,
    val deviceKind: String,
    val deviceId: String?,
    val serverUrl: String,
    val installedVersionLabel: String,
    val update: UpdateCardModel,
)

/** Visual state of the update card. */
sealed interface UpdateCardModel {
    val statusLabel: UiText
    val canInstall: Boolean get() = false
    val canRetryCheck: Boolean get() = true
    val isBusy: Boolean get() = false
    val isError: Boolean get() = false

    data class UpToDate(
        override val statusLabel: UiText = UiText.of("This device is up to date."),
    ) : UpdateCardModel

    data class Available(
        val versionCode: Int,
        override val statusLabel: UiText,
    ) : UpdateCardModel {
        override val canInstall: Boolean = true
    }

    data class Downloading(
        override val statusLabel: UiText = UiText.of("Downloading update…"),
    ) : UpdateCardModel {
        override val canRetryCheck: Boolean = false
        override val isBusy: Boolean = true
    }

    data class Installing(
        override val statusLabel: UiText = UiText.of("Starting the installer…"),
    ) : UpdateCardModel {
        override val canRetryCheck: Boolean = false
        override val isBusy: Boolean = true
    }

    data class Failed(
        override val statusLabel: UiText,
    ) : UpdateCardModel {
        override val isError: Boolean = true
    }
}

/** Named settings groupings used by the Settings screen. */
enum class SettingsSectionId {
    DEVICE,
    SERVER,
    UPDATES,
    DANGER_ZONE,
}

fun SettingsSectionId.title(): String = when (this) {
    SettingsSectionId.DEVICE -> "Device"
    SettingsSectionId.SERVER -> "Server"
    SettingsSectionId.UPDATES -> "Updates"
    SettingsSectionId.DANGER_ZONE -> "Danger Zone"
}
